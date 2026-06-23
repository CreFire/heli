package persist

import (
	"game/deps/server"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// GetMongoDB 返回 Mongo 客户端
func GetMongoDB() *mongo.Client {
	return server.MS.MongoDB.Clients
}

// GetDatabase 获取指定数据库
func GetDatabase() *mongo.Database {
	return GetMongoDB().Database(server.MS.ConfBase.Global.Mongo.DbName)
}

const (
	mongoOpTimeout                = 10 * time.Second
	COLLECTION_AUTH_ACCOUNT       = "auth_account"
	COLLECTION_GAMER_DATA         = "gamer_data"
	COLLECTION_BATTLE_RECORD_DATA = "battle_record_data"
	COLLECTION_PUBLIC_GAMER_DATA  = "public_gamer_data"
	COLLECTION_PUBLIC_DATA        = "public_global_data"
	COLLECTION_WHITE_LIST         = "white_list"
)

func InitCollections() {
	authAccountCollection = GetDatabase().Collection(COLLECTION_AUTH_ACCOUNT)
	gamerDataCollection = GetDatabase().Collection(COLLECTION_GAMER_DATA)
	battleRecordCollection = GetDatabase().Collection(COLLECTION_BATTLE_RECORD_DATA)
	secondaryGamerDataCollection = GetDatabase().Collection(COLLECTION_GAMER_DATA, options.Collection().SetReadPreference(readpref.Secondary()))
	publicGamerDataCollection = GetDatabase().Collection(COLLECTION_PUBLIC_GAMER_DATA)
	publicDataCollection = GetDatabase().Collection(COLLECTION_PUBLIC_DATA)
	whiteListCollection = GetDatabase().Collection(COLLECTION_WHITE_LIST)
}

var (
	authAccountCollection        *mongo.Collection
	gamerDataCollection          *mongo.Collection
	battleRecordCollection       *mongo.Collection
	publicGamerDataCollection    *mongo.Collection
	publicDataCollection         *mongo.Collection
	whiteListCollection          *mongo.Collection
	secondaryGamerDataCollection *mongo.Collection
)

func GetCollection(name string) *mongo.Collection {
	switch name {
	case COLLECTION_AUTH_ACCOUNT:
		return GetAuthAccountCollection()
	case COLLECTION_GAMER_DATA:
		return GetGamerDataCollection()
	case COLLECTION_BATTLE_RECORD_DATA:
		return GetBattleRecordCollection()
	case COLLECTION_PUBLIC_GAMER_DATA:
		return GetPublicGamerDataCollection()
	case COLLECTION_PUBLIC_DATA:
		return GetPublicDataCollection()
	case COLLECTION_WHITE_LIST:
		return GetWhiteListCollection()
	}
	return GetDatabase().Collection(name)
}

func GetAuthAccountCollection() *mongo.Collection {
	return authAccountCollection
}

func GetGamerDataCollection() *mongo.Collection {
	return gamerDataCollection
}

func GetBattleRecordCollection() *mongo.Collection {
	return battleRecordCollection
}

func GetSecondaryGamerDataCollection() *mongo.Collection {
	if secondaryGamerDataCollection != nil {
		return secondaryGamerDataCollection
	}
	secondaryGamerDataCollection = GetDatabase().Collection(COLLECTION_GAMER_DATA, options.Collection().SetReadPreference(readpref.Secondary()))
	return secondaryGamerDataCollection
}

func GetPublicDataCollection() *mongo.Collection {
	return publicDataCollection
}

func GetPublicGamerDataCollection() *mongo.Collection {
	return publicGamerDataCollection
}

func GetWhiteListCollection() *mongo.Collection {
	return whiteListCollection
}
