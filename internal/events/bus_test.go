package events

import (
	"testing"
	"time"
)

func TestPublishSubscribe(t *testing.T) {
	id, ch, cancel := Subscribe(4)
	_ = id
	defer cancel()
	e := Event{Type: ConfigChanged, Version: time.Now().UnixNano(), Source: "test", At: time.Now()}
	Publish(e)
	select {
	case got := <-ch:
		if got.Version != e.Version || got.Type != e.Type {
			t.Fatalf("event mismatch")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("no event received")
	}
}

func TestCancelUnsubscribe(t *testing.T) {
	_, ch, cancel := Subscribe(1)
	cancel()
	// After cancel, channel is closed; ensure non-blocking behavior
	Publish(Event{Type: ConfigChanged, Version: time.Now().UnixNano(), Source: "x", At: time.Now()})
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("expected closed channel")
		}
	default:
	}
}
