package grpc

import (
	"context"
	"game/deps/etcd"
	"game/deps/selector"
	"game/deps/selector/random"
	"game/deps/transport"
	"game/deps/transport/grpc/resolver/discovery"

	"crypto/tls"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/credentials"
	grpcinsecure "google.golang.org/grpc/credentials/insecure"
)

func init() {
	if selector.GlobalSelector() == nil {
		selector.SetGlobalSelector(random.NewBuilder())
	}

	b := base.NewBalancerBuilder(
		balancerName,
		&balancerBuilder{
			builder: selector.GlobalSelector(),
		},
		base.Config{HealthCheck: true},
	)
	balancer.Register(b)
}

// ClientOption is gRPC client option.
type ClientOption func(o *clientOptions)

// WithEndpoint with client endpoint.
func WithEndpoint(endpoint string) ClientOption {
	return func(o *clientOptions) {
		o.endpoint = endpoint
	}
}

// WithSubset with client discovery subset size.
// zero value means subset filter disabled
func WithSubset(size int) ClientOption {
	return func(o *clientOptions) {
		o.subsetSize = size
	}
}

// WithTimeout with client timeout.
func WithTimeout(timeout time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.timeout = timeout
	}
}

//
//// WithMiddleware with client middleware.
//func WithMiddleware(m ...middleware.Middleware) ClientOption {
//	return func(o *clientOptions) {
//		o.middleware = m
//	}
//}

// WithDiscovery with client discovery.
func WithDiscovery(d etcd.Discovery) ClientOption {
	return func(o *clientOptions) {
		o.discovery = d
	}
}

// WithTLSConfig with TLS config.
func WithTLSConfig(c *tls.Config) ClientOption {
	return func(o *clientOptions) {
		o.tlsConf = c
	}
}

// WithUnaryInterceptor returns a DialOption that specifies the interceptor for unary RPCs.
func WithUnaryInterceptor(in ...grpc.UnaryClientInterceptor) ClientOption {
	return func(o *clientOptions) {
		o.ints = in
	}
}

// WithStreamInterceptor returns a DialOption that specifies the interceptor for streaming RPCs.
func WithStreamInterceptor(in ...grpc.StreamClientInterceptor) ClientOption {
	return func(o *clientOptions) {
		o.streamInts = in
	}
}

// WithOptions with gRPC options.
func WithOptions(opts ...grpc.DialOption) ClientOption {
	return func(o *clientOptions) {
		o.grpcOpts = opts
	}
}

// WithNodeFilter with select filters
func WithNodeFilter(filters ...selector.NodeFilter) ClientOption {
	return func(o *clientOptions) {
		o.filters = filters
	}
}

// WithHealthCheck with health check
func WithHealthCheck(healthCheck bool) ClientOption {
	return func(o *clientOptions) {
		if !healthCheck {
			o.healthCheckConfig = ""
		}
	}
}

// WithLogger with logger
// Deprecated: use global logger instead.
func WithLogger(_ log.Logger) ClientOption {
	return func(o *clientOptions) {}
}

func WithPrintDiscoveryDebugLog(p bool) ClientOption {
	return func(o *clientOptions) {
		o.printDiscoveryDebugLog = p
	}
}

// clientOptions is gRPC Client
type clientOptions struct {
	endpoint   string
	subsetSize int
	tlsConf    *tls.Config
	timeout    time.Duration
	discovery  etcd.Discovery
	//middleware             []middleware.Middleware
	ints                   []grpc.UnaryClientInterceptor
	streamInts             []grpc.StreamClientInterceptor
	grpcOpts               []grpc.DialOption
	balancerName           string
	filters                []selector.NodeFilter
	healthCheckConfig      string
	printDiscoveryDebugLog bool
}

// Dial returns a GRPC connection.
func Dial(ctx context.Context, opts ...ClientOption) (*grpc.ClientConn, error) {
	return dial(ctx, false, opts...)
}

