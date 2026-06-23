package cache

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

func AddUserToLoginQueue(GamerId int64, unixMill int64) (int64, error) {
	rc := GetRedis()
	member := redis.Z{Score: float64(unixMill), Member: GamerId}
	err := rc.ZAdd(context.Background(), LoginQueueKey(), member).Err()
	if err != nil {
		return 0, err
	}

	// 获取用户在队列中的位置 (从0开始)
	rank, err := rc.ZRank(context.Background(), LoginQueueKey(), strconv.Itoa(int(GamerId))).Result()
	if err != nil {
		return 0, err
	}

	return rank + 1, nil // 返回从1开始的位置
}

func RemoveUserFromLoginQueue(GamerId []int64) error {
	rc := GetRedis()
	args := make([]any, len(GamerId))
	for i, id := range GamerId {
		args[i] = strconv.Itoa(int(id))
	}
	return rc.ZRem(context.Background(), LoginQueueKey(), args...).Err()
}

func GetTopNTimeFromLoginQueue(n int) (int64, error) {
	rc := GetRedis()
	result, err := rc.ZRangeWithScores(context.Background(), LoginQueueKey(), int64(n-1), int64(n-1)).Result()
	if err != nil {
		return 0, err
	}
	if len(result) == 0 {
		return time.Now().UnixMilli(), nil
	}
	return int64(result[0].Score), nil
}

func RemoveUserLessTimeFromLoginQueue(unixMill int64) error {
	rc := GetRedis()
	return rc.ZRemRangeByScore(context.Background(), LoginQueueKey(), "-inf", fmt.Sprintf("(%d", unixMill)).Err()
}

func GetUserLoginQueueRank(GamerId int64) (int64, error) {
	rc := GetRedis()

	rank, err := rc.ZRank(context.Background(), LoginQueueKey(), strconv.Itoa(int(GamerId))).Result()
	if err != nil && err != redis.Nil {
		return 0, err
	}

	return rank + 1, nil // 返回从1开始的位置
}

func GetCurQUeueSize() (int64, error) {
	rc := GetRedis()
	return rc.ZCard(context.Background(), LoginQueueKey()).Result()
}
