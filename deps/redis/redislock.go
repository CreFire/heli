package redisclient

import (
	"context"
	"errors"
	"game/deps/xlog"
	"sync"
	"time"

	"github.com/bsm/redislock"
	"github.com/redis/go-redis/v9"
)

const (
	defaultRedLockTTL      = time.Minute
	defaultShortRedLockTTL = 10 * time.Second
	defaultLockReleaseTTL  = time.Second
)

func RedLock(rc redis.Cmdable, ctx context.Context, key string) (cancel context.CancelFunc, err error) {
	ttl := defaultRedLockTTL
	locker, err := redislock.Obtain(ctx, rc, key, ttl, nil)
	if err != nil {
		return nil, err
	}

	ticker := time.NewTicker(ttl / 3)
	ctx, cancelFunc := context.WithCancel(context.Background())
	xlog.Debugf("redislock watch dog start %s", locker.Key())

	go func(ctx context.Context, t *time.Ticker) {
		defer locker.Release(context.Background())
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				xlog.Debugf("redislock watch dog stop %s", locker.Key())
				return
			case <-t.C:
				err := locker.Refresh(ctx, ttl, nil)
				if err != nil {
					xlog.Errorf("redislock fresh %s error %s", locker.Key(), err.Error())
				}
			}
		}
	}(ctx, ticker)

	return cancelFunc, err
}

// RedLockShort 获取一个短生命周期锁，不启动 watch dog。
// 适用于临界区足够短、允许锁到期自然释放的场景，以降低 goroutine/ticker 开销。
func RedLockShort(rc redis.Cmdable, ctx context.Context, key string) (cancel context.CancelFunc, err error) {
	return RedLockShortWithTTL(rc, ctx, key, defaultShortRedLockTTL)
}

// RedLockShortWithTTL 获取一个短生命周期锁，不启动 watch dog，TTL 由调用方指定。
func RedLockShortWithTTL(rc redis.Cmdable, ctx context.Context, key string, ttl time.Duration) (cancel context.CancelFunc, err error) {
	if ttl <= 0 {
		ttl = defaultShortRedLockTTL
	}
	locker, err := redislock.Obtain(ctx, rc, key, ttl, nil)
	if err != nil {
		return nil, err
	}

	var once sync.Once
	release := func() {
		once.Do(func() {
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), defaultLockReleaseTTL)
			defer releaseCancel()
			if releaseErr := locker.Release(releaseCtx); releaseErr != nil && !errors.Is(releaseErr, redislock.ErrLockNotHeld) {
				xlog.Warnf("redislock release %s error %s", locker.Key(), releaseErr.Error())
			}
		})
	}
	return release, nil
}
