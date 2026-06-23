package etcd

import (
	"context"
	"game/deps/xlog"
	"strings"
	"testing"
	"time"

	mvccpb "go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestParseDSN(t *testing.T) {
	t.Run("parses comma separated endpoints credentials and timeout", func(t *testing.T) {
		cfg, err := ParseDSN("etcd://user:pass@127.0.0.1:2379,127.0.0.2:2379,127.0.0.3:2379?dialTimeout=5s")
		if err != nil {
			t.Fatalf("ParseDSN returned error: %v", err)
		}
		if cfg == nil {
			t.Fatal("expected config")
		}
		if cfg.Username != "user" || cfg.Password != "pass" {
			t.Fatalf("unexpected credentials: %q %q", cfg.Username, cfg.Password)
		}
		if cfg.TLS != nil {
			t.Fatal("expected no TLS config for etcd scheme")
		}
		if cfg.DialTimeout != 5*time.Second {
			t.Fatalf("expected dial timeout 5s, got %v", cfg.DialTimeout)
		}
		want := []string{"127.0.0.1:2379", "127.0.0.2:2379", "127.0.0.3:2379"}
		if len(cfg.Endpoints) != len(want) {
			t.Fatalf("expected %d endpoints, got %v", len(want), cfg.Endpoints)
		}
		for i, endpoint := range want {
			if cfg.Endpoints[i] != endpoint {
				t.Fatalf("expected endpoint %q at index %d, got %q", endpoint, i, cfg.Endpoints[i])
			}
		}
	})

	t.Run("parses comma separated endpoints without credentials", func(t *testing.T) {
		cfg, err := ParseDSN("etcd://127.0.0.1:2379,127.0.0.2:2379,127.0.0.3:2379")
		if err != nil {
			t.Fatalf("ParseDSN returned error: %v", err)
		}
		if cfg == nil {
			t.Fatal("expected config")
		}
		if cfg.Username != "" || cfg.Password != "" {
			t.Fatalf("unexpected credentials: %q %q", cfg.Username, cfg.Password)
		}
		want := []string{"127.0.0.1:2379", "127.0.0.2:2379", "127.0.0.3:2379"}
		if len(cfg.Endpoints) != len(want) {
			t.Fatalf("expected %d endpoints, got %v", len(want), cfg.Endpoints)
		}
		for i, endpoint := range want {
			if cfg.Endpoints[i] != endpoint {
				t.Fatalf("expected endpoint %q at index %d, got %q", endpoint, i, cfg.Endpoints[i])
			}
		}
	})

	t.Run("rejects invalid input", func(t *testing.T) {
		cases := []struct {
			name string
			dsn  string
		}{
			{name: "empty", dsn: ""},
			{name: "unsupported scheme", dsn: "http://127.0.0.1:2379"},
			{name: "etcds unsupported", dsn: "etcds://127.0.0.1:2379"},
			{name: "missing endpoints", dsn: "etcd://"},
			{name: "invalid timeout", dsn: "etcd://127.0.0.1:2379?dialTimeout=nope"},
			{name: "legacy addr query format", dsn: "etcd://127.0.0.1:2379?addr=127.0.0.2:2379&addr=127.0.0.3:2379"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if _, err := ParseDSN(tc.dsn); err == nil {
					t.Fatalf("expected ParseDSN(%q) to fail", tc.dsn)
				}
			})
		}
	})
}

func TestRegistrarHelpers(t *testing.T) {
	if got := RegistrarKey("cluster-a", "logic", 3); got != "/cluster-a/logic/3" {
		t.Fatalf("unexpected registrar key: %q", got)
	}
	if got := RegistrarPrefix("cluster-a", "logic"); got != "/cluster-a/logic/" {
		t.Fatalf("unexpected registrar prefix: %q", got)
	}

	cluster, service, id := ParseServerInfoFromKey("/cluster-a/logic/3")
	if cluster != "cluster-a" || service != "logic" || id != 3 {
		t.Fatalf("unexpected parsed key: cluster=%q service=%q id=%d", cluster, service, id)
	}

	cluster, service, id = ParseServerInfoFromKey("/cluster-a/logic/battle/3")
	if cluster != "" || service != "" || id != 0 {
		t.Fatalf("expected nested service key to be invalid, got cluster=%q service=%q id=%d", cluster, service, id)
	}
}

