package controller

import (
	"testing"

	"game/deps/server"
	servicemgr "game/deps/service_mgr"
	"game/src/configdoc"
	"game/src/proto/pb"
)

func TestFillUseRoleServerEndpointUsesHostWhenConfigured(t *testing.T) {
	oldMS := server.MS
	defer func() {
		server.MS = oldMS
	}()

	server.MS = &server.Server{
		ConfBase: &configdoc.ConfigBase{
			Global: &configdoc.GlobalCfg{
				GatePublicHost: "10.0.0.1:9527",
			},
		},
	}

	res := &pb.AuthUseRoleRSP{}
	fillUseRoleServerEndpoint(res, &servicemgr.ServiceInstance{
		Host: "127.0.0.1",
		Port: 5001,
	})

	if res.Host != "10.0.0.1:9527" {
		t.Fatalf("unexpected host: %q", res.Host)
	}
	if res.Addr != nil {
		t.Fatalf("expected addr to stay nil when host is configured")
	}
}

func TestFillUseRoleServerEndpointFallsBackToAddr(t *testing.T) {
	oldMS := server.MS
	defer func() {
		server.MS = oldMS
	}()

	server.MS = &server.Server{
		ConfBase: &configdoc.ConfigBase{
			Global: &configdoc.GlobalCfg{},
		},
	}

	res := &pb.AuthUseRoleRSP{}
	fillUseRoleServerEndpoint(res, &servicemgr.ServiceInstance{
		Host: "192.168.1.10",
		Port: 5001,
	})

	if res.Host != "" {
		t.Fatalf("expected empty host, got %q", res.Host)
	}
	if res.Addr == nil {
		t.Fatalf("expected addr to be filled")
	}
	if res.Addr.Ip != "192.168.1.10" || res.Addr.Port != 5001 {
		t.Fatalf("unexpected addr: %+v", res.Addr)
	}
}
