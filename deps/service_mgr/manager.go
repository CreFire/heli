package servicemgr

import (
	"context"
	"errors"
	"fmt"
	"game/deps/etcd"
	"game/src/configdoc"
)

var errManagerRegistrarNil = errors.New("service registrar is nil")

type serviceRegistry interface {
	Register() error
	Update(inst *ServiceInstance) error
	Close()
}

type serviceDiscovery interface {
	Watch(ctx context.Context, specs ...ListenSpec) (<-chan *WatchEvent, error)
	Close()
}

type HandlerFunc struct {
	OnlineFn  func(serviceName string, instance *ServiceInstance) error
	OfflineFn func(serviceName string, instance *ServiceInstance) error
	UpdateFn  func(serviceName string, instance *ServiceInstance) error
}

func (h HandlerFunc) OnlineInstance(serviceName string, instance *ServiceInstance) error {
	if h.OnlineFn == nil {
		return nil
	}
	return h.OnlineFn(serviceName, instance)
}

func (h HandlerFunc) OfflineInstance(serviceName string, instance *ServiceInstance) error {
	if h.OfflineFn == nil {
		return nil
	}
	return h.OfflineFn(serviceName, instance)
}

func (h HandlerFunc) UpdateInstance(serviceName string, instance *ServiceInstance) error {
	if h.UpdateFn == nil {
		return nil
	}
	return h.UpdateFn(serviceName, instance)
}

func New(conf *configdoc.ServerCfg, et *etcd.EtcdClient) (*Manager, error) {
	return newEtcdManager(conf, et, true)
}

func NewUnregistered(conf *configdoc.ServerCfg, et *etcd.EtcdClient) (*Manager, error) {
	return newEtcdManager(conf, et, false)
}

func newEtcdManager(conf *configdoc.ServerCfg, et *etcd.EtcdClient, register bool) (*Manager, error) {
	if conf == nil {
		return nil, fmt.Errorf("server conf is nil")
	}
	if et == nil {
		return nil, fmt.Errorf("etcd client is nil")
	}

	inst := NewServiceInstance(conf)
	return newManager(inst, newRegistrar(et, inst), newWatcher(et), register)
}

// NewWithComponents is a narrow test seam for packages that need a Manager
// without a live etcd client. Production code should use New or NewUnregistered.
func NewWithComponents(ins *ServiceInstance, registry serviceRegistry, discovery serviceDiscovery) (*Manager, error) {
	return newManager(ins, registry, discovery, true)
}

func newManager(ins *ServiceInstance, registry serviceRegistry, discovery serviceDiscovery, register bool) (*Manager, error) {
	if ins == nil {
		return nil, fmt.Errorf("service instance is nil")
	}

	m := &Manager{
		Cluster:       ins.ClusterName,
		ServiceName:   ins.ServiceName,
		services:      newServiceMap(),
		rwmutex:       newManagerMutex(),
		listenInfoMap: make(map[string]*ListenSpec),
		discovery:     discovery,
		registry:      registry,
		instance:      ins,
	}
	if register {
		if err := m.Register(); err != nil {
			return nil, err
		}
	}
	return m, nil
}
