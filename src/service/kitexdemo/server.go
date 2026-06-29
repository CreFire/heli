package kitexdemo

import (
	"context"
	"fmt"
	depkitex "game/deps/kitex"
	"game/src/configdoc"
	"game/src/kitexpb"
	"game/src/kitexpb/echoservice"
	"net"

	kitexserver "github.com/cloudwego/kitex/server"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
)

type EchoImpl struct{}

func (s *EchoImpl) Echo(ctx context.Context, req *kitexpb.EchoRequest) (*kitexpb.EchoResponse, error) {
	if req == nil {
		return &kitexpb.EchoResponse{Message: ""}, nil
	}
	return &kitexpb.EchoResponse{Message: req.Message}, nil
}

func Run() error {
	globalCfg, err := configdoc.LoadGlobalCfg("conf")
	if err != nil {
		return fmt.Errorf("load global config failed: %w", err)
	}
	reg, err := depkitex.NewEtcdRegistry(globalCfg.EtcdDsn)
	if err != nil {
		return fmt.Errorf("create etcd registry failed: %w", err)
	}
	addr, err := net.ResolveTCPAddr("tcp", ":8899")
	if err != nil {
		return err
	}
	svr := echoservice.NewServer(
		new(EchoImpl),
		kitexserver.WithServiceAddr(addr),
		kitexserver.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: "kitex.echo"}),
		kitexserver.WithRegistry(reg),
	)
	return svr.Run()
}
