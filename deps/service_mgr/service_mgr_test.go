package servicemgr

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestManager(t testing.TB) *Manager {
	t.Helper()

	sm, err := newManager(&ServiceInstance{
		ClusterName: "test",
		ServiceName: "gate",
		InstanceId:  1,
		Healthy:     ServiceStatusHealth,
		Enable:      true,
	}, nil, nil, false)
	if err != nil {
		t.Fatalf("newManager failed: %v", err)
	}
	return sm
}

func TestEnsureServiceSingleton(t *testing.T) {
	sm := newTestManager(t)
	const serviceName = "logic"
	const goroutines = 32

	var wg sync.WaitGroup
	wg.Add(goroutines)
	results := make(chan *Service, goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			results <- sm.ensureService(serviceName)
		}()
	}

	wg.Wait()
	close(results)

	var first *Service
	for svc := range results {
		if svc == nil {
			t.Fatalf("ensureService returned nil")
		}
		if first == nil {
			first = svc
			continue
		}
		if first != svc {
			t.Fatalf("ensureService returned different instances: %p vs %p", first, svc)
		}
	}

	if _, ok := sm.services.Get(serviceName); !ok {
		t.Fatalf("service %s not stored in shardmap", serviceName)
	}
}

func TestAddGetAndRemoveServiceInstance(t *testing.T) {
	sm := newTestManager(t)
	inst := &ServiceInstance{
		InstanceId:  1,
		ServiceName: "logic",
		Enable:      true,
		Healthy:     "health",
		NetStatus:   NetConnect,
	}

	sm.addServiceInstance(inst.ServiceName, inst)
	got, err := sm.Get(inst.ServiceName, inst.InstanceId)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != inst {
		t.Fatalf("got %+v, want %+v", got, inst)
	}

	removed := sm.removeServiceInstance(inst.ServiceName, inst.InstanceId)
	if removed != inst {
		t.Fatalf("removeServiceInstance returned %+v, want %+v", removed, inst)
	}

	if _, err := sm.Get(inst.ServiceName, inst.InstanceId); err == nil {
		t.Fatalf("expected error after removing instance")
	}
}

func TestUpdateLoads(t *testing.T) {
	sm := newTestManager(t)
	inst := &ServiceInstance{
		InstanceId:   10,
		ServiceName:  "logic",
		Enable:       true,
		Healthy:      "health",
		NetStatus:    NetConnect,
		OnlineCount_: 5,
		Load_:        8,
	}

	sm.addServiceInstance(inst.ServiceName, inst)

	updates := []*ServiceInstance{
		{
			InstanceId:   inst.InstanceId,
			ServiceName:  inst.ServiceName,
			OnlineCount_: 15,
			Load_:        20,
		},
	}

	if err := sm.UpdateLoads(updates); err != nil {
		t.Fatalf("UpdateLoads failed: %v", err)
	}

	if got := atomic.LoadInt32(&inst.OnlineCount_); got != 15 {
		t.Fatalf("OnlineCount = %d, want 15", got)
	}
	if got := atomic.LoadInt32(&inst.Load_); got != 20 {
		t.Fatalf("BattleLoad = %d, want 20", got)
	}
}

func TestUpdateLoadsAcrossServices(t *testing.T) {
	sm := newTestManager(t)

	logic := &ServiceInstance{
		InstanceId:   10,
		ServiceName:  "logic",
		Enable:       true,
		Healthy:      ServiceStatusHealth,
		NetStatus:    NetConnect,
		OnlineCount_: 5,
		Load_:        8,
	}
	battle := &ServiceInstance{
		InstanceId:   20,
		ServiceName:  "battle",
		Enable:       true,
		Healthy:      ServiceStatusHealth,
		NetStatus:    NetConnect,
		OnlineCount_: 7,
		Load_:        9,
	}
	sm.addServiceInstance(logic.ServiceName, logic)
	sm.addServiceInstance(battle.ServiceName, battle)

	if err := sm.UpdateLoads([]*ServiceInstance{
		{
			InstanceId:   battle.InstanceId,
			ServiceName:  battle.ServiceName,
			OnlineCount_: 70,
			Load_:        90,
		},
		{
			InstanceId:   logic.InstanceId,
			ServiceName:  logic.ServiceName,
			OnlineCount_: 50,
			Load_:        80,
		},
	}); err != nil {
		t.Fatalf("UpdateLoads failed: %v", err)
	}

	if got := battle.OnlineCount(); got != 70 {
		t.Fatalf("battle OnlineCount = %d, want 70", got)
	}
	if got := battle.Load(); got != 90 {
		t.Fatalf("battle Load = %d, want 90", got)
	}
	if got := logic.OnlineCount(); got != 50 {
		t.Fatalf("logic OnlineCount = %d, want 50", got)
	}
	if got := logic.Load(); got != 80 {
		t.Fatalf("logic Load = %d, want 80", got)
	}
}

