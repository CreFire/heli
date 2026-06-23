package module

import (
	"game/deps/router"
	rpcmgr "game/deps/rpc_mgr"
	"game/src/service/gate/module/login"
)

type ModuleMgr struct {
	login *login.LoginModule
}

func NewModuleMgr() *ModuleMgr {
	return &ModuleMgr{
		login: login.NewLoginModule(),
	}
}

func (m *ModuleMgr) OnStart(rpc *rpcmgr.RpcMgr, r *router.Router) error {
	return m.login.RegisterHandler(rpc, r)
}

func (m *ModuleMgr) OnBeforeStop() {
}
func (m *ModuleMgr) OnStop() {
}
