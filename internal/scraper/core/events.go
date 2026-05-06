package core

import "context"

const (
	EventTypeTorrentCompleted = "TorrentCompleted"
	EventTypeLibraryScanned   = "LibraryScanned"
)

type Event struct {
	Type    string
	Payload any
}

type EventBus interface {
	Publish(ctx context.Context, ev Event) error
	Subscribe(ctx context.Context, eventType string) (<-chan Event, func(), error)
}

type NopEventBus struct{}

func (NopEventBus) Publish(context.Context, Event) error {
	return nil
}

func (NopEventBus) Subscribe(context.Context, string) (<-chan Event, func(), error) {
	ch := make(chan Event)
	close(ch)
	return ch, func() {}, nil
}
