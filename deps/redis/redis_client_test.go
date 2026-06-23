package redisclient

import (
	"runtime"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisClientDefaults(t *testing.T) {
	cpu := runtime.GOMAXPROCS(0)

	opt := &redis.Options{}
	if opt.PoolSize == 0 {
		opt.PoolSize = min(5*cpu, 100)
	}
	if opt.ConnMaxIdleTime == 0 {
		opt.ConnMaxIdleTime = 5 * time.Minute
	}
	if opt.MaxIdleConns == 0 {
		opt.MaxIdleConns = min(2*cpu, 30)
	}

	if opt.PoolSize != min(5*cpu, 100) {
		t.Fatalf("unexpected PoolSize: %d", opt.PoolSize)
	}
	if opt.ConnMaxIdleTime != 5*time.Minute {
		t.Fatalf("unexpected ConnMaxIdleTime: %s", opt.ConnMaxIdleTime)
	}
	if opt.MaxIdleConns != min(2*cpu, 30) {
		t.Fatalf("unexpected MaxIdleConns: %d", opt.MaxIdleConns)
	}
}

func TestRedisClientExplicitValuesArePreserved(t *testing.T) {
	opt := &redis.Options{
		PoolSize:        11,
		ConnMaxIdleTime: time.Minute,
		MaxIdleConns:    7,
	}

	cpu := runtime.GOMAXPROCS(0)
	if opt.PoolSize == 0 {
		opt.PoolSize = min(5*cpu, 100)
	}
	if opt.ConnMaxIdleTime == 0 {
		opt.ConnMaxIdleTime = 5 * time.Minute
	}
	if opt.MaxIdleConns == 0 {
		opt.MaxIdleConns = min(2*cpu, 30)
	}

	if opt.PoolSize != 11 {
		t.Fatalf("PoolSize overwritten: %d", opt.PoolSize)
	}
	if opt.ConnMaxIdleTime != time.Minute {
		t.Fatalf("ConnMaxIdleTime overwritten: %s", opt.ConnMaxIdleTime)
	}
	if opt.MaxIdleConns != 7 {
		t.Fatalf("MaxIdleConns overwritten: %d", opt.MaxIdleConns)
	}
}
