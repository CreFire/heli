package xgrpc

import (
	"context"
	"fmt"
	"game/deps/xlog"
	"math"
	"math/rand"
	"slices"
	"time"

	"github.com/sasha-s/go-deadlock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

var GrpcConnMgr = NewConnManager()
var ErrCantConnect = fmt.Errorf("cannot connect to server")

////////////////////////////////////////////////////////////////////////////////
// ConnManager
////////////////////////////////////////////////////////////////////////////////

type ConnManager struct {
	mu    deadlock.RWMutex
	conns map[string]*grpc.ClientConn
	opts  []grpc.DialOption
}

func NewConnManager(opts ...grpc.DialOption) *ConnManager {
	para := keepalive.ClientParameters{
		Time:                10 * time.Second,
		Timeout:             20 * time.Second,
		PermitWithoutStream: true,
	}
	opts = append(opts, grpc.WithKeepaliveParams(para))
	opts = append(opts, grpc.WithUnaryInterceptor(UnaryRetryInterceptor(DefaultRetryOptions())))
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	return &ConnManager{
		conns: make(map[string]*grpc.ClientConn),
		opts:  opts,
	}
}

func (m *ConnManager) GetConn(ctx context.Context, addr string) (*grpc.ClientConn, error) {
	m.mu.RLock()
	if c, ok := m.conns[addr]; ok {
		state := c.GetState()
		m.mu.RUnlock()
		if state == connectivity.Ready || state == connectivity.Idle {
			return c, nil
		}
	} else {
		m.mu.RUnlock()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if c, ok := m.conns[addr]; ok {
		if s := c.GetState(); s == connectivity.Ready || s == connectivity.Idle {
			return c, nil
		}
		_ = c.Close()
		delete(m.conns, addr)
	}

	conn, err := grpc.NewClient(addr, m.opts...)
	if err != nil {
		return nil, err
	}

	conn.Connect()

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			break
		}
		if !conn.WaitForStateChange(waitCtx, state) {
			xlog.Errorf("cannot connect to server: state=%v err=%v", conn.GetState(), waitCtx.Err())
			_ = conn.Close()
			return nil, ErrCantConnect
		}
	}

	m.conns[addr] = conn
	return conn, nil
}

func (m *ConnManager) CloseAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var lastErr error
	for addr, c := range m.conns {
		if err := c.Close(); err != nil {
			lastErr = err
		}
		delete(m.conns, addr)
	}
	return lastErr
}

////////////////////////////////////////////////////////////////////////////////
// 重试拦截器
////////////////////////////////////////////////////////////////////////////////

type RetryOptions struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
	RetryCodes     []codes.Code
}

func DefaultRetryOptions() RetryOptions {
	return RetryOptions{
		MaxRetries:     3,
		InitialBackoff: 200 * time.Millisecond,
		MaxBackoff:     2 * time.Second,
		Multiplier:     2.0,
		RetryCodes: []codes.Code{
			codes.Unavailable,
			codes.DeadlineExceeded,
		},
	}
}

func nextBackoff(base time.Duration, attempt int, multiplier float64, max time.Duration) time.Duration {
	backoff := float64(base) * math.Pow(multiplier, float64(attempt))
	if backoff > float64(max) {
		backoff = float64(max)
	}
	jitter := backoff * (0.5 + rand.Float64()/2) // 加抖动
	return time.Duration(jitter)
}

func UnaryRetryInterceptor(opt RetryOptions) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var lastErr error
		for attempt := 0; attempt <= opt.MaxRetries; attempt++ {
			lastErr = invoker(ctx, method, req, reply, cc, opts...)
			if lastErr == nil {
				return nil
			}
			st, ok := status.FromError(lastErr)
			if !ok {
				return lastErr
			}
			retryable := slices.Contains(opt.RetryCodes, st.Code())
			if !retryable {
				return lastErr
			}

			if attempt < opt.MaxRetries {
				backoff := nextBackoff(opt.InitialBackoff, attempt, opt.Multiplier, opt.MaxBackoff)
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
		return lastErr
	}
}
