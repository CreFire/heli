package matchbiz

import (
	"fmt"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/service/logic/gamedata"
	"game/src/service/logic/iface"
	battlemodule "game/src/service/logic/module/battle"
	"time"

	"google.golang.org/protobuf/proto"
)

type CreateRoomRequest struct {
	PlayerID   int64
	RoomID     string
	TowerDeck  []int32
	CombatType int32
	LevelID    int32
}

type CreateRoomResult struct {
	RoomID      string
	BattleAddr  string
	BattleToken string
	PlayerIDs   []int64
}

type CreateRoomFunc func(req CreateRoomRequest) (*CreateRoomResult, error)

type MatchModule struct {
	ctx        iface.IGamerContext
	model      *gamedata.GamerModel
	store      *MatchData
	createRoom CreateRoomFunc
}

func NewMatchModule(ctx iface.IGamerContext, model *gamedata.GamerModel) *MatchModule {
	m := &MatchModule{ctx: ctx, model: model, store: NewMatchData()}
	m.createRoom = m.defaultCreateRoom
	return m
}

func (m *MatchModule) SetCreateRoomFunc(f CreateRoomFunc) {
	if f != nil {
		m.createRoom = f
	}
}

func (m *MatchModule) Join(playerID int64, req *pb.C2SMatchJoin) (errorpb.ERROR, proto.Message) {
	if req == nil || playerID <= 0 {
		return errorpb.ERROR_REQUEST_PARAMS, nil
	}
	if m == nil || m.createRoom == nil {
		return errorpb.ERROR_FAILED, nil
	}
	roomID := fmt.Sprintf("room-%d-%d", playerID, time.Now().UnixNano())
	result, err := m.createRoom(CreateRoomRequest{
		PlayerID:   playerID,
		RoomID:     roomID,
		TowerDeck:  append([]int32(nil), req.GetTowerDeck()...),
		CombatType: req.GetCombatType(),
		LevelID:    req.GetLevelId(),
	})
	if err != nil || result == nil {
		return errorpb.ERROR_FIGHT_PVP_CREATE_EXCEPT, nil
	}
	return errorpb.ERROR_SUCCESS, &pb.S2CMatchJoin{
		RoomId:      result.RoomID,
		BattleAddr:  result.BattleAddr,
		BattleToken: result.BattleToken,
		PlayerIds:   result.PlayerIDs,
	}
}

func (m *MatchModule) defaultCreateRoom(req CreateRoomRequest) (*CreateRoomResult, error) {
	rsp, err := battlemodule.CreateRoom(req.RoomID, []int64{req.PlayerID}, req.TowerDeck, req.CombatType, req.LevelID)
	if err != nil {
		return nil, err
	}
	if rsp == nil || !rsp.GetOk() {
		return nil, fmt.Errorf("battle create room rejected roomId:%s", req.RoomID)
	}
	return &CreateRoomResult{
		RoomID:      rsp.GetRoomId(),
		BattleAddr:  rsp.GetBattleAddr(),
		BattleToken: rsp.GetBattleToken(),
		PlayerIDs:   []int64{req.PlayerID},
	}, nil
}
