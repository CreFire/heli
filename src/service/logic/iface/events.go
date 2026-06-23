package iface

type IEvent interface {
	EventType() int
}

type EventHandler func(event IEvent)

type IEventBus interface {
	Publish(event IEvent)
	Subscribe(eventType int, handler EventHandler)
}
