package xhttp

import (
	"context"
	"errors"
	"fmt"
	"net"

	"game/deps/xlog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
)

type HttpServer struct {
	Gin        *gin.Engine
	GrpcServer *grpc.Server
	HttpServer *http.Server
	handleMap  map[string]*HandlerMgr
}

const (
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 15 * time.Second
	defaultWriteTimeout      = 15 * time.Second
	defaultIdleTimeout       = 60 * time.Second
)

func NewHttpServer() *HttpServer {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery(), CORSMiddleware())
	grpcSrv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(UnaryServerLoggingInterceptor()),
	)

	return &HttpServer{
		Gin:        engine,
		GrpcServer: grpcSrv,
		HttpServer: nil,
		handleMap:  make(map[string]*HandlerMgr),
	}
}

func (s *HttpServer) Init(serverName string) error {
	ServerName = strings.ToLower(serverName)
	return nil
}

func (s *HttpServer) StartServe(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind address %s: %w", addr, err)
	}

	combinedHandler := h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
			s.GrpcServer.ServeHTTP(w, r)
		} else {
			s.Gin.ServeHTTP(w, r)
		}
	}), &http2.Server{})

	s.HttpServer = &http.Server{
		Addr:              addr,
		Handler:           combinedHandler,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		ReadTimeout:       defaultReadTimeout,
		WriteTimeout:      defaultWriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
	}

	go func() {
		if err := s.HttpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			xlog.Errorf("listener serve failed: %s\n", err)
			panic(err)
		}
	}()
	return nil
}

func (s *HttpServer) StopServe() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.HttpServer.Shutdown(ctx); err != nil {
		xlog.Errorf("Server Shutdown: %v", err)
		return err
	}
	s.HttpServer = nil
	return nil
}

func (s *HttpServer) RegisterProtoHandler(urlPath string, handlers []*ProtoMessageHandler) error {
	if urlPath == "" {
		return fmt.Errorf("register proto handler failed: empty urlPath")
	}

	if v := s.handleMap[urlPath]; v == nil {
		s.handleMap[urlPath] = NewHttpHandlerMgr()
	}

	for _, v := range handlers {
		if err := s.handleMap[urlPath].RegisterProtoHandler(v); err != nil {
			return fmt.Errorf("register proto handler failed, path:%s: %w", urlPath, err)
		}
	}

	s.Gin.POST(urlPath+"/*action", s.handleMap[urlPath].Handler)
	return nil
}
