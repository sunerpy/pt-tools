package events

import (
	"encoding/json"
	"strings"
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

// TestEventBackwardCompat verifies that old Publish calls work without panic
// when receiving events with nil Payload.
func TestEventBackwardCompat(t *testing.T) {
	_, ch, cancel := Subscribe(4)
	defer cancel()

	e := Event{Type: ConfigChanged, Version: 1, Source: "test", At: time.Now()}
	Publish(e)

	select {
	case got := <-ch:
		if got.Payload != nil {
			t.Fatalf("expected nil Payload in backward-compat event, got: %v", got.Payload)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("no event received")
	}
}

// TestPublishWithPayloadRoundTrip verifies that PublishWithPayload encodes
// the payload correctly and can be unmarshaled back.
func TestPublishWithPayloadRoundTrip(t *testing.T) {
	type TestPayload struct {
		Name string `json:"name"`
		ID   int    `json:"id"`
	}

	_, ch, cancel := Subscribe(4)
	defer cancel()

	payload := TestPayload{Name: "alice", ID: 42}
	if err := PublishWithPayload(ConfigChanged, payload); err != nil {
		t.Fatalf("PublishWithPayload failed: %v", err)
	}

	select {
	case got := <-ch:
		var decoded TestPayload
		if err := json.Unmarshal(got.Payload, &decoded); err != nil {
			t.Fatalf("Unmarshal Payload failed: %v", err)
		}
		if decoded.Name != "alice" || decoded.ID != 42 {
			t.Fatalf("payload mismatch: got %+v, want {alice 42}", decoded)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("no event received")
	}
}

// TestPayloadOmitemptyJSON verifies that nil Payload is omitted in JSON
// serialization (omitempty tag).
func TestPayloadOmitemptyJSON(t *testing.T) {
	e := Event{Type: ConfigChanged, Version: 1, At: time.Now()}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	s := string(b)
	if strings.Contains(s, `"Payload"`) {
		t.Fatalf(`expected no "Payload" key in JSON when nil, got: %s`, s)
	}
}
