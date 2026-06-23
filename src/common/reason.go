package common

import (
	"fmt"
	"game/deps/basal"

	"github.com/bytedance/sonic"
)

type Reason struct {
	Id      int32
	Str     string
	MetaStr string // 业务附加元数据，约定为扁平 JSON object 字符串
}

func (r *Reason) String() string {
	if r == nil {
		return "Reason<nil>"
	}
	return basal.Sprintf("Reason{Id: %d, Str: %s, MetaStr: %s}", r.Id, r.Str, r.MetaStr)
}

func (r *Reason) IDMeta() (int32, string) {
	if r == nil {
		return 0, ""
	}
	return r.Id, r.MetaStr
}

var idReasonMap = map[int32]*Reason{}
var reasonIdMap = map[*Reason]int32{}

func NewReason(str string, id int32) *Reason {
	reason := &Reason{id, str, ""}
	if _, ok := idReasonMap[id]; ok {
		panic(fmt.Sprintf("NewReason id error: %d", id))
	}
	idReasonMap[id] = reason

	if _, ok := reasonIdMap[reason]; ok {
		panic(fmt.Sprintf("NewReason str error: %d", id))
	}
	reasonIdMap[reason] = id
	return reason
}

func GetReason(reasonId int32) *Reason {
	if reason, ok := idReasonMap[reasonId]; ok {
		return reason
	}
	return ReasonUnknown
}

func GetIdReasonMap() map[int32]*Reason {
	return idReasonMap
}

func CloneReason(reason *Reason) *Reason {
	if reason == nil {
		return &Reason{
			Id:      ReasonUnknown.Id,
			Str:     ReasonUnknown.Str,
			MetaStr: ReasonUnknown.MetaStr,
		}
	}
	return &Reason{
		Id:      reason.Id,
		Str:     reason.Str,
		MetaStr: reason.MetaStr,
	}
}

func WithReasonMeta(reason *Reason, meta map[string]any) *Reason {
	r := CloneReason(reason)
	if len(meta) == 0 {
		return r
	}
	r.MetaStr = MustReasonMetaJSON(meta)
	return r
}

func WithReasonMetaKV(reason *Reason, key string, value any) *Reason {
	if key == "" {
		return CloneReason(reason)
	}
	return WithReasonMeta(reason, map[string]any{key: value})
}

func BuildReason(reasonId int32, _, metaStr string) *Reason {
	r := CloneReason(GetReason(reasonId))
	if metaStr != "" {
		if _, err := parseReasonMeta(metaStr); err != nil {
			metaStr = MustReasonMetaJSON(map[string]any{"detail": metaStr})
		}
	}
	r.MetaStr = metaStr
	return r
}

func ReasonMeta(reason *Reason) map[string]any {
	if reason == nil || reason.MetaStr == "" {
		return nil
	}
	meta, err := parseReasonMeta(reason.MetaStr)
	if err != nil {
		panic(fmt.Sprintf("invalid reason meta json: %v", err))
	}
	return meta
}

func parseReasonMeta(metaStr string) (map[string]any, error) {
	if metaStr == "" {
		return nil, nil
	}
	var meta map[string]any
	if err := sonic.UnmarshalString(metaStr, &meta); err != nil {
		return nil, err
	}
	for key, value := range meta {
		switch value.(type) {
		case string, float64, bool, nil:
		default:
			return nil, fmt.Errorf("invalid reason meta value type for key %s", key)
		}
	}
	return meta, nil
}

func MustReasonMetaJSON(meta map[string]any) string {
	if len(meta) == 0 {
		return ""
	}
	for key, value := range meta {
		if key == "" {
			panic("reason meta key is empty")
		}
		switch value.(type) {
		case string, int, int32, int64, float64, bool:
		default:
			panic(fmt.Sprintf("invalid reason meta value type for key %s", key))
		}
	}
	data, err := sonic.Marshal(meta)
	if err != nil {
		panic(fmt.Sprintf("marshal reason meta failed: %v", err))
	}
	return string(data)
}

func ReasonMetaInt64(reason *Reason, key string) (int64, bool) {
	value, ok := ReasonMeta(reason)[key]
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case float64:
		return int64(v), true
	}
	return 0, false
}

func ReasonMetaString(reason *Reason, key string) (string, bool) {
	value, ok := ReasonMeta(reason)[key]
	if !ok {
		return "", false
	}
	v, ok := value.(string)
	return v, ok
}

func ReasonMetaBool(reason *Reason, key string) (bool, bool) {
	value, ok := ReasonMeta(reason)[key]
	if !ok {
		return false, false
	}
	v, ok := value.(bool)
	return v, ok
}

var (
	ReasonUnknown        = NewReason("未知原因", 0)
	ReasonNewGamer       = NewReason("新建角色", 1)
	ReasonGMCMD          = NewReason("GM命令", 2)
	ReasonPvpFight       = NewReason("战斗掉落", 3)
	ReasonShop           = NewReason("商店", 4)
	ReasonTask           = NewReason("任务领奖", 5)
	ReasonPass4          = NewReason("通行证", 6)
	ReasonWeapon         = NewReason("专武强化", 7)
	ReasonEquip          = NewReason("装备分解", 8)
	ReasonHeroUpLv       = NewReason("英雄升级", 9)
	ReasonEquipUp        = NewReason("装备升级", 10)
	ReasonAfkDrop        = NewReason("挂机掉落", 11)
	ReasonLinkReward     = NewReason("世界链接领奖", 12)
	ReasonWorldBoss      = NewReason("世界boss购买战斗次数", 13)
	ReasonUseItem        = NewReason("使用物品", 14)
	ReasonRealm          = NewReason("无序秘境", 15)
	ReasonInscription    = NewReason("铭刻", 16)
	ReasonActivityReward = NewReason("活跃奖励", 17)
	ReasonLottery        = NewReason("抽卡", 18)
	ReasonMailPick       = NewReason("邮件领取", 19)
	ReasonHeroUpStar     = NewReason("英雄升星", 20)
	ReasonAfkTime        = NewReason("挂机奖励", 21)
)
