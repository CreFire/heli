package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"game/deps/async"
	"game/deps/etcd"
	"game/deps/misc"
	"game/deps/mongoclient"
	"game/deps/netmgr"
	redisclient "game/deps/redis"
	rpcmgr "game/deps/rpc_mgr"
	servicemgr "game/deps/service_mgr"
	timermgr "game/deps/timer_mgr"
	"game/deps/xhttp"
	"game/src/configdoc"
	"game/src/proto/eventpb"

	"github.com/redis/go-redis/v9"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type stopTestService struct {
	server *Server
	t      *testing.T
}

func (s *stopTestService) OnInit() error {
	return nil
}

func (s *stopTestService) BeforeStart() error {
	return nil
}

func (s *stopTestService) OnStart() error {
	return nil
}

func (s *stopTestService) BeforeStop() error {
	s.assertBeforeStopPreconditions()
	return nil
}

func (s *stopTestService) OnStop() error {
	s.assertOnStopPreconditions()
	return nil
}

func (s *stopTestService) OnReload(oldCfg, newCfg *configdoc.ConfigBase) error {
	return nil
}

func (s *stopTestService) OnHeart(now int64) error {
	return nil
}

func (s *stopTestService) OnEventHandle(*eventpb.Event) {}

func (s *stopTestService) assertBeforeStopPreconditions() {
	if s.server.HttpServe.HttpServer != nil {
		s.t.Fatal("BeforeStop: http serve still running")
	}
	if s.server.BackendServe.HttpServer != nil {
		s.t.Fatal("BeforeStop: backend serve still running")
	}
	if !s.server.TimerMgr.IsRunning() {
		s.t.Fatal("BeforeStop: timer manager already stopped")
	}
	if _, err := s.server.TimerMgr.AddOneShotTimer("stop-test-before-stop", 1, false, func(string, int64, any) {}); err != nil {
		s.t.Fatalf("BeforeStop: timer manager should still accept timers, got err=%v", err)
	}
}

func (s *stopTestService) assertOnStopPreconditions() {
	if s.server.HttpServe.HttpServer != nil {
		s.t.Fatal("OnStop: http serve still running")
	}
	if s.server.BackendServe.HttpServer != nil {
		s.t.Fatal("OnStop: backend serve still running")
	}
	if s.server.TimerMgr.IsRunning() {
		s.t.Fatal("OnStop: timer manager still running")
	}
}

type stopErrorService struct {
	beforeStopErr error
	onStopErr     error
}

func (s *stopErrorService) OnInit() error {
	return nil
}

func (s *stopErrorService) BeforeStart() error {
	return nil
}

func (s *stopErrorService) OnStart() error {
	return nil
}

func (s *stopErrorService) BeforeStop() error {
	return s.beforeStopErr
}

func (s *stopErrorService) OnStop() error {
	return s.onStopErr
}

func (s *stopErrorService) OnReload(oldCfg, newCfg *configdoc.ConfigBase) error {
	return nil
}

func (s *stopErrorService) OnHeart(now int64) error {
	return nil
}

func (s *stopErrorService) OnEventHandle(*eventpb.Event) {}

type stopNoopService struct{}

func (s *stopNoopService) OnInit() error {
	return nil
}

func (s *stopNoopService) BeforeStart() error {
	return nil
}

func (s *stopNoopService) OnStart() error {
	return nil
}

func (s *stopNoopService) BeforeStop() error {
	return nil
}

func (s *stopNoopService) OnStop() error {
	return nil
}

func (s *stopNoopService) OnReload(oldCfg, newCfg *configdoc.ConfigBase) error {
	return nil
}

func (s *stopNoopService) OnHeart(now int64) error {
	return nil
}

func (s *stopNoopService) OnEventHandle(*eventpb.Event) {}

type reloadPanicService struct{}

func (s *reloadPanicService) OnInit() error { return nil }

func (s *reloadPanicService) BeforeStart() error { return nil }

func (s *reloadPanicService) OnStart() error { return nil }

func (s *reloadPanicService) BeforeStop() error { return nil }

func (s *reloadPanicService) OnStop() error { return nil }

func (s *reloadPanicService) OnReload(oldCfg, newCfg *configdoc.ConfigBase) error {
	panic("reload panic")
}

func (s *reloadPanicService) OnHeart(now int64) error { return nil }

func (s *reloadPanicService) OnEventHandle(*eventpb.Event) {}

type stopTestRegistry struct{}

func (stopTestRegistry) Register() error {
	return nil
}

func (stopTestRegistry) Update(*servicemgr.ServiceInstance) error {
	return nil
}

func (stopTestRegistry) Close() {}

type reloadCaptureRegistry struct {
	updated *servicemgr.ServiceInstance
}

func (reloadCaptureRegistry) Register() error {
	return nil
}

