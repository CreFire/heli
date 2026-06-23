package etcd

import (
	"context"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var (
	rawClient *clientv3.Client
)

// TestMain sets up the etcd client for all tests in this package.
func TestMain(m *testing.M) {
	// NOTE: This test requires an etcd server running on localhost:2379.
	endpoints := []string{"localhost:2379"}
	var err error

	// We need a logger for the watcher code.

	// Use clientv3.New directly to avoid dependency issues with xlog.
	rawClient, err = clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		log.Printf("Failed to create etcd client for testing: %v", err)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err = rawClient.Status(ctx, endpoints[0])
		cancel()
		if err != nil {
			log.Printf("etcd test server unavailable on %s: %v", endpoints[0], err)
			rawClient.Close()
			rawClient = nil
		}
	}

	// Run tests
	code := m.Run()

	// Teardown
	if rawClient != nil {
		rawClient.Close()
	}
	os.Exit(code)
}

// cleanupKeys deletes all keys with a given prefix.
func cleanupKeys(t *testing.T, prefix string) {
	t.Helper()
	_, err := rawClient.Delete(context.Background(), prefix, clientv3.WithPrefix())
	require.NoError(t, err, "Failed to clean up keys with prefix %s", prefix)
}

func requireEtcd(t *testing.T) {
	t.Helper()
	if rawClient == nil {
		t.Skip("etcd test client unavailable")
	}
}

func TestMultiWatcher_Lifecycle(t *testing.T) {
	requireEtcd(t)

	// Define prefixes for two different services
	prefix1 := "/test/multiwatch/service1/"
	prefix2 := "/test/multiwatch/service2/"
	keysToWatch := []string{prefix1, prefix2}

	// Ensure a clean state before the test
	cleanupKeys(t, "/test/multiwatch/")
	defer cleanupKeys(t, "/test/multiwatch/")

	// --- 1. Setup initial data ---
	initialData := map[string]string{
		prefix1 + "node1": watcherTestValue(t, "service1", 1),
		prefix1 + "node2": watcherTestValue(t, "service1", 2),
		prefix2 + "nodeA": watcherTestValue(t, "service2", 1),
	}
	for k, v := range initialData {
		_, err := rawClient.Put(context.Background(), k, v)
		require.NoError(t, err)
	}

	// --- 2. Create MultiWatcher ---
	ctx, cancel := context.WithCancel(context.Background())

	multiWatcher, err := NewMultiWatcher(rawClient, keysToWatch, ctx, clientv3.WithPrefix())
	require.NoError(t, err)
	require.NotNil(t, multiWatcher)

	eventChan := multiWatcher.EventChan()

	// --- 3. Verify SNAPSHOT events ---
	receivedSnapshots := make(map[string]string)
	waitForEvents(t, eventChan, 3*time.Second, func(event *EtcdWatchEvent) bool {
		if event.Type != EventTypeSnapshot {
			return false
		}
		receivedSnapshots[event.Key] = string(event.Value)
		return len(receivedSnapshots) == len(initialData)
	})
	assert.Equal(t, initialData, receivedSnapshots, "Snapshot data does not match initial data")

	// --- 4. Trigger live events ---
	putValue := watcherTestValue(t, "service1", 3)
	_, err = rawClient.Put(context.Background(), prefix1+"node3", putValue)
	require.NoError(t, err)
	waitForEvents(t, eventChan, 3*time.Second, func(event *EtcdWatchEvent) bool {
		return event.Type == EventTypePut && event.Key == prefix1+"node3" && string(event.Value) == putValue && event.Inst != nil
	})

	_, err = rawClient.Delete(context.Background(), prefix2+"nodeA")
	require.NoError(t, err)
	waitForEvents(t, eventChan, 3*time.Second, func(event *EtcdWatchEvent) bool {
		return event.Type == EventTypeDelete && event.Key == prefix2+"nodeA" && event.Value == nil
	})

	// --- 5. Close the watcher and wait for goroutine to finish ---
	multiWatcher.Close()
	cancel()
}

