// Package controller provides authentication policy handling and HTTP/gRPC request handlers.
package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"game/deps/server"
	servicemgr "game/deps/service_mgr"
	"game/deps/transport"
	"game/deps/xhttp"
	"game/deps/xlog"
	"game/deps/xtime"
	"game/deps/xtoken"
	"game/src/cache"
	"game/src/common"
	"game/src/persist"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"google.golang.org/protobuf/proto"
)

// ensureAccountFormat 验证账户格式是否符合要求
func ensureAccountFormat(account string) error {
	reg, _ := regexp.Compile(`^(?:con_(?:riot|tiki|k6|fu))_([1-9]\d{3})$`)
	if !reg.MatchString(account) {
		return fmt.Errorf("account format error")
	}

	return nil
}

// Login 统一处理 HTTP 的登录请求.
func Login(ctx context.Context, req proto.Message) (_ proto.Message, httpErr *xhttp.HttpError) {
	loginReq, ok := req.(*pb.AuthLoginREQ)
	if !ok {
		return nil, xhttp.NewHttpError(int32(errorpb.ERROR_REQUEST_PARAMS), "invalid request type")
	}

	loginCtx, err := newLoginContext(ctx, loginReq)
	if err != nil {
		return nil, err
	}

	if err := beforecheckLogin(loginCtx); err != nil {
		return nil, err
	}

	xlog.Infof("login request. account:%s gid:%d deviceId:%s ip:%s", loginCtx.accountForLog, loginReq.DeviceInfo.Gid, loginReq.DeviceInfo.DeviceId, loginCtx.clientIP)
	dbAccount, err := loadLoginAccount(ctx, loginCtx)
	if err != nil {
		return nil, err
	}
	if dbAccount == nil {
		return nil, xhttp.NewHttpError(int32(errorpb.ERROR_AUTH_ACCOUNT_CREATE_CLOSED), "account error")
	}
	if err := fillLoginSession(ctx, loginCtx.result, dbAccount, loginReq.DeviceInfo, loginCtx.clientIP); err != nil {
		return nil, err
	}

	return loginCtx.result, nil
}

type loginContext struct {
	req           *pb.AuthLoginREQ          // 原始登录请求
	result        *pb.AuthLoginRSP          // 登录响应，后续流程逐步填充
	code2Session  *pb.Code2SessionLoginInfo // code2Session 登录参数，普通账号登录时为空
	loginType     pb.AuthLoginType          // 归一化后的登录类型
	accountForLog string                    // 用于日志、白名单、限流上下文的账号标识
	clientIP      string                    // 客户端 IP
}

// newLoginContext 校验登录基础参数，并整理后续流程需要的上下文数据.
func newLoginContext(ctx context.Context, loginReq *pb.AuthLoginREQ) (*loginContext, *xhttp.HttpError) {
	code2Session := loginReq.GetCode2Session()
	loginType := normalizeLoginType(loginReq)
	// if loginReq.DeviceInfo == nil || loginReq.DeviceInfo.DeviceId == "" {
	// 	return nil, xhttp.NewHttpError(int32(errorpb.ERROR_REQUEST_PARAMS), "invalid request, param error ")
	// }
	if loginType == pb.AuthLoginType_AUTH_LOGIN_TYPE_CODE2_SESSION {
		if code2Session == nil || code2Session.GetCode() == "" {
			return nil, xhttp.NewHttpError(int32(errorpb.ERROR_REQUEST_PARAMS), "invalid code2Session login params")
		}
	} else if loginReq.Account == "" {
		return nil, xhttp.NewHttpError(int32(errorpb.ERROR_REQUEST_PARAMS), "invalid request, param error ")
	}

	accountForLog := loginReq.Account
	if loginType == pb.AuthLoginType_AUTH_LOGIN_TYPE_CODE2_SESSION {
		accountForLog = persist.MiniProgramAccountName(code2SessionApp(code2Session), code2Session.GetOpenId())
	}

	var clientIP string
	if tr, ok := transport.FromClientContext(ctx); ok {
		clientIP = tr.ClientIP()
		xlog.Infof("Login request from IP: %s via %s", clientIP, tr.Kind())
	}

	return &loginContext{
		req:           loginReq,
		result:        &pb.AuthLoginRSP{},
		code2Session:  code2Session,
		loginType:     loginType,
		accountForLog: accountForLog,
		// clientIP:      clientIP,
	}, nil
}

