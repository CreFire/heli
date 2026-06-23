package xhttp

import (
	"context"
	"errors"
	"fmt"
	"game/deps/transport"
	httptransport "game/deps/transport/http"
	"game/deps/xlog"
	"game/deps/xtoken"
	"game/src/proto/errorpb"
	"io"
	"net/http"
	"reflect"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var ServerName string

type TokenType int32

const (
	TOKEN_TYPE_CLIENT  TokenType = 0
	TOKEN_TYPE_SERVER  TokenType = 1
	TOKEN_TYPE_NO_NEED TokenType = 2
)

const maxReqBodyBytes = 4 * 1024

type HandlerMgr struct {
	HandlerMap map[string]*ProtoMessageHandler
}

func NewHttpHandlerMgr() *HandlerMgr {
	return &HandlerMgr{HandlerMap: make(map[string]*ProtoMessageHandler)}
}

type ProtoMessageHandler struct {
	Method    string
	Req       proto.Message   //消息req
	Res       proto.Message   //消息res
	Handle    ProtoHandleFunc //处理函数
	TokenType TokenType       //token类型
}
type ProtoHandleFunc func(ctx context.Context, req proto.Message) (proto.Message, *HttpError)

func (mgr *HandlerMgr) RegisterProtoHandler(h *ProtoMessageHandler) error {
	if h == nil {
		return fmt.Errorf("register failed, handler is nil")
	}
	if h.Method == "" {
		return fmt.Errorf("register failed, handler method is empty")
	}
	if v := mgr.HandlerMap[h.Method]; v != nil {
		return fmt.Errorf("register failed, duplicate method: %s", h.Method)
	}
	mgr.HandlerMap[h.Method] = h
	return nil
}

func (mgr *HandlerMgr) Handler(ctx *gin.Context) {
	path := strings.Split(ctx.Request.URL.Path, "/")
	method := path[len(path)-1]
	h, ok := mgr.HandlerMap[method]
	if !ok {
		ctx.String(http.StatusNotFound, "404 page not found")
		xlog.Infof("http route not found, method:%s path:%s ip:%s", ctx.Request.Method, ctx.Request.URL.Path, ctx.ClientIP())
		return
	}
	req := proto.Clone(h.Req)
	if ctx.Request.Method != "POST" {
		replayWithError(ctx, req, h.Res, ErrorHttpUnSupportMethod)
		return
	}

	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxReqBodyBytes)
	buff, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			replayWithError(ctx, req, h.Res, ErrorHttpBodyTooLarge)
			return
		}
		xlog.Infof("http read body failed, path:%s err:%v", ctx.Request.URL.Path, err)
		replayWithError(ctx, req, h.Res, ErrorHttpUnexpect)
		return
	}
	contentType := ctx.ContentType()
	if strings.Contains(contentType, "application/json") {
		if err := protojson.Unmarshal(buff, req); err != nil {
			replayWithError(ctx, req, h.Res, ErrorHttpJsonUnMarshalFailed)
			return
		}
	} else if strings.Contains(contentType, "application/octet-stream") {
		if err := proto.Unmarshal(buff, req); err != nil {
			replayWithError(ctx, req, h.Res, ErrorHttpOctetUnMarshalFailed)
			return
		}
	} else {
		xlog.Infof("got unsupported content type %s", contentType)
		replayWithError(ctx, req, h.Res, ErrorHttpUnSupportContentType)
		return
	}

	tr := httptransport.NewTransport(ctx.Request, ctx.Request.URL.Path, ctx.ClientIP())

	token, machineId := ctx.GetHeader("Authorization"), ctx.GetHeader("MachineId")
	if userId, er := tokenDecode(token, machineId, h.TokenType); er != nil {
		replayWithError(ctx, req, h.Res, NewWithError(int32(errorpb.ERROR_AUTH_TOKEN_INVALID), er))
		return
	} else {
		tr.RequestHeader().Set("GamerId", fmt.Sprintf("%d", userId))
	}

	newCtx := transport.NewClientContext(ctx.Request.Context(), tr)

	defer func() {
		if r := recover(); r != nil {
			json := protojson.MarshalOptions{Multiline: false}.Format(req)
			xlog.Errorf("%s handle error %s, req param: %s , stack info %s ", ctx.Request.URL.Path, r, json, string(debug.Stack()))
			replayWithError(ctx, req, h.Res, ErrorHttpUnexpect)
		}
	}()

	start := time.Now()
	res, er := h.Handle(newCtx, req)
	cost := time.Since(start)
	if er != nil {
		replayWithError(ctx, req, h.Res, er)
		return
	}

	replyHeader := tr.ReplyHeader()
	for _, key := range replyHeader.Keys() {
		for _, val := range replyHeader.Values(key) {
			ctx.Writer.Header().Add(key, val)
		}
	}

	if xlog.GetLogLevel() == xlog.LOG_LEVEL_DEBUG {
		json_req := protojson.MarshalOptions{Multiline: false}.Format(req)
		json_res := protojson.MarshalOptions{Multiline: false}.Format(res)

		xlog.Debugw("[HTTP-SUCCESS]",
			"path", ctx.Request.URL.Path,
			"Ip", ctx.ClientIP(),
			"cost", cost,
			"req", json_req,
			"res", json_res)
	}

	if strings.Contains(contentType, "application/json") {
		json_res := protojson.MarshalOptions{Multiline: false}.Format(res)
		ctx.String(http.StatusOK, json_res)
		return
	}

	ctx.ProtoBuf(http.StatusOK, res)
}

func replayWithError(ctx *gin.Context, req proto.Message, res proto.Message, e *HttpError) {
	if e == nil {
		xlog.Errorf("ReplayWithError get error nil")
		return
	}

	r := proto.Clone(res)
	vo := reflect.ValueOf(r).Elem()
	if va := vo.FieldByName("Err"); va.IsValid() {
		pe := e.ProtoHttpError()
		value := reflect.ValueOf(pe)
		va.Set(value)
	} else {
		xlog.Errorf("need HttpError Field Err")
	}

	contentType := ctx.ContentType()
	if strings.Contains(contentType, "application/json") {
		json := protojson.MarshalOptions{Multiline: false}.Format(r)
		ctx.String(http.StatusOK, json)
	} else if strings.Contains(contentType, "application/octet-stream") {
		ctx.ProtoBuf(http.StatusOK, r)
	}

	json := protojson.MarshalOptions{Multiline: false}.Format(req)
	xlog.Infof("  handle %s ,  req: %v ,  err_code: %v  err_desc: %s",
		ctx.Request.URL.Path, json, e.ErrCode, e.ErrDesc)
}

func tokenDecode(token, machineId string, t TokenType) (int64, error) {

	switch t {
	case TOKEN_TYPE_SERVER:
		if len(token) == 0 {
			return 0, fmt.Errorf("server token is empty")
		}
		err := xtoken.DefaultCoder.InternalTokenDecode(token, "")
		return 0, err
	case TOKEN_TYPE_NO_NEED:
		return 0, nil
	default:
		if len(token) == 0 {
			return 0, fmt.Errorf("client token is empty")
		}
		Uid, err := xtoken.DefaultCoder.SimpleTokenDecode(token, machineId)
		if err != nil {
			return 0, err
		}
		return Uid, nil
	}
}
