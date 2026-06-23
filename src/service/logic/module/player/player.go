package player

import (
	"game/deps/mongoclient"
	"game/src/persist"
	"game/src/proto/pb"
	"game/src/service/logic/gamedata/base"
)

type GamerPlayerData struct {
	*base.GamerModBase
	gid    int64
	base   *mongoclient.DataPersister[*pb.PlayerBase]
	change map[string]bool
}

func NewGamerPlayerData(modIndex int, data *persist.GamerData, doc *base.ExcelConf, docExt *base.ExcelConfExt) base.IGamerModBase {
	return &GamerPlayerData{
		GamerModBase: base.NewGamerModBase(modIndex, doc, docExt),
		gid:          data.GamerId,
		base:         data.Base,
		change:       make(map[string]bool),
	}
}

func (m *GamerPlayerData) GetPlayerBase() *pb.PlayerBase {
	return m.base.Data()
}

func (m *GamerPlayerData) SetPlayerBase(data *pb.PlayerBase) {
	if data == nil {
		return
	}
	m.base.SetData(data)
	m.base.SaveAllDoc()
	m.markChange("all")
}

func (m *GamerPlayerData) UpdatePlayerBase(update func(base *pb.PlayerBase)) {
	if update == nil {
		return
	}
	update(m.base.Data())
	m.base.SaveAllDoc()
	m.markChange("all")
}

func (m *GamerPlayerData) Send() *pb.PlayerInfoNTF {
	if len(m.change) == 0 {
		return nil
	}

	return &pb.PlayerInfoNTF{Base: m.base.Data()}
}

func (m *GamerPlayerData) markChange(field string) {
	if m.change == nil {
		m.change = make(map[string]bool)
	}
	m.change[field] = true
}

func (m *GamerPlayerData) updateField(field string, value any) {
	m.base.AddUpdateOp(field, value)
	m.markChange(field)
}

// GetGid 获取玩家ID
func (m *GamerPlayerData) GetGid() int64 {
	return m.base.Data().Gid
}

// SetGid 设置玩家ID
func (m *GamerPlayerData) SetGid(gid int64) {
	m.base.Data().Gid = gid
	m.updateField("gid", gid)
}

// GetNickname 获取昵称
func (m *GamerPlayerData) GetNickname() string {
	return m.base.Data().Nickname
}

// SetNickname 设置
func (m *GamerPlayerData) SetNickname(nickname string) {
	m.base.Data().Nickname = nickname
	m.updateField("nickname", nickname)
}

// GetServerId 获取服务器ID
func (m *GamerPlayerData) GetServerId() int32 {
	return m.base.Data().ServerId
}

// SetServerId 设置服务器ID
func (m *GamerPlayerData) SetServerId(serverId int32) {
	m.base.Data().ServerId = serverId
	m.updateField("sid", serverId)
}

// GetLv 获取等级
func (m *GamerPlayerData) GetLv() int32 {
	return m.base.Data().Lv
}

// SetLv 设置等级
func (m *GamerPlayerData) SetLv(lv int32) {
	m.base.Data().Lv = lv
	m.updateField("lv", lv)
}
