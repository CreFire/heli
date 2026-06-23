package common

import (
	"game/src/proto/pb"
)

const (
	SERVER_MSG_ID_ROUTER_ROUND = 1000
)

func CheckRouter(msgId pb.MSG_ID) pb.SER_TYPE {
	svrType := msgId / SERVER_MSG_ID_ROUTER_ROUND
	svr := pb.SER_TYPE(svrType)

	return svr
}

func CheckSvrType(msgId pb.MSG_ID) string {
	svr := CheckRouter(msgId)
	switch svr {
	case pb.SER_TYPE_ID_LOGIC:
		return InnerServerTypeLogic
	}
	return ""
}