func TestParseServerInfoFromKeyInvalidInput(t *testing.T) {
	cases := []string{
		"",
		"/cluster-a/logic",
		"/cluster-a/logic/not-a-number",
	}

	for _, key := range cases {
		t.Run(key, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("ParseServerInfoFromKey(%q) panicked: %v", key, r)
				}
			}()

			cluster, service, id := ParseServerInfoFromKey(key)
			if cluster != "" || service != "" || id != 0 {
				t.Fatalf("expected zero values for invalid key %q, got cluster=%q service=%q id=%d", key, cluster, service, id)
			}
		})
	}
}

func TestServicePrefix(t *testing.T) {
	cases := []struct {
		name        string
		cluster     string
		serviceName string
		want        string
		wantErr     bool
	}{
		{name: "plain", cluster: "test", serviceName: "logic", want: "/test/logic/"},
		{name: "trim spaces", cluster: "  test  ", serviceName: "  logic  ", want: "/test/logic/"},
		{name: "empty cluster", cluster: "   ", serviceName: "logic", wantErr: true},
		{name: "empty service", cluster: "test", serviceName: "   ", wantErr: true},
		{name: "service contains slash", cluster: "test", serviceName: "prefix/logic", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := servicePrefix(tc.cluster, tc.serviceName)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected servicePrefix(%q, %q) to fail", tc.cluster, tc.serviceName)
				}
				return
			}
			if err != nil {
				t.Fatalf("servicePrefix(%q, %q) returned error: %v", tc.cluster, tc.serviceName, err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestParseDiscoveryTarget(t *testing.T) {
	cluster, serviceName, err := ParseDiscoveryTarget("/cluster-a/logic")
	if err != nil {
		t.Fatalf("ParseDiscoveryTarget returned error: %v", err)
	}
	if cluster != "cluster-a" || serviceName != "logic" {
		t.Fatalf("unexpected parsed target: cluster=%q service=%q", cluster, serviceName)
	}

	if _, _, err := ParseDiscoveryTarget("/logic"); err == nil {
		t.Fatal("expected missing cluster to fail")
	}
	if _, _, err := ParseDiscoveryTarget("/cluster-a/prefix/logic"); err == nil {
		t.Fatal("expected nested service to fail")
	}
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	inst := &ServiceInstance{
		ClusterName: "cluster-a",
		ServiceName: "logic",
		InstanceId:  3,
		Host:        "127.0.0.1",
		Port:        9000,
		Healthy:     "health",
		Enable:      true,
		OnlineCount: 12,
		ProVersion:  1001,
		ConfVersion: 2002,
		Weight:      5,
		MetaData: map[string]string{
			"zone":         "1",
			"pro_version":  "legacy",
			"conf_version": "legacy",
		},
	}

	data, err := marshal(inst)
	if err != nil {
		t.Fatalf("marshal returned error: %v", err)
	}

	got, err := unmarshal([]byte(data))
	if err != nil {
		t.Fatalf("unmarshal returned error: %v", err)
	}
	if got == nil || !got.Equal(inst) || got.OnlineCount != inst.OnlineCount {
		t.Fatalf("unexpected round-trip result: %#v", got)
	}
	if got.MetaData["pro_version"] != "legacy" || got.MetaData["conf_version"] != "legacy" {
		t.Fatalf("metadata should preserve version keys: %#v", got.MetaData)
	}

	if _, err := unmarshal([]byte("{")); err == nil {
		t.Fatal("expected invalid json to fail unmarshal")
	}
}

func TestServiceInstanceEqualMatchesManagerSemantics(t *testing.T) {
	base := &ServiceInstance{
		ClusterName: "cluster-a",
		ServiceName: "logic",
		InstanceId:  3,
		Host:        "127.0.0.1",
		Port:        9000,
		Healthy:     "health",
		Enable:      true,
		OnlineCount: 12,
		Weight:      5,
		ProVersion:  1001,
		ConfVersion: 2002,
		MetaData:    map[string]string{"zone": "1"},
	}
	same := *base
	same.OnlineCount = 99
	if !base.Equal(&same) {
		t.Fatal("expected OnlineCount to be ignored by Equal")
	}

	diffWeight := same
	diffWeight.Weight = 6
	if base.Equal(&diffWeight) {
		t.Fatal("expected Weight change to affect Equal")
	}

	diffProVersion := same
	diffProVersion.ProVersion = 1002
	if base.Equal(&diffProVersion) {
		t.Fatal("expected ProVersion change to affect Equal")
	}

	diffConfVersion := same
	diffConfVersion.ConfVersion = 2003
	if base.Equal(&diffConfVersion) {
		t.Fatal("expected ConfVersion change to affect Equal")
	}
}

func TestSendWatchResponse(t *testing.T) {
	t.Run("delivers response", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := make(chan *WatchResponse, 1)
		resp := &WatchResponse{Events: []*EtcdWatchEvent{{Type: EventTypePut, Key: "/logic/1"}}}
		if !sendWatchResponse(ctx, ch, resp) {
			t.Fatal("expected sendWatchResponse to succeed")
		}

		select {
		case got := <-ch:
			if got != resp {
				t.Fatal("received unexpected response pointer")
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for response")
		}
	})

	t.Run("returns false when context canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if sendWatchResponse(ctx, make(chan *WatchResponse), &WatchResponse{}) {
			t.Fatal("expected sendWatchResponse to fail for canceled context")
		}
	})

	t.Run("waits when channel is full and exits on context cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		ch := make(chan *WatchResponse, 1)
		ch <- &WatchResponse{}
		done := make(chan bool, 1)
		go func() {
			done <- sendWatchResponse(ctx, ch, &WatchResponse{})
		}()

		select {
		case got := <-done:
			t.Fatalf("sendWatchResponse returned before channel space or cancel: %v", got)
		case <-time.After(50 * time.Millisecond):
		}

		cancel()
		select {
		case got := <-done:
			if got {
				t.Fatal("expected sendWatchResponse to fail after context cancel")
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for sendWatchResponse to exit after cancel")
		}
	})
}

func TestServiceInstanceFromValue(t *testing.T) {
	inst := &ServiceInstance{
		ClusterName: "cluster-a",
		ServiceName: "logic",
		InstanceId:  3,
	}
	data, err := marshal(inst)
	if err != nil {
		t.Fatalf("marshal returned error: %v", err)
	}

	got := serviceInstanceFromValue([]byte("/cluster-a/logic/3"), []byte(data))
	if got == nil || !got.Equal(inst) {
		t.Fatalf("unexpected decoded instance: %#v", got)
	}

	if got := serviceInstanceFromValue([]byte("/cluster-a/logic/3"), []byte("not-json")); got != nil {
		t.Fatalf("expected invalid json to return nil instance, got %#v", got)
	}
}

func TestBuildWatchOptionsPreservesCallerOptions(t *testing.T) {
	opts := buildWatchOptions([]clientv3.OpOption{clientv3.WithPrefix()}, 7)
	op := clientv3.OpGet("/logic", opts...)
	if !op.IsOptsWithPrefix() {
		t.Fatal("expected caller WithPrefix option to be preserved")
	}
	if op.Rev() != 7 {
		t.Fatalf("expected revision 7, got %d", op.Rev())
	}

	opts = buildWatchOptions(nil, 9)
	op = clientv3.OpGet("/logic", opts...)
	if op.IsOptsWithPrefix() {
		t.Fatal("expected nil caller options to keep exact-key watch semantics")
	}
	if op.Rev() != 9 {
		t.Fatalf("expected revision 9, got %d", op.Rev())
	}
}

func TestWatchEventsFromEtcd(t *testing.T) {
	inst := &ServiceInstance{
		ClusterName: "cluster-a",
		ServiceName: "logic",
		InstanceId:  3,
		Host:        "127.0.0.1",
		Port:        9000,
		Healthy:     "health",
		Enable:      true,
	}
	value, err := marshal(inst)
	if err != nil {
		t.Fatalf("marshal returned error: %v", err)
	}

	events := watchEventsFromEtcd([]*clientv3.Event{
		{
			Type: clientv3.EventTypePut,
			Kv:   &mvccpb.KeyValue{Key: []byte("/cluster-a/logic/3"), Value: []byte(value)},
		},
		{
			Type: clientv3.EventTypeDelete,
			Kv:   &mvccpb.KeyValue{Key: []byte("/cluster-a/logic/4")},
		},
	})

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != EventTypePut || events[0].Key != "/cluster-a/logic/3" ||
		string(events[0].Value) != value || events[0].Inst == nil || !events[0].Inst.Equal(inst) {
		t.Fatalf("unexpected put event: %#v", events[0])
	}
	if events[1].Type != EventTypeDelete || events[1].Key != "/cluster-a/logic/4" ||
		events[1].Value != nil || events[1].Inst != nil {
		t.Fatalf("unexpected delete event: %#v", events[1])
	}
}

func TestWatchResponseErrorHandlesCanceledResponses(t *testing.T) {
	if err := watchResponseError(clientv3.WatchResponse{}); err != nil {
		t.Fatalf("expected normal watch response to have no error, got %v", err)
	}

	err := watchResponseError(clientv3.WatchResponse{Canceled: true, CompactRevision: 12})
	if err == nil || !strings.Contains(err.Error(), "compacted") || !strings.Contains(err.Error(), "12") {
		t.Fatalf("expected compacted error with revision, got %v", err)
	}

	err = watchResponseError(clientv3.WatchResponse{Canceled: true})
	if err == nil || !strings.Contains(err.Error(), "watch canceled") {
		t.Fatalf("expected canceled watch error, got %v", err)
	}
}

func TestNewClientWatcherAndRegistryDiscoveryEdgeCases(t *testing.T) {
	if _, err := NewClientWatcher(nil, "/logic/", context.Background(), clientv3.WithPrefix()); err == nil {
		t.Fatal("expected NewClientWatcher to reject nil client")
	}

	reg := NewRegistry(nil, true)
	if _, err := reg.GetService(context.Background(), "test", "logic"); err == nil {
		t.Fatal("expected GetService to reject nil client")
	}
	if _, err := reg.Watch(context.Background(), "test", "logic"); err == nil {
		t.Fatal("expected Watch to reject nil client")
	}
	if err := reg.Register(context.Background(), nil); err == nil {
		t.Fatal("expected Register to reject nil service")
	}
	if err := reg.Deregister(context.Background(), nil); err == nil {
		t.Fatal("expected Deregister to reject nil service")
	}

	regNoRegister := NewRegistry(nil, false)
	if err := regNoRegister.Register(context.Background(), nil); err != nil {
		t.Fatalf("expected no-op Register when regist=false, got %v", err)
	}
	if err := regNoRegister.Deregister(context.Background(), nil); err != nil {
		t.Fatalf("expected no-op Deregister when regist=false, got %v", err)
	}
}

func TestNewEtcdClientReturnsContextOnStartupFailure(t *testing.T) {
	logger := xlog.NewMyLogger(t.TempDir()+"/etcd_unit.log", "INFO", 0)
	defer logger.Close()

	_, err := NewEtcdClient("etcd://127.0.0.1:1?dialTimeout=100ms", logger)
	if err == nil {
		t.Fatal("expected NewEtcdClient to fail against an unavailable endpoint")
	}
	if !strings.Contains(err.Error(), "etcd ") || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("expected startup failure to include etcd context, got %v", err)
	}
}

func TestNewEtcdClientWithLocalEtcd(t *testing.T) {
	requireEtcd(t)

	logger := xlog.NewMyLogger(t.TempDir()+"/etcd_local.log", "INFO", 0)
	defer logger.Close()

	client, err := NewEtcdClient("etcd://localhost:2379?dialTimeout=1s", logger)
	if err != nil {
		t.Fatalf("NewEtcdClient returned error: %v", err)
	}
	defer client.Close()

	if len(client.Config.Endpoints) != 1 || client.Config.Endpoints[0] != "localhost:2379" {
		t.Fatalf("unexpected endpoints: %v", client.Config.Endpoints)
	}
	if err := client.Ping(); err != nil {
		t.Fatalf("Ping returned error: %v", err)
	}
}
