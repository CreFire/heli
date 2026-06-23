package module

import "game/src/proto/pb"

type OpenFunctionModule struct{}

func (OpenFunctionModule) CheckFunctionOpenByMsgId(msgId pb.MSG_ID) bool { return true }
func (OpenFunctionModule) CheckFunctionOpen(functionType int32) bool     { return true }
func (OpenFunctionModule) OpenAllFunction()                              {}
func (OpenFunctionModule) SendFunctionInfo()                             {}
