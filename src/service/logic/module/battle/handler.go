package battle

import (
	"fmt"
	"game/deps/msg"
	"game/deps/netmgr"
	"game/deps/router"
	rpcmgr "game/deps/rpc_mgr"
	"game/deps/server"
	"game/deps/xlog"
	"game/src/common"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/proto/pbrpc"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) RegisterHandler(rpc *rpcmgr.RpcMgr, r *router.Router) error {
	rpc.RpcRegister(pb.MSG_ID_S2S_BATTLE_SETTLE_REQ, h.rpcBattleSettle)
	return nil
}

func (h *Handler) rpcBattleSettle(_ netmgr.IMsgQue, reqMsg *msg.Message) *pbrpc.S2SRpcRSP {
	req, ok := reqMsg.Message().(*pb.S2SBattleSettleREQ)
	if !ok || req == nil {
		return &pbrpc.S2SRpcRSP{Error: &errorpb.RpcError{ErrCode: errorpb.ERROR_REQUEST_PARAMS, ErrDesc: "invalid battle settle req"}}
	}
	xlog.Infof("logic recv battle settle roomId:%s battleId:%s win:%v finish:%s players:%d", req.GetRoomId(), req.GetBattleId(), req.GetWin(), req.GetFinishReason().String(), len(req.GetPlayers()))

	accepted := req.GetRoomId() != ""
	message := "ok"
	if !accepted {
		message = "room id is empty"
	}

	payload := &pb.S2SBattleSettleRSP{
		RoomId:   req.GetRoomId(),
		Accepted: accepted,
		Message:  message,
	}
	data, err := msg.PBPack(payload)
	if err != nil {
		return &pbrpc.S2SRpcRSP{Error: &errorpb.RpcError{ErrCode: errorpb.ERROR_RPC_SERVER_RSP_ERROR, ErrDesc: err.Error()}}
	}
	return &pbrpc.S2SRpcRSP{
		MsgId:   pb.MSG_ID_S2S_BATTLE_SETTLE_RSP,
		PayLoad: data,
	}
}

func CreateRoom(roomID string, playerIDs []int64, towerDeck []int32, combatType int32, levelID int32) (*pbrpc.S2SBattleCreateRoomRSP, error) {
	instance, err := server.MS.SvrMgr.PickMinOnline(common.InnerServerTypeBattle, true)
	if err != nil || instance == nil {
		return nil, fmt.Errorf("pick battle server failed: %w", err)
	}

	req := &pbrpc.S2SBattleCreateRoomREQ{
		RoomId:     roomID,
		PlayerIds:  playerIDs,
		TowerDeck:  towerDeck,
		CombatType: combatType,
		LevelId:    levelID,
	}
	m := msg.NewMsgWithProto(pb.MSG_ID_S2S_BATTLE_CREATE_ROOM_REQ, req)
	if len(playerIDs) > 0 {
		m.SetHashKey(playerIDs[0])
	}

	rpcRsp, err := server.MS.Rpc.SendRequestWithBlock(common.InnerServerTypeBattle, instance.InstanceId, m, nil)
	if err != nil {
		xlog.Warnf("logic create battle room rpc failed. battleSvrId:%d roomId:%s err:%v", instance.InstanceId, roomID, err)
		return nil, err
	}
	return rpcRsp.GetBattleCreateRoomRsp(), nil
}
