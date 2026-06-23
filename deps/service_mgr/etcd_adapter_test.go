package servicemgr

import (
	"context"
	"encoding/json"
	"fmt"
	"game/deps/etcd"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestNewWatcherKeepsClient(t *testing.T) {
	client := &etcd.EtcdClient{}
	watcher := newWatcher(client)
	if watcher.client != client {
		t.Fatalf("client = %p, want %p", watcher.client, client)
	}
}

func TestConvertEventType(t *testing.T) {
	cases := map[etcd.EventType]EventType{
		etcd.EventTypePut:      EventTypePut,
		etcd.EventTypeDelete:   EventTypeDelete,
		etcd.EventTypeSnapshot: EventTypeSnapshot,
		etcd.EventType("junk"): EventTypePut,
	}

	for in, want := range cases {
		if got := convertEventType(in); got != want {
			t.Fatalf("convertEventType(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestWatcherDoesNotMixSimilarServicePrefixes(t *testing.T) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: time.Second,
	})
	if err != nil {
		t.Skipf("local etcd unavailable: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	_, err = client.Status(ctx, "localhost:2379")
	cancel()
	if err != nil {
		t.Skipf("local etcd unavailable: %v", err)
	}

	cluster := fmt.Sprintf("service-mgr-prefix-%d", time.Now().UnixNano())
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, _ = client.Delete(ctx, "/"+cluster+"/", clientv3.WithPrefix())
		cancel()
	}
	cleanup()
	defer cleanup()

	watcher := newWatcher(&etcd.EtcdClient{Client: client})
	defer watcher.Close()

	watchCtx, watchCancel := context.WithCancel(context.Background())
	defer watchCancel()

	events, err := watcher.Watch(watchCtx, ListenSpec{Cluster: cluster, ServiceName: "logic"})
	if err != nil {
		t.Fatalf("Watch returned error: %v", err)
	}

	putEtcdServiceInstance(t, client, &etcd.ServiceInstance{
		ClusterName: cluster,
		ServiceName: "logic2",
		InstanceId:  2,
		Host:        "127.0.0.1",
		Port:        9002,
	})

	select {
	case event := <-events:
		t.Fatalf("received event from unwatched service: %#v", event)
	case <-time.After(500 * time.Millisecond):
	}

	putEtcdServiceInstance(t, client, &etcd.ServiceInstance{
		ClusterName: cluster,
		ServiceName: "logic",
		InstanceId:  1,
		Host:        "127.0.0.1",
		Port:        9001,
	})

	select {
	case event := <-events:
		if event.ServiceName != "logic" || event.InstanceID != 1 {
			t.Fatalf("unexpected watched event: %#v", event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for watched logic event")
	}
}

func putEtcdServiceInstance(t *testing.T, client *clientv3.Client, inst *etcd.ServiceInstance) {
	t.Helper()

	data, err := json.Marshal(inst)
	if err != nil {
		t.Fatalf("marshal service instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	_, err = client.Put(ctx, etcd.RegistrarKey(inst.ClusterName, inst.ServiceName, inst.InstanceId), string(data))
	cancel()
	if err != nil {
		t.Fatalf("put service instance: %v", err)
	}
}
