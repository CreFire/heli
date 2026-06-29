package task

import (
	"game/deps/mongoclient"
	"game/src/persist"
	"game/src/proto/pb"
	"game/src/service/logic/gamedata/base"
)

type GamerTask struct {
	*base.GamerModBase
	*mongoclient.DataPersister[*pb.GamerTaskData]
}

func NewGamerTask(modIndex int, data *persist.GamerData, doc *base.ExcelConf, docExt *base.ExcelConfExt) *GamerTask {
	return &GamerTask{
		GamerModBase:  base.NewGamerModBase(modIndex, doc, docExt),
		DataPersister: persist.GetGamerModData[*pb.GamerTaskData](data, persist.GamerTaskModIndex),
	}
}

func (m *GamerTask) EnsureTaskMap() map[int32]*pb.GameTask {
	if m.Data().TaskMap == nil {
		m.Data().TaskMap = make(map[int32]*pb.GameTask)
	}
	return m.Data().TaskMap
}
