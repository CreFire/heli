package filter

import (
	"context"
	"game/deps/etcd"
	"game/deps/selector"

	"reflect"
	"testing"
)

func TestVersion(t *testing.T) {
	f := Version("2")
	var nodes []selector.Node
	nodes = append(nodes, selector.NewNode(
		"http",
		"127.0.0.1:9090",
		&etcd.ServiceInstance{
			Host:        "127.0.0.1:9090",
			ServiceName: "helloworld",
			ProVersion:  1,
		}))

	nodes = append(nodes, selector.NewNode(
		"http",
		"127.0.0.2:9090",
		&etcd.ServiceInstance{
			Host:        "127.0.0.2:9090",
			ServiceName: "helloworld",
			ProVersion:  2,
		}))

	nodes = f(context.Background(), nodes)
	if !reflect.DeepEqual(len(nodes), 1) {
		t.Errorf("expect %v, got %v", 1, len(nodes))
	}
	if !reflect.DeepEqual(nodes[0].Address(), "127.0.0.2:9090") {
		t.Errorf("expect %v, got %v", nodes[0].Address(), "127.0.0.2:9090")
	}
}
