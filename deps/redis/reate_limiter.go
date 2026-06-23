package redisclient

import (
	"context"
	"game/deps/xtime"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"
)

func CheckModuleKeyLimit(module, key string, limit redis_rate.Limit, client redis.Cmdable) (bool, int32) {
	LimitKey := module + ":limiter:" + key
	limiter := redis_rate.NewLimiter(client)
	res, err := limiter.Allow(context.Background(), LimitKey, limit)
	if err != nil {
		return false, 0
	}

	if res.Allowed == 0 {
		return false, int32(res.ResetAfter.Seconds())
	}

	return true, 0
}

type DailyLimit struct {
	DayCount      int32 `redis:"day_count"`
	LastCountTime int64 `redis:"last_count_time"`
}

var incDailyScript = redis.NewScript(`
	if redis.call("EXISTS", KEYS[1]) == 0 then
		redis.call("HMSET", KEYS[1], "day_count", ARGV[1], "last_count_time", ARGV[3])
		redis.call("EXPIRE", KEYS[1], 86400)
		return ARGV[1]
	end
	local nowDayZero = tonumber(ARGV[3])
	local max = tonumber(ARGV[2])
	local inc = tonumber(ARGV[1])
	local lt = tonumber(redis.call("HGET", KEYS[1], "last_count_time"))

	if lt <  nowDayZero then
		redis.call("HMSET", KEYS[1], "day_count", ARGV[1], "last_count_time", ARGV[3])
		redis.call("EXPIRE", KEYS[1], 86400)
		return ARGV[1]
	end
	local nc =tonumber(redis.call("HGET", KEYS[1], "day_count"))
	if inc + nc <= max then
		redis.call("HINCRBY", KEYS[1], "day_count", ARGV[1])
		return tostring(inc + nc)
	else
		return tostring(inc + nc)
	end`)

func CheckIncDailyLimit(key string, inc, max int32, client *RedisClient) (bool, int32) {
	dayFresh := xtime.GetLocalDayZeroUnixWithOffSet(time.Now().Unix())
	Key := "dailyLimit:" + key
	cmder := incDailyScript.Run(context.Background(), client.Client, []string{Key}, inc, max, dayFresh)
	count, err := cmder.Int()
	if err != nil {
		return false, 0
	}

	if int32(count) > max {
		return false, int32(count) - inc
	}

	return true, int32(count)
}

func GetDailyLimit(key string, client redis.Cmdable) (*DailyLimit, error) {
	dl := DailyLimit{}
	key = "dailyLimit:" + key
	err := client.HGetAll(context.Background(), key).Scan(&dl)
	if err == nil || err == redis.Nil {
		return &dl, nil
	}

	return nil, err
}
