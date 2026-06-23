package servicemgr

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type apiTestRegistrar struct {
	registerCalls atomic.Int32
	updateCalls   atomic.Int32
	updated       *ServiceInstance
}

func (r *apiTestRegistrar) Register() error {
	r.registerCalls.Add(1)
	return nil
}

func (r *apiTestRegistrar) Update(inst *ServiceInstance) error {
	r.updateCalls.Add(1)
	r.updated = inst.Copy()
	return nil
}

func (r *apiTestRegistrar) Close() {}

type apiTestWatcher struct {
	ch chan *WatchEvent
}

func (w *apiTestWatcher) Watch(ctx context.Context, specs ...ListenSpec) (<-chan *WatchEvent, error) {
	go func() {
		<-ctx.Done()
		close(w.ch)
	}()
	return w.ch, nil
}

func (w *apiTestWatcher) Close() {}

func TestNewWithComponentsRegistersImmediately(t *testing.T) {
	reg := &apiTestRegistrar{}
	mgr, err := NewWithComponents(&ServiceInstance{
		ClusterName: "test",
		ServiceName: "logic",
		InstanceId:  1,
		Healthy:     ServiceStatusHealth,
		Enable:      true,
	}, reg, &apiTestWatcher{ch: make(chan *WatchEvent, 1)})
	if err != nil {
		t.Fatalf("NewWithComponents failed: %v", err)
	}
	defer mgr.Close()

	if got := reg.registerCalls.Load(); got != 1 {
		t.Fatalf("register calls = %d, want 1", got)
	}
}

func TestNewUnregisteredRejectsNilEtcdClient(t *testing.T) {
	_, err := NewUnregistered(nil, nil)
	if err == nil {
		t.Fatal("expected nil input error")
	}
}

func TestWatchAppliesEventAndCallsHandler(t *testing.T) {
	watcher := &apiTestWatcher{ch: make(chan *WatchEvent, 1)}
	mgr, err := newManager(&ServiceInstance{
		ClusterName: "test",
		ServiceName: "gate",
		InstanceId:  1,
		Healthy:     ServiceStatusHealth,
		Enable:      true,
	}, &apiTestRegistrar{}, watcher, false)
	if err != nil {
		t.Fatalf("newManager failed: %v", err)
	}
	defer mgr.Close()

	online := make(chan int32, 1)
	if err := mgr.Watch(ListenSpec{
		Cluster:     "test",
		ServiceName: "logic",
		Handler: HandlerFunc{
			OnlineFn: func(serviceName string, instance *ServiceInstance) error {
				online <- instance.InstanceId
				return nil
			},
		},
	}); err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	watcher.ch <- &WatchEvent{
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

	select {
	case got := <-online:
		if got != 101 {
			t.Fatalf("online instance = %d, want 101", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for online callback")
	}

	inst, err := mgr.Get("logic", 101)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if inst.InstanceId != 101 {
		t.Fatalf("instance id = %d, want 101", inst.InstanceId)
	}
}

func TestWatchUpdateAndDeleteDriveHandlersAndState(t *testing.T) {
	watcher := &apiTestWatcher{ch: make(chan *WatchEvent, 3)}
	mgr, err := newManager(&ServiceInstance{
		ClusterName: "test",
		ServiceName: "gate",
		InstanceId:  1,
		Healthy:     ServiceStatusHealth,
		Enable:      true,
	}, &apiTestRegistrar{}, watcher, false)
	if err != nil {
		t.Fatalf("newManager failed: %v", err)
	}
	defer mgr.Close()

	updated := make(chan int32, 1)
	offline := make(chan int32, 1)
	if err := mgr.Watch(ListenSpec{
		Cluster:     "test",
		ServiceName: "logic",
		Handler: HandlerFunc{
			UpdateFn: func(serviceName string, instance *ServiceInstance) error {
				updated <- instance.InstanceId
				return nil
			},
			OfflineFn: func(serviceName string, instance *ServiceInstance) error {
				offline <- instance.InstanceId
				return nil
			},
		},
	}); err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	watcher.ch <- &WatchEvent{
		Type:        EventTypePut,
		ServiceName: "logic",
		InstanceID:  101,
		Instance: &ServiceInstance{
			InstanceId:  101,
			ServiceName: "logic",
			Healthy:     ServiceStatusHealth,
			Enable:      true,
			Host:        "127.0.0.1",
		},
	}
	watcher.ch <- &WatchEvent{
		Type:        EventTypePut,
		ServiceName: "logic",
		InstanceID:  101,
		Instance: &ServiceInstance{
			InstanceId:  101,
			ServiceName: "logic",
			Healthy:     ServiceStatusHealth,
			Enable:      true,
			Host:        "127.0.0.2",
		},
	}
	watcher.ch <- &WatchEvent{
		Type:        EventTypeDelete,
		ServiceName: "logic",
		InstanceID:  101,
	}

	select {
	case got := <-updated:
		if got != 101 {
			t.Fatalf("updated instance = %d, want 101", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for update callback")
	}

	select {
	case got := <-offline:
		if got != 101 {
			t.Fatalf("offline instance = %d, want 101", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for offline callback")
	}

	if _, err := mgr.Get("logic", 101); err == nil {
		t.Fatal("expected deleted instance to be removed from manager state")
	}
}

func TestUpdateSelfPublishesCopiedInstance(t *testing.T) {
	reg := &apiTestRegistrar{}
	mgr, err := newManager(&ServiceInstance{
		ClusterName: "test",
		ServiceName: "logic",
		InstanceId:  1,
		Healthy:     ServiceStatusHealth,
		Enable:      true,
	}, reg, &apiTestWatcher{ch: make(chan *WatchEvent, 1)}, false)
	if err != nil {
		t.Fatalf("newManager failed: %v", err)
	}
	defer mgr.Close()

	if err := mgr.UpdateSelf(func(inst *ServiceInstance) {
		inst.Healthy = ServiceStatusGray
		inst.ConfVersion = 2002
	}); err != nil {
		t.Fatalf("UpdateSelf failed: %v", err)
	}

	if got := reg.updateCalls.Load(); got != 1 {
		t.Fatalf("update calls = %d, want 1", got)
	}
	if reg.updated == nil {
		t.Fatal("expected updated instance copy")
	}
	if reg.updated.Healthy != ServiceStatusGray || reg.updated.ConfVersion != 2002 {
		t.Fatalf("unexpected updated instance: %#v", reg.updated)
	}
}

func TestUpdateSelfFailsWithoutRegistrar(t *testing.T) {
	mgr, err := newManager(&ServiceInstance{
		ClusterName: "test",
		ServiceName: "logic",
		InstanceId:  1,
		Healthy:     ServiceStatusHealth,
		Enable:      true,
	}, nil, nil, false)
	if err != nil {
		t.Fatalf("newManager failed: %v", err)
	}

	err = mgr.UpdateSelf(func(inst *ServiceInstance) {
		inst.Healthy = ServiceStatusStopping
	})
	if !errors.Is(err, errManagerRegistrarNil) {
		t.Fatalf("UpdateSelf error = %v, want %v", err, errManagerRegistrarNil)
	}
}
