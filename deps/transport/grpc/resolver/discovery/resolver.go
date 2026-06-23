package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"game/deps/etcd"
	"game/deps/xlog"
	"net"
	"strconv"
	"time"

	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"
)

type discoveryResolver struct {
	watchCh <-chan *etcd.WatchResponse
	cc      resolver.ClientConn

	ctx    context.Context
	cancel context.CancelFunc

	insecure    bool
	debugLog    bool
	selectorKey string
	subsetSize  int
}

func (r *discoveryResolver) watch() {
	instances := make(map[string]*etcd.ServiceInstance)
	for {
		select {
		case <-r.ctx.Done():
			return
		case resp, ok := <-r.watchCh:
			if !ok {
				return
			}
			if resp == nil {
				continue
			}
			if resp.Err != nil {
				if errors.Is(resp.Err, context.Canceled) {
					return
				}
				xlog.Infof("[resolver] Failed to watch discovery endpoint: %v", resp.Err)
				time.Sleep(time.Second)
				continue
			}
			if len(resp.Events) == 0 {
				continue
			}

			reset := true
			for _, ev := range resp.Events {
				if ev.Type != etcd.EventTypeSnapshot {
					reset = false
					break
				}
			}
			if reset {
				instances = make(map[string]*etcd.ServiceInstance, len(resp.Events))
			}
			for _, ev := range resp.Events {
				switch ev.Type {
				case etcd.EventTypePut, etcd.EventTypeSnapshot:
					if ev.Inst != nil {
						instances[ev.Key] = ev.Inst
					}
				case etcd.EventTypeDelete:
					delete(instances, ev.Key)
				}
			}

			r.update(snapshotInstances(instances))
		}
	}
}

func (r *discoveryResolver) update(ins []*etcd.ServiceInstance) {
	var (
		endpoints          = make(map[string]struct{})
		endpointByInstance = make(map[*etcd.ServiceInstance]string, len(ins))
		filtered           = make([]*etcd.ServiceInstance, 0, len(ins))
	)
	for _, in := range ins {
		ept := endpointFromInstance(in)
		if ept == "" {
			continue
		}
		// filter redundant endpoints
		if _, ok := endpoints[ept]; ok {
			continue
		}
		endpoints[ept] = struct{}{}
		endpointByInstance[in] = ept
		filtered = append(filtered, in)
	}
	if r.subsetSize != 0 {
		//filtered = subset.Subset(r.selectorKey, filtered, r.subsetSize)
	}

	addrs := make([]resolver.Address, 0, len(filtered))
	for _, in := range filtered {
		ept := endpointByInstance[in]
		if ept == "" {
			continue
		}
		addr := resolver.Address{
			ServerName: in.ServiceName,
			//Attributes: parseAttributes(in.Metadata).WithValue("rawServiceInstance", in),
			Addr: ept,
		}
		addrs = append(addrs, addr)
	}
	if len(addrs) == 0 {
		xlog.Errorf("[resolver] Zero endpoint found,refused to write, instances: %v", ins)
		return
	}
	err := r.cc.UpdateState(resolver.State{Addresses: addrs})
	if err != nil {
		xlog.Errorf("[resolver] failed to update state: %s", err)
	}
	if r.debugLog {
		b, _ := json.Marshal(filtered)
		xlog.Infof("[resolver] update instances: %s", b)
	}
}

func (r *discoveryResolver) Close() {
	r.cancel()
}

func (r *discoveryResolver) ResolveNow(_ resolver.ResolveNowOptions) {}

func endpointFromInstance(in *etcd.ServiceInstance) string {
	if in == nil {
		return ""
	}
	if in.Host == "" {
		return ""
	}
	if in.Port > 0 {
		return net.JoinHostPort(in.Host, strconv.Itoa(int(in.Port)))
	}
	return in.Host
}

func parseAttributes(md map[string]string) (a *attributes.Attributes) {
	for k, v := range md {
		a = a.WithValue(k, v)
	}
	return a
}

// snapshotInstances 将输入的map类型的实例转换为切片类型并返回
// 该函数接收一个map类型的参数，键为字符串类型，值为etcd.ServiceInstance指针类型
// 返回一个etcd.ServiceInstance指针类型的切片
func snapshotInstances(instances map[string]*etcd.ServiceInstance) []*etcd.ServiceInstance {
	// 创建一个与输入map长度相同的切片，用于存储结果
	result := make([]*etcd.ServiceInstance, 0, len(instances))
	for _, inst := range instances {
		result = append(result, inst)
	}
	return result
}
