package persist

import (
	"game/deps/mongoclient"
	"game/deps/xlog"
	"game/src/proto/pb"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
)

type mongoRecorder interface {
	GenSingleUpdateDoc() bson.M
}
type partMeta struct {
	field  string        // mongo 字段名
	loaded *bool         // 对应 loaded 标记地址
	target any           // Decode 目标指针（*GamerLevelData 等）
	record mongoRecorder // 生成增量更新文档
}

const (
	GamerLevelModIndex = iota
	GamerHeroModIndex
	GamerTaskModIndex
	GamerPackModIndex
	GamerMainModIndex
	GamerBaseModIndex
	GamerEquipModIndex
	GamerRecordModIndex
	GamerFormationModIndex
	GamerShopModIndex
	GamerDeviceModIndex
	GamerActivityModIndex // 添加每日活动模块索引
	GamerLotteryIndex
	GamerQuestModIndex
	GamerFunctionModIndex
	GamerArtifactModIndex
	GamerDataModIndexMax // 索引最大值标记
)

const (
	GAMER_LEVEL_MONGO_FIELD     = "level_data"
	GAMER_HERO_MONGO_FIELD      = "hero_data"
	GAMER_TASK_MONGO_FIELD      = "task_data"
	GAMER_PACK_MONGO_FIELD      = "pack_data"
	GAMER_MAIN_MONGO_FIELD      = "main_data"
	GAMER_PLAYER_MONGO_FIELD    = "player_data"
	GAMER_EQUIP_MONGO_FIELD     = "equip_data"
	GAMER_RECORD_MONGO_FIELD    = "record_data"
	GAMER_FORMATION_MONGO_FIELD = "formation_data"
	GAMER_SHOP_MONGO_FIELD      = "shop_data"
	GAMER_DEVICE_MONGO_FIELD    = "device_data"
	GAMER_ACTIVITY_FIELD        = "activity_data" // 添加每日活动模块字段名
	GAMER_LOTTERY_FIELD         = "lottery_data"
	GAMER_QUEST_MONGO_FIELD     = "quest_data"
	GAMER_FUNCTION_OPEN_FIELD   = "function_data"
	GAMER_ARTIFACT_FIELD        = "artifact_data"
)

var IndexToFiledMap = []string{
	GamerLevelModIndex:     GAMER_LEVEL_MONGO_FIELD,
	GamerHeroModIndex:      GAMER_HERO_MONGO_FIELD,
	GamerTaskModIndex:      GAMER_TASK_MONGO_FIELD,
	GamerPackModIndex:      GAMER_PACK_MONGO_FIELD,
	GamerMainModIndex:      GAMER_MAIN_MONGO_FIELD,
	GamerBaseModIndex:      GAMER_PLAYER_MONGO_FIELD,
	GamerEquipModIndex:     GAMER_EQUIP_MONGO_FIELD,
	GamerRecordModIndex:    GAMER_RECORD_MONGO_FIELD,
	GamerFormationModIndex: GAMER_FORMATION_MONGO_FIELD,
	GamerShopModIndex:      GAMER_SHOP_MONGO_FIELD,
	GamerDeviceModIndex:    GAMER_DEVICE_MONGO_FIELD,
	GamerActivityModIndex:  GAMER_ACTIVITY_FIELD, // 添加每日活动模块映射
	GamerLotteryIndex:      GAMER_LOTTERY_FIELD,
	GamerQuestModIndex:     GAMER_QUEST_MONGO_FIELD,
	GamerFunctionModIndex:  GAMER_FUNCTION_OPEN_FIELD,
	GamerArtifactModIndex:  GAMER_ARTIFACT_FIELD,
}

//type Pers[T proto.Message] = mongoclient.DataPersister[T]

type GamerData struct {
	GamerId int64                                            `bson:"_id"`
	Task    *mongoclient.DataPersister[*pb.GamerTaskData]    `bson:"task_data"`
	Pack    *mongoclient.DataPersister[*pb.GamerPackData]    `bson:"pack_data"`
	Main    *mongoclient.DataPersister[*pb.Gamer]            `bson:"main_data"`
	Base    *mongoclient.DataPersister[*pb.PlayerBase]       `bson:"player_data"`
	Record  *mongoclient.DataPersister[*pb.GamerRecordData]  `bson:"record_data"`
	Shop    *mongoclient.DataPersister[*pb.GamerShopData]    `bson:"shop_data"`
	pes     [GamerDataModIndexMax]mongoclient.MongoPersister `bson:"-"`
}

