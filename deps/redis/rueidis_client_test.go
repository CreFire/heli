package redisclient

import (
	"context"
	"game/deps/xlog"
	"testing"
	"time"

	"github.com/redis/rueidis"
)

func TestNewRedisClientWithRueidis(t *testing.T) {
	// 创建一个测试用的logger
	logger := xlog.NewMyLogger("./logs/test", "debug", 2)

	xclient, err := NewRedisClientWithRueidis("redis://127.0.0.1:6379", logger)
	if err != nil {
		t.Errorf("NewRedisClientWithRueidis() error = %v", err)
		return
	}
	xclient.Do(context.Background(), xclient.B().Set().Key("test").Value("value").Ex(time.Minute).Build())
	str, err := xclient.DoCache(context.Background(), xclient.B().Get().Key("test").Cache(), time.Minute).ToString()
	if err != nil {
		t.Errorf("NewRedisClientWithRueidis() error = %v", err)
		return
	}
	str, err = xclient.DoCache(context.Background(), xclient.B().Get().Key("test").Cache(), time.Minute).ToString()
	t.Logf("NewRedisClientWithRueidis() str = %v", str)
	rueidis.MGetCache(xclient, context.Background(), time.Hour, []string{"json", "key2"})
}
