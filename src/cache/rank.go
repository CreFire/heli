package cache

import (
	"context"
	"errors"
	"game/deps/xlog"

	"github.com/redis/go-redis/v9"
)

const (
	REDIS_COUNT_KEY = "wb_gamer_count"
)

func UpdateWBCountRank(rankList []string) map[string]int64 {
	if len(rankList) == 0 {
		return nil
	}
	ctx := context.Background()
	pipe := GetRedis().Pipeline()
	cmds := map[string]*redis.IntCmd{}
	for _, strGid := range rankList {
		cmds[strGid] = pipe.HIncrBy(ctx, REDIS_COUNT_KEY, strGid, 1)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		xlog.Errorf("pipe.Exec() failed... err : %v", err.Error())
		return nil
	}
	temp := map[string]int64{}
	for strGid, cmd := range cmds {
		num, err := cmd.Result()
		if err != nil {
			continue
		}
		temp[strGid] = num
	}
	return temp

}

func CalcRankSection(rankKey string, maxCount int64, rateList []int64) ([]int64, error) {
	if len(rateList) == 0 {
		return []int64{}, nil
	}
	pipe := GetRedis().Pipeline()
	ctx := context.Background()

	var cmds []*redis.ZSliceCmd
	for _, rate := range rateList {
		num := max(maxCount*rate/10000, 0)
		cmds = append(cmds, pipe.ZRevRangeWithScores(ctx, rankKey, num, num))
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	var newSection []int64
	for _, cmd := range cmds {
		val, err := cmd.Result()
		if err != nil || len(val) == 0 {
			continue
		}
		//_, score := common.SplitInt64ToInt8AndInt64(int64(val[0].Score))
		newSection = append(newSection, int64(val[0].Score))
	}
	return newSection, nil
}

func GetRankMaxCountAndRanking(rankName, mem string) (int64, int64, int64, error) {
	ctx := context.Background()
	pipe := GetRedis().Pipeline()
	maxCountCmd := pipe.ZCard(ctx, rankName)
	rankingCmd := pipe.ZRevRank(ctx, rankName, mem)
	scoreCmd := pipe.ZScore(ctx, rankName, mem)
	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return 0, -1, 0, err
	}

	maxCount, err := maxCountCmd.Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return 0, -1, 0, err
	}

	ranking, err := rankingCmd.Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return maxCount, -1, 0, nil
		}
		return 0, -1, 0, err
	}
	score, err := scoreCmd.Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return maxCount, ranking, 0, nil
		}
		return 0, -1, 0, err
	}
	return maxCount, ranking, int64(score), nil
}
