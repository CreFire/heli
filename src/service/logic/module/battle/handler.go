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
	"sync"
)

type Handler struct {
	mu        sync.Mutex
	processed map[string]*pb.S2SBattleSettleRSP
}

func NewHandler() *Handler {
	return &Handler{processed: make(map[string]*pb.S2SBattleSettleRSP)}
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
	rsp := h.validateAndMark(req)
	xlog.Infof("logic recv battle settle roomId:%s battleId:%s accepted:%v message:%s players:%d", req.GetRoomId(), req.GetBattleId(), rsp.GetAccepted(), rsp.GetMessage(), len(req.GetPlayers()))
	data, err := msg.PBPack(rsp)
	if err != nil {
		return &pbrpc.S2SRpcRSP{Error: &errorpb.RpcError{ErrCode: errorpb.ERROR_RPC_SERVER_RSP_ERROR, ErrDesc: err.Error()}}
	}
	return &pbrpc.S2SRpcRSP{MsgId: pb.MSG_ID_S2S_BATTLE_SETTLE_RSP, PayLoad: data}
}

func (h *Handler) validateAndMark(req *pb.S2SBattleSettleREQ) *pb.S2SBattleSettleRSP {
	if req == nil {
		return &pb.S2SBattleSettleRSP{Accepted: false, Message: "settle req is nil"}
	}
	if req.GetRoomId() == "" {
		return &pb.S2SBattleSettleRSP{RoomId: req.GetRoomId(), Accepted: false, Message: "room id is empty"}
	}
	if req.GetBattleId() == "" {
		return &pb.S2SBattleSettleRSP{RoomId: req.GetRoomId(), Accepted: false, Message: "battle id is empty"}
	}
	if req.GetEndTick() < req.GetStartTick() {
		return &pb.S2SBattleSettleRSP{RoomId: req.GetRoomId(), Accepted: false, Message: "end tick is smaller than start tick"}
	}
	if len(req.GetPlayers()) == 0 {
		return &pb.S2SBattleSettleRSP{RoomId: req.GetRoomId(), Accepted: false, Message: "players is empty"}
	}
	for _, player := range req.GetPlayers() {
		if player == nil || player.GetPlayerId() <= 0 {
			return &pb.S2SBattleSettleRSP{RoomId: req.GetRoomId(), Accepted: false, Message: "player settle data invalid"}
		}
	}

	key := fmt.Sprintf("%s:%s", req.GetRoomId(), req.GetBattleId())
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.processed == nil {
		h.processed = make(map[string]*pb.S2SBattleSettleRSP)
	}
	if cached, ok := h.processed[key]; ok {
		return &pb.S2SBattleSettleRSP{RoomId: cached.GetRoomId(), Accepted: cached.GetAccepted(), Message: "duplicate settle accepted"}
	}
	rsp := &pb.S2SBattleSettleRSP{RoomId: req.GetRoomId(), Accepted: true, Message: "ok"}
	h.processed[key] = rsp
	return rsp
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
