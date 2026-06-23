package main

import (
	"context"
	"game/deps/server"
	"game/deps/xhttp"
	"game/deps/xlog"
	"game/src/proto/pb"
	"game/src/service/auth/controller"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RegisterHandlers 注册 auth 服务的所有 HTTP 和 gRPC 处理器.
func RegisterHandlers() error {
	// 1. 从全局服务实例中获取 gRPC 和 Gin 服务器.
	grpcServer := server.MS.HttpServe.GrpcServer

	// 2. 注册 gRPC 处理器.
	pb.RegisterAuthLoginServiceServer(grpcServer, &authGrpcServerWrapper{})

	handlers := []*xhttp.ProtoMessageHandler{}
	handlers = append(handlers, &xhttp.ProtoMessageHandler{
		Method:    "login",
		Req:       &pb.AuthLoginREQ{},
		Res:       &pb.AuthLoginRSP{},
		Handle:    controller.Login,
		TokenType: xhttp.TOKEN_TYPE_NO_NEED,
	})

	handlers = append(handlers, &xhttp.ProtoMessageHandler{
		Method: "userole",
		Req:    &pb.AuthUseRoleREQ{},
		Res:    &pb.AuthUseRoleRSP{},
		Handle: controller.UseRole,
	})

	if err := server.MS.HttpServe.RegisterProtoHandler("/api/v1/auth", handlers); err != nil {
		return err
	}
	if server.MS.ConfBase.Global.IsDebug {
		server.MS.HttpServe.Gin.Any("/api/v1/query/*action", xhttp.ReverseProxy("query"))
	}
	xlog.Infof("HTTP handlers for auth registered.")
	return nil
}

// authGrpcServerWrapper 是一个 gRPC 服务器的包装器，
// 它在调用实际的服务逻辑之前，将 transport 信息注入到 context 中.
type authGrpcServerWrapper struct {
	pb.UnimplementedAuthLoginServiceServer
}

// AuthLogin 处理 gRPC 请求, 包装上下文, 并调用统一的业务逻辑.
func (w *authGrpcServerWrapper) AuthLogin(ctx context.Context, req *pb.AuthLoginREQ) (*pb.AuthLoginRSP, error) {

	res, err := controller.Login(ctx, req)
	if err != nil {
		// 如果需要，可以将自定义错误映射到 gRPC 状态码，否则使用 Unknown.
		return nil, status.Error(codes.Unknown, err.GetErrDesc())
	}

	loginRes := res.(*pb.AuthLoginRSP)
	return loginRes, nil
}
