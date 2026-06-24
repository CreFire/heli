package main

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
	server.MS.Rpc.RpcRegister(pb.MSG_ID_S2S_BATTLE_CREATE_ROOM_REQ, rpcCreateBattleRoom)
	server.MS.Router.CSRegister(pb.MSG_ID_BATTLE_JOIN_REQ, reqBattleJoin)
	server.MS.Router.CSRegister(pb.MSG_ID_BATTLE_OP_REQ, reqBattleOp)
	return nil
}

func rpcCreateBattleRoom(_ netmgr.IMsgQue, reqMsg *msg.Message) *pbrpc.S2SRpcRSP {
	req := reqMsg.Message().(*pbrpc.S2SBattleCreateRoomREQ)
	battleToken := battleSvr.buildBattleToken(req.RoomId, req.PlayerIds)
	rsp := &pbrpc.S2SRpcRSP{RspType: &pbrpc.S2SRpcRSP_BattleCreateRoomRsp{BattleCreateRoomRsp: &pbrpc.S2SBattleCreateRoomRSP{}}}

	room, err := battleSvr.roomMgr.createRoom(req.RoomId, req.PlayerIds, req.TowerDeck, battleToken)
	if err != nil {
		xlog.Warnf("battle create room failed. roomId:%s playerCount:%d err:%v", req.RoomId, len(req.PlayerIds), err)
		rsp.Error = &errorpb.RpcError{ErrCode: errorpb.ERROR_RPC_LOGIC_FAILED, ErrDesc: err.Error()}
		return rsp
	}

	rsp.GetBattleCreateRoomRsp().Ok = true
	rsp.GetBattleCreateRoomRsp().RoomId = room.id
	rsp.GetBattleCreateRoomRsp().PlayerCount = int64(len(room.playerIDs))
	rsp.GetBattleCreateRoomRsp().BattleAddr = battleSvr.battleAddr()
	rsp.GetBattleCreateRoomRsp().BattleToken = battleToken
	return rsp
}

func reqBattleJoin(msgque netmgr.IMsgQue, reqMsg *msg.Message) (errorpb.ERROR, proto.Message) {
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
	rsp.Snapshot = battlesync.SnapshotToProto(room.id, room.room.Snapshot())
	return errorpb.ERROR_SUCCESS, rsp
}

func reqBattleOp(msgque netmgr.IMsgQue, reqMsg *msg.Message) (errorpb.ERROR, proto.Message) {
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
