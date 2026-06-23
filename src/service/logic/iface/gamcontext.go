package iface

import (
	"game/src/configdoc"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/service/logic/gamedata"

	"google.golang.org/protobuf/proto"
)

type IGamerContext interface {
	GetGamerId() int64
	IsOnline() bool
	GetGateSessId() int64
	GetPlayerSessId() int64
	GetSessIds() (playerSessId, gateSessId int64)

	GetModel() *gamedata.GamerModel

	SendMsg(msg proto.Message)
	SendCode(code errorpb.ERROR, msg proto.Message)
	SendErrorCode(msgId pb.MSG_ID, err errorpb.ERROR)
	AddMsgTask(msgId pb.MSG_ID, f func()) error
	Post(f func())

	Doc() *configdoc.DocPbConfig
	DocExt() *configdoc.DocExtendConfig

	Now() int64
	NowMs() int64
	RandInt(min, max int32) int32
	RandFloat() float32

	Logger() ILogger

	Item() IItemModule
	Hero() IHeroModule
	Task() ITaskModule
	Shop() IShopModule
	Mail() IMailModule
	Activity() IActivityModule
	Player() IPlayerModule
	Function() IFunctionModule
}

type IHeroModule interface {
	SendHeroInfo() bool
}