func TestNewMultiWatcher_EdgeCases(t *testing.T) {
	t.Run("NilClient", func(t *testing.T) {
		_, err := NewMultiWatcher(nil, []string{"/key"}, context.Background())
		require.Error(t, err)
		assert.Equal(t, "etcd client is nil", err.Error())
	})

	t.Run("NoKeys", func(t *testing.T) {
		_, err := NewMultiWatcher(&clientv3.Client{}, []string{}, context.Background())
		require.Error(t, err)
		assert.Equal(t, "keys cannot be empty", err.Error())
	})
}

func TestMultiWatcher_ContextCancellation(t *testing.T) {
	requireEtcd(t)

	prefix1 := "/test/cancel/service1/"
	keysToWatch := []string{prefix1}

	cleanupKeys(t, "/test/cancel/")
	defer cleanupKeys(t, "/test/cancel/")

	// Create a cancellable context
	pCtx, cancel := context.WithCancel(context.Background())

	multiWatcher, err := NewMultiWatcher(rawClient, keysToWatch, pCtx)
	require.NoError(t, err)
	require.NotNil(t, multiWatcher)

	eventChan := multiWatcher.EventChan()
	done := make(chan struct{})

	// Start a consumer goroutine
	go func() {
		for range eventChan {
			// We might receive a snapshot before cancellation, which is fine.
		}
		// When the channel is closed, this loop will exit.
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for event channel to close after context cancellation")
	}
}

func TestServiceRegistrar_RegistersUpdatesAndCloses(t *testing.T) {
	requireEtcd(t)

	prefix := "/test/registrar/"
	cleanupKeys(t, prefix)
	defer cleanupKeys(t, prefix)

	client := &EtcdClient{Client: rawClient}
	inst := testInstance("test", "registrar", 1)
	registrar := NewServiceRegistrar(client, inst.ClusterName, inst)

	require.NoError(t, registrar.Register())
	key := RegistrarKey(inst.ClusterName, inst.ServiceName, inst.InstanceId)
	assertStoredInstance(t, key, func(got *ServiceInstance) bool {
		return got != nil && got.InstanceId == inst.InstanceId && got.Enable
	})

	duplicate := NewServiceRegistrar(client, inst.ClusterName, testInstance("test", "registrar", 1))
	err := duplicate.Register()
	require.Error(t, err)
	want := "Etcd registration key: /test/registrar/1 already exists; refusing to start duplicate instance."
	if err.Error() != "initial registration failed: "+want {
		t.Fatalf("unexpected duplicate registration error: %v", err)
	}

	update := testInstance("test", "registrar", 1)
	update.Enable = false
	update.Healthy = "gray"
	update.OnlineCount = 7
	update.MetaData = map[string]string{"role": "battle"}
	require.NoError(t, registrar.UpdateInstanceData(update))
	assertStoredInstance(t, key, func(got *ServiceInstance) bool {
		if got == nil || got.Enable || got.Healthy != "gray" || got.OnlineCount != 7 {
			return false
		}
		if got.MetaData["role"] != "battle" {
			return false
		}
		_, hasZone := got.MetaData["zone"]
		return !hasZone
	})

	registrar.Close()
	assertKeyDeleted(t, key)
}

func TestServiceRegistrar_ReRegisterReplacesLeaseAndKeepsKey(t *testing.T) {
	requireEtcd(t)

	prefix := "/test/reregister/"
	cleanupKeys(t, prefix)
	defer cleanupKeys(t, prefix)

	client := &EtcdClient{Client: rawClient}
	inst := testInstance("test", "reregister", 1)
	registrar := NewServiceRegistrar(client, inst.ClusterName, inst)

	require.NoError(t, registrar.Register())
	key := RegistrarKey(inst.ClusterName, inst.ServiceName, inst.InstanceId)
	oldLease := clientv3.LeaseID(registrar.leaseID.Load())
	require.NotZero(t, oldLease)

	require.NoError(t, registrar.reRegister())
	newLease := clientv3.LeaseID(registrar.leaseID.Load())
	require.NotZero(t, newLease)
	require.NotEqual(t, oldLease, newLease)
	assertStoredInstance(t, key, func(got *ServiceInstance) bool {
		return got != nil && got.InstanceId == inst.InstanceId && got.Enable
	})

	registrar.Close()
	assertKeyDeleted(t, key)
}

