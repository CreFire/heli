package servicemgr

import (
	"game/deps/misc"
	"game/src/configdoc"
	"math"
	"testing"
)

func TestHandlerFuncUpdateInstanceUsesUpdateFunc(t *testing.T) {
	inst := &ServiceInstance{
		ServiceName: "battle",
		InstanceId:  7,
	}

	called := false
	handler := HandlerFunc{
		UpdateFn: func(serviceName string, instance *ServiceInstance) error {
			called = true
			if serviceName != inst.ServiceName {
				t.Fatalf("serviceName = %s, want %s", serviceName, inst.ServiceName)
			}
			if instance != inst {
				t.Fatalf("instance = %p, want %p", instance, inst)
			}
			return nil
		},
	}

	if err := handler.UpdateInstance(inst.ServiceName, inst); err != nil {
		t.Fatalf("UpdateInstance returned error: %v", err)
	}
	if !called {
		t.Fatal("UpdateFunc was not called")
	}
}

func TestNewServiceInstanceStoresVersionsInFieldsOnly(t *testing.T) {
	oldProgVer := misc.ProgVer
	oldExcelVer := misc.ExcelVer
	misc.ProgVer = "1001"
	misc.ExcelVer = "2002"
	defer func() {
		misc.ProgVer = oldProgVer
		misc.ExcelVer = oldExcelVer
	}()

	inst := NewServiceInstance(&configdoc.ServerCfg{
		Id:      7,
		Type:    "logic",
		Cluster: "cluster-a",
		Ip:      "127.0.0.1",
		Port:    9000,
	})

	if inst.ProVersion != 1001 || inst.ConfVersion != 2002 {
		t.Fatalf("unexpected version fields: pro=%d conf=%d", inst.ProVersion, inst.ConfVersion)
	}
	if _, ok := inst.MetaData["pro_version"]; ok {
		t.Fatalf("pro_version should not be stored in metadata: %#v", inst.MetaData)
	}
	if _, ok := inst.MetaData["conf_version"]; ok {
		t.Fatalf("conf_version should not be stored in metadata: %#v", inst.MetaData)
	}
}

func TestEtcdInstanceConversionPreservesRuntimeFields(t *testing.T) {
	inst := &ServiceInstance{
		ClusterName:  "cluster-a",
		ServiceName:  "logic",
		InstanceId:   7,
		Host:         "127.0.0.1",
		Port:         9000,
		Healthy:      ServiceStatusHealth,
		Enable:       true,
		OnlineCount_: 12,
		Weight:       5,
		ProVersion:   1001,
		ConfVersion:  2002,
		UpdateTime:   "2026-05-08 17:30:00",
		MetaData:     map[string]string{"zone": "1", "pro_version": "legacy", "conf_version": "legacy"},
	}

	etcdInst := toEtcdInstance(inst)
	if etcdInst.ClusterName != inst.ClusterName ||
		etcdInst.ServiceName != inst.ServiceName ||
		etcdInst.InstanceId != inst.InstanceId ||
		etcdInst.Host != inst.Host ||
		etcdInst.Port != inst.Port ||
		etcdInst.Healthy != string(inst.Healthy) ||
		etcdInst.Enable != inst.Enable ||
		etcdInst.OnlineCount != inst.OnlineCount() ||
		etcdInst.Weight != inst.Weight ||
		etcdInst.UpdateTime != inst.UpdateTime ||
		etcdInst.ProVersion != inst.ProVersion ||
		etcdInst.ConfVersion != inst.ConfVersion {
		t.Fatalf("unexpected etcd instance: %#v", etcdInst)
	}
	if etcdInst.MetaData["pro_version"] != "legacy" || etcdInst.MetaData["conf_version"] != "legacy" {
		t.Fatalf("etcd metadata should preserve original keys: %#v", etcdInst.MetaData)
	}

	roundTrip := fromEtcdInstance(etcdInst)
	if roundTrip.ClusterName != inst.ClusterName ||
		roundTrip.ServiceName != inst.ServiceName ||
		roundTrip.InstanceId != inst.InstanceId ||
		roundTrip.Host != inst.Host ||
		roundTrip.Port != inst.Port ||
		roundTrip.Healthy != inst.Healthy ||
		roundTrip.Enable != inst.Enable ||
		roundTrip.OnlineCount() != inst.OnlineCount() ||
		roundTrip.Weight != inst.Weight ||
		roundTrip.ProVersion != inst.ProVersion ||
		roundTrip.ConfVersion != inst.ConfVersion ||
		roundTrip.UpdateTime != inst.UpdateTime ||
		roundTrip.MetaData["zone"] != "1" {
		t.Fatalf("unexpected service manager instance: %#v", roundTrip)
	}
	if roundTrip.MetaData["pro_version"] != "legacy" || roundTrip.MetaData["conf_version"] != "legacy" {
		t.Fatalf("round trip should preserve original metadata: %#v", roundTrip.MetaData)
	}
}

func TestServiceInstanceEqualAndCopyIncludeVersionFields(t *testing.T) {
	inst := &ServiceInstance{
		ClusterName:  "cluster-a",
		ServiceName:  "logic",
		InstanceId:   7,
		Host:         "127.0.0.1",
		Port:         9000,
		Healthy:      ServiceStatusHealth,
		Enable:       true,
		OnlineCount_: 12,
		Load_:        3,
		NetStatus:    NetConnect,
		Weight:       5,
		ProVersion:   1001,
		ConfVersion:  2002,
		UpdateTime:   "2026-05-08 17:30:00",
		MetaData:     map[string]string{"zone": "1"},
	}

	copied := inst.Copy()
	if copied.ProVersion != inst.ProVersion ||
		copied.ConfVersion != inst.ConfVersion ||
		copied.UpdateTime != inst.UpdateTime ||
		copied.NetState() != inst.NetState() {
		t.Fatalf("copy lost version/runtime fields: %#v", copied)
	}

	diff := inst.Copy()
	diff.ProVersion++
	if inst.Equal(diff) {
		t.Fatal("expected ProVersion change to affect equality")
	}

	diff = inst.Copy()
	diff.ConfVersion++
	if inst.Equal(diff) {
		t.Fatal("expected ConfVersion change to affect equality")
	}
}

func TestVersionTextToInt32PanicsOnOverflow(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected overflow to panic")
		}
	}()

	_ = versionTextToInt32("2147483648")
}

func TestVersionTextToInt32AcceptsMaxInt32(t *testing.T) {
	if got := versionTextToInt32("2147483647"); got != math.MaxInt32 {
		t.Fatalf("versionTextToInt32 returned %d, want %d", got, math.MaxInt32)
	}
}
