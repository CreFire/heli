package servicemgr

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/sasha-s/go-deadlock"
)

func seedService(sm *Manager, serviceName string, instances []*ServiceInstance) {
	svc := sm.ensureService(serviceName)
	for _, inst := range instances {
		svc.setInstance(inst)
	}
}

func BenchmarkPickMinOnline(b *testing.B) {
	deadlock.Opts.Disable = true
	sm := newTestManager(b)

	serviceName := "test-service"
	instanceCount := 20
	instances := make([]*ServiceInstance, instanceCount)
	for i := range instanceCount {
		instances[i] = &ServiceInstance{
			InstanceId:   int32(i + 1),
			ServiceName:  serviceName,
			Enable:       true,
			Healthy:      "health",
			NetStatus:    NetConnect,
			OnlineCount_: rand.Int32N(10000),
		}
	}
	seedService(sm, serviceName, instances)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sm.List(serviceName, func(si *ServiceInstance) bool { return true })
	}
}

func BenchmarkListNilFilter(b *testing.B) {
	deadlock.Opts.Disable = true
	sm := newTestManager(b)

	serviceName := "test-service"
	instanceCount := 1000
	instances := make([]*ServiceInstance, instanceCount)
	for i := range instanceCount {
		instances[i] = &ServiceInstance{
			InstanceId:   int32(i + 1),
			ServiceName:  serviceName,
			Enable:       true,
			Healthy:      ServiceStatusHealth,
			NetStatus:    NetConnect,
			OnlineCount_: rand.Int32N(10000),
		}
	}
	seedService(sm, serviceName, instances)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sm.List(serviceName, nil)
	}
}

func BenchmarkPickMinOnlineWithVaryingSize(b *testing.B) {
	instanceSizes := []int{10, 100, 1000, 5000}
	deadlock.Opts.Disable = true

	for _, size := range instanceSizes {
		b.Run(fmt.Sprintf("instances_%d", size), func(b *testing.B) {
			sm := newTestManager(b)
			serviceName := "test-service"
			instances := make([]*ServiceInstance, size)
			for i := range size {
				instances[i] = &ServiceInstance{
					InstanceId:   int32(i + 1),
					ServiceName:  serviceName,
					Enable:       true,
					Healthy:      "health",
					NetStatus:    NetConnect,
					OnlineCount_: rand.Int32N(10000),
				}
			}
			seedService(sm, serviceName, instances)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := sm.PickMinOnline(serviceName, true); err != nil {
					b.Fatalf("PickMinOnline failed: %v", err)
				}
			}
		})
	}
}

func BenchmarkPickMinOnlineWithUnhealthyInstances(b *testing.B) {
	deadlock.Opts.Disable = true
	sm := newTestManager(b)

	serviceName := "test-service"
	totalInstances := 1000
	healthyInstances := 800
	instances := make([]*ServiceInstance, totalInstances)
	for i := range totalInstances {
		status := "health"
		if i >= healthyInstances {
			status = "unhealthy"
		}
		instances[i] = &ServiceInstance{
			InstanceId:   int32(i + 1),
			ServiceName:  serviceName,
			Enable:       true,
			Healthy:      ServiceStatus(status),
			NetStatus:    NetConnect,
			OnlineCount_: rand.Int32N(10000),
		}
	}
	seedService(sm, serviceName, instances)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := sm.PickMinOnline(serviceName, true); err != nil {
			b.Fatalf("PickMinOnline failed: %v", err)
		}
	}
}

func BenchmarkPickMinOnlineWithoutNetCheck(b *testing.B) {
	deadlock.Opts.Disable = true
	sm := newTestManager(b)

	serviceName := "test-service"
	instanceCount := 1000
	instances := make([]*ServiceInstance, instanceCount)
	for i := range instanceCount {
		instances[i] = &ServiceInstance{
			InstanceId:   int32(i + 1),
			ServiceName:  serviceName,
			Enable:       true,
			Healthy:      "health",
			NetStatus:    NetUnValid,
			OnlineCount_: rand.Int32N(10000),
		}
	}
	seedService(sm, serviceName, instances)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := sm.PickMinOnline(serviceName, false); err != nil {
			b.Fatalf("PickMinOnline failed: %v", err)
		}
	}
}

func BenchmarkPickWeight(b *testing.B) {
	deadlock.Opts.Disable = true
	sm := newTestManager(b)

	serviceName := "test-service"
	instanceCount := 1000
	instances := make([]*ServiceInstance, instanceCount)
	for i := range instanceCount {
		instances[i] = &ServiceInstance{
			InstanceId:  int32(i + 1),
			ServiceName: serviceName,
			Enable:      true,
			Healthy:     "health",
			NetStatus:   NetConnect,
			Weight:      rand.Int32N(100) + 1,
		}
	}
	seedService(sm, serviceName, instances)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := sm.pickWeight(serviceName); err != nil {
			b.Fatalf("pickWeight failed: %v", err)
		}
	}
}

