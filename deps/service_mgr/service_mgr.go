package servicemgr

import (
	"context"
	"errors"
	"fmt"
	"game/deps/basal"
	"game/deps/xlog"
	"math/rand/v2"

	"github.com/phuslu/shardmap"
	"github.com/sasha-s/go-deadlock"
)

var ErrInstanceNotFound = errors.New("instance not found")

// Manager handles service registration, watching, and in-process instance state.
type Manager struct {
	Cluster     string
	ServiceName string

	services      *shardmap.Map[string, *Service]
	rwmutex       *deadlock.RWMutex
	listenInfoMap map[string]*ListenSpec
	discovery     serviceDiscovery
	registry      serviceRegistry
	instance      *ServiceInstance
	cancelListen  context.CancelFunc
}

func newServiceMap() *shardmap.Map[string, *Service] {
	return shardmap.New[string, *Service](16)
}

func newManagerMutex() *deadlock.RWMutex {
	return &deadlock.RWMutex{}
}

func (m *Manager) Register() error {
	if m.registry == nil {
		return fmt.Errorf("service registrar is nil")
	}
	if err := m.registry.Register(); err != nil {
		return fmt.Errorf("failed to register service: %w", err)
	}
	xlog.Infof("service registered: %s/%s id=%d", m.Cluster, m.ServiceName, m.instance.InstanceId)
	return nil
}

func (m *Manager) Watch(specs ...ListenSpec) error {
	if len(specs) == 0 {
		return nil
	}
	if m.discovery == nil {
		return fmt.Errorf("service watcher is nil")
	}

	m.rwmutex.Lock()
	defer m.rwmutex.Unlock()

	added := make([]string, 0, len(specs))
	for _, spec := range specs {
		if _, exists := m.listenInfoMap[spec.ServiceName]; exists {
			for _, serviceName := range added {
				delete(m.listenInfoMap, serviceName)
			}
			return fmt.Errorf("watch service already exists: %s", spec.ServiceName)
		}
		cp := spec
		m.listenInfoMap[spec.ServiceName] = &cp
		added = append(added, spec.ServiceName)
	}

	watchList := make([]ListenSpec, 0, len(m.listenInfoMap))
	for _, spec := range m.listenInfoMap {
		watchList = append(watchList, *spec)
	}

	if err := m.startWatchingLocked(watchList...); err != nil {
		for _, serviceName := range added {
			delete(m.listenInfoMap, serviceName)
		}
		return err
	}
	return nil
}

func (m *Manager) startWatchingLocked(specs ...ListenSpec) error {
	ctx, cancel := context.WithCancel(context.Background())
	eventChan, err := m.discovery.Watch(ctx, specs...)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to start watching: %w", err)
	}

	prevCancel := m.cancelListen
	m.cancelListen = cancel
	if prevCancel != nil {
		prevCancel()
	}

	basal.SafeGo(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-eventChan:
				if !ok {
					xlog.Infof("service watch channel closed")
					return
				}
				basal.SafeRun(func() {
					m.updateAndTriggerInstance(event)
				})
			}
		}
	})
	return nil
}

func (m *Manager) Self() *ServiceInstance {
	return m.instance
}

func (m *Manager) SelfCopy() *ServiceInstance {
	return m.instance.Copy()
}

// List returns instances for a service.
//
// Fast path: when filter is nil, this may return the currently published
// internal slice directly instead of a copied result.
//
// Callers must treat the returned slice as read-only. Do not sort it, append
// to it, reslice and append, swap elements, overwrite elements, or mutate it
// through any helper that writes in place.
//
// Code review rule: any caller of List(service, nil) that writes to the
// returned slice must be treated as a bug and fixed by copying first.
func (m *Manager) List(serviceName string, filter func(*ServiceInstance) bool) []*ServiceInstance {
	svc, ok := m.getService(serviceName)
	if !ok {
		return nil
	}
	return svc.snapshot(filter)
}

func (m *Manager) Get(serviceName string, serviceID int32) (*ServiceInstance, error) {
	svc, ok := m.getService(serviceName)
	if !ok {
		xlog.Infof("now not valid service name: %s", serviceName)
		return nil, fmt.Errorf("%w: service=%s", ErrInstanceNotFound, serviceName)
	}

	if inst, ok := svc.getInstance(serviceID); ok {
		return inst, nil
	}
	return nil, fmt.Errorf("%w: service=%s instanceId=%d", ErrInstanceNotFound, serviceName, serviceID)
}

