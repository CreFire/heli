package etcd

import "context"

// Registrar is service registrar.
type Registrar interface {
	// Register the registration.
	Register(ctx context.Context, service *ServiceInstance) error
	// Deregister the registration.
	Deregister(ctx context.Context, service *ServiceInstance) error
}

// Discovery is service discovery.
type Discovery interface {
	// GetService returns instances under cluster + serviceName.
	// Example:
	//   - cluster=prod, serviceName=logic
	GetService(ctx context.Context, cluster string, serviceName string) ([]*ServiceInstance, error)
	// Watch watches instances under cluster + serviceName.
	// Example:
	//   - cluster=prod, serviceName=logic
	Watch(ctx context.Context, cluster string, serviceName string) (<-chan *WatchResponse, error)
}