// normalizeLoginType 归一化登录类型，兼容客户端只传业务参数、不显式传 loginType 的情况.
func normalizeLoginType(loginReq *pb.AuthLoginREQ) pb.AuthLoginType {
	if loginReq.GetLoginType() != pb.AuthLoginType_AUTH_LOGIN_TYPE_ACCOUNT {
		return loginReq.GetLoginType()
	}
	if loginReq.GetCode2Session() != nil {
		return pb.AuthLoginType_AUTH_LOGIN_TYPE_CODE2_SESSION
	}
	return pb.AuthLoginType_AUTH_LOGIN_TYPE_ACCOUNT
}

// beforecheckLogin 处理登录前置准入检查：白名单、队列、IP 限流和全局限流.
func beforecheckLogin(loginCtx *loginContext) *xhttp.HttpError {
	loginReq := loginCtx.req
	if whiteListSwitch {
		ok, err := persist.IsInWhiteList(loginCtx.accountForLog, loginReq.DeviceInfo.DeviceId, loginCtx.clientIP)
		if err != nil {
			xlog.Warnf("login whitelist check failed. account:%s ip:%s err:%v", loginCtx.accountForLog, loginCtx.clientIP, err)
			return xhttp.NewHttpError(int32(errorpb.ERROR_FAILED), "white list error")
		}

		if !ok {
			xlog.Infof("login reject whitelist. account:%s gid:%d deviceId:%s ip:%s",
				loginCtx.accountForLog, loginReq.DeviceInfo.Gid, loginReq.DeviceInfo.DeviceId, loginCtx.clientIP)
			return xhttp.NewHttpError(int32(errorpb.ERROR_AREA_FIX), "account  ip dev_id not in white list")
		}
	}

	if ok := GetLoginLimiter().Allow(); !ok {
		xlog.Debugf("login reject global rate limit. account:%s ip:%s gateServerCount:%d",
			loginCtx.accountForLog, loginCtx.clientIP, gateServerCount)
		return xhttp.NewHttpError(int32(errorpb.ERROR_HTTP_TOO_FAST), "too many login requests")
	}

	return nil
}

// loadLoginAccount 按登录类型加载账号数据.
func loadLoginAccount(ctx context.Context, loginCtx *loginContext) (*persist.Account, *xhttp.HttpError) {
	switch loginCtx.loginType {
	case pb.AuthLoginType_AUTH_LOGIN_TYPE_CODE2_SESSION:
		return loadCode2SessionAccount(ctx, loginCtx)
	default:
		return loadAccountLoginAccount(ctx, loginCtx)
	}
}

// loadCode2SessionAccount 处理小程序 / H5 主登录；本地 debug 下 code 直接当 openId/unionId.
func loadCode2SessionAccount(ctx context.Context, loginCtx *loginContext) (*persist.Account, *xhttp.HttpError) {
	info := loginCtx.code2Session
	appId := code2SessionApp(info)
	openId, unionId, err := resolveCode2SessionOpenID(ctx, info)
	if err != nil {
		xlog.Warnf("code2Session provider failed. app:%s platform:%s codeLen:%d err:%v", appId, info.GetPlatform(), len(info.GetCode()), err)
		return nil, xhttp.NewHttpError(int32(errorpb.ERROR_FAILED), "code2Session failed")
	}
	dbAccount, err := persist.GetOrCreateMiniProgramAccount(ctx, appId, openId, unionId)
	if err != nil {
		xlog.Warnf("get code2Session account failed. app:%s openId:%s gid:%d err:%v", appId, openId, loginCtx.req.DeviceInfo.Gid, err)
		return nil, xhttp.NewHttpError(int32(errorpb.ERROR_FAILED), "code2Session account error")
	}
	loginCtx.result.LoginType = pb.AuthLoginType_AUTH_LOGIN_TYPE_CODE2_SESSION
	loginCtx.result.OpenId = dbAccount.OpenId
	loginCtx.result.UnionId = dbAccount.UnionId
	loginCtx.result.Account = dbAccount.Account
	return dbAccount, nil
}

