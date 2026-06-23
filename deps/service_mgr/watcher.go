package servicemgr

import (
	"context"
	"fmt"
	"game/deps/basal"
	"game/deps/etcd"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type watcher struct {
	client       *etcd.EtcdClient
	multiWatcher *etcd.MultiWatcher
}

func newWatcher(client *etcd.EtcdClient) *watcher {
	return &watcher{client: client}
}

func (w *watcher) Watch(ctx context.Context, specs ...ListenSpec) (<-chan *WatchEvent, error) {
	keys := make([]string, 0, len(specs))
	for _, spec := range specs {
		keys = append(keys, etcd.RegistrarPrefix(spec.Cluster, spec.ServiceName))
	}

	multiWatcher, err := etcd.NewMultiWatcher(w.client.Client, keys, ctx, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to create MultiWatcher: %w", err)
	}
	w.multiWatcher = multiWatcher

	outCh := make(chan *WatchEvent, 128)
	basal.SafeGo(func() {
		defer close(outCh)
		for {
			select {
			case <-ctx.Done():
				return
			case resp, ok := <-multiWatcher.EventChan():
				if !ok {
					return
				}
				if resp.Err != nil {
					continue
				}
				for _, event := range resp.Events {
					_, serviceName, serverID := etcd.ParseServerInfoFromKey(event.Key)
					watchEvent := &WatchEvent{
						Type:        convertEventType(event.Type),
						ServiceName: serviceName,
						InstanceID:  int32(serverID),
						Instance:    fromEtcdInstance(event.Inst),
					}
					select {
					case <-ctx.Done():
						return
					case outCh <- watchEvent:
					}
				}
			}
		}
	})

	return outCh, nil
}

func (w *watcher) Close() {
	if w.multiWatcher != nil {
		w.multiWatcher.Close()
	}
}
