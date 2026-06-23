package msg

import "game/src/proto/pb"

const maxCmdActValue = 0xffff

func CmdAct(cmd pb.CMD, act pb.ACT) uint32 {
	return uint32(cmd) | uint32(act)<<16
}

func MakeMSGID(mainCmdID uint16, subCmdID uint16) uint32 {
	return uint32(mainCmdID) | uint32(subCmdID)<<16
}

func GetCmd(msgId uint32) (cmd, act uint16) {
	cmd = uint16(msgId)
	act = uint16(msgId >> 16)
	return cmd, act
}

func CheckCmdAct(cmd pb.CMD, act pb.ACT) bool {
	if cmd < 0 || act < 0 || cmd > maxCmdActValue || act > maxCmdActValue {
		return false
	}
	if _, ok := pb.CMD_name[int32(cmd)]; !ok {
		return false
	}
	if _, ok := pb.ACT_name[int32(act)]; !ok {
		return false
	}
	return true
}

func CheckMsgId(msgid uint32) bool {
	cmd, act := GetCmd(msgid)
	return CheckCmdAct(pb.CMD(cmd), pb.ACT(act))
}
