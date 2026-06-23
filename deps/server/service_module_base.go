package server

import (
	"game/src/configdoc"
	"game/src/proto/eventpb"
)

type GameService interface {
	OnInit() error                                       //before the game service starts and before load config
	BeforeStart() error                                  //after load config, before the game service starts
	OnStart() error                                      //after the game service starts
	BeforeStop() error                                   //before the game service stops
	OnStop() error                                       //after the game service stops
	OnReload(oldCfg, newCfg *configdoc.ConfigBase) error //after the game service reloads config
	OnHeart(now int64) error                             //the main loop of the game service, usually used for periodic tasks
	OnEventHandle(*eventpb.Event)                        //event handle
}
