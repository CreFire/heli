package main

import (
	"fmt"
	"strings"
	"time"

	"game/deps/misc"
	"game/deps/msg"
	"game/deps/server"
	"game/deps/xlog"
	"game/deps/xtime"
	"game/src/cache"
	"game/src/configdoc"
	"game/src/persist"
	"game/src/proto/errorpb"
	"game/src/proto/eventpb"
	"game/src/proto/pb"
	battlesync "game/src/service/battle/sync"

	"google.golang.org/protobuf/proto"
)

var battleSvr = NewBattleSvr()

func NewBattleSvr() *BattleSvr {
	return &BattleSvr{
		roomMgr: newRoomManager(),
	}
}

type BattleSvr struct {
	roomMgr *roomManager
}

func (b *BattleSvr) OnInit() error                                       { return RegisterBattleHandlers() }
func (b *BattleSvr) BeforeStart() error                                  { return nil }
func (b *BattleSvr) OnStop() error                                       { return nil }
func (b *BattleSvr) OnReload(oldCfg, newCfg *configdoc.ConfigBase) error { return nil }
func (b *BattleSvr) OnHeart(now int64) error                             { return nil }
func (b *BattleSvr) OnEventHandle(_ *eventpb.Event)                      {}

func (b *BattleSvr) OnStart() error {
	persist.InitCollections()
	if _, err := server.MS.TimerMgr.AddSimpleTimer("report_server_info", 3, true, b.ReportServerInfo); err != nil {
		return err
	}
	if _, err := server.MS.TimerMgr.AddSimpleTimer("print_status", 20, true, b.printStatus); err != nil {
		return err
	}
	return nil
}

func (b *BattleSvr) BeforeStop() error {
	inst := server.MS.SvrMgr.SelfCopy()
	if err := cache.DelServerInfo(inst); err != nil {
		xlog.Warnf("battle before stop delete server info failed. service:%s instanceId:%d err:%v", inst.ServiceName, inst.InstanceId, err)
	}
	return nil
}

func (b *BattleSvr) ReportServerInfo(name string, now int64, value any) {
	inst := server.MS.SvrMgr.SelfCopy()
	inst.UpdateOnlineCount(int32(b.roomMgr.roomCount()))
	if err := cache.UpdateServerInfoWithOnline(inst); err != nil {
		xlog.Warnf("battle report server info failed. service:%s instanceId:%d err:%v", inst.ServiceName, inst.InstanceId, err)
	}
}

func (b *BattleSvr) printStatus(name string, now int64, value any) {
	xlog.Infof("[run-status] buildTime=%v | progVer=%v | excelVer=%v | launchTime=%v | gmTime=%v | roomCount=%v",
		misc.BuildTime, misc.ProgVer, misc.ExcelVer, server.MS.LaunchTime.Format(time.DateTime), xtime.Now().Format(time.DateTime), b.roomMgr.roomCount())
}

func (b *BattleSvr) battleAddr() string {
	if server.MS == nil || server.MS.ConfBase == nil || server.MS.ConfBase.Server == nil {
		return ""
	}
	conf := server.MS.ConfBase.Server
	if conf.Ip == "" || conf.Port == 0 {
		return ""
	}
	return fmt.Sprintf("%s:%d", conf.Ip, conf.Port)
}

func (b *BattleSvr) buildBattleToken(roomID string, playerIDs []int64) string {
	parts := make([]string, 0, len(playerIDs))
	for _, playerID := range playerIDs {
		parts = append(parts, fmt.Sprintf("%d", playerID))
	}
	return fmt.Sprintf("battle:%s:%s", roomID, strings.Join(parts, ","))
}

func (b *BattleSvr) verifyBattleToken(room *battleRoom, playerID int64, roomID, token string) error {
	if room == nil {
		return fmt.Errorf("room not found")
	}
	if roomID == "" || room.id != roomID {
		return fmt.Errorf("room id mismatch")
	}
	if !room.hasPlayer(playerID) {
		return fmt.Errorf("player not in room")
	}
	if token == "" {
		return fmt.Errorf("battle token is empty")
	}
	if room.allowedToken != "" && room.allowedToken != token {
		return fmt.Errorf("battle token invalid")
	}
	if !strings.HasPrefix(token, fmt.Sprintf("battle:%s:", roomID)) {
		return fmt.Errorf("battle token malformed")
	}
	return nil
}