func TestServiceRegistrar_UpdateRejectsStaleLeaseOwner(t *testing.T) {
	requireEtcd(t)

	prefix := "/test/stale-lease/"
	cleanupKeys(t, prefix)
	defer cleanupKeys(t, prefix)

	client := &EtcdClient{Client: rawClient}
	inst := testInstance("test", "stale-lease", 1)
	registrar := NewServiceRegistrar(client, inst.ClusterName, inst)

	require.NoError(t, registrar.Register())
	key := RegistrarKey(inst.ClusterName, inst.ServiceName, inst.InstanceId)
	oldLease := clientv3.LeaseID(registrar.leaseID.Load())
	require.NotZero(t, oldLease)

	replacementLease, err := rawClient.Grant(context.Background(), serviceLeaseTTL)
	require.NoError(t, err)
	defer rawClient.Revoke(context.Background(), replacementLease.ID)

	replacement := testInstance("test", "stale-lease", 1)
	replacement.Healthy = "replacement"
	replacementVal, err := marshal(replacement)
	require.NoError(t, err)
	_, err = rawClient.Put(context.Background(), key, replacementVal, clientv3.WithLease(replacementLease.ID))
	require.NoError(t, err)

	staleUpdate := testInstance("test", "stale-lease", 1)
	staleUpdate.Healthy = "stale"
	err = registrar.UpdateInstanceData(staleUpdate)
	require.Error(t, err)
	if !strings.Contains(err.Error(), "lease changed") {
		t.Fatalf("expected lease changed error, got %v", err)
	}

	resp, err := rawClient.Get(context.Background(), key)
	require.NoError(t, err)
	require.Len(t, resp.Kvs, 1)
	got, err := unmarshal(resp.Kvs[0].Value)
	require.NoError(t, err)
	require.Equal(t, "replacement", got.Healthy)
	require.Equal(t, int64(replacementLease.ID), resp.Kvs[0].Lease)

	registrar.Close()
}

func TestServiceRegistrar_PutServiceWithSameLeaseUsesTxnLeaseCompare(t *testing.T) {
	requireEtcd(t)

	prefix := "/test/txn-update/"
	cleanupKeys(t, prefix)
	defer cleanupKeys(t, prefix)

	client := &EtcdClient{Client: rawClient}
	inst := testInstance("test", "txn-update", 1)
	registrar := NewServiceRegistrar(client, inst.ClusterName, inst)
	require.NoError(t, registrar.Register())
	defer registrar.Close()

	key := RegistrarKey(inst.ClusterName, inst.ServiceName, inst.InstanceId)
	leaseID := clientv3.LeaseID(registrar.leaseID.Load())
	require.NotZero(t, leaseID)

	current := testInstance("test", "txn-update", 1)
	current.Healthy = "gray"
	current.MetaData = map[string]string{"role": "same-lease"}
	currentVal, err := marshal(current)
	require.NoError(t, err)
	require.NoError(t, registrar.putServiceWithSameLease(currentVal, leaseID))
	assertStoredInstance(t, key, func(got *ServiceInstance) bool {
		return got != nil && got.Healthy == "gray" && got.MetaData["role"] == "same-lease"
	})

	replacementLease, err := rawClient.Grant(context.Background(), serviceLeaseTTL)
	require.NoError(t, err)
	defer rawClient.Revoke(context.Background(), replacementLease.ID)

	replacement := testInstance("test", "txn-update", 1)
	replacement.Healthy = "replacement"
	replacement.MetaData = map[string]string{"role": "replacement"}
	replacementVal, err := marshal(replacement)
	require.NoError(t, err)
	_, err = rawClient.Put(context.Background(), key, replacementVal, clientv3.WithLease(replacementLease.ID))
	require.NoError(t, err)

	stale := testInstance("test", "txn-update", 1)
	stale.Healthy = "stale"
	stale.MetaData = map[string]string{"role": "stale"}
	staleVal, err := marshal(stale)
	require.NoError(t, err)

	err = registrar.putServiceWithSameLease(staleVal, leaseID)
	require.Error(t, err)
	require.ErrorIs(t, err, errLeaseChanged)

	resp, err := rawClient.Get(context.Background(), key)
	require.NoError(t, err)
	require.Len(t, resp.Kvs, 1)
	require.Equal(t, int64(replacementLease.ID), resp.Kvs[0].Lease)
	got, err := unmarshal(resp.Kvs[0].Value)
	require.NoError(t, err)
	require.Equal(t, "replacement", got.Healthy)
	require.Equal(t, "replacement", got.MetaData["role"])
}

