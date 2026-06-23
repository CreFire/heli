package etcd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"game/deps/kit"
	"game/deps/xlog"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EventType 定义了 Watcher 事件的类型
type EventType string

const (
	// EventTypePut 表示一个 key 被创建或更新
	EventTypePut EventType = "PUT"
	// EventTypeDelete 表示一个 key 被删除
	EventTypeDelete EventType = "DELETE"
	// EventTypeSnapshot 表示一次全量数据快照
	EventTypeSnapshot EventType = "SNAPSHOT"
)

const (
	watchRetryDelay         = 3 * time.Second
	watcherEventBuffer      = 512
	multiWatcherEventBuffer = 512
)

// EtcdWatchEvent 代表一个具体的 watch 事件
type EtcdWatchEvent struct {
	Type  EventType
	Key   string
	Value []byte
	Inst  *ServiceInstance
}

// WatchResponse 是发送给业务层的 watch 响应
type WatchResponse struct {
	Events []*EtcdWatchEvent
	Err    error
}

// ClientWatcher 封装了 etcd 的 watch 机制，提供了自动重连和快照同步功能
type ClientWatcher struct {
	client    *clientv3.Client
	key       string
	opts      []clientv3.OpOption
	ctx       context.Context
	cancel    context.CancelFunc
	eventChan chan *WatchResponse
	done      chan struct{}
}

// NewClientWatcher 创建一个新的 ClientWatcher 实例
// 它会立即启动一个后台 goroutine 来监听指定的 key
func NewClientWatcher(client *clientv3.Client, key string, pCtx context.Context, opts ...clientv3.OpOption) (*ClientWatcher, error) {
	if client == nil {
		return nil, errors.New("etcd client is nil")
	}

	if pCtx == nil {
		pCtx = context.Background()
	}
	// 使用父 context 创建一个可取消的 context，用于控制 watcher 的生命周期
	ctx, cancel := context.WithCancel(pCtx)

	w := &ClientWatcher{
		client:    client,
		key:       key,
		opts:      opts,
		ctx:       ctx,
		cancel:    cancel,
		eventChan: make(chan *WatchResponse, watcherEventBuffer),
		done:      make(chan struct{}),
	}

	go w.watchLoop()

	return w, nil
}

// EventChan 返回一个只读的 channel，业务层可以从中接收 WatchResponse
func (w *ClientWatcher) EventChan() <-chan *WatchResponse {
	return w.eventChan
}

// Close 停止 watcher 并关闭事件 channel
func (w *ClientWatcher) Close() {
	w.cancel()
	<-w.done
}

// watchLoop 是 watcher 的主循环，负责处理连接和重连
func (w *ClientWatcher) watchLoop() {
	defer kit.Exception(nil)
	defer close(w.eventChan)
	defer close(w.done)

	for {
		if err := w.watchOnce(); err != nil {
			if isContextDone(err) {
				return
			}
			w.reportError(err)
			if !sleepContext(w.ctx, watchRetryDelay) {
				return
			}
			continue
		}
	}
}

func (w *ClientWatcher) watchOnce() error {
	events, revision, err := w.loadSnapshot()
	if err != nil {
		return err
	}
	sendWatchResponse(w.ctx, w.eventChan, &WatchResponse{Events: events})
	xlog.Infof("[ClientWatcher] snapshot key '%s' revision %d count %d", w.key, revision, len(events))

	return w.streamEvents(revision + 1)
}

func (w *ClientWatcher) loadSnapshot() ([]*EtcdWatchEvent, int64, error) {
	resp, err := w.client.Get(w.ctx, w.key, w.opts...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get initial snapshot for key '%s': %w", w.key, err)
	}

	events := make([]*EtcdWatchEvent, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		events = append(events, &EtcdWatchEvent{
			Type:  EventTypeSnapshot,
			Key:   string(kv.Key),
			Value: kv.Value,
			Inst:  serviceInstanceFromValue(kv.Key, kv.Value),
		})
		xlog.Debugf("[ClientWatcher] snapshot key: %s, value: %s", kv.Key, kv.Value)
	}
	return events, resp.Header.Revision, nil
}

func (w *ClientWatcher) streamEvents(revision int64) error {
	watchChan := w.client.Watch(w.ctx, w.key, buildWatchOptions(w.opts, revision)...)
	xlog.Infof("[ClientWatcher] watching key '%s' from revision %d", w.key, revision)

	for {
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case resp, ok := <-watchChan:
			if !ok {
				return errors.New("watch channel closed by etcd server")
			}
			if err := w.ctx.Err(); err != nil {
				return err
			}
			if err := watchResponseError(resp); err != nil {
				return err
			}

			events := watchEventsFromEtcd(resp.Events)
			if len(events) == 0 {
				continue
			}
			sendWatchResponse(w.ctx, w.eventChan, &WatchResponse{Events: events})
		}
	}
}