func (b *BattleSvr) settleRoom(roomID string, settlement battlesync.Settlement) (*pb.S2SBattleSettleRSP, error) {
	inst, err := server.MS.SvrMgr.PickMinOnline("logic", true)
	if err != nil || inst == nil {
		return nil, fmt.Errorf("pick logic server failed: %w", err)
	}
	m := msg.NewMsgWithProto(pb.MSG_ID_S2S_BATTLE_SETTLE_REQ, battlesync.SettlementToProto(settlement))
	if room := b.roomMgr.getRoom(roomID); room != nil && len(room.playerIDs) > 0 {
		m.SetHashKey(room.playerIDs[0])
	}
	resp, err := server.MS.Rpc.SendRequestWithBlock("logic", inst.InstanceId, m, nil)
	if err != nil {
		return nil, err
	}
	msgBody := server.MS.Rpc.GetPlayLoadMessage(resp)
	rsp, _ := msgBody.(*pb.S2SBattleSettleRSP)
	if rsp == nil {
		return nil, fmt.Errorf("battle settle rsp decode failed")
	}
	return rsp, nil
}

func (b *BattleSvr) sendProtoToSess(sessID, playerID int64, body proto.Message) {
	if sessID <= 0 || body == nil || server.MS == nil || server.MS.NetMgr == nil {
		return
	}
	server.MS.NetMgr.SendMsg2Sess(sessID, msg.NewRspMsgWithProtoAndCode(0, errorpb.ERROR_SUCCESS, body).SetUserInfo(sessID, playerID), nil)
}

func (b *BattleSvr) broadcastRoomDelta(room *battleRoom) {
	if room == nil || room.room == nil {
		return
	}
	deltas := room.room.FlushDeltas()
	if len(deltas) == 0 {
		return
	}
	snapshot := room.room.Snapshot()
	ntf := battlesync.DeltasToProto(room.id, snapshot.ServerTick, deltas)
	for playerID, sessID := range room.joinedSess {
		b.sendProtoToSess(sessID, playerID, ntf)
	}
}

func (b *BattleSvr) applyBattleOp(room *battleRoom, playerID int64, req *pb.C2SBattleOpREQ) *pb.S2CBattleOpRSP {
	rsp := &pb.S2CBattleOpRSP{RoomId: req.GetRoomId(), OpId: req.GetOpId(), Code: errorpb.ERROR_SUCCESS, Message: "ok"}
	if room == nil || room.room == nil || req == nil || req.GetOp() == nil {
		rsp.Code = errorpb.ERROR_REQUEST_PARAMS
		rsp.Message = "invalid battle op request"
		return rsp
	}
	var result battlesync.OpResult
	switch req.GetOp().GetType() {
	case pb.BattleOpType_BATTLE_OP_BUILD_TOWER:
		build := req.GetOp().GetBuildTower()
		if build == nil {
			rsp.Code = errorpb.ERROR_REQUEST_PARAMS
			rsp.Message = "build_tower is nil"
			return rsp
		}
		result = room.room.BuildTower(playerID, req.GetOpId(), build.GetGridId())
	case pb.BattleOpType_BATTLE_OP_REROLL_TOWER:
		reroll := req.GetOp().GetRerollTower()
		if reroll == nil {
			rsp.Code = errorpb.ERROR_REQUEST_PARAMS
			rsp.Message = "reroll_tower is nil"
			return rsp
		}
		result = room.room.RerollTower(playerID, req.GetOpId(), reroll.GetTowerId())
	case pb.BattleOpType_BATTLE_OP_MERGE_TOWER:
		merge := req.GetOp().GetMergeTower()
		if merge == nil {
			rsp.Code = errorpb.ERROR_REQUEST_PARAMS
			rsp.Message = "merge_tower is nil"
			return rsp
		}
		result = room.room.MergeTower(playerID, req.GetOpId(), merge.GetMainTowerId(), merge.GetMaterialTowerId())
	case pb.BattleOpType_BATTLE_OP_BUY_MINER:
		result = room.room.BuyMiner(playerID, req.GetOpId())
	default:
		rsp.Code = errorpb.ERROR_REQUEST_PARAMS
		rsp.Message = "unsupported battle op type"
		return rsp
	}
	if !result.OK {
		rsp.Code = errorpb.ERROR_FAILED
		rsp.Message = string(result.Code)
		return rsp
	}
	rsp.ServerTick = room.room.Snapshot().ServerTick
	rsp.TowerId = result.TowerID
	rsp.MinerId = result.MinerID
	return rsp
}
