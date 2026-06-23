package redisclient

import (
	"context"
	"fmt"
	"game/deps/xlog"
	"log"
	"runtime"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type RedisClient struct {
	options *redis.Options
	*redis.Client
}

type logger struct {
	log *log.Logger
}

func (l *logger) Printf(ctx context.Context, format string, v ...any) {
	l.log.Printf(format, v...)
}

func NewRedisClient(dsn string, myLogger *xlog.MyLogger) (*RedisClient, error) {
	option, err := redis.ParseURL(dsn)
	if err != nil {
		return nil, err
	}

	CPU := runtime.GOMAXPROCS(0)
	if option.PoolSize == 0 {
		option.PoolSize = min(5*CPU, 100) // 设置连接池大小
	}
	if option.ConnMaxIdleTime == 0 {
		option.ConnMaxIdleTime = 5 * time.Minute
	}
	if option.MaxIdleConns == 0 {
		option.MaxIdleConns = min(2*CPU, 30)
	}

	client := redis.NewClient(option)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	rc := &RedisClient{
		options: option,
		Client:  client,
	}

	redisLogger := &logger{log: xlog.StdLogger(myLogger, zap.WarnLevel, 1)}
	redis.SetLogger(redisLogger)

	return rc, nil
}

func (rc *RedisClient) Close() error {
	return rc.Client.Close()
}

type RedisClusterClient struct {
	options *redis.ClusterOptions
	*redis.ClusterClient
}

func NewRedisClientCluster(dsn string, myLogger *xlog.MyLogger) (*RedisClusterClient, error) {
	option, err := redis.ParseClusterURL(dsn)
	if err != nil {
		return nil, err
	}

	CPU := runtime.GOMAXPROCS(0)
	if option.PoolSize == 0 {
		option.PoolSize = min(5*CPU, 100) // 设置连接池大小
	}
	if option.ConnMaxIdleTime == 0 {
		option.ConnMaxIdleTime = 5 * time.Minute
	}
	if option.MaxIdleConns == 0 {
		option.MaxIdleConns = min(2*CPU, 30)
	}

	cluster := redis.NewClusterClient(option)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := cluster.ForEachShard(ctx, func(ctx context.Context, cli *redis.Client) error {
		if err := cli.Ping(ctx).Err(); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}

	rc := &RedisClusterClient{
		options:       option,
		ClusterClient: cluster,
	}

	redisLogger := &logger{log: xlog.StdLogger(myLogger, zap.WarnLevel, 1)}
	redis.SetLogger(redisLogger)

	return rc, nil
}

func (rc *RedisClusterClient) Close() error {
	return rc.ClusterClient.Close()
}
