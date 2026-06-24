package iface

import (
	"game/src/common"
	"game/src/proto/errorpb"
	"game/src/proto/pb"

	"google.golang.org/protobuf/proto"
)

type ILogger interface {
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
	Debug(format string, args ...any)
}

type IItemModule interface {
	SendPackInfo()
	LegacyLoadBag() map[string]interface{}
	LegacyOpenLibao(item *pb.Item) ([]*pb.Item, errorpb.ERROR)
	AddItems(items []*pb.Item, reason *common.Reason) ([]*pb.Item, errorpb.ERROR)
	SubItems(items []*pb.Item, reason *common.Reason) ([]*pb.Item, errorpb.ERROR)
	CheckEnough(items []*pb.Item) errorpb.ERROR
	CheckEnoughMap(consume map[string]*pb.Item) errorpb.ERROR
	GetItemCount(item *pb.Item) int64
	UseItem(item *pb.Item) []*pb.Item
	DelAllItem() errorpb.ERROR
	GetAfkRewardsWithTransItem(transItems []*pb.Item) []*pb.Item
}

type ITaskModule interface {
	SendTaskInfo()
	SendTaskSync()
	CheckTaskCompleted(id int32) bool
}

type IShopModule interface {
	SendShopInfo(shopId int32)
	BuyGoods(shopId, goodsId int32, num int32) ([]*pb.Item, errorpb.ERROR)
	RefreshShop(shopId int32) errorpb.ERROR
	GetBuyCount(shopId, goodsId int32) int32
}

type IMailModule interface {
	SendMail(title, content string, attachments []*pb.Item, reason *common.Reason)
	SendSystemMail(mailConfId int32, attachments []*pb.Item, sendTime int64, params []string, reason *common.Reason)
	SendSystemMailBySS(mailConfId int32, attachments []*pb.Item, sendTime int64, params []string, reason *common.Reason)
	SendMailToSelf(items []*pb.Item, templateId int32, sendTime int64, title, content string, param []string, reason *common.Reason) error
}

type IActivityModule interface {
	SendActivityInfo()
	SendActivitySync()
	AddActive(activeId int32, num int32, reason *common.Reason) int64
}

type IPlayerModule interface {
	SendPlayerInfo()
	GetGamer() *pb.Gamer
	GetPlayerBase() *pb.PlayerBase
	LegacyLoadUser(gid int64) map[string]interface{}
	LegacyLoadUpdateNum() map[string]interface{}
	NicknameUpdateNum() int32
	UpdateNickname(name string) errorpb.ERROR
}

type ILevelModule interface {
	SendLevelInfo()
}

type IFunctionModule interface {
	CheckFunctionOpenByMsgId(msgId pb.MSG_ID) bool
	CheckFunctionOpen(functionType int32) bool
	OpenAllFunction()
	SendFunctionInfo()
}

type IPackModule interface {
	AddItem(item ...*pb.Item)
	SubItem(item ...*pb.Item)
}

type IMatchModule interface {
	Join(playerID int64, req *pb.C2SMatchJoin) (errorpb.ERROR, proto.Message)
}