func TestServiceRegistrar_PutMissingServiceUsesTxnCreateCompare(t *testing.T) {
	requireEtcd(t)

	prefix := "/test/txn-create/"
	cleanupKeys(t, prefix)
	defer cleanupKeys(t, prefix)

	client := &EtcdClient{Client: rawClient}
	inst := testInstance("test", "txn-create", 1)
	registrar := NewServiceRegistrar(client, inst.ClusterName, inst)
	defer registrar.Close()

	key := RegistrarKey(inst.ClusterName, inst.ServiceName, inst.InstanceId)

	firstLease, err := rawClient.Grant(context.Background(), serviceLeaseTTL)
	require.NoError(t, err)
	defer rawClient.Revoke(context.Background(), firstLease.ID)

	first := testInstance("test", "txn-create", 1)
	first.MetaData = map[string]string{"role": "first"}
	firstVal, err := marshal(first)
	require.NoError(t, err)
	require.NoError(t, registrar.putMissingService(firstVal, firstLease.ID))

	resp, err := rawClient.Get(context.Background(), key)
	require.NoError(t, err)
	require.Len(t, resp.Kvs, 1)
	require.Equal(t, int64(firstLease.ID), resp.Kvs[0].Lease)

	secondLease, err := rawClient.Grant(context.Background(), serviceLeaseTTL)
	require.NoError(t, err)
	defer rawClient.Revoke(context.Background(), secondLease.ID)

	second := testInstance("test", "txn-create", 1)
	second.Healthy = "second"
	second.MetaData = map[string]string{"role": "second"}
	secondVal, err := marshal(second)
	require.NoError(t, err)

	err = registrar.putMissingService(secondVal, secondLease.ID)
	require.Error(t, err)
	require.ErrorIs(t, err, errLeaseChanged)

	resp, err = rawClient.Get(context.Background(), key)
	require.NoError(t, err)
	require.Len(t, resp.Kvs, 1)
	require.Equal(t, int64(firstLease.ID), resp.Kvs[0].Lease)
	got, err := unmarshal(resp.Kvs[0].Value)
	require.NoError(t, err)
	require.Equal(t, "health", got.Healthy)
	require.Equal(t, "first", got.MetaData["role"])
}

