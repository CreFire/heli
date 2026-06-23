package servicemgr

import (
	"encoding/json"
	"game/deps/misc"
	"game/src/configdoc"
	"maps"
	"math"
	"strconv"
	"sync/atomic"

	"github.com/sasha-s/go-deadlock"
)

type NetStatus int32

const (
	NetUnValid NetStatus = 0
	NetConnect NetStatus = 1
)

type ServiceStatus string

const (
	ServiceStatusHealth   ServiceStatus = "health"
	ServiceStatusGray     ServiceStatus = "gray"
	ServiceStatusDisabled ServiceStatus = "disabled"
	ServiceStatusStopping ServiceStatus = "stopping"
)

type ServiceInstance struct {
	InstanceId   int32             `json:"id" bson:"id"`
	Healthy      ServiceStatus     `json:"health" bson:"health"`
	Load_        int32             `json:"-" bson:"-"`
	OnlineCount_ int32             `json:"online" bson:"online"`
	ProVersion   int32             `json:"pro_version" bson:"pro_version"`
	ConfVersion  int32             `json:"conf_version" bson:"conf_version"`
	NetStatus    NetStatus         `json:"-" bson:"-"`
	Enable       bool              `json:"enable" bson:"enable"`
	Weight       int32             `json:"weight" bson:"weight"`
	ClusterName  string            `json:"cluster" bson:"cluster"`
	ServiceName  string            `json:"service" bson:"service"`
	Host         string            `json:"host" bson:"host"`
	Port         int32             `json:"port" bson:"port"`
	UpdateTime   string            `json:"update_time" bson:"update_time"`
	MetaData     map[string]string `json:"meta" bson:"meta"`
	RwMutex      deadlock.RWMutex  `json:"-" bson:"-"`
}

func NewServiceInstance(conf *configdoc.ServerCfg) (inst *ServiceInstance) {
	instance := &ServiceInstance{
		InstanceId:  conf.Id,
		ClusterName: conf.Cluster,
		ServiceName: conf.Type,
		Host:        conf.Ip,
		Port:        conf.Port,
		Enable:      true,
		Healthy:     ServiceStatusHealth,
		MetaData:    make(map[string]string),
		ProVersion:  versionTextToInt32(misc.ProgVer),
		ConfVersion: versionTextToInt32(misc.ExcelVer),
	}
	return instance
}

func (si *ServiceInstance) Equal(other *ServiceInstance) bool {
	if other == nil {
		return false
	}
	si.RwMutex.RLock()
	defer si.RwMutex.RUnlock()

	if si.InstanceId != other.InstanceId || si.ServiceName != other.ServiceName || si.Weight != other.Weight ||
		si.ClusterName != other.ClusterName || si.Host != other.Host || si.Port != other.Port ||
		si.Healthy != other.Healthy || si.Enable != other.Enable ||
		si.ProVersion != other.ProVersion || si.ConfVersion != other.ConfVersion {
		return false
	}
	return maps.Equal(si.MetaData, other.MetaData)
}

//go:norace
func (si *ServiceInstance) Copy() *ServiceInstance {
	si.RwMutex.RLock()
	defer si.RwMutex.RUnlock()
	return si.copyLocked()
}

func (si *ServiceInstance) copyLocked() *ServiceInstance {
	return &ServiceInstance{
		InstanceId:   si.InstanceId,
		ClusterName:  si.ClusterName,
		ServiceName:  si.ServiceName,
		Host:         si.Host,
		Port:         si.Port,
		Healthy:      si.Healthy,
		Enable:       si.Enable,
		Weight:       si.Weight,
		ProVersion:   si.ProVersion,
		ConfVersion:  si.ConfVersion,
		NetStatus:    si.NetState(),
		UpdateTime:   si.UpdateTime,
		OnlineCount_: si.OnlineCount(),
		Load_:        si.Load(),
		MetaData:     maps.Clone(si.MetaData),
	}
}

func versionTextToInt32(text string) int32 {
	if text == "" {
		return 0
	}
	version, err := strconv.ParseInt(text, 10, 32)
	if err != nil {
		panic(err)
	}
	if version > math.MaxInt32 || version < math.MinInt32 {
		panic("version out of int32 range")
	}
	return int32(version)
}

//go:norace
func (si *ServiceInstance) String() string {
	s2 := si.Copy()
	js, _ := json.Marshal(s2)
	return string(js)
}

//go:norace
func (si *ServiceInstance) IncOnlineCount(count int32) {
	atomic.AddInt32(&si.OnlineCount_, count)
}

//go:norace
func (si *ServiceInstance) OnlineCount() int32 {
	return si.OnlineCount_
}

//go:norace
func (si *ServiceInstance) UpdateOnlineCount(count int32) {
	atomic.StoreInt32(&si.OnlineCount_, count)
}

//go:norace
func (si *ServiceInstance) UpdateLoad(load int32) {
	atomic.StoreInt32(&si.Load_, load)
}

//go:norace
func (si *ServiceInstance) Load() int32 {
	return si.Load_
}

func (si *ServiceInstance) SetNetStatus(status NetStatus) {
	atomic.StoreInt32((*int32)(&si.NetStatus), int32(status))
}

func (si *ServiceInstance) NetState() NetStatus {
	return NetStatus(atomic.LoadInt32((*int32)(&si.NetStatus)))
}