func (r *reloadCaptureRegistry) Update(inst *servicemgr.ServiceInstance) error {
	r.updated = inst.Copy()
	return nil
}

func (reloadCaptureRegistry) Close() {}

type stopTestDiscovery struct{}

func (stopTestDiscovery) Watch(ctx context.Context, _ ...servicemgr.ListenSpec) (<-chan *servicemgr.WatchEvent, error) {
	ch := make(chan *servicemgr.WatchEvent)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func (stopTestDiscovery) Close() {}

func TestServerStopKeepsTimerRunningUntilBeforeStop(t *testing.T) {
	server := newStopTestServer(t)
	server.GameService = &stopTestService{server: server, t: t}

	if err := server.stop(); err != nil {
		t.Fatalf("stop server: %v", err)
	}
}

func TestServerStopReturnsHookErrors(t *testing.T) {
	server := newStopTestServer(t)
	server.GameService = &stopErrorService{
		beforeStopErr: fmt.Errorf("before stop failed"),
		onStopErr:     fmt.Errorf("on stop failed"),
	}

	err := server.stop()
	if err == nil {
		t.Fatal("expected stop error, got nil")
	}
	if got := err.Error(); got != "BeforeStop: before stop failed\nOnStop: on stop failed" && got != "OnStop: on stop failed\nBeforeStop: before stop failed" {
		t.Fatalf("unexpected stop error: %v", err)
	}
}

func TestServerStopWaitsForStoppingBroadcast(t *testing.T) {
	server := newStopTestServer(t)
	server.GameService = &stopNoopService{}

	start := time.Now()
	if err := server.stop(); err != nil {
		t.Fatalf("stop server: %v", err)
	}
	if cost := time.Since(start); cost < 10*time.Millisecond {
		t.Fatalf("stop finished too fast, want at least 10ms, got %v", cost)
	}
}

func TestServerReloadKeepsOldConfigOnPanic(t *testing.T) {
	confPath := filepath.Join("..", "..", "conf", "logic.yaml")

	oldCfg := &configdoc.ConfigBase{}
	if err := oldCfg.LoadYamlConfig(confPath); err != nil {
		t.Fatalf("load yaml config: %v", err)
	}
	if err := oldCfg.LoadTableConfig(); err != nil {
		t.Fatalf("load table config: %v", err)
	}
	oldExcelVer := misc.ExcelVer
	misc.ExcelVer = "123"
	t.Cleanup(func() {
		misc.ExcelVer = oldExcelVer
	})

	server := &Server{
		ConfBase:    oldCfg,
		GameService: &reloadPanicService{},
		confPath:    confPath,
		quit:        make(chan struct{}),
	}

	err := server.Reload()
	if err == nil {
		t.Fatal("expected reload panic error, got nil")
	}
	if server.ConfBase != oldCfg {
		t.Fatal("expected old config to stay active after reload panic")
	}
	if misc.ExcelVer != "123" {
		t.Fatalf("expected misc.ExcelVer to stay unchanged after reload panic, got %q", misc.ExcelVer)
	}
}

func TestServerReloadSyncsExcelVersionToRuntimeAndRegistry(t *testing.T) {
	confPath := writeReloadTestConfig(t)
	oldCfg := &configdoc.ConfigBase{}
	if err := oldCfg.LoadYamlConfig(confPath); err != nil {
		t.Fatalf("load yaml config: %v", err)
	}
	if err := oldCfg.LoadTableConfig(); err != nil {
		t.Fatalf("load table config: %v", err)
	}

	oldExcelVer := misc.ExcelVer
	misc.ExcelVer = "123"
	t.Cleanup(func() {
		misc.ExcelVer = oldExcelVer
	})

	registry := &reloadCaptureRegistry{}
	svrMgr, err := servicemgr.NewWithComponents(&servicemgr.ServiceInstance{
		ClusterName: "test",
		ServiceName: "logic",
		InstanceId:  1,
		Enable:      true,
		Healthy:     servicemgr.ServiceStatusHealth,
		ConfVersion: 123,
		MetaData:    map[string]string{"zone": "1"},
	}, registry, stopTestDiscovery{})
	if err != nil {
		t.Fatalf("new service manager: %v", err)
	}

	server := &Server{
		ConfBase:    oldCfg,
		GameService: &stopNoopService{},
		SvrMgr:      svrMgr,
		confPath:    confPath,
		quit:        make(chan struct{}),
	}

	if err := server.Reload(); err != nil {
		t.Fatalf("reload server: %v", err)
	}

	want := int32(server.ConfBase.Doc.ExcelVersion)
	if got := misc.ExcelVer; got != fmt.Sprint(want) {
		t.Fatalf("misc.ExcelVer = %q, want %d", got, want)
	}
	if registry.updated == nil {
		t.Fatal("expected service registry update")
	}
	if registry.updated.ConfVersion != want {
		t.Fatalf("registry ConfVersion = %d, want %d", registry.updated.ConfVersion, want)
	}
	if _, ok := registry.updated.MetaData["conf_version"]; ok {
		t.Fatalf("registry metadata should not contain conf_version: %#v", registry.updated.MetaData)
	}
	if _, ok := registry.updated.MetaData["pro_version"]; ok {
		t.Fatalf("registry metadata should not contain pro_version: %#v", registry.updated.MetaData)
	}
}

func writeReloadTestConfig(t *testing.T) string {
	t.Helper()

	docconf, err := filepath.Abs(filepath.Join("..", "..", "docconf"))
	if err != nil {
		t.Fatalf("resolve docconf: %v", err)
	}

	dir := t.TempDir()
	confPath := filepath.Join(dir, "logic.yaml")
	if err := os.WriteFile(confPath, []byte(`
id: 201
type: "logic"
cluster: "test"
ip: "127.0.0.1"
port: 10201
asyncPoolSize: 1
aSyncQueueSize: 8
log:
  level: "info"
`), 0o644); err != nil {
		t.Fatalf("write logic.yaml: %v", err)
	}

	global := fmt.Sprintf(`
mongo:
  dsn: "mongodb://127.0.0.1:27017"
  dbName: "game"
redisDsn: "redis://127.0.0.1:6379/0"
etcdDsn: "etcd://127.0.0.1:2379"
gameDataPath: "%s"
bi:
  enabled: false
log:
  level: "info"
`, filepath.ToSlash(docconf))
	if err := os.WriteFile(filepath.Join(dir, "global.yaml"), []byte(global), 0o644); err != nil {
		t.Fatalf("write global.yaml: %v", err)
	}
	return confPath
}

func newStopTestServer(t *testing.T) *Server {
	t.Helper()

	asyncPool, err := async.NewAsync(1, 8)
	if err != nil {
		t.Fatalf("new async: %v", err)
	}

	timerMgr := timermgr.NewTimerMgr()
	if err := timerMgr.Start(); err != nil {
		t.Fatalf("start timer mgr: %v", err)
	}

	netMgr := netmgr.NewNetMgr()
	netMgr.Start()

	httpServe := xhttp.NewHttpServer()
	if err := httpServe.StartServe(":0"); err != nil {
		t.Fatalf("start http serve: %v", err)
	}

	backendServe := xhttp.NewHttpServer()
	if err := backendServe.StartServe(":0"); err != nil {
		t.Fatalf("start backend serve: %v", err)
	}

	svrMgr, err := servicemgr.NewWithComponents(&servicemgr.ServiceInstance{
		ClusterName: "test",
		ServiceName: "logic",
		InstanceId:  1,
		Enable:      true,
		Healthy:     servicemgr.ServiceStatusHealth,
	}, stopTestRegistry{}, stopTestDiscovery{})
	if err != nil {
		t.Fatalf("new service manager: %v", err)
	}

	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"127.0.0.1:1"},
		DialTimeout: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new etcd client: %v", err)
	}

	mongoClient, err := mongo.Connect(
		options.Client().
			ApplyURI("mongodb://127.0.0.1:1").
			SetConnectTimeout(time.Millisecond).
			SetServerSelectionTimeout(time.Millisecond),
	)
	if err != nil {
		t.Fatalf("new mongo client: %v", err)
	}

	server := &Server{
		SvrMgr:       svrMgr,
		TimerMgr:     timerMgr,
		Etcd:         &etcd.EtcdClient{Client: etcdClient},
		MongoDB:      &mongoclient.MongoClient{Clients: mongoClient},
		RedisDB:      &redisclient.RedisClient{Client: redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: time.Millisecond})},
		Async:        asyncPool,
		NetMgr:       netMgr,
		HttpServe:    httpServe,
		BackendServe: backendServe,
		Rpc:          rpcmgr.NewRpcMgr(netMgr),
		quit:         make(chan struct{}),
	}

	t.Cleanup(func() {
		if server.HttpServe != nil && server.HttpServe.HttpServer != nil {
			_ = server.HttpServe.StopServe()
		}
		if server.BackendServe != nil && server.BackendServe.HttpServer != nil {
			_ = server.BackendServe.StopServe()
		}
		if server.TimerMgr != nil {
			_ = server.TimerMgr.Stop()
		}
		if server.RedisDB != nil && server.RedisDB.Client != nil {
			_ = server.RedisDB.Close()
		}
		if server.MongoDB != nil && server.MongoDB.Clients != nil {
			server.MongoDB.Close()
		}
		if server.Etcd != nil && server.Etcd.Client != nil {
			server.Etcd.Close()
		}
	})
	return server
}
