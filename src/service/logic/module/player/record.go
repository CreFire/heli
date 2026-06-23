package player

import (
	"fmt"
	"game/deps/mongoclient"
	"game/deps/xtime"
	"game/src/persist"
	"game/src/proto/pb"
	"game/src/service/logic/gamedata/base"
)

// GamerRecord 结构体用于统计玩家信息
type GamerRecord struct {
	*base.GamerModBase
	gid int64
	*mongoclient.DataPersister[*pb.GamerRecordData]
	changes map[int32]struct{} // 时间记录是否需要保存
}

// 目前只记录不发送
func (m *GamerRecord) Send() {
	if len(m.changes) == 0 {
		return
	}
}

func (m *GamerRecord) SetRecord(key int32, val int64) {
	if m.Data().RecordMap == nil {
		m.Data().RecordMap = make(map[int32]int64)
	}
	m.Data().RecordMap[key] = int64(val)
	m.changes[key] = struct{}{}
	m.AddUpdateOp(fmt.Sprintf("r.%d", key), val)
}

func (m *GamerRecord) GetRecord(key int32) int64 {
	if val, ok := m.Data().RecordMap[key]; ok {
		return val
	}
	return 0
}

func (m *GamerRecord) CheckDiffDay(resetType int32, unix int64) bool {
	time, ok := m.Data().RecordMap[resetType]
	if !ok {
		return false
	}
	t1 := xtime.GetLocalDayZeroUnixWithOffSet(time)
	t2 := xtime.GetLocalDayZeroUnixWithOffSet(unix)
	if t1 != t2 {
		return false
	}
	return true
}

func (m *GamerRecord) Has(login int32) bool {
	_, ok := m.Data().RecordMap[login]
	return ok
}

// NewGamerRecord 创建新的 GamerStats 实例
func NewGamerRecord(modIndex int, data *persist.GamerData, doc *base.ExcelConf, docExt *base.ExcelConfExt) *GamerRecord {
	return &GamerRecord{
		GamerModBase:  base.NewGamerModBase(modIndex, doc, docExt),
		gid:           data.GamerId,
		DataPersister: persist.GetGamerModData[*pb.GamerRecordData](data, persist.GamerRecordModIndex),
		changes:       map[int32]struct{}{},
	}
}
