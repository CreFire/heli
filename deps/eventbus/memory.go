package eventbus

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"game/deps/async"
	"game/deps/xlog"
	"game/src/proto/eventpb"
)

var (
	ErrClosed = errors.New("eventbus closed")
)

type EventHandler struct {
	Handlers []Handler
	InAsync  bool
	KeyFunc  KeyFunc
	Event    eventpb.EVENT_TYPE
}

func NewMemoryBus(a *async.Async, defaultKeyFunc KeyFunc) Bus {
	if a == nil {
		panic("eventbus async must not be nil")
	}
	return &memoryBus{
		async:          a,
		subs:           make(map[eventpb.EVENT_TYPE]*EventHandler),
		defaultKeyFunc: defaultKeyFunc,
	}
}

type memoryBus struct {
	async          *async.Async
	defaultKeyFunc KeyFunc
	subs           map[eventpb.EVENT_TYPE]*EventHandler // built during init, then treated as read-only
	closed         atomic.Bool
	mu             sync.RWMutex
}

func (b *memoryBus) PublishAsync(ctx context.Context, evt *eventpb.Event, arg any) error {
	if b.closed.Load() {
		xlog.Warnf("publish event failed, cause:event close, type:%v", evt.GetType())
		return nil
	}

	b.mu.RLock()
	handlers := b.subs[evt.GetType()]
	b.mu.RUnlock()
	if handlers == nil {
		return nil
	}

	if !handlers.InAsync {
		xlog.Errorf("event is not async event , %v", evt.Type)
		return fmt.Errorf("event is not async event , %v", evt.Type)
	}

	key := uint64(0)
	if handlers.KeyFunc != nil {
		key = handlers.KeyFunc(evt)
	}
	dispatch := func() {
		err := b.dispatch(context.Background(), evt, handlers.Handlers, arg)
		if err != nil {
			xlog.Errorf("event handle failed, event: %v arg: %v err:%v", evt.EventId, arg, err)
		}
	}
	if key == 0 {
		return b.async.Post("eventbus", dispatch)
	}
	return b.async.PostFixed(key, "eventbus-fixed", dispatch)

}

func (b *memoryBus) Publish(ctx context.Context, evt *eventpb.Event, arg any) error {
	if evt == nil {
		return nil
	}
	if b.closed.Load() {
		return ErrClosed
	}

	b.mu.RLock()
	handlers := b.subs[evt.GetType()]
	b.mu.RUnlock()

	if handlers == nil {
		return nil
	}
	return b.dispatch(ctx, evt, handlers.Handlers, arg)
}

func (b *memoryBus) Subscribe(t eventpb.EVENT_TYPE, keyFunc KeyFunc, inAsync bool, h Handler) {
	if h == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if v, ok := b.subs[t]; ok {
		v.Handlers = append(v.Handlers, h)
		if v.InAsync != inAsync {
			xlog.Errorf("event InAsync not Same!, Event %v", t)
		}

		if keyFunc != nil {
			v.KeyFunc = keyFunc
		}
		return
	}

	if keyFunc == nil {
		keyFunc = b.defaultKeyFunc
	}

	b.subs[t] = &EventHandler{
		Event:    t,
		Handlers: []Handler{h},
		InAsync:  inAsync,
		KeyFunc:  keyFunc,
	}
}

func (b *memoryBus) Close() {
	b.closed.Store(true)
}

func (b *memoryBus) dispatch(ctx context.Context, evt *eventpb.Event, handlers []Handler, arg any) error {
	var err error
	for _, h := range handlers {
		if hErr := b.handle(ctx, h, evt, arg); hErr != nil {
			err = errors.Join(err, hErr)
		}
	}
	return err
}

func (b *memoryBus) handle(ctx context.Context, handler Handler, evt *eventpb.Event, arg any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Join(err, fmt.Errorf("event handler panic: %v", r))
		}
	}()
	return handler(ctx, evt, arg)
}
