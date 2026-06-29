package main

import (
	"log"
	kitexdemo "game/src/service/kitexdemo"
)

func main() {
	if err := kitexdemo.Run(); err != nil {
		log.Fatalf("kitex demo server start failed: %v", err)
	}
}
