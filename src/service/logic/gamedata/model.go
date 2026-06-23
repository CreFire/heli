package gamedata

import (
	"game/deps/xlog"
	"game/src/persist"
	"game/src/service/logic/gamedata/base"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type GamerModel struct { // gamer data container
	gid       int64
	data      *persist.GamerData
	GamerMods [persist.GamerDataModIndexMax]base.IGamerModBase
}

func GetGamerModel[T base.IGamerModBase](m *GamerModel, modIndex int) T {
	if modIndex < 0 || modIndex >= persist.GamerDataModIndexMax {
		panic("GamerModel.GetGamerMod: invalid modIndex")
	}
	modBase := m.GamerMods[modIndex]
	if modBase == nil {
		panic("GamerModel.GetGamerMod: mod not registered")
	}
	if modBase.ModIndex() != modIndex {
		panic("GamerModel.GetGamerMod: modIndex mismatch")
	}
	return modBase.(T)
}

func (m *GamerModel) SaveDocRaw() bson.Raw {
	return m.data.SaveDocs()
}

func NewGamerModel(gid int64, doc *base.ExcelConf, docExt *base.ExcelConfExt) (*GamerModel, error) {
	data := persist.NewGamerData(gid)
	if err := data.InitLoad(); err != nil {
		xlog.Errorf("GamerModel InitLoad error: %v, gid: %v", err, gid)
		return nil, err
	}
	m := &GamerModel{gid: gid, data: data}

	for idx := range persist.GamerDataModIndexMax {
		factory, ok := modFactories[idx]
		if !ok {
			continue
		}
		m.GamerMods[idx] = factory(idx, data, doc, docExt)
	}
	return m, nil
}

func NewGamerModelProxy(gid int64) *GamerModel {
	return &GamerModel{gid: gid}
}

// FunctionUnlockOK is a placeholder for feature unlock checks.
func (m *GamerModel) FunctionUnlockOK(unlockId int32) bool {
	return true
}

// Save all module data.
func (m *GamerModel) Save() error {
	return m.data.Save()
}

func (m *GamerModel) OnDocReload(doc *base.ExcelConf, docExt *base.ExcelConfExt) {
	for _, v := range m.GamerMods {
		if v == nil {
			continue
		}
		v.OnDocReload(doc, docExt)
	}
}
