package main

import (
	"game/deps/server"
	"game/deps/xlog"
	"game/src/service/battle/battleapp"
	"time"
)

func main() {
	if err := server.MS.Init(battleapp.App()); err != nil {
		xlog.Errorf("server init error: %v", err)
		panic("server init error")
	}
	if err := server.MS.Start(); err != nil {
		xlog.Errorf("server start error: %v", err)
		panic("server start error")
	}
	server.MS.WaitStop()
	<-time.After(2 * time.Second)
}
