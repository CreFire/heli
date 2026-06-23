package main

import (
	"fmt"
	"game/deps/netmgr/options"
	"game/deps/server"
	servicemgr "game/deps/service_mgr"
	"game/deps/xlog"
)

type serverListenInfo struct {
}

func (l *serverListenInfo) OnlineInstance(serviceName string, instance *servicemgr.ServiceInstance) error {
	xlog.Infof("instance online %v", instance)
	addr := fmt.Sprintf("%s:%d", instance.Host, instance.Port)
	connOpt := options.NewMsgQueOptions()
	connOpt.SetConnectParams(options.NewConnectParams(addr, serviceName, instance.InstanceId))
	connOpt.SetWriteChanSize(options.WRITE_CHAN_SIZE_S)

	if err := server.MS.NetMgr.StartConnect(connOpt, gateSvr.sh); err != nil {
		xlog.Warnf("peer connect failed. service:%s instanceId:%d host:%s port:%d addr:%s err:%v",
			serviceName, instance.InstanceId, instance.Host, instance.Port, addr, err)
		return err
	}
	return nil
}

func (l *serverListenInfo) OfflineInstance(serviceName string, instance *servicemgr.ServiceInstance) error {
	xlog.Infof("instance offline %v", instance)
	server.MS.NetMgr.RemoveSvr(serviceName, instance.InstanceId)
	return nil
}

func (l *serverListenInfo) UpdateInstance(serviceName string, instance *servicemgr.ServiceInstance) error {
	xlog.Infof("instance update %v", instance)
	return nil
}
