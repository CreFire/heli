package direct

import (
	"context"
	"game/deps/etcd"
	"game/deps/selector"
	"reflect"
	"testing"
	"time"
)

func TestDirect(t *testing.T) {
	b := &Builder{}
	wn := b.Build(selector.NewNode(
		"http",
		"127.0.0.1:9090",
		&etcd.ServiceInstance{
			Host:        "127.0.0.1:9090",
			ServiceName: "helloworld",
			Weight:      10,
			ProVersion:  1,
		}))

	done := wn.Pick()
	if done == nil {
		t.Errorf("expect %v, got %v", nil, done)
	}
	time.Sleep(time.Millisecond * 10)
	done(context.Background(), selector.DoneInfo{})
	if !reflect.DeepEqual(float64(10), wn.Weight()) {
		t.Errorf("expect %v, got %v", float64(10), wn.Weight())
	}
	if time.Millisecond*20 <= wn.PickElapsed() {
		t.Errorf("20ms <= wn.PickElapsed()(%s)", wn.PickElapsed())
	}
	if time.Millisecond*10 >= wn.PickElapsed() {
		t.Errorf("10ms >= wn.PickElapsed()(%s)", wn.PickElapsed())
	}
}

func TestDirectDefaultWeight(t *testing.T) {
	b := &Builder{}
	wn := b.Build(selector.NewNode(
		"http",
		"127.0.0.1:9090",
		&etcd.ServiceInstance{
			Host:        "127.0.0.1:9090",
			ServiceName: "helloworld",
			ProVersion:  1,
		}))

	done := wn.Pick()
	if done == nil {
		t.Errorf("expect %v, got %v", nil, done)
	}
	time.Sleep(time.Millisecond * 10)
	done(context.Background(), selector.DoneInfo{})
	if !reflect.DeepEqual(float64(100), wn.Weight()) {
		t.Errorf("expect %v, got %v", float64(100), wn.Weight())
	}
	if time.Millisecond*20 <= wn.PickElapsed() {
		t.Errorf("time.Millisecond*20 <= wn.PickElapsed()(%s)", wn.PickElapsed())
	}
	if time.Millisecond*5 >= wn.PickElapsed() {
		t.Errorf("time.Millisecond*5 >= wn.PickElapsed()(%s)", wn.PickElapsed())
	}
}
