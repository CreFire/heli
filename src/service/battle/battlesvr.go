package main

import (
	"game/deps/misc"
	"game/deps/server"
	"game/deps/xlog"
	"game/deps/xtime"
	"game/src/cache"
	"game/src/configdoc"
	"game/src/persist"
	"game/src/proto/eventpb"
	"time"
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

func (b *BattleSvr) OnInit() error {
	return RegisterBattleHandlers()
}

func (b *BattleSvr) BeforeStart() error {
	return nil
}

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
		xlog.Warnf("battle before stop delete server info failed. service:%s instanceId:%d err:%v",
			inst.ServiceName, inst.InstanceId, err)
	}
	return nil
}

func (b *BattleSvr) OnStop() error {
	return nil
}

func (b *BattleSvr) OnReload(oldCfg, newCfg *configdoc.ConfigBase) error {
	return nil
}

func (b *BattleSvr) OnHeart(now int64) error {
	return nil
}

func (b *BattleSvr) OnEventHandle(_ *eventpb.Event) {}

func (b *BattleSvr) ReportServerInfo(name string, now int64, value any) {
	inst := server.MS.SvrMgr.SelfCopy()
	inst.UpdateOnlineCount(int32(b.roomMgr.roomCount()))
	if err := cache.UpdateServerInfoWithOnline(inst); err != nil {
		xlog.Warnf("battle report server info failed. service:%s instanceId:%d err:%v",
			inst.ServiceName, inst.InstanceId, err)
	}
}

func (b *BattleSvr) printStatus(name string, now int64, value any) {
	xlog.Infof("[run-status] buildTime=%v | progVer=%v | excelVer=%v | launchTime=%v | gmTime=%v | roomCount=%v",
		misc.BuildTime,
		misc.ProgVer,
		misc.ExcelVer,
		server.MS.LaunchTime.Format(time.DateTime),
		xtime.Now().Format(time.DateTime),
		b.roomMgr.roomCount(),
	)
}
