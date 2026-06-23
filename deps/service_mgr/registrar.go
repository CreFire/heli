package servicemgr

import (
	"game/deps/etcd"
	"maps"
	"time"
)

type registrar struct {
	inner *etcd.ServiceRegistrar
}

func newRegistrar(client *etcd.EtcdClient, instance *ServiceInstance) *registrar {
	etcdInstance := toEtcdInstance(instance)
	etcdInstance.UpdateTime = time.Now().Format("2006-01-02 15:04:05")
	return &registrar{
		inner: etcd.NewServiceRegistrar(client, instance.ClusterName, etcdInstance),
	}
}

func (r *registrar) Register() error {
	return r.inner.Register()
}

func (r *registrar) Update(inst *ServiceInstance) error {
	return r.inner.UpdateInstanceData(toEtcdInstance(inst))
}

func (r *registrar) Close() {
	if r.inner != nil {
		r.inner.Close()
	}
}

func toEtcdInstance(si *ServiceInstance) *etcd.ServiceInstance {
	if si == nil {
		return nil
	}
	return &etcd.ServiceInstance{
		ClusterName: si.ClusterName,
		ServiceName: si.ServiceName,
		InstanceId:  si.InstanceId,
		Host:        si.Host,
		Port:        si.Port,
		Healthy:     string(si.Healthy),
		Enable:      si.Enable,
		OnlineCount: si.OnlineCount(),
		ProVersion:  si.ProVersion,
		ConfVersion: si.ConfVersion,
		Weight:      si.Weight,
		MetaData:    maps.Clone(si.MetaData),
		UpdateTime:  si.UpdateTime,
	}
}

func fromEtcdInstance(esi *etcd.ServiceInstance) *ServiceInstance {
	if esi == nil {
		return nil
	}
	return &ServiceInstance{
		ClusterName:  esi.ClusterName,
		ServiceName:  esi.ServiceName,
		InstanceId:   esi.InstanceId,
		Host:         esi.Host,
		Port:         esi.Port,
		Healthy:      ServiceStatus(esi.Healthy),
		Enable:       esi.Enable,
		OnlineCount_: esi.OnlineCount,
		ProVersion:   esi.ProVersion,
		ConfVersion:  esi.ConfVersion,
		Weight:       esi.Weight,
		MetaData:     maps.Clone(esi.MetaData),
		UpdateTime:   esi.UpdateTime,
	}
}

func convertEventType(etcdEventType etcd.EventType) EventType {
	switch etcdEventType {
	case etcd.EventTypePut:
		return EventTypePut
	case etcd.EventTypeDelete:
		return EventTypeDelete
	case etcd.EventTypeSnapshot:
		return EventTypeSnapshot
	default:
		return EventTypePut
	}
}
