package cache

import (
	"context"
	"errors"
	"game/deps/xlog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type GamerOnlineData struct {
	Account     string `redis:"acc"`
	GamerId     int64  `redis:"gid"`
	AuthToken   string `redis:"token"`
	GateSession int64  `redis:"sess"`
	GateSvrId   int32  `redis:"gsid"`
	LoginTime   int64  `redis:"lgin,omitempty"`
	LogoutTime  int64  `redis:"lgou,omitempty"`
	PubSvrId    int32  `redis:"psid"`
	LogicSvrId  int32  `redis:"lsid"`
	ModuleFlag  int64  `redis:"modf,omitempty"`
}

var clearGateOnlineDataBySessionScript = redis.NewScript(`
	local currentSess = redis.call("HGET", KEYS[1], "sess")
	if not currentSess then
		return 0
	end
	if tonumber(currentSess) ~= tonumber(ARGV[1]) then
		return 0
	end
	redis.call("HSET", KEYS[1], "gsid", 0, "sess", 0)
	return 1
`)

func SetGamerOnlineData(gamerId int64, data *GamerOnlineData) error {
	_, err := GetRedis().Pipelined(context.Background(), func(p redis.Pipeliner) error {
		p.HSet(context.Background(), GamerOnlineKey(gamerId), data)
		p.Expire(context.Background(), GamerOnlineKey(gamerId), 30*24*time.Hour)
		return nil
	})
	if err != nil {
		xlog.Errorf("SetGamerOnlineData failed, gamerId: %d, error: %v", gamerId, err)
	}
	return err
}

func GetGamerOnlineData(gamerId int64) (*GamerOnlineData, error) {
	data := &GamerOnlineData{}
	err := GetRedis().HGetAll(context.Background(), GamerOnlineKey(gamerId)).Scan(data)
	if err == redis.Nil {
		return &GamerOnlineData{
			GamerId: gamerId,
		}, nil
	}
	return data, err
}

func SetGamerToken(gamerId int64, token string) error {
	return GetRedis().HSet(context.Background(), GamerOnlineKey(gamerId), "token", token).Err()
}

func GetGamerToken(gamerId int64) (string, error) {
	return GetRedis().HGet(context.Background(), GamerOnlineKey(gamerId), "token").Result()
}

func DelGamerToken(gamerId int64) error {
	err := GetRedis().HDel(context.Background(), GamerOnlineKey(gamerId), "token").Err()
	return err
}

func GetGateSvrId(gamerId int64) (int32, int64, error) {
	rc := GetRedis()
	rets, err := rc.HMGet(context.Background(), GamerOnlineKey(gamerId), "gsid", "sess").Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, 0, nil
		}
		xlog.Errorf("Get GateSvrId failed, gamerId: %d error: %v", gamerId, err)
		return 0, 0, err
	}

	gateSvrId, _ := rets[0].(int32)
	session, _ := rets[1].(int64)

	return gateSvrId, session, nil
}

func SetGateSvrId(gamerId int64, gateSvrId int32, session int64) error {
	rc := GetRedis()
	return rc.HSet(context.Background(), GamerOnlineKey(gamerId), "gsid", gateSvrId, "sess", session).Err()
}

func ClearGateOnlineDataBySession(gamerId int64, session int64) (bool, error) {
	ret, err := clearGateOnlineDataBySessionScript.Run(
		context.Background(),
		GetRedis(),
		[]string{GamerOnlineKey(gamerId)},
		session,
	).Int()
	if err != nil {
		xlog.Errorf("clear gate online data by session failed. gid:%d sess:%d err:%v", gamerId, session, err)
		return false, err
	}
	return ret == 1, nil
}

func GetLogicSvrId(gamerId int64) (int32, error) {
	rc := GetRedis()
	logicSvrId, err := rc.HGet(context.Background(), GamerOnlineKey(gamerId), "lsid").Int()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		xlog.Errorf("Get LogicSvrId failed, gamerId: %d error: %v", gamerId, err)
	}
	return int32(logicSvrId), err
}

