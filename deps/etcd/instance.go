package etcd

import (
	"encoding/json"
	"fmt"
	"game/deps/xlog"
	"maps"
)

// ServiceInstance is an instance of a service in a discovery system.
type ServiceInstance struct {
	InstanceId  int32             `json:"id" bson:"id"`
	ClusterName string            `json:"cluster" bson:"cluster"`
	ServiceName string            `json:"service" bson:"service"`
	Host        string            `json:"host" bson:"host"`
	Port        int32             `json:"port" bson:"port"`
	Healthy     string            `json:"health" bson:"health"`
	Enable      bool              `json:"enable" bson:"enable"`
	OnlineCount int32             `json:"online" bson:"online"`
	ProVersion  int32             `json:"pro_version" bson:"pro_version"`
	ConfVersion int32             `json:"conf_version" bson:"conf_version"`
	Weight      int32             `json:"weight" bson:"weight"`
	MetaData    map[string]string `json:"meta" bson:"meta"`
	UpdateTime  string            `json:"update_time" bson:"update_time"`
}

func (i *ServiceInstance) String() string {
	return fmt.Sprintf("%s-%d", i.ServiceName, i.InstanceId)
}

// Equal returns whether i and other are equivalent for discovery updates.
func (i *ServiceInstance) Equal(other *ServiceInstance) bool {
	if i == nil && other == nil {
		return true
	}
	if i == nil || other == nil {
		return false
	}
	if i.InstanceId != other.InstanceId || i.ClusterName != other.ClusterName || i.ServiceName != other.ServiceName ||
		i.Healthy != other.Healthy || i.Enable != other.Enable || i.Host != other.Host || i.Port != other.Port ||
		i.Weight != other.Weight || i.ProVersion != other.ProVersion || i.ConfVersion != other.ConfVersion {
		return false
	}
	return maps.Equal(i.MetaData, other.MetaData)
}

func marshal(si *ServiceInstance) (string, error) {
	data, err := json.Marshal(si)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshal(data []byte) (si *ServiceInstance, err error) {
	err = json.Unmarshal(data, &si)
	return
}

func serviceInstanceFromValue(key, value []byte) *ServiceInstance {
	inst, err := unmarshal(value)
	if err != nil {
		xlog.Errorf("failed to unmarshal instance, key: %s, value: %s, err: %v", key, value, err)
		return nil
	}
	return inst
}
