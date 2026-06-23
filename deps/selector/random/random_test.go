package random

import (
	"context"
	"game/deps/etcd"
	"game/deps/selector"
	"game/deps/selector/filter"
	"testing"
)

func TestWrr(t *testing.T) {
	random := New()
	var nodes []selector.Node
	nodes = append(nodes, selector.NewNode(
		"http",
		"127.0.0.1:8080",
		&etcd.ServiceInstance{
			Host:       "127.0.0.1:8080",
			Weight:     10,
			ProVersion: 2,
		}))
	nodes = append(nodes, selector.NewNode(
		"http",
		"127.0.0.1:9090",
		&etcd.ServiceInstance{
			Host:       "127.0.0.1:9090",
			Weight:     20,
			ProVersion: 2,
		}))
	random.Apply(nodes)
	var count1, count2 int
	for range 200 {
		n, done, err := random.Select(context.Background(), selector.WithNodeFilter(filter.Version("2")))
		if err != nil {
			t.Errorf("expect no error, got %v", err)
		}
		if done == nil {
			t.Errorf("expect not nil, got:%v", done)
		}
		if n == nil {
			t.Errorf("expect not nil, got:%v", n)
		}
		done(context.Background(), selector.DoneInfo{})
		if n.Address() == "127.0.0.1:8080" {
			count1++
		} else if n.Address() == "127.0.0.1:9090" {
			count2++
		}
	}
	if count1 <= 80 {
		t.Errorf("count1(%v) <= 80", count1)
	}
	if count1 >= 120 {
		t.Errorf("count1(%v) >= 120", count1)
	}
	if count2 <= 80 {
		t.Errorf("count2(%v) <= 80", count2)
	}
	if count2 >= 120 {
		t.Errorf("count2(%v) >= 120", count2)
	}
}

func TestEmpty(t *testing.T) {
	b := &Balancer{}
	_, _, err := b.Pick(context.Background(), []selector.WeightedNode{})
	if err == nil {
		t.Errorf("expect nil, got %v", err)
	}
}
