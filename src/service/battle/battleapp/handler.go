package battleapp

import (
	"game/deps/msg"
	"game/deps/netmgr"
	"game/deps/server"
	"game/deps/xlog"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/proto/pbrpc"
	battlesync "game/src/service/battle/sync"

	"google.golang.org/protobuf/proto"
)

func RegisterBattleHandlers() error {
	// battle 当前对外只暴露三条最小链路：
	// 1. logic -> battle 创房
	// 2. client -> battle 进房
	// 3. client -> battle 局内操作
	server.MS.Rpc.RpcRegister(pb.MSG_ID_S2S_BATTLE_CREATE_ROOM_REQ, rpcCreateBattleRoom)
	server.MS.Router.CSRegister(pb.MSG_ID_BATTLE_JOIN_REQ, reqBattleJoin)
	server.MS.Router.CSRegister(pb.MSG_ID_BATTLE_OP_REQ, reqBattleOp)
	return nil
}

// rpcCreateBattleRoom 是 logic -> battle 的建房入口。
// logic 负责匹配和组房，battle 负责把房间真正落到内存并启动战斗推进。
func rpcCreateBattleRoom(_ netmgr.IMsgQue, reqMsg *msg.Message) *pbrpc.S2SRpcRSP {
	// logic 在匹配/开局阶段调用 battle 创房。
	// 当前为 P0 最小闭环：battle 负责生成 battle_token、创建内存房间并启动 tick loop。
	req := reqMsg.Message().(*pbrpc.S2SBattleCreateRoomREQ)
	battleToken := battleSvr.buildBattleToken(req.RoomId, req.PlayerIds)
	rsp := &pbrpc.S2SRpcRSP{RspType: &pbrpc.S2SRpcRSP_BattleCreateRoomRsp{BattleCreateRoomRsp: &pbrpc.S2SBattleCreateRoomRSP{}}}

	room, err := battleSvr.roomMgr.createRoom(req.RoomId, req.PlayerIds, req.TowerDeck, battleToken)
	if err != nil {
		xlog.Warnf("battle create room failed. roomId:%s playerCount:%d err:%v", req.RoomId, len(req.PlayerIds), err)
		rsp.Error = &errorpb.RpcError{ErrCode: errorpb.ERROR_RPC_LOGIC_FAILED, ErrDesc: err.Error()}
		return rsp
	}
	battleSvr.startRoomLoop(room)

	rsp.GetBattleCreateRoomRsp().Ok = true
	rsp.GetBattleCreateRoomRsp().RoomId = room.id
	rsp.GetBattleCreateRoomRsp().PlayerCount = int64(len(room.playerIDs))
	rsp.GetBattleCreateRoomRsp().BattleAddr = battleSvr.battleAddr()
	rsp.GetBattleCreateRoomRsp().BattleToken = battleToken
	return rsp
}

// reqBattleJoin 处理客户端战斗服直连后的入场请求。
// 成功后 battle 会返回完整 snapshot，客户端应以此作为局内初始状态。
func reqBattleJoin(msgque netmgr.IMsgQue, reqMsg *msg.Message) (errorpb.ERROR, proto.Message) {
	// join 是客户端直连 battle 后的第一步。
	// P0 目标：校验 room_id / token / player 归属，并把完整 snapshot 返回给客户端做局内初始化。
	req, ok := reqMsg.Message().(*pb.C2SBattleJoinREQ)
	if !ok || req == nil || msgque == nil {
		return errorpb.ERROR_REQUEST_PARAMS, nil
	}
	room := battleSvr.roomMgr.getRoom(req.GetRoomId())
	playerID := reqMsg.GId()
	rsp := &pb.S2CBattleJoinRSP{RoomId: req.GetRoomId(), PlayerId: playerID}
	if err := battleSvr.verifyBattleToken(room, playerID, req.GetRoomId(), req.GetBattleToken()); err != nil {
		rsp.Code = errorpb.ERROR_REQUEST_PARAMS
		rsp.Message = err.Error()
		return errorpb.ERROR_REQUEST_PARAMS, rsp
	}
	room.bindPlayerSession(playerID, msgque.SessId())
	rsp.Code = errorpb.ERROR_SUCCESS
	rsp.Message = "ok"
	rsp.Snapshot = battlesync.SnapshotToProto(room.id, room.roomSnapshot())
	return errorpb.ERROR_SUCCESS, rsp
}

// reqBattleOp 处理客户端局内操作请求。
// 当前 P0 约束为“先 rsp，再广播 delta”，避免客户端在失败时误消费状态更新。
func reqBattleOp(msgque netmgr.IMsgQue, reqMsg *msg.Message) (errorpb.ERROR, proto.Message) {
	// op 走 battle 权威执行：
	// - 先做最小 session 归属校验
	// - 再进入 sync.Room 执行状态变更
	// - 成功后先返回 rsp，再广播本 tick 产生的 delta
	req, ok := reqMsg.Message().(*pb.C2SBattleOpREQ)
	if !ok || req == nil || msgque == nil || req.GetOp() == nil {
		return errorpb.ERROR_REQUEST_PARAMS, nil
	}
	room := battleSvr.roomMgr.getRoom(req.GetRoomId())
	playerID := reqMsg.GId()
	if room == nil || room.room == nil {
		return errorpb.ERROR_REQUEST_PARAMS, &pb.S2CBattleOpRSP{RoomId: req.GetRoomId(), OpId: req.GetOpId(), Code: errorpb.ERROR_REQUEST_PARAMS, Message: "room not found"}
	}
	if !room.matchPlayerSession(playerID, msgque.SessId()) {
		return errorpb.ERROR_LOGIN_SESSION_INVALID, &pb.S2CBattleOpRSP{RoomId: req.GetRoomId(), OpId: req.GetOpId(), Code: errorpb.ERROR_LOGIN_SESSION_INVALID, Message: "battle join required"}
	}
	out := battleSvr.applyBattleOp(room, playerID, req)
	if out.GetCode() != errorpb.ERROR_SUCCESS {
		return out.GetCode(), out
	}
	battleSvr.broadcastRoomDelta(room)
	return errorpb.ERROR_SUCCESS, out
}