func (m *Manager) PickRandom(serviceName string, checkNet bool) (*ServiceInstance, error) {
	svc, ok := m.getService(serviceName)
	if !ok {
		return nil, fmt.Errorf("%w: service=%s", ErrInstanceNotFound, serviceName)
	}
	instances := svc.snapshot(nil)
	if len(instances) == 0 {
		return nil, fmt.Errorf("%w: no service instances, service=%s", ErrInstanceNotFound, serviceName)
	}
	i := rand.Int32N(int32(len(instances)))
	if instances[i].Enable && instances[i].Healthy == ServiceStatusHealth && (!checkNet || instances[i].NetState() == NetConnect) {
		return instances[i], nil
	}

	ins := make([]*ServiceInstance, 0, len(instances))
	for _, instance := range instances {
		if instance.Enable && instance.Healthy == ServiceStatusHealth && (!checkNet || instance.NetState() == NetConnect) {
			ins = append(ins, instance)
		}
	}
	if len(ins) == 0 {
		return nil, fmt.Errorf("%w: no valid service instances, service=%s", ErrInstanceNotFound, serviceName)
	}
	i = rand.Int32N(int32(len(ins)))
	return ins[i], nil
}

func (m *Manager) PickMinOnline(serviceName string, checkNet bool) (*ServiceInstance, error) {
	svc, ok := m.getService(serviceName)
	if !ok {
		return nil, fmt.Errorf("%w: service=%s", ErrInstanceNotFound, serviceName)
	}
	instances := svc.snapshot(nil)
	if len(instances) == 0 {
		return nil, fmt.Errorf("%w: no service instances, service=%s", ErrInstanceNotFound, serviceName)
	}

	var minOnline *ServiceInstance
	for _, instance := range instances {
		if instance.Enable && instance.Healthy == ServiceStatusHealth && (!checkNet || instance.NetState() == NetConnect) {
			if minOnline == nil || instance.OnlineCount() < minOnline.OnlineCount() {
				minOnline = instance
			}
		}
	}
	if minOnline == nil {
		return nil, fmt.Errorf("%w: no valid service instances, service=%s", ErrInstanceNotFound, serviceName)
	}
	return minOnline, nil
}

func (m *Manager) pickWeight(serviceName string) (*ServiceInstance, error) {
	svc, ok := m.getService(serviceName)
	if !ok {
		return nil, fmt.Errorf("%w: service=%s", ErrInstanceNotFound, serviceName)
	}
	instances := svc.snapshot(nil)
	if len(instances) == 0 {
		return nil, fmt.Errorf("%w: no service instances, service=%s", ErrInstanceNotFound, serviceName)
	}
	allWeight := 0
	ins := make([]*ServiceInstance, 0, len(instances))
	for _, instance := range instances {
		if instance.Enable && instance.Healthy == ServiceStatusHealth && instance.NetState() == NetConnect {
			ins = append(ins, instance)
			allWeight += int(instance.Weight)
		}
	}
	if len(ins) == 0 || allWeight == 0 {
		return nil, fmt.Errorf("%w: no valid service instances, service=%s", ErrInstanceNotFound, serviceName)
	}

	randWeight := rand.Int32N(int32(allWeight))
	for _, instance := range ins {
		randWeight -= int32(instance.Weight)
		if randWeight <= 0 {
			return instance, nil
		}
	}
	return ins[0], nil
}

func (m *Manager) PickByHash(serviceName string, key any) (*ServiceInstance, error) {
	svc, ok := m.getService(serviceName)
	if !ok {
		return nil, fmt.Errorf("%w: service=%s", ErrInstanceNotFound, serviceName)
	}
	inst, found := svc.GetByHashKey(key)
	if !found || inst == nil {
		return nil, fmt.Errorf("%w: instance not found by hash, service=%s", ErrInstanceNotFound, serviceName)
	}
	return inst, nil
}

func (m *Manager) UpdateSelf(f func(*ServiceInstance)) error {
	if m.registry == nil {
		return errManagerRegistrarNil
	}
	m.instance.RwMutex.Lock()
	f(m.instance)
	inst := m.instance.copyLocked()
	m.instance.RwMutex.Unlock()
	return m.registry.Update(inst)
}

