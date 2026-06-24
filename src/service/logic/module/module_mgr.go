package module

import (
	"game/deps/router"
	rpcmgr "game/deps/rpc_mgr"
	servicemgr "game/deps/service_mgr"
	battlemodule "game/src/service/logic/module/battle"
	itemmodule "game/src/service/logic/module/item"
	matchmodule "game/src/service/logic/module/match"
	playermodule "game/src/service/logic/module/player"
	systemmodule "game/src/service/logic/module/system"
)

type IModuleHandler interface {
	RegisterHandler(rpc *rpcmgr.RpcMgr, r *router.Router) error
}

type ModuleMgr struct {
	battle *battlemodule.Handler
	match  *matchmodule.Handler
	item   *itemmodule.Handler
	player *playermodule.Handler
	system *systemmodule.SystemHandler
}

func NewModuleMgr() *ModuleMgr {
	return &ModuleMgr{
		battle: battlemodule.NewHandler(),
		match:  matchmodule.NewHandler(),
		item:   itemmodule.NewHandler(),
		player: playermodule.NewHandler(),
		system: &systemmodule.SystemHandler{},
	}
}

func (m *ModuleMgr) OnStart(rpc *rpcmgr.RpcMgr, r *router.Router) error {
	if m.battle != nil {
		if err := m.battle.RegisterHandler(rpc, r); err != nil {
			return err
		}
	}
	if m.match != nil {
		if err := m.match.RegisterHandler(rpc, r); err != nil {
			return err
		}
	}
	if m.item != nil {
		if err := m.item.RegisterHandler(rpc, r); err != nil {
			return err
		}
	}
	if m.player != nil {
		if err := m.player.RegisterHandler(rpc, r); err != nil {
			return err
		}
	}
	if m.system != nil {
		if err := m.system.RegisterHandler(rpc, r); err != nil {
			return err
		}
	}
	return nil
}

func (m *ModuleMgr) OnBeforeStop()                                                     {}
func (m *ModuleMgr) OnStop()                                                           {}
func (m *ModuleMgr) OnHeart(now int64)                                                 {}
func (m *ModuleMgr) OnLogicInstanceChange(online, offline *servicemgr.ServiceInstance) {}
