package eventbus

import (
	"context"

	"game/src/proto/eventpb"
)

type noopBus struct{}

func NewNoopBus() Bus {
	return noopBus{}
}

func (noopBus) PublishAsync(context.Context, *eventpb.Event, any) error { return nil }
func (noopBus) Publish(context.Context, *eventpb.Event, any) error      { return nil }
func (noopBus) Subscribe(eventpb.EVENT_TYPE, KeyFunc, bool, Handler)    {}
func (noopBus) Close()                                                  {}
