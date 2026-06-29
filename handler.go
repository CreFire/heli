package main

import (
	"context"
	kitexpb "game/src/kitexpb"
)

// EchoServiceImpl implements the last service interface defined in the IDL.
type EchoServiceImpl struct{}

// Echo implements the EchoServiceImpl interface.
func (s *EchoServiceImpl) Echo(ctx context.Context, req *kitexpb.EchoRequest) (resp *kitexpb.EchoResponse, err error) {
	// TODO: Your code here...
	return
}