func (m *Manager) UpdateLoads(insts []*ServiceInstance) error {
	if len(insts) == 0 {
		return nil
	}
	svc, _ := m.getService(insts[0].ServiceName)
	for _, v := range insts {
		if v == nil {
			continue
		}
		if svc == nil || v.ServiceName != svc.name {
			svc, _ = m.getService(v.ServiceName)
			if svc == nil {
				xlog.Infof("now not valid service name: %s instance id: %d", v.ServiceName, v.InstanceId)
				continue
			}
		}

		inst, ok := svc.getInstance(v.InstanceId)
		if !ok {
			continue
		}
		inst.UpdateLoad(v.Load())
		inst.UpdateOnlineCount(v.OnlineCount())
	}
	return nil
}

func (m *Manager) UpdateNetStatus(serviceName string, instanceID int32, netStatus NetStatus) error {
	svc, ok := m.getService(serviceName)
	if !ok {
		return fmt.Errorf("%w: service=%s", ErrInstanceNotFound, serviceName)
	}
	inst, ok := svc.getInstance(instanceID)
	if !ok {
		return fmt.Errorf("%w: service=%s instanceId=%d", ErrInstanceNotFound, serviceName, instanceID)
	}
	inst.SetNetStatus(netStatus)
	return nil
}

func (m *Manager) Close() {
	if m.cancelListen != nil {
		m.cancelListen()
		m.cancelListen = nil
	}

	if m.registry != nil {
		m.registry.Close()
		m.registry = nil
	}

	if m.discovery != nil {
		m.discovery.Close()
		m.discovery = nil
	}
}

func (m *Manager) ensureService(serviceName string) *Service {
	var svc *Service
	m.services.Mutate(serviceName, func(old *Service, exists bool) (*Service, bool) {
		if exists && old != nil {
			svc = old
			return old, true
		}
		svc = newService(serviceName)
		return svc, true
	})
	return svc
}

func (m *Manager) getService(serviceName string) (*Service, bool) {
	return m.services.Get(serviceName)
}

func (m *Manager) addServiceInstance(serviceName string, instance *ServiceInstance) {
	svc := m.ensureService(serviceName)
	svc.setInstance(instance)
}

func (m *Manager) removeServiceInstance(serviceName string, instanceID int32) *ServiceInstance {
	svc, ok := m.getService(serviceName)
	if !ok {
		return nil
	}
	return svc.removeInstance(instanceID)
}

func (m *Manager) updateAndTriggerInstance(event *WatchEvent) {
	m.rwmutex.RLock()
	spec := m.listenInfoMap[event.ServiceName]
	m.rwmutex.RUnlock()
	if spec == nil {
		xlog.Errorf("no listener found for service: %s", event.ServiceName)
		return
	}
	if event.Type == EventTypeSnapshot || event.Type == EventTypePut {
		m.ensureService(event.ServiceName)
	}

	online, offline, update := m.updateServiceState(event.Type, event.ServiceName, event.InstanceID, event.Instance)
	handler := spec.Handler
	if handler == nil {
		return
	}

	if online != nil {
		xlog.Infof("Service %s instance %d is online, instance: %v", event.ServiceName, online.InstanceId, online.String())
		handler.OnlineInstance(event.ServiceName, online)
	}
	if offline != nil {
		xlog.Infof("Service %s instance %d is offline, instance: %v ", event.ServiceName, offline.InstanceId, offline.String())
		handler.OfflineInstance(event.ServiceName, offline)
	}
	if update != nil {
		xlog.Infof("Service %s instance %d is updated, instance: %v", event.ServiceName, update.InstanceId, update.String())
		handler.UpdateInstance(event.ServiceName, update)
	}
}

func (m *Manager) updateServiceState(event EventType, serviceName string, serviceID int32, inst *ServiceInstance) (online, offline, update *ServiceInstance) {
	switch event {
	case EventTypeSnapshot, EventTypePut:
		if inst == nil {
			return
		}
		svc, ok := m.getService(serviceName)
		if !ok {
			xlog.Errorf("service cache missing for watched service: %s", serviceName)
			return
		}
		prev, exists := svc.setInstance(inst)
		if !exists || prev == nil {
			online = inst
			return
		}
		if inst.Equal(prev) {
			xlog.Infof("Service %s instance %d already exists, no update needed %v %v", serviceName, serviceID, prev, inst)
			return
		}
		update = inst
		return
	case EventTypeDelete:
		svc, ok := m.getService(serviceName)
		if !ok {
			return
		}
		removedInstance := svc.removeInstance(serviceID)
		if removedInstance != nil {
			offline = removedInstance
		}
		return
	}
	return
}
