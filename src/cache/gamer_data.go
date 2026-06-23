package cache

import (
	"context"

	"github.com/redis/go-redis/v9"
)

func SetRedFlag(gamerIds []int64, FlagIndex int64) error {
	return batchPipeline(context.Background(), gamerIds, 1000, func(pipe redis.Pipeliner, gamerId int64) error {
		pipe.SetBit(context.Background(), GamerRedFlagKey(gamerId), FlagIndex, 1)
		return nil
	})
}

func GetRedFlag(gamerId int64, FlagIndex int64) ([]byte, error) {
	rc := GetRedis()
	return rc.Get(context.Background(), GamerRedFlagKey(gamerId)).Bytes()
}

func ClearRedFlag(gamerId int64) error {
	rc := GetRedis()
	return rc.Del(context.Background(), GamerRedFlagKey(gamerId)).Err()
}
