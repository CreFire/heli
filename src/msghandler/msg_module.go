package msghandler

import (
	"game/deps/router"
	rpcmgr "game/deps/rpc_mgr"
)

type IModuleMsgHandler interface {
	RegisterHandlers(rpc *rpcmgr.RpcMgr, r *router.Router)
}