func SetLogicSvrId(gamerId int64, logicSvrId int32) error {
	rc := GetRedis()
	if err := rc.HSet(context.Background(), GamerOnlineKey(gamerId), "lsid", logicSvrId).Err(); err != nil {
		xlog.Errorf("gamer online op failed. op:set_logic_svr_id gid:%d logicSvrId:%d err:%v", gamerId, logicSvrId, err)
		return err
	}
	return nil
}

func ClearGamerPubServerId(gamerId int64) error {
	rc := GetRedis()
	return rc.HSet(context.Background(), GamerOnlineKey(gamerId), "psid", 0).Err()
}

func GetPubSvrId(gamerId int64) (int32, error) {
	rc := GetRedis()
	pubSvrId, err := rc.HGet(context.Background(), GamerOnlineKey(gamerId), "psid").Int()
	return int32(pubSvrId), err
}

func ClearGamerOnlineData(gamerId int64) error {
	return GetRedis().HMSet(context.Background(), GamerOnlineKey(gamerId), "gsid", 0, "lsid", 0, "match_svr_id", 0, "sess", "").Err()
}

func ClearGamersLogicServerId(gamerIds []int64) error {
	return batchPipeline(context.Background(), gamerIds, 1000, func(pipe redis.Pipeliner, gamerId int64) error {
		pipe.HSet(context.Background(), GamerOnlineKey(gamerId), "lsid", 0)
		return nil
	})
}

/*---------------------------account---------------------------------*/
func GetAccountSession(account string) (string, error) {
	return GetRedis().Get(context.Background(), AccountSessionKey(account)).Result()
}

func SetAccountSession(account, session string) error {
	return GetRedis().Set(context.Background(), AccountSessionKey(account), session, 0).Err()
}

func DelAccountSession(account string) error {
	return GetRedis().Del(context.Background(), AccountSessionKey(account)).Err()
}

// 批量获取玩家的gatesvrid 和 session
func GetGamerGateSvrIdAndSession(gamerIds []int64) (map[int64]int32, map[int64]int64, error) {
	if len(gamerIds) == 0 {
		return make(map[int64]int32), make(map[int64]int64), nil
	}

	rc := GetRedis()
	ctx := context.Background()

	// 初始化返回结果的map
	gateSvrIds := make(map[int64]int32, len(gamerIds))
	sessions := make(map[int64]int64, len(gamerIds))

	// 构建管道操作
	pipe := rc.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, len(gamerIds))

	for i, gamerId := range gamerIds {
		cmds[i] = pipe.HGetAll(ctx, GamerOnlineKey(gamerId))
	}

	// 执行管道操作
	_, err := pipe.Exec(ctx)
	if err != nil {
		xlog.Errorf("GetGamerGateSvrIdAndSession failed, error: %v", err)
		return nil, nil, err
	}

	// 处理结果
	for i, cmd := range cmds {
		gamerId := gamerIds[i]

		result, err := cmd.Result()
		if err != nil {
			if !errors.Is(err, redis.Nil) {
				xlog.Errorf("GetGamerGateSvrIdAndSession failed for gamerId: %d, error: %v", gamerId, err)
			}
			// 如果没有找到数据，将值设置为默认值（0）
			continue
		}

		// 解析结果
		var gateSvrId int32 = 0
		var session int64 = 0

		if val, exists := result["gsid"]; exists {
			if parsedVal, err := strconv.ParseInt(val, 10, 32); err == nil {
				gateSvrId = int32(parsedVal)
			}
		}

		if val, exists := result["sess"]; exists {
			if parsedVal, err := strconv.ParseInt(val, 10, 64); err == nil {
				session = parsedVal
			}
		}

		gateSvrIds[gamerId] = gateSvrId
		sessions[gamerId] = session
	}

	return gateSvrIds, sessions, nil
}
