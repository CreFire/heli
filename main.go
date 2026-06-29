package main

import (
	kitexpb "game/src/kitexpb/echoservice"
	"log"
)

func main() {
	svr := kitexpb.NewServer(new(EchoServiceImpl))

	err := svr.Run()

	if err != nil {
		log.Println(err.Error())
	}
}
