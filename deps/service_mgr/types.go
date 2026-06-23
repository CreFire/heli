package servicemgr

type EventType int

const (
	EventTypePut EventType = iota
	EventTypeDelete
	EventTypeSnapshot
)

type WatchEvent struct {
	Type        EventType
	ServiceName string
	InstanceID  int32
	Instance    *ServiceInstance
}

type Handler interface {
	OnlineInstance(serviceName string, instance *ServiceInstance) error
	OfflineInstance(serviceName string, instance *ServiceInstance) error
	UpdateInstance(serviceName string, instance *ServiceInstance) error
}

type ListenSpec struct {
	Cluster     string
	ServiceName string
	Handler     Handler
}
