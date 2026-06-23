package mongoclient

import (
	"context"
	"game/deps/xlog"
	"runtime"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

func NewMongoClient(dsn string, myLogger *xlog.MyLogger) (*MongoClient, error) {
	//1. 创建 MongoDB 客户端选项
	option := options.Client().ApplyURI(dsn)
	// 设置连接池大小和其他选项
	CPU := runtime.GOMAXPROCS(0)
	if option.MaxPoolSize == nil || *option.MaxPoolSize == 0 {
		option.SetMaxPoolSize(min(uint64(CPU)*10, 100)) // 设置连接池大小
	}

	if option.MaxConnIdleTime == nil || *option.MaxConnIdleTime == 0 {
		option.SetMaxConnIdleTime(5 * time.Minute) // 设置连接空闲时间
	}

	// 设置连接超时时间
	if option.ConnectTimeout == nil || *option.ConnectTimeout == 0 {
		option.SetConnectTimeout(30 * time.Second)
	}

	// 设置服务器选择超时时间
	if option.ServerSelectionTimeout == nil || *option.ServerSelectionTimeout == 0 {
		option.SetServerSelectionTimeout(30 * time.Second)
	}

	if option.Timeout == nil || *option.Timeout == 0 {
		option.SetTimeout(60 * time.Second)
	}

	option.LoggerOptions = options.Logger().SetSink(newMongoLogAdapter(myLogger)).
		SetComponentLevel(options.LogComponentAll, 1)
	poolStats := newMongoPoolStatsRecorder()
	option.SetPoolMonitor(poolStats.Monitor())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// 2. 连接 MongoDB
	client, errConnect := mongo.Connect(option)
	if errConnect != nil {
		return nil, errConnect
	}

	// 3. Ping 检查连接是否健康
	if errPing := client.Ping(ctx, readpref.Primary()); errPing != nil {
		client.Disconnect(ctx)
		return nil, errPing
	}

	return &MongoClient{
		options:   option,
		Clients:   client,
		poolStats: poolStats,
	}, nil
}

type MongoClient struct {
	options   *options.ClientOptions
	Clients   *mongo.Client
	poolStats *mongoPoolStatsRecorder
}

type mongoPoolStatsRecorder struct {
	totalConns     int64
	inUseConns     int64
	checkOutFailed int64
	poolCleared    int64
	poolClosed     int64
	connClosed     int64
}
type MongoPoolEvents struct {
	CheckOutFailed int64
	PoolCleared    int64
	PoolClosed     int64
	ConnClosed     int64
}
type MongoPoolStats struct {
	TotalConns int64
	IdleConns  int64
	InUseConns int64
}

func newMongoPoolStatsRecorder() *mongoPoolStatsRecorder {
	return &mongoPoolStatsRecorder{}
}

func createUniqueIndex(c *mongo.Client, dbName, colName string, keys []string) {
	if c == nil || len(keys) == 0 {
		return
	}
	var docs bson.D
	for _, key := range keys {
		docs = append(docs, bson.E{Key: key, Value: 1})
	}

	col := c.Database(dbName).Collection(colName)
	if col == nil {
		xlog.Errorf("[DB] creating index failed on db[%s]-col[%s]", dbName, colName)
		return
	}
	_, err := col.Indexes().CreateOne(context.Background(),
		mongo.IndexModel{Keys: docs, Options: options.Index().SetUnique(true)})
	if err != nil {
		xlog.Errorf("[DB] creating index failed on db[%s]-col[%s] : %s", dbName, colName, err.Error())
	}
}

func (mc *MongoClient) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mc.Clients.Disconnect(ctx)
}

func (r *mongoPoolStatsRecorder) Monitor() *event.PoolMonitor {
	return &event.PoolMonitor{
		Event: func(evt *event.PoolEvent) {
			switch evt.Type {
			case event.ConnectionCreated:
				atomic.AddInt64(&r.totalConns, 1)
			case event.ConnectionClosed:
				atomic.AddInt64(&r.connClosed, 1)
				decAtomicInt64(&r.totalConns)
			case event.ConnectionCheckedOut:
				atomic.AddInt64(&r.inUseConns, 1)
			case event.ConnectionCheckedIn:
				decAtomicInt64(&r.inUseConns)
			case event.ConnectionCheckOutFailed:
				atomic.AddInt64(&r.checkOutFailed, 1)
			case event.ConnectionPoolCleared:
				atomic.AddInt64(&r.poolCleared, 1)
				atomic.StoreInt64(&r.totalConns, 0)
				atomic.StoreInt64(&r.inUseConns, 0)
			case event.ConnectionPoolClosed:
				atomic.AddInt64(&r.poolClosed, 1)
				atomic.StoreInt64(&r.totalConns, 0)
				atomic.StoreInt64(&r.inUseConns, 0)
			}
		},
	}
}
func decAtomicInt64(v *int64) {
	for {
		cur := atomic.LoadInt64(v)
		if cur == 0 {
			return
		}
		if atomic.CompareAndSwapInt64(v, cur, cur-1) {
			return
		}
	}
}
