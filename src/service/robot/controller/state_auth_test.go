package controller

import (
	"game/deps/netmgr/options"
	"game/src/configdoc"
	"game/src/proto/pb"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveRobotGateConnectAddr_DefaultTCP(t *testing.T) {
	cfg := &configdoc.ConfigBase{Server: &configdoc.ServerCfg{Net: &configdoc.Net{}}}
	prev := currentRobotRuntime
	currentRobotRuntime = &RobotRuntime{cfg: cfg}
	defer func() { currentRobotRuntime = prev }()

	addr, transport, err := resolveRobotGateConnectAddr(&pb.AuthUseRoleRSP{Host: "127.0.0.1:10301"})
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:10301", addr)
	require.Equal(t, options.TransportTCP, transport)
}

func TestResolveRobotGateConnectAddr_WebSocketFromHost(t *testing.T) {
	cfg := &configdoc.ConfigBase{Server: &configdoc.ServerCfg{Net: &configdoc.Net{Transport: "websocket", WSPath: "/ws"}}}
	prev := currentRobotRuntime
	currentRobotRuntime = &RobotRuntime{cfg: cfg}
	defer func() { currentRobotRuntime = prev }()

	addr, transport, err := resolveRobotGateConnectAddr(&pb.AuthUseRoleRSP{Host: "127.0.0.1:10301"})
	require.NoError(t, err)
	require.Equal(t, "ws://127.0.0.1:10301/ws", addr)
	require.Equal(t, options.TransportWebSocket, transport)
}

func TestResolveRobotGateConnectAddr_WebSocketFromAddr(t *testing.T) {
	cfg := &configdoc.ConfigBase{Server: &configdoc.ServerCfg{Net: &configdoc.Net{Transport: "ws", WSPath: "robot-gate"}}}
	prev := currentRobotRuntime
	currentRobotRuntime = &RobotRuntime{cfg: cfg}
	defer func() { currentRobotRuntime = prev }()

	addr, transport, err := resolveRobotGateConnectAddr(&pb.AuthUseRoleRSP{Addr: &pb.ServerAddr{Ip: "127.0.0.1", Port: 10301}})
	require.NoError(t, err)
	require.Equal(t, "ws://127.0.0.1:10301/robot-gate", addr)
	require.Equal(t, options.TransportWebSocket, transport)
}
