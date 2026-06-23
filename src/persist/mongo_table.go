package persist

import (
	"context"
	"game/deps/server"
	"game/deps/xlog"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const SERVER_INFO = "server_info"
const SERVER_AREA = "server_area"

const SERVER_MASTER = "server_master" // 服务器信息
const SERVER_STATUS = "server_status" //  服务器状态
const LOGIN_SESSION = "login_session" // 登录会话
const (
	USER_BASE = "user_base" // 用户初始化信息
	GATE_LOCK = "gate_lock"
)
const (
	GAMER_HERO     = "gamer_hero"     // 玩家英雄
	GAMER_PACK     = "gamer_pack"     // 玩家背包
	GAMER_TASK     = "gamer_task"     // 玩家任务
	GAMER_EQUIP    = "gamer_equip"    // 玩家装备
	GAMER_PLAYER   = "gamer_player"   // 玩家数据
	GAMER_NAME     = "gamer_name"     // 玩家名字
	GAMER_CURRENCY = "gamer_currency" // 玩家货币背包
	GAMER_PASS     = "gamer_pass"     // 玩家通行证
	GAMER_SHOP     = "gamer_shop"     // 玩家商店
	GAMER_RECORD   = "gamer_record"   // 玩家记录

)
const (
	GAMER_DEVICE = "gamer_device" // 玩家设备信息
	LOGIC_MAIN   = "logic_main"   // 逻辑服信息
	LOGIC_SEASON = "logic_season" // 逻辑服赛季信息
)

func InitTable() {
	ctx := context.TODO()
	db := GetMongoDB().Database(server.MS.ConfBase.Global.Mongo.DbName)

	// 1. TTL 索引：expireAt 字段
	_, err := db.Collection(LOGIN_SESSION).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.M{"expireAt": 1},
		Options: options.Index().SetExpireAfterSeconds(0),
	})
	if err != nil {
		xlog.Warnf("create TTL index on LOGIN_SESSION.expireAt failed: %v", err)
	}

	// 3. 玩家名字唯一索引
	_, err = db.Collection(GAMER_NAME).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.M{"name": 1},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		xlog.Warnf("create unique index on GAMER_NAME.name failed: %v", err)
	}

}

// func MongoKeyGamerQueue(gid int64) string {
// 	return fmt.Sprintf("gamer_queue_%d", gid)
// }

// type GamerName struct {
// 	Name string `bson:"name"` // 玩家名字
// 	Gid  int64  `bson:"gid"`  // 玩家ID
// 	LsId int32  `bson:"lsid"` // 区服ID
// }

// 玩家名字重复性检查
// func IsExistName(ctx context.Context, name string) (exist bool, err error) {
// 	var gamerName GamerName
// 	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
// 	defer cancel()
// 	err = GetMongoDB().Database(GAME).Collection(GAMER_NAME).FindOne(ctx, bson.M{"name": name}).Decode(&gamerName)
// 	if err != nil {
// 		if errors.Is(err, mongo.ErrNoDocuments) {
// 			return false, nil
// 		}
// 		return false, err
// 	}
// 	return true, nil
// }

// func SetGamerName(ctx context.Context, name string, gid int64, lsid int32) (errorpb.ERROR, error) {
// 	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
// 	defer cancel()
// 	_, err := GetMongoDB().Database(GAME).Collection(GAMER_NAME).InsertOne(ctx, &GamerName{
// 		Name: name,
// 		Gid:  gid,
// 		LsId: lsid,
// 	})
// 	if err == nil {
// 		return errorpb.ERROR_SUCCESS, nil
// 	}
// 	return errorpb.ERROR_FAILED, err
// }
