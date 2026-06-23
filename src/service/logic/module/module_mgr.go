package module

import (
	"game/deps/router"
	rpcmgr "game/deps/rpc_mgr"
	servicemgr "game/deps/service_mgr"
	itemmodule "game/src/service/logic/module/item"
	playermodule "game/src/service/logic/module/player"
)

type IModuleHandler interface {
	RegisterHandler(rpc *rpcmgr.RpcMgr, r *router.Router) error
}

type ModuleMgr struct {
	item   *itemmodule.Handler
	player *playermodule.Handler
}

func NewModuleMgr() *ModuleMgr {
	return &ModuleMgr{
		item:   itemmodule.NewHandler(),
		player: playermodule.NewHandler(),
	}
}

func (m *ModuleMgr) OnStart(rpc *rpcmgr.RpcMgr, r *router.Router) error {
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
	return nil
}

func (m *ModuleMgr) OnBeforeStop()                                                     {}
func (m *ModuleMgr) OnStop()                                                           {}
func (m *ModuleMgr) OnHeart(now int64)                                                 {}
func (m *ModuleMgr) OnLogicInstanceChange(online, offline *servicemgr.ServiceInstance) {}
