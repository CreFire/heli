package xhttp

import (
	"context"
	"runtime/debug"
	"time"

	"game/deps/transport"
	tgrpc "game/deps/transport/grpc"
	"game/deps/xlog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// UnaryServerLoggingInterceptor 返回一个新的一元服务器拦截器，用于记录 gRPC 请求、响应、错误和崩溃。
func UnaryServerLoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		startTime := time.Now()

		// 从 context 中获取客户端 IP
		clientIP := tgrpc.GrpcClientRealIP(ctx)

		// 从 panic 中恢复
		defer func() {
			if r := recover(); r != nil {
				reqProto, _ := req.(proto.Message)
				// 记录 panic
				xlog.Errorw("[GRPC-PANIC]",
					"method", info.FullMethod,
					"panic", r,
					"client_ip", clientIP,
					"request", protojson.MarshalOptions{Multiline: false}.Format(reqProto),
					"stack", string(debug.Stack()),
				)
				// 将 panic 转换为 gRPC 错误
				err = status.Errorf(codes.Internal, "panic: %v", r)
			}
		}()

		tr := tgrpc.NewServerTransport(ctx, info.FullMethod)
		newCtx := transport.NewClientContext(ctx, tr)
		// 调用 handler 处理 RPC
		resp, err = handler(newCtx, req)
		duration := time.Since(startTime)
		reqProto, _ := req.(proto.Message)
		if err != nil {
			// 记录错误
			jsonReq := protojson.MarshalOptions{Multiline: false, EmitUnpopulated: true}.Format(reqProto)
			st, _ := status.FromError(err)
			xlog.Warnw("[GRPC-ERROR]",
				"method", info.FullMethod,
				"code", st.Code(),
				"message", st.Message(),
				"duration", duration.String(),
				"client_ip", clientIP,
				"request", jsonReq,
			)
		} else {
			if xlog.GetLogLevel() == xlog.LOG_LEVEL_DEBUG {
				jsonReq := protojson.MarshalOptions{Multiline: false, EmitUnpopulated: true}.Format(reqProto)
				respProto, _ := resp.(proto.Message)
				xlog.Debugw("[GRPC-SUCCESS]",
					"method", info.FullMethod,
					"duration", duration.String(),
					"client_ip", clientIP,
					"request", jsonReq,
					"response", protojson.MarshalOptions{Multiline: false, EmitUnpopulated: true}.Format(respProto),
				)
			}
		}

		return resp, err
	}
}

// StreamServerLoggingInterceptor 返回一个新的流式服务器拦截器，用于记录流的生命周期和错误。
func StreamServerLoggingInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		startTime := time.Now()
		clientIP := "unknown"
		p, ok := peer.FromContext(ss.Context())
		if ok && p != nil && p.Addr != nil {
			clientIP = p.Addr.String()
		}

		xlog.Infow("[GRPC-STREAM-START]",
			"method", info.FullMethod,
			"client_ip", clientIP,
		)

		defer func() {
			if r := recover(); r != nil {
				xlog.Errorw("[GRPC-STREAM-PANIC]",
					"method", info.FullMethod,
					"panic", r,
					"client_ip", clientIP,
					"stack", string(debug.Stack()),
				)
				err = status.Errorf(codes.Internal, "panic: %v", r)
			}

			duration := time.Since(startTime)
			xlog.Infow("[GRPC-STREAM-END]",
				"method", info.FullMethod,
				"duration", duration.String(),
				"error", err,
				"client_ip", clientIP,
			)
		}()

		return handler(srv, ss)
	}
}
