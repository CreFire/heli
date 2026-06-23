package player

import (
	"game/src/persist"
	"game/src/proto/pb"
	"game/src/service/logic/gamedata"
	"game/src/service/logic/gamedata/base"
)

func init() {
	gamedata.RegisterMod(persist.GamerRecordModIndex, func(modIndex int, data *persist.GamerData, doc *base.ExcelConf, docExt *base.ExcelConfExt) base.IGamerModBase {
		return NewGamerRecordData(modIndex, data, doc, docExt)
	})
}

func GetRecordModel(m *gamedata.GamerModel) *GamerRecord {
	return gamedata.GetGamerModel[*GamerRecord](m, persist.GamerRecordModIndex)
}

func NewGamerRecordData(modIndex int, m *persist.GamerData, doc *base.ExcelConf, docExt *base.ExcelConfExt) base.IGamerModBase {
	return &GamerRecord{
		GamerModBase:  base.NewGamerModBase(modIndex, doc, docExt),
		gid:           m.GamerId,
		DataPersister: persist.GetGamerModData[*pb.GamerRecordData](m, persist.GamerRecordModIndex),
	}
}

func newGamerRecordData() *pb.GamerRecordData {
	return &pb.GamerRecordData{
		RecordMap: make(map[int32]int64),
	}
}