func NewGamerData(gamerId int64) *GamerData {
	col := GetGamerDataCollection()
	gamer := &GamerData{
		GamerId: gamerId,
		Task:    mongoclient.NewDataPersister(newGamerTaskData(), GAMER_TASK_MONGO_FIELD, col, gamerId),
		Pack:    mongoclient.NewDataPersister(newGamerPackData(), GAMER_PACK_MONGO_FIELD, col, gamerId),
		Main:    mongoclient.NewDataPersister(newGamerMainData(), GAMER_MAIN_MONGO_FIELD, col, gamerId),
		Base:    mongoclient.NewDataPersister(newGamerPlayerData(), GAMER_PLAYER_MONGO_FIELD, col, gamerId),
		Shop:    mongoclient.NewDataPersister(newGamerShopData(), GAMER_SHOP_MONGO_FIELD, col, gamerId),
		Record:  mongoclient.NewDataPersister(newGamerRecordData(), GAMER_RECORD_MONGO_FIELD, col, gamerId),
	}

	// 填充persister数组
	gamer.pes[GamerTaskModIndex] = gamer.Task
	gamer.pes[GamerPackModIndex] = gamer.Pack
	gamer.pes[GamerBaseModIndex] = gamer.Base
	gamer.pes[GamerMainModIndex] = gamer.Main
	gamer.pes[GamerRecordModIndex] = gamer.Record
	gamer.pes[GamerShopModIndex] = gamer.Shop
	return gamer
}

func NewGamerPartData(gamerId int64, modIndex ...int32) *GamerData {
	if len(modIndex) == 0 {
		return NewGamerData(gamerId)
	}

	gamer := &GamerData{
		GamerId: gamerId,
	}
	col := GetGamerDataCollection()
	// 根据提供的modIndex初始化指定的数据
	for _, index := range modIndex {
		switch index {
		case GamerTaskModIndex:
			gamer.Task = mongoclient.NewDataPersister(newGamerTaskData(), GAMER_TASK_MONGO_FIELD, col, gamerId)
			gamer.pes[index] = gamer.Task
		case GamerPackModIndex:
			gamer.Pack = mongoclient.NewDataPersister(newGamerPackData(), GAMER_PACK_MONGO_FIELD, col, gamerId)
			gamer.pes[index] = gamer.Pack
		case GamerBaseModIndex:
			gamer.Base = mongoclient.NewDataPersister(newGamerPlayerData(), GAMER_PLAYER_MONGO_FIELD, col, gamerId)
			gamer.pes[index] = gamer.Base
		case GamerMainModIndex:
			gamer.Main = mongoclient.NewDataPersister(newGamerMainData(), GAMER_MAIN_MONGO_FIELD, col, gamerId)
			gamer.pes[index] = gamer.Main
		case GamerShopModIndex:
			gamer.Shop = mongoclient.NewDataPersister(newGamerShopData(), GAMER_SHOP_MONGO_FIELD, col, gamerId)
			gamer.pes[index] = gamer.Shop
		case GamerRecordModIndex:
			gamer.Record = mongoclient.NewDataPersister(newGamerRecordData(), GAMER_RECORD_MONGO_FIELD, col, gamerId)
			gamer.pes[index] = gamer.Record
		}
	}
	return gamer
}

func GetGamerModData[T proto.Message](gd *GamerData, modIndex int32) *mongoclient.DataPersister[T] {
	if modIndex < 0 || modIndex >= GamerDataModIndexMax {
		xlog.Errorf("GamerData GetModData failed: invalid modIndex %d", modIndex)
		return nil
	}

	if gd.pes[modIndex] == nil {
		xlog.Errorf("GamerData GetModData failed: persister at index %d not init", modIndex)
		return nil
	}

	// 类型断言并返回
	pers, ok := gd.pes[modIndex].(*mongoclient.DataPersister[T])
	if !ok {
		xlog.Errorf("GamerData GetModData failed: type assertion failed for index %d", modIndex)
		return nil
	}
	if pers.IsLoaded() {
		return pers
	}

	if err := pers.Load(); err != nil {
		xlog.Errorf("GamerData GetModData failed: %v, Gid=%v Err:%s", err, gd.GamerId, err.Error())
		return nil
	}

	return pers
}

func GetGamerRawDataByMod[T proto.Message](id int64, modIndex int) T {
	gd := NewGamerPartData(id, int32(modIndex))
	data := gd.pes[modIndex].RawData()
	t, ok := data.(T)
	if !ok {
		xlog.Errorf("GamerData GetModData failed: type assertion failed for index %d", modIndex)
	}
	return t
}
