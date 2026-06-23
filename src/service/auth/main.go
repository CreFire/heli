package main

import (
	"game/deps/server"
	"game/deps/xlog"
	"time"
)

func main() {

	if err := server.MS.Init(authSvr); err != nil {
		xlog.Errorf("server init error: %v", err)
		return
	}

	if err := server.MS.Start(); err != nil {
		return
	}
	xlog.Infof("server start success")

	server.MS.WaitStop()
	<-time.After(2 * time.Second)
}
