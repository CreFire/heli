package etcd

import (
	"context"
	"errors"
	"fmt"
	"sync"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// RegistryDiscovery implements both Registrar and Discovery interfaces.
type RegistryDiscovery struct {
	client          *EtcdClient
	mu              sync.Mutex
	registrars      map[string]*ServiceRegistrar
	registerEnabled bool
	watchers        []*ClientWatcher
}

// NewRegistry constructs a registry backed by the provided EtcdClient.
func NewRegistry(client *EtcdClient, registerEnabled bool) *RegistryDiscovery {
	return &RegistryDiscovery{
		client:          client,
		registrars:      make(map[string]*ServiceRegistrar),
		registerEnabled: registerEnabled,
	}
}

func (r *RegistryDiscovery) Close() {
	r.mu.Lock()
	registrars := make([]*ServiceRegistrar, 0, len(r.registrars))
	for _, registrar := range r.registrars {
		registrars = append(registrars, registrar)
	}
	watchers := r.watchers
	r.registrars = make(map[string]*ServiceRegistrar)
	r.watchers = nil
	r.mu.Unlock()

	for _, registrar := range registrars {
		registrar.Close()
	}
	for _, watcher := range watchers {
		watcher.Close()
	}
}

// Register registers a service instance and starts its heartbeat.
func (r *RegistryDiscovery) Register(ctx context.Context, service *ServiceInstance) error {
	if !r.registerEnabled {
		return nil
	}
	if service == nil {
		return errors.New("service instance is nil")
	}
	if r.client == nil || r.client.Client == nil {
		return errors.New("etcd client is nil")
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	key := RegistrarKey(service.ClusterName, service.ServiceName, service.InstanceId)
	r.mu.Lock()
	if _, ok := r.registrars[key]; ok {
		r.mu.Unlock()
		return fmt.Errorf("service already registered: %s", key)
	}
	registrar := NewServiceRegistrar(r.client, service.ClusterName, service)
	r.registrars[key] = registrar
	r.mu.Unlock()

	if err := registrar.Register(); err != nil {
		r.mu.Lock()
		delete(r.registrars, key)
		r.mu.Unlock()
		return err
	}
	return nil
}

// Deregister removes a service instance from etcd.
func (r *RegistryDiscovery) Deregister(ctx context.Context, service *ServiceInstance) error {
	if !r.registerEnabled {
		return nil
	}
	if service == nil {
		return errors.New("service instance is nil")
	}
	if r.client == nil || r.client.Client == nil {
		return errors.New("etcd client is nil")
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	key := RegistrarKey(service.ClusterName, service.ServiceName, service.InstanceId)
	r.mu.Lock()
	registrar := r.registrars[key]
	if registrar != nil {
		delete(r.registrars, key)
	}
	r.mu.Unlock()

	if registrar != nil {
		registrar.Close()
		return nil
	}

	if ctx == nil {
		ctx = context.Background()
	}
	_, err := r.client.Client.Delete(ctx, key)
	return err
}

// GetService returns service instances under the given cluster and service.
func (r *RegistryDiscovery) GetService(ctx context.Context, cluster string, serviceName string) ([]*ServiceInstance, error) {
	if r.client == nil || r.client.Client == nil {
		return nil, errors.New("etcd client is nil")
	}
	prefix, err := servicePrefix(cluster, serviceName)
	if err != nil {
		return nil, err
	}

	if ctx == nil {
		ctx = context.Background()
	}
	resp, err := r.client.Client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	instances := make([]*ServiceInstance, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		inst, err := unmarshal(kv.Value)
		if err != nil || inst == nil {
			continue
		}
		instances = append(instances, inst)
	}
	return instances, nil
}

// Watch creates a watcher for the given cluster and service.
func (r *RegistryDiscovery) Watch(ctx context.Context, cluster string, serviceName string) (<-chan *WatchResponse, error) {
	if r.client == nil || r.client.Client == nil {
		return nil, errors.New("etcd client is nil")
	}
	prefix, err := servicePrefix(cluster, serviceName)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	watcher, err := NewClientWatcher(r.client.Client, prefix, ctx, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.watchers = append(r.watchers, watcher)
	r.mu.Unlock()
	return watcher.EventChan(), nil
}
