package pack

import (
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/service/logic/iface"
)

type IPackControl interface {
	GetItemCount(confId int32) int64
	CheckSpace(items ...*pb.Item) errorpb.ERROR
	CheckEnough(items ...*pb.Item) errorpb.ERROR
	SubItems(subItems ...*pb.Item) errorpb.ERROR // 消耗道具
	AddItems(addItems ...*pb.Item) errorpb.ERROR
	UseItems(useItems ...*pb.Item) []*pb.Item
	GetItem(instId int64) *pb.Item
}

type PackControl struct {
	gamer iface.IGamerContext
}

func (m *PackControl) GetItem(instId int64) *pb.Item {
	item := m.gamer.GetModel().ItemPack().GetItem(instId)
	return item
}
