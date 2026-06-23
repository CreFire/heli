package grpc

import (
	"context"
	"game/deps/transport"
	"net"
	"strings"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

var _ transport.Transporter = (*ServerTransport)(nil)

// ServerTransport is a gRPC server transport.
type ServerTransport struct {
	operation   string
	reqHeader   headerCarrier
	replyHeader headerCarrier
	clientIP    string
	endpoint    string
}

// NewServerTransport creates a new gRPC server transport.
func NewServerTransport(ctx context.Context, operation string) *ServerTransport {
	var clientIP, endpoint string
	clientIP = GrpcClientRealIP(ctx)

	md, _ := metadata.FromIncomingContext(ctx)
	if e := md.Get(":authority"); len(e) > 0 {
		endpoint = e[0]
	}

	return &ServerTransport{
		operation:   operation,
		reqHeader:   headerCarrier(md),
		replyHeader: headerCarrier(metadata.MD{}),
		clientIP:    clientIP,
		endpoint:    endpoint,
	}
}

// Kind returns the transport kind.
func (t *ServerTransport) Kind() transport.Kind {
	return transport.KindGRPC
}

// Endpoint returns the transport endpoint.
func (t *ServerTransport) Endpoint() string { return t.endpoint }

// Operation returns the transport operation.
func (t *ServerTransport) Operation() string { return t.operation }

// RequestHeader returns the request header.
func (t *ServerTransport) RequestHeader() transport.Header { return t.reqHeader }

// ReplyHeader returns the reply header.
func (t *ServerTransport) ReplyHeader() transport.Header { return t.replyHeader }

// ClientIP returns the client's IP address.
func (t *ServerTransport) ClientIP() string { return t.clientIP }

func GrpcClientRealIP(ctx context.Context) string {
	var clientIP string
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if xRealIP := md.Get("x-real-ip"); len(xRealIP) > 0 && xRealIP[0] != "" {
			clientIP = strings.Split(xRealIP[0], ",")[0]
		} else if xForwardedFor := md.Get("x-forwarded-for"); len(xForwardedFor) > 0 && xForwardedFor[0] != "" {
			ips := strings.Split(xForwardedFor[0], ",")
			if len(ips) > 0 {
				clientIP = strings.TrimSpace(ips[0])
			}
		}
	}

	if clientIP == "" {
		if p, ok := peer.FromContext(ctx); ok {
			clientIP = p.Addr.String()
			if host, _, err := net.SplitHostPort(clientIP); err == nil {
				clientIP = host
			}
		}
	}

	return clientIP
}
