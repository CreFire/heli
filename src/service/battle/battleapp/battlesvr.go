package battleapp

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

const battleRoomTickInterval = 200 * time.Millisecond

var battleSvr = NewBattleSvr()

func App() *BattleSvr {
	return battleSvr
}

func NewBattleSvr() *BattleSvr {
	return &BattleSvr{
		roomMgr: newRoomManager(),
	}
}

type battleSettleSender interface {
	Send(roomID string, settlement battlesync.Settlement) (*pb.S2SBattleSettleRSP, error)
}

type defaultBattleSettleSender struct{}

func (defaultBattleSettleSender) Send(roomID string, settlement battlesync.Settlement) (*pb.S2SBattleSettleRSP, error) {
	inst, err := server.MS.SvrMgr.PickMinOnline("logic", true)
	if err != nil || inst == nil {
		return nil, fmt.Errorf("pick logic server failed: %w", err)
	}
	m := msg.NewMsgWithProto(pb.MSG_ID_S2S_BATTLE_SETTLE_REQ, battlesync.SettlementToProto(settlement))
	if room := battleSvr.roomMgr.getRoom(roomID); room != nil && len(room.playerIDs) > 0 {
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

type BattleSvr struct {
	roomMgr      *roomManager
	settleSender battleSettleSender // 可替换发送器，便于单测 mock battle -> logic 结算链路。
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
	// 当前 token 只作为 P0 最小直连凭证，不承担正式安全职责。
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

func (b *BattleSvr) startRoomLoop(room *battleRoom) {
	if room == nil || room.room == nil {
		return
	}
	room.mu.Lock()
	if room.loop != nil {
		room.mu.Unlock()
		return
	}
	loop := &roomLoop{
		ticker:    time.NewTicker(battleRoomTickInterval),
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
		interval:  battleRoomTickInterval,
	}
	room.loop = loop
	room.mu.Unlock()

	go func() {
		defer close(loop.stoppedCh)
		defer loop.ticker.Stop()
		// 每个战斗房间独立推进；当前不依赖客户端帧驱动，而是 battle 服权威 tick。
		for {
			select {
			case <-loop.stopCh:
				return
			case <-loop.ticker.C:
				b.runRoomTick(room)
			}
		}
	}()
}

func (b *BattleSvr) stopRoomLoop(room *battleRoom) {
	if room == nil {
		return
	}
	room.mu.Lock()
	loop := room.loop
	room.loop = nil
	room.mu.Unlock()
	if loop == nil {
		return
	}
	close(loop.stopCh)
	<-loop.stoppedCh
}

func (b *BattleSvr) runRoomTick(room *battleRoom) {
	if room == nil {
		return
	}
	snapshot := room.advanceLoopTick()
	// 每次 tick 后先广播本 tick 产生的状态增量；若房间已结束，再进入结算收口。
	b.broadcastRoomDelta(room)
	if snapshot.State == battlesync.RoomStateClosed {
		b.finishBattleRoom(room, snapshot)
	}
}

func (b *BattleSvr) finishBattleRoom(room *battleRoom, snapshot battlesync.Snapshot) {
	if room == nil {
		return
	}
	room.mu.Lock()
	loop := room.loop
	room.mu.Unlock()
	if loop == nil {
		return
	}
	loop.settleOnce.Do(func() {
		go func() {
			defer b.stopRoomLoop(room)
			if snapshot.FinishReason == battlesync.FinishNone {
				snapshot = room.roomSnapshot()
			}
			// sync.Room 和 roomLoop 双层防重：
			// 1. settleOnce 防止 finishBattleRoom 并发重入
			// 2. MarkSettled 防止房间状态层重复生成结算
			if !room.markSettled() {
				return
			}
			settlement := room.buildSettlement()
			rsp, err := b.sendSettlement(room.id, settlement)
			if err != nil {
				room.markSettleAck(false, err.Error())
				xlog.Warnf("battle settle send failed. roomId:%s battleId:%s err:%v", room.id, settlement.BattleID, err)
				return
			}
			room.markSettleAck(rsp.GetAccepted(), rsp.GetMessage())
			if !rsp.GetAccepted() {
				xlog.Warnf("battle settle rejected. roomId:%s battleId:%s message:%s", room.id, settlement.BattleID, rsp.GetMessage())
				return
			}
			xlog.Infof("battle settle acked. roomId:%s battleId:%s finish:%s tick:%d", room.id, settlement.BattleID, settlement.FinishReason, settlement.EndTick)
		}()
		xlog.Infof("battle room finished. roomId:%s tick:%d finish:%s baseHP:%d", room.id, snapshot.ServerTick, snapshot.FinishReason, snapshot.BaseHP)
	})
}

func (b *BattleSvr) sendSettlement(roomID string, settlement battlesync.Settlement) (*pb.S2SBattleSettleRSP, error) {
	sender := b.settleSender
	if sender == nil {
		sender = defaultBattleSettleSender{}
	}
	return sender.Send(roomID, settlement)
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
	deltas := room.flushRoomDeltas()
	if len(deltas) == 0 {
		return
	}
	snapshot := room.roomSnapshot()
	ntf := battlesync.DeltasToProto(room.id, snapshot.ServerTick, deltas)
	// 仅广播已成功 join 并绑定 session 的玩家。
	for playerID, sessID := range room.snapshotJoinedSessions() {
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
	if req.GetOpId() == "" {
		rsp.Code = errorpb.ERROR_REQUEST_PARAMS
		rsp.Message = "op id is empty"
		return rsp
	}
	if room.state() == battlesync.RoomStateClosed {
		rsp.Code = errorpb.ERROR_FAILED
		rsp.Message = "battle already finished"
		return rsp
	}
	var result battlesync.OpResult
	room.withRoom(func(syncRoom *battlesync.Room) {
		// battle handler 只负责协议解包和最小路由，具体玩法规则全部下沉到 sync.Room。
		switch req.GetOp().GetType() {
		case pb.BattleOpType_BATTLE_OP_BUILD_TOWER:
			build := req.GetOp().GetBuildTower()
			if build == nil {
				rsp.Code = errorpb.ERROR_REQUEST_PARAMS
				rsp.Message = "build_tower is nil"
				return
			}
			result = syncRoom.BuildTower(playerID, req.GetOpId(), build.GetGridId())
		case pb.BattleOpType_BATTLE_OP_REROLL_TOWER:
			reroll := req.GetOp().GetRerollTower()
			if reroll == nil {
				rsp.Code = errorpb.ERROR_REQUEST_PARAMS
				rsp.Message = "reroll_tower is nil"
				return
			}
			result = syncRoom.RerollTower(playerID, req.GetOpId(), reroll.GetTowerId())
		case pb.BattleOpType_BATTLE_OP_MERGE_TOWER:
			merge := req.GetOp().GetMergeTower()
			if merge == nil {
				rsp.Code = errorpb.ERROR_REQUEST_PARAMS
				rsp.Message = "merge_tower is nil"
				return
			}
			result = syncRoom.MergeTower(playerID, req.GetOpId(), merge.GetMainTowerId(), merge.GetMaterialTowerId())
		case pb.BattleOpType_BATTLE_OP_BUY_MINER:
			result = syncRoom.BuyMiner(playerID, req.GetOpId())
		default:
			rsp.Code = errorpb.ERROR_REQUEST_PARAMS
			rsp.Message = "unsupported battle op type"
		}
	})
	if rsp.GetCode() != errorpb.ERROR_SUCCESS {
		return rsp
	}
	if !result.OK {
		rsp.Code = errorpb.ERROR_FAILED
		rsp.Message = string(result.Code)
		return rsp
	}
	rsp.ServerTick = room.roomSnapshot().ServerTick
	rsp.TowerId = result.TowerID
	rsp.MinerId = result.MinerID
	return rsp
}
