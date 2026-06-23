package xhttp

import (
	"fmt"
	servicemgr "game/deps/service_mgr"
	"game/deps/xlog"
	"game/deps/xtoken"
	"net/http"
	"net/http/httputil"
	"runtime/debug"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap/zapcore"
)

var (
	proxy *httputil.ReverseProxy
)

func InitProxy(svm *servicemgr.Manager) *httputil.ReverseProxy {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 100
	t.MaxConnsPerHost = 200
	t.MaxIdleConnsPerHost = 5
	t.IdleConnTimeout = time.Minute * 5
	director := func(req *http.Request) {
		serviceName := req.Header.Get("TargetServiceName")

		ins, err := svm.PickRandom(serviceName, false)
		if err != nil {
			xlog.Errorf("get service instance failed , service %s error %s", serviceName, err.Error())
			return
		}
		host := fmt.Sprintf("%s:%d", ins.Host, ins.Port)
		req.URL.Scheme = "http"
		req.URL.Host = host
		req.Host = host
	}

	bufferPool := NewHTTPBufferPool(32 * 1024) // 32KB 缓冲区

	p := &httputil.ReverseProxy{
		Director:   director,
		Transport:  t,
		ErrorLog:   xlog.StdLogger(xlog.DefaultLogger, zapcore.ErrorLevel, 1),
		BufferPool: bufferPool,
	}
	proxy = p
	return p
}

func ReverseProxy(serviceName string) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		userId := int64(0)
		defer func() {
			if err := recover(); err != nil {
				xlog.Errorf("ReverseProxy: %s  userId: %d, err: %v, panic stack info %s ",
					ctx.Request.URL.Path, userId, err, string(debug.Stack()))
				ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "server error"})
			}
		}()
		start := time.Now()
		if token := ctx.GetHeader("Authorization"); len(token) > 0 {
			userId, _ = xtoken.UserTokenDecode(token, "")
		}

		ctx.Request.Header.Add("TargetServiceName", serviceName)
		ctx.Request.Header.Add("UserId", fmt.Sprintf("%d", userId))

		proxy.ServeHTTP(ctx.Writer, ctx.Request)
		cost := time.Since(start)
		xlog.Debugf("reverse proxy: %s cost: %v , userId: %d", ctx.Request.URL.Path, cost, userId)
	}
}

// HTTPBufferPool 实现了 httputil.ReverseProxy 的 BufferPool 接口
// 用于提供可复用的缓冲区，以提高 io.CopyBuffer 的性能
type HTTPBufferPool struct {
	pool sync.Pool
}

// NewHTTPBufferPool 创建一个新的 HTTPBufferPool 实例
// size 参数指定缓冲区大小
func NewHTTPBufferPool(size int) *HTTPBufferPool {
	if size <= 0 {
		size = 32 * 1024 // 默认 32KB
	}
	return &HTTPBufferPool{
		pool: sync.Pool{
			New: func() any {
				b := make([]byte, size)
				return &b
			},
		},
	}
}

// Get 从池中获取一个缓冲区
func (bp *HTTPBufferPool) Get() []byte {
	return *bp.pool.Get().(*[]byte)
}

// Put 将缓冲区放回池中
func (bp *HTTPBufferPool) Put(buffer []byte) {
	// 可选：检查缓冲区大小是否符合预期，防止放入不合适的切片
	bp.pool.Put(&buffer)
}
