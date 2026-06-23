package player

import (
	"game/deps/mongoclient"
	"game/src/persist"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/service/logic/gamedata/base"
)

type GAMER_MAIN_CHANGE_TYPE uint8

const (
	GAMER_MAIN_CHANGE_EXP   GAMER_MAIN_CHANGE_TYPE = 1 << 0
	GAMER_MAIN_CHANGE_FLAGS GAMER_MAIN_CHANGE_TYPE = 1 << 1
)

type GamerMain struct {
	*base.GamerModBase
	gid int64
	*mongoclient.DataPersister[*pb.Gamer]
	changes GAMER_MAIN_CHANGE_TYPE
}

func (m *GamerMain) SetRegisterTime(t int64) {
	m.Data().RegisterTime = t
	m.AddUpdateOp("reg_t", t)
}

func (m *GamerMain) SetLoginTime(t int64) {
	m.Data().LoginTime = t
	m.AddUpdateOp("lg_t", t)
}

func (m *GamerMain) SetLogoutTimeAndVersion(t int64, v int32) {
	m.Data().OfflineTime = t
	m.Data().LastConfVersion = v
	m.AddUpdateOp("of_t", t)
	m.AddUpdateOp("lcv", v)
}

func (m *GamerMain) SetConfVersion(v int32) {
	m.Data().LastConfVersion = v
	m.AddUpdateOp("lcv", v)
}

func (m *GamerMain) GetLastPassDayTime() int64 {
	return m.Data().LastPassDayTime
}
func (m *GamerMain) SetLastPassDayTime(t int64) {
	m.Data().LastPassDayTime = t
	m.AddUpdateOp("pd_t", t)
}

func (m *GamerMain) SetMain(gamer *pb.Gamer) {
	m.SetData(gamer)
	m.SaveAllDoc()
}

func (m *GamerMain) setChange(typ GAMER_MAIN_CHANGE_TYPE) {
	m.changes |= typ
}

func (m *GamerMain) getChange(typ GAMER_MAIN_CHANGE_TYPE) bool {
	return m.changes&typ == typ
}

// func (m *GamerMain) SetFlag(t bool, offset pb.GAMER_FLAGS) {
// 	m.Data().Flags = xbinary.SetBitUint64(m.Data().GetFlags(), t, int(offset))
// 	m.setChange(GAMER_MAIN_CHANGE_FLAGS)
// 	m.AddUpdateOp("flags", m.Data().GetFlags())
// }

// func (m *GamerMain) GetFlag(offset pb.GAMER_FLAGS) bool {
// 	return xbinary.GetBitUint64(m.Data().GetFlags(), int(offset))
// }

func (m *GamerMain) Rename(name string) errorpb.ERROR {
	return errorpb.ERROR_FAILED
}

func NewGamerMain(modIndex int, data *persist.GamerData, doc *base.ExcelConf, docExt *base.ExcelConfExt) *GamerMain {
	return &GamerMain{
		GamerModBase:  base.NewGamerModBase(modIndex, doc, docExt),
		gid:           data.GamerId,
		DataPersister: persist.GetGamerModData[*pb.Gamer](data, persist.GamerMainModIndex),
	}
}
