package persist

import (
	"context"
	"fmt"
	"game/deps/xlog"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func CreateDBIndex() {
	createIndex(GetAuthAccountCollection(), "account")
	
	createIndexWithOptions(GetAuthAccountCollection(),
		bson.D{{Key: "appId", Value: 1}, {Key: "openId", Value: 1}},
		options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"appId": bson.M{"$exists": true}, "openId": bson.M{"$exists": true}}))

	createIndexWithOptions(GetBattleRecordCollection(),
		bson.D{{Key: "gid", Value: 1}, {Key: "bt", Value: 1}},
		options.Index())

	createIndexWithOptions(GetBattleRecordCollection(),
		bson.D{{Key: "exp_at", Value: 1}},
		options.Index().SetExpireAfterSeconds(0))
}

// =====================================================================================================================

func createIndex(collection *mongo.Collection, keys ...string) {
	if collection == nil || len(keys) == 0 {
		return
	}

	var docs bson.D
	for _, key := range keys {
		docs = append(docs, bson.E{Key: key, Value: 1})
	}
	ctx, cancel := context.WithTimeout(context.Background(), mongoOpTimeout)
	defer cancel()

	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    docs,
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		xlog.Errorf("[DB] create index failed on db:%s collection: %s, keys: %v, err: %v", collection.Database().Name(), collection.Name(), keys, err)
	}
}

func createIndexWithOptions(collection *mongo.Collection, keys bson.D, opts *options.IndexOptionsBuilder) {
	if collection == nil || len(keys) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), mongoOpTimeout)
	defer cancel()
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    keys,
		Options: opts,
	})
	if err != nil {
		xlog.Errorf("[DB] create index failed on db:%s collection: %s, keys: %v, err: %v", collection.Database().Name(), collection.Name(), keys, err)
	}
}

func createHashIndex(collection *mongo.Collection, keys ...string) {
	if collection == nil || len(keys) == 0 {
		return
	}

	var docs bson.D
	for _, key := range keys {
		docs = append(docs, bson.E{Key: key, Value: "hashed"})
	}
	ctx, cancel := context.WithTimeout(context.Background(), mongoOpTimeout)
	defer cancel()

	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: docs,
	})
	if err != nil {
		xlog.Errorf("[DB] create hash index failed on db:%s collection: %s, keys: %v, err: %v", collection.Database().Name(), collection.Name(), keys, err)
	}
}

func setShard(dbName, colName string, key string) {
	ctx, cancel := context.WithTimeout(context.Background(), mongoOpTimeout)
	defer cancel()

	ret := GetMongoDB().Database("admin").RunCommand(ctx, bson.D{
		{Key: "shardCollection", Value: fmt.Sprintf("%s.%s", dbName, colName)},
		bson.E{Key: key, Value: "hashed"},
	})

	if ret.Err() != nil {
		xlog.Errorf("[DB] set shard failed on db:%s collection: %s, key: %s, err: %v", dbName, colName, key, ret.Err())
	}
}
