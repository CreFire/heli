package main

import (
	"fmt"
	"os"

	"game/src/service/robot/controller"
)

func main() {
	runtime := controller.NewRobotRuntime()
	if err := runtime.Init(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "robot init failed: %v\n", err)
		return
	}
	if err := runtime.Start(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "robot start failed: %v\n", err)
		return
	}
	if err := runtime.WaitStop(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "robot wait stop failed: %v\n", err)
	}
}
