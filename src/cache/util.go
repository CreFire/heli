package cache

import (
	"context"
	"game/deps/server"

	"github.com/redis/go-redis/v9"
)

func GetRedis() redis.Cmdable {
	return server.MS.RedisDB.Client
}

// batchPipeline splits large batches into chunks and executes them via Redis pipelining.
// fn should only enqueue commands on the provided pipe.
func batchPipeline[T any](ctx context.Context, items []T, batchSize int, fn func(redis.Pipeliner, T) error) error {
	if len(items) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 1000
	}
	rc := GetRedis()
	var lastErr error
	for start := 0; start < len(items); start += batchSize {
		end := min(start+batchSize, len(items))
		_, err := rc.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			for _, item := range items[start:end] {
				if err := fn(pipe, item); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			lastErr = err
		}
	}
	return lastErr
}
