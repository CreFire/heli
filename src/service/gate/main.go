package main

import (
	"game/deps/server"
	"game/deps/xlog"
	"time"
)

func main() {
	if err := server.MS.Init(gateSvr); err != nil {
		xlog.Errorf("gate bootstrap failed. phase:init err:%v", err)
		return
	}

	if err := server.MS.Start(); err != nil {
		xlog.Errorf("gate bootstrap failed. phase:start err:%v", err)
		return
	}
	xlog.Infof("server start success")

	server.MS.WaitStop()
	<-time.After(2 * time.Second)
}