func TestRegistryDiscovery_RegisterGetWatchAndDeregister(t *testing.T) {
	requireEtcd(t)

	prefix := "/test/registry/"
	cleanupKeys(t, prefix)
	defer cleanupKeys(t, prefix)

	reg := NewRegistry(&EtcdClient{Client: rawClient}, true)
	defer reg.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watchCh, err := reg.Watch(ctx, "test", "registry")
	require.NoError(t, err)

	inst := testInstance("test", "registry", 2)
	require.NoError(t, reg.Register(context.Background(), inst))

	got, err := reg.GetService(context.Background(), "test", "registry")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.True(t, got[0].Equal(inst), "unexpected service instance: %#v", got[0])

	waitForEvents(t, watchCh, 3*time.Second, func(event *EtcdWatchEvent) bool {
		return (event.Type == EventTypeSnapshot || event.Type == EventTypePut) &&
			event.Key == RegistrarKey(inst.ClusterName, inst.ServiceName, inst.InstanceId) &&
			event.Inst != nil &&
			event.Inst.InstanceId == inst.InstanceId
	})

	require.NoError(t, reg.Deregister(context.Background(), inst))
	waitForEvents(t, watchCh, 3*time.Second, func(event *EtcdWatchEvent) bool {
		return event.Type == EventTypeDelete && event.Key == RegistrarKey(inst.ClusterName, inst.ServiceName, inst.InstanceId)
	})
}

func TestClientWatcher_ContinuesAfterRegistrarReRegister(t *testing.T) {
	requireEtcd(t)

	prefix := "/test/watch-reregister/"
	cleanupKeys(t, prefix)
	defer cleanupKeys(t, prefix)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watcher, err := NewClientWatcher(rawClient, prefix, ctx, clientv3.WithPrefix())
	require.NoError(t, err)
	defer watcher.Close()

	client := &EtcdClient{Client: rawClient}
	inst := testInstance("test", "watch-reregister", 1)
	registrar := NewServiceRegistrar(client, inst.ClusterName, inst)
	require.NoError(t, registrar.Register())

	key := RegistrarKey(inst.ClusterName, inst.ServiceName, inst.InstanceId)
	waitForEvents(t, watcher.EventChan(), 3*time.Second, func(event *EtcdWatchEvent) bool {
		return (event.Type == EventTypeSnapshot || event.Type == EventTypePut) &&
			event.Key == key &&
			event.Inst != nil &&
			event.Inst.InstanceId == inst.InstanceId
	})

	oldLease := clientv3.LeaseID(registrar.leaseID.Load())
	require.NotZero(t, oldLease)

	require.NoError(t, registrar.reRegister())
	newLease := clientv3.LeaseID(registrar.leaseID.Load())
	require.NotZero(t, newLease)
	require.NotEqual(t, oldLease, newLease)

	update := testInstance("test", "watch-reregister", 1)
	update.Healthy = "gray"
	update.MetaData = map[string]string{"role": "re-registered"}
	require.NoError(t, registrar.UpdateInstanceData(update))

	waitForEvents(t, watcher.EventChan(), 3*time.Second, func(event *EtcdWatchEvent) bool {
		return event.Type == EventTypePut &&
			event.Key == key &&
			event.Inst != nil &&
			event.Inst.Healthy == "gray" &&
			event.Inst.MetaData["role"] == "re-registered"
	})

	registrar.Close()
	waitForEvents(t, watcher.EventChan(), 3*time.Second, func(event *EtcdWatchEvent) bool {
		return event.Type == EventTypeDelete && event.Key == key
	})
}

func TestRegistryDiscovery_DoesNotMixSimilarServicePrefixes(t *testing.T) {
	requireEtcd(t)

	prefix := "/test/logic"
	cleanupKeys(t, prefix)
	defer cleanupKeys(t, prefix)

	logic := testInstance("test", "logic", 1)
	logic2 := testInstance("test", "logic2", 2)
	logicVal, err := marshal(logic)
	require.NoError(t, err)
	logic2Val, err := marshal(logic2)
	require.NoError(t, err)

	_, err = rawClient.Put(context.Background(), RegistrarKey(logic.ClusterName, logic.ServiceName, logic.InstanceId), logicVal)
	require.NoError(t, err)
	_, err = rawClient.Put(context.Background(), RegistrarKey(logic2.ClusterName, logic2.ServiceName, logic2.InstanceId), logic2Val)
	require.NoError(t, err)

	reg := NewRegistry(&EtcdClient{Client: rawClient}, false)
	got, err := reg.GetService(context.Background(), "test", "logic")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, int32(1), got[0].InstanceId)
	require.Equal(t, "logic", got[0].ServiceName)
}

