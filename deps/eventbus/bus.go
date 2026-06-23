package eventbus

import (
	"context"

	"game/src/proto/eventpb"
)

type Handler func(context.Context, *eventpb.Event, any) error
type KeyFunc func(*eventpb.Event) uint64

type Bus interface {
	PublishAsync(context.Context, *eventpb.Event, any) error
	Publish(context.Context, *eventpb.Event, any) error
	// inAsync: true => dispatch via async pool; false => dispatch inline
	// keyFunc: optional sharding key for async; nil falls back to bus default
	Subscribe(eventpb.EVENT_TYPE, KeyFunc, bool, Handler)
	Close()
}