func BenchmarkPickRandom(b *testing.B) {
	deadlock.Opts.Disable = true
	sm := newTestManager(b)

	serviceName := "test-service"
	instanceCount := 1000
	instances := make([]*ServiceInstance, instanceCount)
	for i := range instanceCount {
		instances[i] = &ServiceInstance{
			InstanceId:  int32(i + 1),
			ServiceName: serviceName,
			Enable:      true,
			Healthy:     "health",
			NetStatus:   NetConnect,
		}
	}
	seedService(sm, serviceName, instances)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := sm.PickRandom(serviceName, true); err != nil {
			b.Fatalf("PickRandom failed: %v", err)
		}
	}
}

func BenchmarkDeadlockRLock(b *testing.B) {
	var mu sync.RWMutex
	deadlock.Opts.Disable = true

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mu.RLock()
		mu.RUnlock()
	}
}

func BenchmarkUpdateLoads(b *testing.B) {
	deadlock.Opts.Disable = true
	sm := newTestManager(b)

	serviceName := "test-service"
	instanceCount := 1000
	instances := make([]*ServiceInstance, instanceCount)
	for i := range instanceCount {
		instances[i] = &ServiceInstance{
			InstanceId:   int32(i + 1),
			ServiceName:  serviceName,
			Enable:       true,
			Healthy:      "health",
			NetStatus:    NetConnect,
			OnlineCount_: int32(i * 10),
			Load_:        int32(i * 5),
		}
	}
	seedService(sm, serviceName, instances)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		updateInstances := make([]*ServiceInstance, instanceCount/10)
		for j := range updateInstances {
			idx := rand.Int32N(int32(instanceCount))
			updateInstances[j] = &ServiceInstance{
				InstanceId:   idx + 1,
				ServiceName:  serviceName,
				OnlineCount_: rand.Int32N(10000),
				Load_:        rand.Int32N(100),
			}
		}
		if err := sm.UpdateLoads(updateInstances); err != nil {
			b.Fatalf("UpdateLoads failed: %v", err)
		}
	}
}

func BenchmarkUpdateLoadsVaryingSizes(b *testing.B) {
	deadlock.Opts.Disable = true
	deadlock.Opts.DisableLockOrderDetection = true

	updateSizes := []int{10, 50, 100, 500}
	for _, updateSize := range updateSizes {
		b.Run(fmt.Sprintf("updates_%d", updateSize), func(b *testing.B) {
			sm := newTestManager(b)

			serviceName := "test-service"
			instanceCount := 100
			instances := make([]*ServiceInstance, instanceCount)
			for i := range instanceCount {
				instances[i] = &ServiceInstance{
					InstanceId:   int32(i + 1),
					ServiceName:  serviceName,
					Enable:       true,
					Healthy:      "health",
					NetStatus:    NetConnect,
					OnlineCount_: int32(i * 10),
					Load_:        int32(i * 5),
				}
			}
			seedService(sm, serviceName, instances)

			updateInstances := make([]*ServiceInstance, updateSize)
			for j := range updateInstances {
				idx := rand.Int32N(int32(instanceCount))
				updateInstances[j] = &ServiceInstance{
					InstanceId:   idx + 1,
					ServiceName:  serviceName,
					OnlineCount_: rand.Int32N(10000),
					Load_:        rand.Int32N(100),
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := sm.UpdateLoads(updateInstances); err != nil {
					b.Fatalf("UpdateLoads failed: %v", err)
				}
			}
		})
	}
}

func BenchmarkPickByHash(b *testing.B) {
	deadlock.Opts.Disable = true
	sm := newTestManager(b)

	serviceName := "test-service"
	instanceCount := 5
	instances := make([]*ServiceInstance, instanceCount)
	for i := range instanceCount {
		instances[i] = &ServiceInstance{
			InstanceId:  int32(i + 1),
			ServiceName: serviceName,
			Enable:      true,
			Healthy:     ServiceStatusHealth,
			NetStatus:   NetConnect,
		}
	}
	seedService(sm, serviceName, instances)

	keys := make([]string, 1024)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := keys[i%len(keys)]
		if _, err := sm.PickByHash(serviceName, key); err != nil {
			//b.Fatalf("PickByHash failed: %v", err)
		}
	}
}