// code2SessionApp 获取 code2Session 应用标识，未传时使用默认值 sg.
func code2SessionApp(info *pb.Code2SessionLoginInfo) string {
	if info.GetApp() != "" {
		return info.GetApp()
	}
	return "sg"
}

// resolveCode2SessionOpenID 获取 code2Session 的 openId/unionId；debug 和 h5 下 code 直接当 openId/unionId.
func resolveCode2SessionOpenID(ctx context.Context, info *pb.Code2SessionLoginInfo) (openID string, unionID string, err error) {
	openID = info.GetOpenId()
	if openID == "" {
		if server.MS.ConfBase.Global.IsDebug || info.GetPlatform() == "h5" {
			openID = info.GetCode()
		} else {
			resp, err := code2Session(ctx, code2SessionApp(info), info.GetCode())
			if err != nil {
				return "", "", err
			}
			openID = resp.OpenId
			unionID = resp.UnionId
		}
	}
	unionID = info.GetUnionId()
	if unionID == "" {
		unionID = openID
	}
	if openID == "" {
		return "", "", fmt.Errorf("empty openid")
	}
	return openID, unionID, nil
}

type code2SessionResponse struct {
	OpenId     string `json:"openid"`
	UnionId    string `json:"unionid"`
	SessionKey string `json:"session_key"`
	ErrCode    int32  `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

// code2Session 调用微信 jscode2session，用 code 换 openid/unionid.
func code2Session(ctx context.Context, app string, code string) (*code2SessionResponse, error) {
	authCfg := server.MS.ConfBase.Server.Auth
	if authCfg.AppId == "" || authCfg.SC == "" {
		return nil, fmt.Errorf("wechat code2Session config empty")
	}

	query := url.Values{}
	query.Set("appid", authCfg.AppId)
	query.Set("secret", authCfg.SC)
	query.Set("js_code", code)
	query.Set("grant_type", "authorization_code")
	requestURL := "https://api.weixin.qq.com/sns/jscode2session?" + query.Encode()

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("wechat code2Session http status:%d", resp.StatusCode)
	}

	var result code2SessionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("code2Session decode error:%w", err)
	}
	if result.ErrCode != 0 {
		return nil, fmt.Errorf("wechat code2Session errcode:%d errmsg:%s", result.ErrCode, result.ErrMsg)
	}
	if result.OpenId == "" {
		return nil, fmt.Errorf("wechat code2Session openid empty")
	}
	return &result, nil
}

// loadAccountLoginAccount 处理普通账号登录，并按内部 IP 兼容路径分流.
func loadAccountLoginAccount(ctx context.Context, loginCtx *loginContext) (*persist.Account, *xhttp.HttpError) {
	loginReq := loginCtx.req
	//todo 移除内部判断路径
	//内部使用非标准账号登录
	if ensureAccountFormat(loginReq.Account) != nil {
		return loadInternalAccount(ctx, loginCtx)
	}
	return loadPasswordAccount(ctx, loginCtx)
}

// loadInternalAccount 处理调试账号路径.
func loadInternalAccount(ctx context.Context, loginCtx *loginContext) (*persist.Account, *xhttp.HttpError) {
	loginReq := loginCtx.req
	dbAccount, err := persist.GetAccount(context.Background(), loginReq.Account)
	if err != nil {
		xlog.Warnf("get account failed. account:%s gid:%d err:%v", loginReq.Account, loginReq.DeviceInfo.Gid, err)
		return nil, xhttp.NewHttpError(int32(errorpb.ERROR_FAILED), "account error")
	}

	if err := ensureAccountFormat(loginReq.Account); err == nil {
		loginCtx.result.ShowWaterMark = true
	}

	if dbAccount == nil {
		newAccount := persist.NewAccount(loginReq.Account, xtime.NowUnix())
		err = persist.CreateAccount(ctx, newAccount)
		if err != nil {
			xlog.Errorf("create account failed. account:%s gid:%d err:%v", loginReq.Account, loginReq.DeviceInfo.Gid, err)
			return nil, xhttp.NewHttpError(int32(errorpb.ERROR_FAILED), "create account failed")
		}
		dbAccount = newAccount
	}
	return dbAccount, nil
}

// loadPasswordAccount 处理正式账号密码登录.
func loadPasswordAccount(ctx context.Context, loginCtx *loginContext) (*persist.Account, *xhttp.HttpError) {
	loginReq := loginCtx.req
	if err := ensureAccountFormat(loginReq.Account); err != nil {
		xlog.Infof("login reject account format. account:%s ip:%s", loginReq.Account, loginCtx.clientIP)
		return nil, xhttp.NewHttpError(int32(errorpb.ERROR_AUTH_ACCOUNT_FORMAT), "account or password format error")
	}
	dbAccount, err := persist.GetAccountWithPassword(ctx, loginReq.Account, loginReq.Token)
	if err != nil {
		xlog.Warnf("get account with password failed. account:%s gid:%d err:%v", loginReq.Account, loginReq.DeviceInfo.Gid, err)
		return nil, xhttp.NewHttpError(int32(errorpb.ERROR_AUTH_PASSWORD), "account error")
	}
	loginCtx.result.ShowWaterMark = true
	return dbAccount, nil
}

// fillLoginSession 确保账号有角色、生成登录 session、写入在线数据并填充响应.
func fillLoginSession(ctx context.Context, result *pb.AuthLoginRSP, dbAccount *persist.Account, deviceInfo *pb.DeviceInfo, clientIP string) *xhttp.HttpError {
	roles := dbAccount.Roles
	if len(roles) == 0 {
		gid := persist.CreateGamer(ctx, dbAccount.Account, "")
		if gid == 0 {
			xlog.Errorf("create default role failed. account:%s ip:%s", dbAccount.Account, clientIP)
			return xhttp.NewHttpError(int32(errorpb.ERROR_FAILED), "create role failed")
		}
		roles = map[int32]int64{0: gid}
		dbAccount.Roles = roles
	}

	gid, rolesInfo := getRoles(dbAccount)
	if gid == 0 || len(rolesInfo) == 0 {
		return xhttp.NewHttpError(int32(errorpb.ERROR_FAILED), "no valid roles")
	}
	// 7天
	session, err := xtoken.UserTokenEncode(gid, deviceInfo.DeviceId)
	if err != nil {
		xlog.Errorf("UserTokenEncode gid:%d error:%v", gid, err)
		return xhttp.NewHttpError(int32(errorpb.ERROR_FAILED), "failed")
	}
	god, err := cache.GetGamerOnlineData(gid)
	if err != nil {
		xlog.Warnf("get gamer online data failed. gid:%d err:%v", gid, err)
		return xhttp.NewHttpError(int32(errorpb.ERROR_FAILED), "failed")
	}
	god.Account = dbAccount.Account
	god.AuthToken = session
	god.GamerId = gid
	if err := cache.SetGamerOnlineData(gid, god); err != nil {
		xlog.Errorf("SetGamerOnlineData gid:%d error:%v", gid, err)
		return xhttp.NewHttpError(int32(errorpb.ERROR_FAILED), "failed")
	}

	result.Session = session
	result.Account = dbAccount.Account
	result.Roles = rolesInfo
	result.Ip = clientIP
	xlog.Infof("login success. account:%s gid:%d ip:%s roleCount:%d", dbAccount.Account, gid, clientIP, len(rolesInfo))
	return nil
}

// // isInternalLoginIP 判断是否为内网或本机登录来源.
// func isInternalLoginIP(clientIP string) bool {
// 	return strings.HasPrefix(clientIP, "192.168") ||
// 		strings.HasPrefix(clientIP, "::1") ||
// 		strings.HasPrefix(clientIP, "172.168") ||
// 		strings.HasPrefix(clientIP, "127.0") ||
// 		strings.HasPrefix(clientIP, "10.244")
// }

func getRoles(account *persist.Account) (gid int64, roles []*pb.UserRoleInfo) {
	var rolesInfo []*pb.UserRoleInfo
	for k, gid := range account.Roles {
		PlayerBase := persist.GetGamerRawDataByMod[*pb.PlayerBase](gid, persist.GamerBaseModIndex)
		if PlayerBase == nil {
			continue
		}
		role := &pb.UserRoleInfo{
			Gid:   gid,
			Sid:   k,
			Level: PlayerBase.Lv,
			Icon:  PlayerBase.Icon,
			Name:  PlayerBase.Nickname,
		}
		rolesInfo = append(rolesInfo, role)
		return gid, rolesInfo
	}

	return 0, rolesInfo
}

// UseRole 获取用户角色列表.
func UseRole(ctx context.Context, req proto.Message) (_ proto.Message, httpErr *xhttp.HttpError) {

	Req := req.(*pb.AuthUseRoleREQ)
	resp := &pb.AuthUseRoleRSP{}
	if Req.DeviceInfo == nil || Req.Session == "" || Req.DeviceInfo.Gid <= 0 {
		gid := int64(0)
		if Req.DeviceInfo != nil {
			gid = Req.DeviceInfo.Gid
		}
		xlog.Infof("userole reject invalid request. gid:%d", gid)
		return resp, xhttp.NewHttpError(int32(errorpb.ERROR_LOGIN_PARAM_INVALID), "invalid request")
	}
	tr, ok := transport.FromClientContext(ctx)
	if !ok || tr == nil {
		return resp, xhttp.NewHttpError(int32(errorpb.ERROR_UNEXPECTED), "transport is nil")
	}
	GamerId := Req.Gid
	if GamerId != Req.DeviceInfo.Gid {
		xlog.Infof("userole reject gamerId mismatch. header:%d req:%d ip:%s", GamerId, Req.DeviceInfo.Gid, tr.ClientIP())
		return resp, xhttp.NewHttpError(int32(errorpb.ERROR_UNEXPECTED), "GamerId not match")
	}

	clientIP := tr.ClientIP()

	god, err := cache.GetGamerOnlineData(Req.DeviceInfo.Gid)
	if err != nil {
		xlog.Warnf("userole auth session load failed. gid:%d ip:%s err:%v", Req.DeviceInfo.Gid, clientIP, err)
		return resp, xhttp.NewHttpError(int32(errorpb.ERROR_AUTH_SESSION), "auth session failed")
	}
	if god == nil {
		xlog.Warnf("userole auth session load failed. gid:%d ip:%s reason:nil_online_data", Req.DeviceInfo.Gid, clientIP)
		return resp, xhttp.NewHttpError(int32(errorpb.ERROR_AUTH_SESSION), "auth session failed")
	}
	if god.AuthToken != Req.Session {
		xlog.Infof("userole reject auth session mismatch. gid:%d ip:%s", Req.DeviceInfo.Gid, clientIP)
		return resp, xhttp.NewHttpError(int32(errorpb.ERROR_AUTH_SESSION), "auth session failed")
	}
	account := god.Account

	ins, err := server.MS.SvrMgr.PickMinOnline(common.InnerServerTypeGate, false)
	if err != nil {
		xlog.Warnf("PickMinOnline gate instance failed: %v", err)
		return resp, xhttp.NewHttpError(int32(errorpb.ERROR_AREA_NOT_FOUND), "gate server not found")
	}
	ins.IncOnlineCount(1)
	xlog.Infof("use role bind account:%v gid:%v gate:%v ip:%s session:%v", account, Req.DeviceInfo.Gid, ins.InstanceId, clientIP, Req.Session)

	resp.Gid = Req.DeviceInfo.Gid
	fillUseRoleServerEndpoint(resp, ins) // ws服务器
	return resp, nil
}

func fillUseRoleServerEndpoint(res *pb.AuthUseRoleRSP, ins *servicemgr.ServiceInstance) {
	if host := server.MS.ConfBase.Global.GatePublicHost; host != "" {
		res.Host = host
		return
	}

	res.Addr = &pb.ServerAddr{
		Ip:   ins.Host,
		Port: int32(ins.Port),
	}
}