func TestManagerConcurrentAccess(t *testing.T) {
	sm := newTestManager(t)
	const (
		serviceName    = "logic"
		maxInstances   = 64
		writerCount    = 8
		readerCount    = 4
		updateRounds   = 500
		addRemoveCount = 200
	)

	expected := make([]int32, maxInstances)
	active := make([]bool, maxInstances)
	for i := range maxInstances {
		sm.addServiceInstance(serviceName, &ServiceInstance{
			InstanceId:  int32(i + 1),
			ServiceName: serviceName,
			Enable:      true,
			Healthy:     "health",
			NetStatus:   NetConnect,
		})
		active[i] = true
	}

	var writerWG sync.WaitGroup
	for i := range writerCount {
		writerWG.Add(1)
		go func(id int) {
			defer writerWG.Done()
			localRand := rand.New(rand.NewSource(int64(id + 1)))
			for round := range updateRounds {
				actualIdx := localRand.Intn(maxInstances)
				instID := int32(actualIdx + 1)
				upd := &ServiceInstance{
					InstanceId:   instID,
					ServiceName:  serviceName,
					OnlineCount_: int32(round + 1),
					Load_:        int32(round+1) * 2,
				}
				_ = sm.UpdateLoads([]*ServiceInstance{upd})
				atomic.StoreInt32(&expected[actualIdx], int32(round+1))
			}
		}(i)
	}

	var readerWG sync.WaitGroup
	for range readerCount {
		readerWG.Go(func() {
			for range updateRounds {
				_ = sm.List(serviceName, nil)
			}
		})
	}

	var activeMu sync.Mutex
	var addRemoveWG sync.WaitGroup
	addRemoveWG.Go(func() {
		localRand := rand.New(rand.NewSource(1024))
		for range addRemoveCount {
			instID := int32(localRand.Intn(maxInstances) + 1)
			idx := instID - 1
			activeMu.Lock()
			if active[idx] {
				sm.removeServiceInstance(serviceName, instID)
				active[idx] = false
			} else {
				sm.addServiceInstance(serviceName, &ServiceInstance{
					InstanceId:  instID,
					ServiceName: serviceName,
					Enable:      true,
					Healthy:     "health",
					NetStatus:   NetConnect,
				})
				active[idx] = true
				atomic.StoreInt32(&expected[idx], 0)
			}
			activeMu.Unlock()
		}
	})

	writerWG.Wait()
	readerWG.Wait()
	addRemoveWG.Wait()

	for i := range maxInstances {
		activeMu.Lock()
		isActive := active[i]
		activeMu.Unlock()

		inst, err := sm.Get(serviceName, int32(i+1))
		if !isActive {
			if err == nil {
				t.Fatalf("expected instance %d to be removed", i+1)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		want := atomic.LoadInt32(&expected[i])
		if got := atomic.LoadInt32(&inst.OnlineCount_); got != want {
			t.Fatalf("instance %d OnlineCount = %d, want %d", inst.InstanceId, got, want)
		}
		if got := atomic.LoadInt32(&inst.Load_); got != want*2 {
			t.Fatalf("instance %d BattleLoad = %d, want %d", inst.InstanceId, got, want*2)
		}
	}
}

func TestServiceCOWSnapshots(t *testing.T) {
	service := newService("logic")
	const instanceCount = 32
	for i := range instanceCount {
		service.setInstance(&ServiceInstance{
			InstanceId:  int32(i + 1),
			ServiceName: "logic",
			Enable:      true,
			Healthy:     "health",
			NetStatus:   NetConnect,
		})
	}

	var wg sync.WaitGroup
	const readers = 8
	wg.Add(readers)
	errCh := make(chan error, readers)

	for range readers {
		go func() {
			defer wg.Done()
			for range 200 {
				instances := service.snapshot(nil)
				if len(instances) > instanceCount {
					errCh <- fmt.Errorf("snapshot length exceeded: %d", len(instances))
					return
				}
				for _, inst := range instances {
					if inst == nil {
						errCh <- fmt.Errorf("snapshot contains nil instance")
						return
					}
				}
			}
		}()
	}

	for i := range instanceCount {
		service.setInstance(&ServiceInstance{
			InstanceId:  int32(i + 1),
			ServiceName: "logic",
			Enable:      true,
			Healthy:     "health",
			NetStatus:   NetConnect,
		})
	}
	for i := range instanceCount / 2 {
		service.removeInstance(int32(i + 1))
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestPickByHash(t *testing.T) {
	sm := newTestManager(t)
	const serviceName = "logic"
	instances := []*ServiceInstance{
		{InstanceId: 1, ServiceName: serviceName, Enable: true, Healthy: ServiceStatusHealth, NetStatus: NetConnect},
		{InstanceId: 2, ServiceName: serviceName, Enable: true, Healthy: ServiceStatusHealth, NetStatus: NetConnect},
		{InstanceId: 3, ServiceName: serviceName, Enable: true, Healthy: ServiceStatusHealth, NetStatus: NetConnect},
	}
	for _, inst := range instances {
		sm.addServiceInstance(serviceName, inst)
	}

	keyStr := "user-key"
	inst1, err := sm.PickByHash(serviceName, keyStr)
	if err != nil || inst1 == nil {
		t.Fatalf("PickByHash failed for string key: %v", err)
	}
	inst2, err := sm.PickByHash(serviceName, keyStr)
	if err != nil || inst2 == nil {
		t.Fatalf("PickByHash failed for string key (second call): %v", err)
	}
	if inst1.InstanceId != inst2.InstanceId {
		t.Fatalf("expected stable hash mapping, got %d then %d", inst1.InstanceId, inst2.InstanceId)
	}

	keyInt := int64(42)
	inst3, err := sm.PickByHash(serviceName, keyInt)
	if err != nil || inst3 == nil {
		t.Fatalf("PickByHash failed for int64 key: %v", err)
	}

	if _, err := sm.PickByHash(serviceName, true); err == nil {
		t.Fatalf("expected error for unsupported key type")
	}
}

func TestListNilFilterReusesPublishedSlice(t *testing.T) {
	sm := newTestManager(t)
	const serviceName = "logic"
	instances := []*ServiceInstance{
		{InstanceId: 1, ServiceName: serviceName, Enable: true, Healthy: ServiceStatusHealth, NetStatus: NetConnect},
		{InstanceId: 2, ServiceName: serviceName, Enable: true, Healthy: ServiceStatusHealth, NetStatus: NetConnect},
	}
	for _, inst := range instances {
		sm.addServiceInstance(serviceName, inst)
	}

	svc, ok := sm.getService(serviceName)
	if !ok {
		t.Fatalf("getService(%s) failed", serviceName)
	}

	wantPtr := reflect.ValueOf(svc.snapshot(nil)).Pointer()
	got := sm.List(serviceName, nil)
	if reflect.ValueOf(got).Pointer() != wantPtr {
		t.Fatalf("List(nil) did not reuse the published slice")
	}
}

type testWatchDiscovery struct {
	ch chan *WatchEvent
}

func (d *testWatchDiscovery) Watch(ctx context.Context, servicesToWatch ...ListenSpec) (<-chan *WatchEvent, error) {
	go func() {
		<-ctx.Done()
		close(d.ch)
	}()
	return d.ch, nil
}

func (d *testWatchDiscovery) Close() {}

type testRegistry struct{}

func (testRegistry) Register() error                    { return nil }
func (testRegistry) Update(inst *ServiceInstance) error { return nil }
func (testRegistry) Close()                             {}

type captureRegistry struct {
	updated *ServiceInstance
}

func (captureRegistry) Register() error { return nil }
func (r *captureRegistry) Update(inst *ServiceInstance) error {
	r.updated = inst.Copy()
	return nil
}
func (captureRegistry) Close() {}

type failThenWatchDiscovery struct {
	listenCalls atomic.Int32
	ch          chan *WatchEvent
}

func (d *failThenWatchDiscovery) Watch(ctx context.Context, servicesToWatch ...ListenSpec) (<-chan *WatchEvent, error) {
	if d.listenCalls.Add(1) == 1 {
		return nil, fmt.Errorf("listen failed")
	}

	go func() {
		<-ctx.Done()
		close(d.ch)
	}()
	return d.ch, nil
}

func (d *failThenWatchDiscovery) Close() {}

type watchSession struct {
	services []string
	ch       chan *WatchEvent
	canceled atomic.Bool
}

type scriptedWatchDiscovery struct {
	mu        sync.Mutex
	callCount int
	failCalls map[int]error
	sessions  []*watchSession
}

func newScriptedWatchDiscovery(failCalls map[int]error) *scriptedWatchDiscovery {
	if failCalls == nil {
		failCalls = make(map[int]error)
	}
	return &scriptedWatchDiscovery{failCalls: failCalls}
}

func (d *scriptedWatchDiscovery) Watch(ctx context.Context, servicesToWatch ...ListenSpec) (<-chan *WatchEvent, error) {
	d.mu.Lock()
	d.callCount++
	callNo := d.callCount
	if err := d.failCalls[callNo]; err != nil {
		d.mu.Unlock()
		return nil, err
	}

	services := make([]string, 0, len(servicesToWatch))
	for _, li := range servicesToWatch {
		services = append(services, li.ServiceName)
	}

	session := &watchSession{
		services: services,
		ch:       make(chan *WatchEvent, 8),
	}
	d.sessions = append(d.sessions, session)
	d.mu.Unlock()

	go func() {
		<-ctx.Done()
		session.canceled.Store(true)
		close(session.ch)
	}()

	return session.ch, nil
}

func (d *scriptedWatchDiscovery) Close() {}

func (d *scriptedWatchDiscovery) Session(idx int) *watchSession {
	d.mu.Lock()
	defer d.mu.Unlock()

	if idx < 0 || idx >= len(d.sessions) {
		return nil
	}
	return d.sessions[idx]
}

func waitForOnlineInstance(t *testing.T, ch <-chan int32, want int32) {
	t.Helper()

	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("expected instance %d, got %d", want, got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for instance %d", want)
	}
}

func waitForCondition(t *testing.T, cond func() bool, timeout time.Duration, msg string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(msg)
}

func TestWatchContinuesAfterHandlerPanic(t *testing.T) {
	discovery := &testWatchDiscovery{ch: make(chan *WatchEvent, 4)}
	sm, err := newManager(&ServiceInstance{
		ClusterName: "test",
		ServiceName: "gate",
		InstanceId:  1,
		Healthy:     ServiceStatusHealth,
	}, testRegistry{}, discovery, false)
	if err != nil {
		t.Fatalf("newManager failed: %v", err)
	}
	defer sm.Close()

	var onlineCalls atomic.Int32
	processedSecond := make(chan struct{}, 1)
	handler := HandlerFunc{
		OnlineFn: func(serviceName string, instance *ServiceInstance) error {
			if onlineCalls.Add(1) == 1 {
				panic("boom")
			}
			select {
			case processedSecond <- struct{}{}:
			default:
			}
			return nil
		},
	}

	if err := sm.Watch(ListenSpec{
		Cluster:     "test",
		ServiceName: "logic",
		Handler:     handler,
	}); err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	discovery.ch <- &WatchEvent{
		Type:        EventTypePut,
		ServiceName: "logic",
		InstanceID:  101,
		Instance: &ServiceInstance{
			InstanceId:  101,
			ServiceName: "logic",
			Healthy:     ServiceStatusHealth,
			Enable:      true,
		},
	}
	discovery.ch <- &WatchEvent{
		Type:        EventTypePut,
		ServiceName: "logic",
		InstanceID:  102,
		Instance: &ServiceInstance{
			InstanceId:  102,
			ServiceName: "logic",
			Healthy:     ServiceStatusHealth,
			Enable:      true,
		},
	}

	select {
	case <-processedSecond:
	case <-time.After(2 * time.Second):
		t.Fatalf("second watch event was not processed after handler panic, calls=%d", onlineCalls.Load())
	}
}

func TestWatchCanRetryAfterStartFailure(t *testing.T) {
	discovery := &failThenWatchDiscovery{ch: make(chan *WatchEvent, 1)}
	sm, err := newManager(&ServiceInstance{
		ClusterName: "test",
		ServiceName: "gate",
		InstanceId:  1,
		Healthy:     ServiceStatusHealth,
	}, testRegistry{}, discovery, false)
	if err != nil {
		t.Fatalf("newManager failed: %v", err)
	}
	defer sm.Close()

	listenInfo := ListenSpec{
		Cluster:     "test",
		ServiceName: "logic",
		Handler:     HandlerFunc{},
	}

	if err := sm.Watch(listenInfo); err == nil {
		t.Fatal("expected first Watch to fail")
	}

	if err := sm.Watch(listenInfo); err != nil {
		t.Fatalf("expected retry Watch to succeed, got %v", err)
	}
}

func TestWatchDuplicateInBatchRollsBackNewServices(t *testing.T) {
	discovery := newScriptedWatchDiscovery(nil)
	sm, err := newManager(&ServiceInstance{
		ClusterName: "test",
		ServiceName: "gate",
		InstanceId:  1,
		Healthy:     ServiceStatusHealth,
	}, testRegistry{}, discovery, false)
	if err != nil {
		t.Fatalf("newManager failed: %v", err)
	}
	defer sm.Close()

	err = sm.Watch(
		ListenSpec{Cluster: "test", ServiceName: "logic", Handler: HandlerFunc{}},
		ListenSpec{Cluster: "test", ServiceName: "logic", Handler: HandlerFunc{}},
	)
	if err == nil {
		t.Fatal("expected duplicate Watch batch to fail")
	}
	if _, ok := sm.listenInfoMap["logic"]; ok {
		t.Fatal("expected duplicate Watch batch to roll back logic listener")
	}
	if discovery.Session(0) != nil {
		t.Fatal("duplicate Watch batch should fail before starting watcher")
	}
}

func TestWatchedEventCreatesServiceOnFirstSnapshot(t *testing.T) {
	discovery := newScriptedWatchDiscovery(nil)
	sm, err := newManager(&ServiceInstance{
		ClusterName: "test",
		ServiceName: "gate",
		InstanceId:  1,
		Healthy:     ServiceStatusHealth,
	}, testRegistry{}, discovery, false)
	if err != nil {
		t.Fatalf("newManager failed: %v", err)
	}
	defer sm.Close()

	if err := sm.Watch(ListenSpec{
		Cluster:     "test",
		ServiceName: "logic",
		Handler:     HandlerFunc{},
	}); err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	if _, ok := sm.getService("logic"); ok {
		t.Fatal("Watch should not create service cache before first event")
	}

	sm.updateAndTriggerInstance(&WatchEvent{
		Type:        EventTypeSnapshot,
		ServiceName: "logic",
		InstanceID:  1,
		Instance: &ServiceInstance{
			InstanceId:  1,
			ServiceName: "logic",
			Healthy:     ServiceStatusHealth,
			Enable:      true,
		},
	})

	if _, ok := sm.getService("logic"); !ok {
		t.Fatal("expected watched snapshot to create service cache")
	}
}

func TestUnwatchedEventDoesNotCreateService(t *testing.T) {
	sm := newTestManager(t)

	sm.updateAndTriggerInstance(&WatchEvent{
		Type:        EventTypePut,
		ServiceName: "logic",
		InstanceID:  1,
		Instance: &ServiceInstance{
			InstanceId:  1,
			ServiceName: "logic",
			Healthy:     ServiceStatusHealth,
			Enable:      true,
		},
	})

	if _, ok := sm.getService("logic"); ok {
		t.Fatal("unwatched event should not create service cache")
	}
}

func TestWatchFailureKeepsExistingWatcherActive(t *testing.T) {
	discovery := newScriptedWatchDiscovery(map[int]error{
		2: fmt.Errorf("listen failed"),
	})
	sm, err := newManager(&ServiceInstance{
		ClusterName: "test",
		ServiceName: "gate",
		InstanceId:  1,
		Healthy:     ServiceStatusHealth,
	}, testRegistry{}, discovery, false)
	if err != nil {
		t.Fatalf("newManager failed: %v", err)
	}
	defer sm.Close()

	logicEvents := make(chan int32, 2)
	logicHandler := HandlerFunc{
		OnlineFn: func(serviceName string, instance *ServiceInstance) error {
			logicEvents <- instance.InstanceId
			return nil
		},
	}

	if err := sm.Watch(ListenSpec{
		Cluster:     "test",
		ServiceName: "logic",
		Handler:     logicHandler,
	}); err != nil {
		t.Fatalf("Watch logic failed: %v", err)
	}

	firstSession := discovery.Session(0)
	if firstSession == nil {
		t.Fatal("expected first watcher session")
	}

	firstSession.ch <- &WatchEvent{
		Type:        EventTypePut,
		ServiceName: "logic",
		InstanceID:  101,
		Instance: &ServiceInstance{
			InstanceId:  101,
			ServiceName: "logic",
			Healthy:     ServiceStatusHealth,
			Enable:      true,
		},
	}
	waitForOnlineInstance(t, logicEvents, 101)

	if err := sm.Watch(ListenSpec{
		Cluster:     "test",
		ServiceName: "public",
		Handler:     HandlerFunc{},
	}); err == nil {
		t.Fatal("expected Watch public to fail")
	}

	if _, ok := sm.listenInfoMap["logic"]; !ok {
		t.Fatal("expected existing logic listener to remain registered")
	}
	if _, ok := sm.listenInfoMap["public"]; ok {
		t.Fatal("expected failed public listener to be rolled back")
	}
	if firstSession.canceled.Load() {
		t.Fatal("expected existing watcher to remain active after failed restart")
	}

	firstSession.ch <- &WatchEvent{
		Type:        EventTypePut,
		ServiceName: "logic",
		InstanceID:  102,
		Instance: &ServiceInstance{
			InstanceId:  102,
			ServiceName: "logic",
			Healthy:     ServiceStatusHealth,
			Enable:      true,
		},
	}
	waitForOnlineInstance(t, logicEvents, 102)
}

func TestWatchRestartsWatcherWithFullServiceSet(t *testing.T) {
	discovery := newScriptedWatchDiscovery(nil)
	sm, err := newManager(&ServiceInstance{
		ClusterName: "test",
		ServiceName: "gate",
		InstanceId:  1,
		Healthy:     ServiceStatusHealth,
	}, testRegistry{}, discovery, false)
	if err != nil {
		t.Fatalf("newManager failed: %v", err)
	}
	defer sm.Close()

	logicEvents := make(chan int32, 1)
	publicEvents := make(chan int32, 1)

	if err := sm.Watch(ListenSpec{
		Cluster:     "test",
		ServiceName: "logic",
		Handler: HandlerFunc{OnlineFn: func(serviceName string, instance *ServiceInstance) error {
			logicEvents <- instance.InstanceId
			return nil
		}},
	}); err != nil {
		t.Fatalf("Watch logic failed: %v", err)
	}

	firstSession := discovery.Session(0)
	if firstSession == nil {
		t.Fatal("expected first watcher session")
	}

	if err := sm.Watch(ListenSpec{
		Cluster:     "test",
		ServiceName: "public",
		Handler: HandlerFunc{OnlineFn: func(serviceName string, instance *ServiceInstance) error {
			publicEvents <- instance.InstanceId
			return nil
		}},
	}); err != nil {
		t.Fatalf("Watch public failed: %v", err)
	}

	secondSession := discovery.Session(1)
	if secondSession == nil {
		t.Fatal("expected second watcher session")
	}

	services := append([]string(nil), secondSession.services...)
	slices.Sort(services)
	if !slices.Equal(services, []string{"logic", "public"}) {
		t.Fatalf("expected second watcher to include logic and public, got %v", secondSession.services)
	}

	waitForCondition(t, func() bool {
		return firstSession.canceled.Load()
	}, 2*time.Second, "expected first watcher to be canceled after successful restart")

	secondSession.ch <- &WatchEvent{
		Type:        EventTypePut,
		ServiceName: "logic",
		InstanceID:  201,
		Instance: &ServiceInstance{
			InstanceId:  201,
			ServiceName: "logic",
			Healthy:     ServiceStatusHealth,
			Enable:      true,
		},
	}
	secondSession.ch <- &WatchEvent{
		Type:        EventTypePut,
		ServiceName: "public",
		InstanceID:  301,
		Instance: &ServiceInstance{
			InstanceId:  301,
			ServiceName: "public",
			Healthy:     ServiceStatusHealth,
			Enable:      true,
		},
	}

	waitForOnlineInstance(t, logicEvents, 201)
	waitForOnlineInstance(t, publicEvents, 301)
}

func TestWatchFailureRollsBackAllNewServices(t *testing.T) {
	discovery := newScriptedWatchDiscovery(map[int]error{
		2: fmt.Errorf("listen failed"),
	})
	sm, err := newManager(&ServiceInstance{
		ClusterName: "test",
		ServiceName: "gate",
		InstanceId:  1,
		Healthy:     ServiceStatusHealth,
	}, testRegistry{}, discovery, false)
	if err != nil {
		t.Fatalf("newManager failed: %v", err)
	}
	defer sm.Close()

	if err := sm.Watch(ListenSpec{
		Cluster:     "test",
		ServiceName: "logic",
		Handler:     HandlerFunc{},
	}); err != nil {
		t.Fatalf("Watch logic failed: %v", err)
	}

	if err := sm.Watch(
		ListenSpec{Cluster: "test", ServiceName: "public", Handler: HandlerFunc{}},
		ListenSpec{Cluster: "test", ServiceName: "battle", Handler: HandlerFunc{}},
	); err == nil {
		t.Fatal("expected Watch(public,battle) to fail")
	}

	services := make([]string, 0, len(sm.listenInfoMap))
	for serviceName := range sm.listenInfoMap {
		services = append(services, serviceName)
	}
	slices.Sort(services)
	if !slices.Equal(services, []string{"logic"}) {
		t.Fatalf("expected only logic listener after rollback, got %v", services)
	}

	if err := sm.Watch(
		ListenSpec{Cluster: "test", ServiceName: "public", Handler: HandlerFunc{}},
		ListenSpec{Cluster: "test", ServiceName: "battle", Handler: HandlerFunc{}},
	); err != nil {
		t.Fatalf("expected retry Watch(public,battle) to succeed, got %v", err)
	}

	lastSession := discovery.Session(1)
	if lastSession == nil {
		t.Fatal("expected watcher session after retry")
	}

	watched := append([]string(nil), lastSession.services...)
	slices.Sort(watched)
	if !slices.Equal(watched, []string{"battle", "logic", "public"}) {
		t.Fatalf("expected retry watcher to include full service set, got %v", lastSession.services)
	}
}

func TestUpdateServiceStateDoesNotExposeDisconnectedReplacement(t *testing.T) {
	sm := newTestManager(t)
	const serviceName = "logic"
	const instanceID = int32(1)

	sm.addServiceInstance(serviceName, &ServiceInstance{
		InstanceId:  instanceID,
		ServiceName: serviceName,
		Enable:      true,
		Healthy:     ServiceStatusHealth,
		NetStatus:   NetConnect,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var sawInvalid atomic.Bool
	var started sync.WaitGroup
	started.Add(2)

	go func() {
		started.Done()
		for ctx.Err() == nil {
			inst, err := sm.PickMinOnline(serviceName, true)
			if err != nil || inst == nil || inst.NetState() != NetConnect {
				sawInvalid.Store(true)
				cancel()
				return
			}
		}
	}()

	go func() {
		started.Done()
		for i := 0; i < 200000 && ctx.Err() == nil; i++ {
			sm.updateServiceState(EventTypePut, serviceName, instanceID, &ServiceInstance{
				InstanceId:  instanceID,
				ServiceName: serviceName,
				Enable:      true,
				Healthy:     ServiceStatusHealth,
			})
		}
		cancel()
	}()

	started.Wait()
	<-ctx.Done()

	if sawInvalid.Load() {
		t.Fatal("replacement instance was observed with invalid net status")
	}
}

func TestUpdateServiceStatePreservesLocalLoadOnReplacement(t *testing.T) {
	sm := newTestManager(t)
	const serviceName = "battle"
	const instanceID = int32(1)

	sm.addServiceInstance(serviceName, &ServiceInstance{
		InstanceId:  instanceID,
		ServiceName: serviceName,
		Enable:      true,
		Healthy:     ServiceStatusHealth,
		NetStatus:   NetConnect,
		Load_:       73,
	})

	sm.updateServiceState(EventTypePut, serviceName, instanceID, &ServiceInstance{
		InstanceId:  instanceID,
		ServiceName: serviceName,
		Enable:      true,
		Healthy:     ServiceStatusHealth,
	})

	inst, err := sm.Get(serviceName, instanceID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got := inst.Load(); got != 73 {
		t.Fatalf("Load = %d, want 73", got)
	}
}

func TestUpdateNetStatusRaceFreeWithPickMinOnline(t *testing.T) {
	sm := newTestManager(t)
	const serviceName = "logic"
	const instanceID = int32(1)

	sm.addServiceInstance(serviceName, &ServiceInstance{
		InstanceId:  instanceID,
		ServiceName: serviceName,
		Enable:      true,
		Healthy:     ServiceStatusHealth,
		NetStatus:   NetConnect,
	})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 10000 {
			_ = sm.UpdateNetStatus(serviceName, instanceID, NetUnValid)
			_ = sm.UpdateNetStatus(serviceName, instanceID, NetConnect)
		}
	}()
	go func() {
		defer wg.Done()
		for range 10000 {
			_, _ = sm.PickMinOnline(serviceName, true)
		}
	}()
	wg.Wait()
}

func TestManagerHealthAccessUsesInstanceLock(t *testing.T) {
	sm, err := newManager(&ServiceInstance{
		ClusterName: "test",
		ServiceName: "logic",
		InstanceId:  1,
		Enable:      true,
		Healthy:     ServiceStatusHealth,
	}, testRegistry{}, nil, false)
	if err != nil {
		t.Fatalf("newManager failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for range 1000 {
				if i == 0 {
					_ = sm.UpdateSelf(func(inst *ServiceInstance) {
						inst.Healthy = ServiceStatusGray
					})
					continue
				}
				_ = sm.SelfCopy().Healthy
			}
		}(i)
	}
	wg.Wait()
}

func TestUpdateSelfConfVersionSyncsRegistry(t *testing.T) {
	registry := &captureRegistry{}
	sm, err := newManager(&ServiceInstance{
		ClusterName: "test",
		ServiceName: "logic",
		InstanceId:  1,
		ProVersion:  1001,
		MetaData:    map[string]string{"zone": "1", "pro_version": "legacy", "conf_version": "legacy"},
	}, registry, nil, false)
	if err != nil {
		t.Fatalf("newManager failed: %v", err)
	}

	if err := sm.UpdateSelf(func(inst *ServiceInstance) {
		inst.ConfVersion = 2002
	}); err != nil {
		t.Fatalf("UpdateSelf failed: %v", err)
	}

	self := sm.SelfCopy()
	if self.ConfVersion != 2002 {
		t.Fatalf("ConfVersion = %d, want 2002", self.ConfVersion)
	}
	if self.MetaData["conf_version"] != "legacy" || self.MetaData["pro_version"] != "legacy" {
		t.Fatalf("instance metadata should be preserved: %#v", self.MetaData)
	}
	if registry.updated == nil {
		t.Fatal("expected registry update")
	}
	if registry.updated.ConfVersion != 2002 {
		t.Fatalf("registry update lost conf version: %#v", registry.updated)
	}
	if registry.updated.MetaData["conf_version"] != "legacy" || registry.updated.MetaData["pro_version"] != "legacy" {
		t.Fatalf("registry metadata should be preserved: %#v", registry.updated.MetaData)
	}
}

func TestManagerCloseCancelsActiveWatcher(t *testing.T) {
	discovery := newScriptedWatchDiscovery(nil)
	sm, err := newManager(&ServiceInstance{
		ClusterName: "test",
		ServiceName: "gate",
		InstanceId:  1,
		Healthy:     ServiceStatusHealth,
	}, testRegistry{}, discovery, false)
	if err != nil {
		t.Fatalf("newManager failed: %v", err)
	}

	if err := sm.Watch(ListenSpec{
		Cluster:     "test",
		ServiceName: "logic",
		Handler:     HandlerFunc{},
	}); err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	session := discovery.Session(0)
	if session == nil {
		t.Fatal("expected watcher session")
	}

	sm.Close()

	waitForCondition(t, func() bool {
		return session.canceled.Load()
	}, 2*time.Second, "expected watcher session to be canceled on Manager.Close")

	select {
	case _, ok := <-session.ch:
		if ok {
			t.Fatal("expected watcher session channel to close on Manager.Close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watcher session channel to close on Manager.Close")
	}
}
