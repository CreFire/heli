package redisclient

import (
	"context"
	"game/deps/xlog"
	"runtime"
	"time"

	"github.com/redis/rueidis"
)

type RueidisClient struct {
	rueidis.Client
	options rueidis.ClientOption
}

func NewRedisClientWithRueidis(dsn string, myLogger *xlog.MyLogger) (*RueidisClient, error) {
	opt, err := rueidis.ParseURL(dsn)
	if err != nil {
		return nil, err
	}

	opt.BlockingPoolMinSize = 0
	opt.BlockingPoolSize = runtime.NumCPU() * 5
	opt.BlockingPoolCleanup = 3 * time.Minute
	opt.ConnWriteTimeout = 10 * time.Second
	opt.PipelineMultiplex = 2

	client, err := rueidis.NewClient(opt)
	if err != nil {
		return nil, err
	}
	err = client.Do(context.Background(), client.B().Ping().Build().Pin()).Error()
	if err != nil {
		return nil, err
	}
	return &RueidisClient{options: opt, Client: client}, nil
}