func (w *ClientWatcher) reportError(err error) {
	xlog.Warnf("[ClientWatcher] watch on key '%s' failed, will retry after %s: %v", w.key, watchRetryDelay, err)
	sendWatchResponse(w.ctx, w.eventChan, &WatchResponse{Err: err})
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func isContextDone(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func watchResponseError(resp clientv3.WatchResponse) error {
	if resp.Canceled {
		if resp.CompactRevision != 0 {
			return fmt.Errorf("watch compacted at revision %d", resp.CompactRevision)
		}
		if err := resp.Err(); err != nil {
			return fmt.Errorf("watch canceled: %w", err)
		}
		return errors.New("watch canceled")
	}
	if err := resp.Err(); err != nil {
		return fmt.Errorf("watch response error: %w", err)
	}
	return nil
}

func watchEventsFromEtcd(events []*clientv3.Event) []*EtcdWatchEvent {
	out := make([]*EtcdWatchEvent, 0, len(events))
	for _, ev := range events {
		event := watchEventFromEtcd(ev)
		if event == nil {
			continue
		}
		out = append(out, event)
	}
	return out
}

func watchEventFromEtcd(ev *clientv3.Event) *EtcdWatchEvent {
	if ev == nil || ev.Kv == nil {
		return nil
	}

	switch ev.Type {
	case clientv3.EventTypePut:
		xlog.Debugf("[ClientWatcher] put event key: %s, value: %s", ev.Kv.Key, ev.Kv.Value)
		return &EtcdWatchEvent{
			Type:  EventTypePut,
			Key:   string(ev.Kv.Key),
			Value: ev.Kv.Value,
			Inst:  serviceInstanceFromValue(ev.Kv.Key, ev.Kv.Value),
		}
	case clientv3.EventTypeDelete:
		xlog.Debugf("[ClientWatcher] delete event key: %s", ev.Kv.Key)
		return &EtcdWatchEvent{
			Type: EventTypeDelete,
			Key:  string(ev.Kv.Key),
		}
	default:
		return nil
	}
}

// MultiWatcher 可同时监听多个 key 并将事件聚合到一个 channel 中
type MultiWatcher struct {
	keys      []string
	ctx       context.Context
	cancel    context.CancelFunc
	eventChan chan *WatchResponse
	watchers  []*ClientWatcher
	wg        sync.WaitGroup
	done      chan struct{}
}

// NewMultiWatcher 创建一个新的 MultiWatcher 实例
// 为每个 key 启动一个 ClientWatcher，并聚合所有事件到一个 channel 中
func NewMultiWatcher(client *clientv3.Client, keys []string, pCtx context.Context, opts ...clientv3.OpOption) (*MultiWatcher, error) {
	if client == nil {
		return nil, errors.New("etcd client is nil")
	}
	if len(keys) == 0 {
		return nil, errors.New("keys cannot be empty")
	}

	if pCtx == nil {
		pCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(pCtx)

	mw := &MultiWatcher{
		keys:      keys,
		ctx:       ctx,
		cancel:    cancel,
		eventChan: make(chan *WatchResponse, multiWatcherEventBuffer),
		done:      make(chan struct{}),
	}

	for _, key := range keys {
		if err := mw.addWatcher(client, key, opts...); err != nil {
			mw.cancel()
			mw.closeWatchers()
			mw.wg.Wait()
			return nil, err
		}
	}

	go mw.closeWhenDone()
	return mw, nil
}

func (mw *MultiWatcher) addWatcher(client *clientv3.Client, key string, opts ...clientv3.OpOption) error {
	watcher, err := NewClientWatcher(client, key, mw.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create watcher for key '%s': %w", key, err)
	}

	mw.watchers = append(mw.watchers, watcher)
	mw.wg.Add(1)
	go mw.forward(watcher)
	return nil
}

func (mw *MultiWatcher) forward(watcher *ClientWatcher) {
	defer kit.Exception(nil)
	defer mw.wg.Done()

	for {
		select {
		case resp, ok := <-watcher.EventChan():
			if !ok {
				return
			}
			sendWatchResponse(mw.ctx, mw.eventChan, resp)
		case <-mw.ctx.Done():
			return
		}
	}
}

// EventChan 返回一个只读的 channel，业务层可以从中接收 WatchResponse
func (mw *MultiWatcher) EventChan() <-chan *WatchResponse {
	return mw.eventChan
}

// Close 关闭所有底层的 watcher 并关闭事件 channel
func (mw *MultiWatcher) Close() {
	mw.cancel()
	<-mw.done
	xlog.Infof("[MultiWatcher] Closed watcher for keys: %v", mw.keys)
}

func (mw *MultiWatcher) closeWhenDone() {
	<-mw.ctx.Done()
	mw.closeWatchers()
	mw.wg.Wait()
	close(mw.eventChan)
	close(mw.done)
}

func (mw *MultiWatcher) closeWatchers() {
	for _, watcher := range mw.watchers {
		watcher.Close()
	}
}

// sendWatchResponse applies backpressure instead of dropping discovery events.
func sendWatchResponse(ctx context.Context, ch chan *WatchResponse, resp *WatchResponse) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- resp:
		return true
	}
}

func buildWatchOptions(opts []clientv3.OpOption, revision int64) []clientv3.OpOption {
	watchOpts := make([]clientv3.OpOption, 0, len(opts)+1)
	watchOpts = append(watchOpts, opts...)
	watchOpts = append(watchOpts, clientv3.WithRev(revision))
	return watchOpts
}
