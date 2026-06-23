package cache

import (
	"context"
	"encoding/json"
	"errors"
	"game/deps/misc"
	servicemgr "game/deps/service_mgr"
	"game/deps/xlog"
	"game/deps/xtime"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

func UpdateServerInfo(inst *servicemgr.ServiceInstance) error {
	inst.UpdateTime, _ = xtime.TimestampToString(xtime.NowUnix(), time.DateTime)
	js, _ := json.Marshal(inst)
	key := ServerInfoKey(inst.ServiceName, inst.InstanceId)
	return GetRedis().Set(context.Background(), key, js, time.Hour*24).Err()
}

func UpdateOnlineCount(serviceName string, serverId int32, online int32) error {
	key := ServerOnlineKey(serviceName)
	return GetRedis().HSet(context.Background(), key, misc.IntToStr(serverId), online).Err()
}
func UpdateServerInfoWithOnline(inst *servicemgr.ServiceInstance) error {
	inst.UpdateTime, _ = xtime.TimestampToString(xtime.NowUnix(), time.DateTime)
	js, _ := json.Marshal(inst)
	key := ServerInfoKey(inst.ServiceName, inst.InstanceId)
	rc := GetRedis()
	_, err := rc.Pipelined(context.Background(), func(p redis.Pipeliner) error {
		p.Set(context.Background(), key, js, time.Hour*24)
		p.HSet(context.Background(), ServerOnlineKey(inst.ServiceName), misc.IntToStr(inst.InstanceId), inst.OnlineCount())
		return nil
	})

	return err
}

func GetServerInfo(serverName string, instId int32) (*servicemgr.ServiceInstance, error) {
	key := ServerInfoKey(serverName, instId)
	js, err := GetRedis().Get(context.Background(), key).Result()
	if err != nil {
		return nil, err
	}
	inst := &servicemgr.ServiceInstance{}
	err = json.Unmarshal([]byte(js), inst)
	return inst, err
}

func DelServerInfo(inst *servicemgr.ServiceInstance) error {
	key := ServerInfoKey(inst.ServiceName, inst.InstanceId)
	rc := GetRedis()
	_, err := rc.Pipelined(context.Background(), func(p redis.Pipeliner) error {
		p.Del(context.Background(), key)
		p.HDel(context.Background(), ServerOnlineKey(inst.ServiceName), misc.IntToStr(inst.InstanceId))
		return nil
	})
	return err
}

func GetServiceAllOnline(serviceName string) (map[int]int, error) {
	m := GetRedis().HGetAll(context.Background(), ServerOnlineKey(serviceName))
	if m.Err() != nil {
		if errors.Is(m.Err(), redis.Nil) {
			return nil, nil
		}
		return nil, m.Err()
	}

	ret := make(map[int]int, len(m.Val()))
	for k, v := range m.Val() {
		ret[misc.StrToInt(k)] = misc.StrToInt(v)
	}
	return ret, nil
}

func GetServiceOnlineCount(serviceName string, serverId int) (int, error) {
	m := GetRedis().HGet(context.Background(), ServerOnlineKey(serviceName), misc.IntToStr(serverId))
	if m.Err() != nil {
		return 0, m.Err()
	}

	return misc.StrToInt(m.Val()), nil
}

func GetServiceAllBattleLoadInfo(serviceName string) (map[int]float64, error) {
	m := GetRedis().HGetAll(context.Background(), ServerBattleLoadKey(serviceName))
	if m.Err() != nil {
		return nil, m.Err()
	}

	ret := make(map[int]float64, len(m.Val()))
	for k, v := range m.Val() {
		battleLoad, err := strconv.ParseFloat(v, 64)
		if err != nil {
			xlog.Errorf("GetServiceAllInfo ParseFloat %s error %s", v, err.Error())
			continue
		}
		ret[misc.StrToInt(k)] = battleLoad
	}
	return ret, nil
}
