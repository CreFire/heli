package main

import (
	"game/deps/msg"
	"game/deps/netmgr"
	"game/deps/server"
	"game/deps/xlog"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/proto/pbrpc"
)

func RegisterBattleHandlers() error {
	server.MS.Rpc.RpcRegister(pb.MSG_ID_S2S_BATTLE_CREATE_ROOM_REQ, rpcCreateBattleRoom)
	return nil
}

func rpcCreateBattleRoom(_ netmgr.IMsgQue, reqMsg *msg.Message) *pbrpc.S2SRpcRSP {
	req := reqMsg.Message().(*pbrpc.S2SBattleCreateRoomREQ)
	rsp := &pbrpc.S2SRpcRSP{
		RspType: &pbrpc.S2SRpcRSP_BattleCreateRoomRsp{
			BattleCreateRoomRsp: &pbrpc.S2SBattleCreateRoomRSP{},
		},
	}

	room, err := battleSvr.roomMgr.createRoom(req.RoomId, req.PlayerIds, req.TowerDeck)
	if err != nil {
		xlog.Warnf("battle create room failed. roomId:%s playerCount:%d err:%v", req.RoomId, len(req.PlayerIds), err)
		rsp.Error = &errorpb.RpcError{
			ErrCode: errorpb.ERROR_RPC_LOGIC_FAILED,
			ErrDesc: err.Error(),
		}
		return rsp
	}

	rsp.GetBattleCreateRoomRsp().Ok = true
	rsp.GetBattleCreateRoomRsp().RoomId = room.id
	rsp.GetBattleCreateRoomRsp().PlayerCount = int64(len(room.playerIDs))
	return rsp
}
