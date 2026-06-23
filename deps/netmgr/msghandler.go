package netmgr

import (
	"game/deps/msg"
)

type INetEventHandler interface {
	OnConnectSuccess(msgque IMsgQue) bool               //连接成功
	OnNewMsgQue(msgque IMsgQue) bool                    //新的消息队列
	OnMsgQueStop(msgque IMsgQue)                        //消息队列关闭
	OnProcessMsg(msgque IMsgQue, msg *msg.Message) bool //默认的消息处理函数
}