// DialInsecure returns an insecure GRPC connection.
func DialInsecure(ctx context.Context, opts ...ClientOption) (*grpc.ClientConn, error) {
	return dial(ctx, true, opts...)
}

func dial(ctx context.Context, insecure bool, opts ...ClientOption) (*grpc.ClientConn, error) {
	options := clientOptions{
		timeout:                2000 * time.Millisecond,
		balancerName:           balancerName,
		subsetSize:             25,
		printDiscoveryDebugLog: true,
		healthCheckConfig:      `,"healthCheckConfig":{"serviceName":""}`,
	}
	for _, o := range opts {
		o(&options)
	}
	//ints := []grpc.UnaryClientInterceptor{
	//	//unaryClientInterceptor(options.middleware, options.timeout, options.filters),
	//}
	sints := []grpc.StreamClientInterceptor{
		streamClientInterceptor(options.filters),
	}

	//if len(options.ints) > 0 {
	//	ints = append(ints, options.ints...)
	//}
	if len(options.streamInts) > 0 {
		sints = append(sints, options.streamInts...)
	}
	grpcOpts := []grpc.DialOption{
		grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingConfig": [{"%s":{}}]%s}`,
			options.balancerName, options.healthCheckConfig)),
		//grpc.WithChainUnaryInterceptor(ints...),
		grpc.WithChainStreamInterceptor(sints...),
	}

	if options.discovery != nil {
		grpcOpts = append(grpcOpts,
			grpc.WithResolvers(
				discovery.NewBuilder(
					options.discovery,
					discovery.WithInsecure(insecure),
					discovery.WithSubset(options.subsetSize),
					discovery.PrintDebugLog(options.printDiscoveryDebugLog),
				)))
	}
	if insecure {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(grpcinsecure.NewCredentials()))
	}
	if options.tlsConf != nil {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(credentials.NewTLS(options.tlsConf)))
	}
	if len(options.grpcOpts) > 0 {
		grpcOpts = append(grpcOpts, options.grpcOpts...)
	}
	return grpc.DialContext(ctx, options.endpoint, grpcOpts...)
}

//// 一元拦截器(中间件)
//func unaryClientInterceptor(ms []middleware.Middleware, timeout time.Duration, filters []selector.NodeFilter) grpc.UnaryClientInterceptor {
//	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
//		ctx = transport.NewClientContext(ctx, &Transport{
//			endpoint:    cc.Target(),
//			operation:   method,
//			reqHeader:   headerCarrier{},
//			nodeFilters: filters,
//		})
//		if timeout > 0 {
//			var cancel context.CancelFunc
//			ctx, cancel = context.WithTimeout(ctx, timeout)
//			defer cancel()
//		}
//		h := func(ctx context.Context, req interface{}) (interface{}, error) {
//			if tr, ok := transport.FromClientContext(ctx); ok {
//				header := tr.RequestHeader()
//				keys := header.Keys()
//				keyvals := make([]string, 0, len(keys))
//				for _, k := range keys {
//					keyvals = append(keyvals, k, header.Get(k))
//				}
//				ctx = grpcmd.AppendToOutgoingContext(ctx, keyvals...)
//			}
//			return reply, invoker(ctx, method, req, reply, cc, opts...)
//		}
//		if len(ms) > 0 {
//			h = middleware.Chain(ms...)(h)
//		}
//		var p selector.Peer
//		ctx = selector.NewPeerContext(ctx, &p)
//		_, err := h(ctx, req)
//		return err
//	}
//}

// 流式拦截器(过滤)
func streamClientInterceptor(filters []selector.NodeFilter) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) { // nolint
		ctx = transport.NewClientContext(ctx, &Transport{
			endpoint:    cc.Target(),
			operation:   method,
			reqHeader:   headerCarrier{},
			nodeFilters: filters,
		})
		var p selector.Peer
		ctx = selector.NewPeerContext(ctx, &p)
		return streamer(ctx, desc, cc, method, opts...)
	}
}
