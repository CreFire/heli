package main

import (
	"game/deps/server"
	"game/deps/xhttp"
	"game/deps/xlog"
	"game/src/proto/pb"
	"game/src/service/auth/controller"
)

// RegisterHandlers 注册 auth 服务的所有 HTTP 和 gRPC 处理器.
func RegisterHandlers() error {

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

	if err := server.MS.HttpServe.RegisterProtoHandler("/api/auth", handlers); err != nil {
		return err
	}
	xlog.Infof("HTTP handlers for auth registered.")
	return nil
}
