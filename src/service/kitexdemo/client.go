package kitexdemo

import (
	"context"
	"fmt"
	depkitex "game/deps/kitex"
	"game/src/configdoc"
	"game/src/kitexpb"
	"game/src/kitexpb/echoservice"

	kitexclient "github.com/cloudwego/kitex/client"
)

func Echo(message string) (string, error) {
	globalCfg, err := configdoc.LoadGlobalCfg("conf")
	if err != nil {
		return "", fmt.Errorf("load global config failed: %w", err)
	}
	resolver, err := depkitex.NewEtcdResolver(globalCfg.EtcdDsn)
	if err != nil {
		return "", fmt.Errorf("create etcd resolver failed: %w", err)
	}
	cli, err := echoservice.NewClient("kitex.echo", kitexclient.WithResolver(resolver))
	if err != nil {
		return "", fmt.Errorf("new kitex client failed: %w", err)
	}
	resp, err := cli.Echo(context.Background(), &kitexpb.EchoRequest{Message: message})
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", nil
	}
	return resp.Message, nil
}