func TestRegistryDiscovery_DoesNotMixClustersForSameServiceName(t *testing.T) {
	requireEtcd(t)

	prefix := "/test/cluster-scope/"
	cleanupKeys(t, prefix)
	defer cleanupKeys(t, prefix)

	clusterA := testInstance("cluster-a", "logic", 1)
	clusterB := testInstance("cluster-b", "logic", 2)
	clusterAVal, err := marshal(clusterA)
	require.NoError(t, err)
	clusterBVal, err := marshal(clusterB)
	require.NoError(t, err)

	_, err = rawClient.Put(context.Background(), RegistrarKey(clusterA.ClusterName, clusterA.ServiceName, clusterA.InstanceId), clusterAVal)
	require.NoError(t, err)
	_, err = rawClient.Put(context.Background(), RegistrarKey(clusterB.ClusterName, clusterB.ServiceName, clusterB.InstanceId), clusterBVal)
	require.NoError(t, err)

	reg := NewRegistry(&EtcdClient{Client: rawClient}, false)
	got, err := reg.GetService(context.Background(), "cluster-a", "logic")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, int32(1), got[0].InstanceId)
	require.Equal(t, "cluster-a", got[0].ClusterName)
}

func TestRegistryDiscovery_CloseClosesWatchChannel(t *testing.T) {
	requireEtcd(t)

	reg := NewRegistry(&EtcdClient{Client: rawClient}, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watchCh, err := reg.Watch(ctx, "test", "close-watch")
	require.NoError(t, err)

	reg.Close()

	select {
	case _, ok := <-watchCh:
		require.False(t, ok, "expected watch channel to close after registry close")
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for watch channel to close after registry close")
	}
}

func waitForEvents(t *testing.T, ch <-chan *WatchResponse, timeout time.Duration, match func(*EtcdWatchEvent) bool) {
	t.Helper()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case resp, ok := <-ch:
			require.True(t, ok, "watch event channel closed before expected event")
			require.NoError(t, resp.Err)
			for _, event := range resp.Events {
				if match(event) {
					return
				}
			}
		case <-timer.C:
			t.Fatal("timed out waiting for watch event")
		}
	}
}

func watcherTestValue(t *testing.T, serviceName string, instanceID int32) string {
	t.Helper()

	value, err := marshal(testInstance("test", serviceName, instanceID))
	require.NoError(t, err)
	return value
}

func testInstance(cluster, service string, instanceID int32) *ServiceInstance {
	return &ServiceInstance{
		ClusterName: cluster,
		ServiceName: service,
		InstanceId:  instanceID,
		Host:        "127.0.0.1",
		Port:        10000 + instanceID,
		Healthy:     "health",
		Enable:      true,
		OnlineCount: 12,
		ProVersion:  1001,
		ConfVersion: 2002,
		Weight:      1,
		MetaData: map[string]string{
			"zone": "test",
		},
	}
}

func assertStoredInstance(t *testing.T, key string, match func(*ServiceInstance) bool) {
	t.Helper()

	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for stored instance at %s", key)
		case <-ticker.C:
			resp, err := rawClient.Get(context.Background(), key)
			require.NoError(t, err)
			if len(resp.Kvs) == 0 {
				continue
			}
			inst, err := unmarshal(resp.Kvs[0].Value)
			require.NoError(t, err)
			if match(inst) {
				return
			}
		}
	}
}

func assertKeyDeleted(t *testing.T, key string) {
	t.Helper()

	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for key deletion: %s", key)
		case <-ticker.C:
			resp, err := rawClient.Get(context.Background(), key)
			require.NoError(t, err)
			if len(resp.Kvs) == 0 {
				return
			}
		}
	}
}
