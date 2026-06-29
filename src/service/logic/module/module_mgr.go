package module

import (
	"game/deps/msg"
	"game/deps/netmgr"
	"game/deps/router"
	rpcmgr "game/deps/rpc_mgr"
	servicemgr "game/deps/service_mgr"
	"game/deps/xlog"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/service/logic/actor"
	"game/src/service/logic/iface"
	battlemodule "game/src/service/logic/module/battle"
	itemmodule "game/src/service/logic/module/item"
	matchmodule "game/src/service/logic/module/match"
	playermodule "game/src/service/logic/module/player"
	shopmodule "game/src/service/logic/module/shop"
	systemmodule "game/src/service/logic/module/system"
	taskmodule "game/src/service/logic/module/task"

	"google.golang.org/protobuf/proto"
)

type ModuleMgr struct {
	battle *battlemodule.Handler
	match  *matchmodule.Handler
	item   *itemmodule.Handler
	player *playermodule.Handler
	shop   *shopmodule.Handler
	task   *taskmodule.Handler
	system *systemmodule.SystemHandler
}

func NewModuleMgr() *ModuleMgr {
	return &ModuleMgr{
		battle: battlemodule.NewHandler(),
		match:  matchmodule.NewHandler(),
		item:   itemmodule.NewHandler(),
		player: playermodule.NewHandler(),
		shop:   shopmodule.NewHandler(),
		task:   taskmodule.NewHandler(),
		system: &systemmodule.SystemHandler{},
	}
}

func (m *ModuleMgr) OnStart(rpc *rpcmgr.RpcMgr, r *router.Router) error {
	if m.battle != nil {
		rpc.RpcRegister(pb.MSG_ID_S2S_BATTLE_SETTLE_REQ, m.battle.HandleRpcBattleSettle)
	}
	if m.match != nil {
		r.CSRegister(pb.MSG_ID_MATCH_JOIN_REQ, WrapC2S(m.match.HandleMatchJoin))
	}
	if m.item != nil {
		r.CSRegister(pb.MSG_ID_USE_ITEM_REQ, WrapC2S(m.item.HandleUseItem))
		rpc.RpcRegister(pb.MSG_ID_S2S_ADD_ITEM_REQ, m.item.HandleRpcGamerAddItem)
		rpc.RpcRegister(pb.MSG_ID_S2S_SUB_ITEM_REQ, m.item.HandleRpcGamerSubItem)
		rpc.RpcRegister(pb.MSG_ID_S2S_CHECK_ITEM_REQ, m.item.HandleRpcGamerCheckItem)
	}
	if m.player != nil {
		r.CSRegister(pb.MSG_ID_PLAYER_LOAD_USER_REQ, WrapC2S(m.player.HandlePlayerLoadUser))
	}
	if m.shop != nil {
		r.CSRegister(pb.MSG_ID_SHOP_INFO_REQ, WrapC2S(func(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message) {
			return m.shop.HandleShopInfo(ctx, data)
		}))
		r.CSRegister(pb.MSG_ID_SHOP_BUY_REQ, WrapC2S(func(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message) {
			return m.shop.HandleShopBuy(ctx, data)
		}))
	}
	if m.task != nil {
		r.CSRegister(pb.MSG_ID_TASK_REWARD_REQ, WrapC2S(m.task.HandleTaskReward))
		r.CSRegister(pb.MSG_ID_TASK_REFRESH_REQ, WrapC2S(m.task.HandleTaskRefresh))
	}
	if m.system != nil {
		r.SSRegister(pb.MSG_ID_GATE_2_LOGIC_KICK_SESSION_REQ, m.system.HandleSsKickSession)
		r.CSRegister(pb.MSG_ID_SAY_HELLO_REQ, WrapC2S(m.system.HandleSayHello))
		r.CSRegister(pb.MSG_ID_HEART_REQ, WrapC2S(m.system.HandleHeart))
		r.CSRegister(pb.MSG_ID_STRESS_REQ, WrapC2S(m.system.HandleStress))
		rpc.RpcRegister(pb.MSG_ID_S2S_SWITCH_SERVER_REQ, m.system.HandleRpcSwitchServer)
		rpc.RpcRegister(pb.MSG_ID_S2S_USER_LOGIN_REQ, m.system.HandleRpcGamerLogin)
	}
	return nil
}

func (m *ModuleMgr) OnBeforeStop()                                                     {}
func (m *ModuleMgr) OnStop()                                                           {}
func (m *ModuleMgr) OnHeart(now int64)                                                 {}
func (m *ModuleMgr) OnLogicInstanceChange(online, offline *servicemgr.ServiceInstance) {}

func WrapC2S(h func(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message)) router.C2SHandlerFunc {
	return func(_ netmgr.IMsgQue, data *msg.Message) (errorpb.ERROR, proto.Message) {
		if data == nil {
			return errorpb.ERROR_REQUEST_PARAMS, nil
		}
		ctx := actor.FindGamerWithGid(data.GId())
		if ctx == nil {
			xlog.Warnf("handler ctx missing msgId:%v gid:%v sess:%v", data.MsgId(), data.GId(), data.PlayerSessId())
			return errorpb.ERROR_LOGIN_SESSION_INVALID, nil
		}
		return h(ctx, data)
	}
}
