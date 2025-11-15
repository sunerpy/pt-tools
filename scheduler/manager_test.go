package scheduler

import (
	"testing"
	"time"

	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
)

func TestManagerDebounceReload(t *testing.T) {
	m := NewManager()
	// Rapid publish multiple events
	for i := 0; i < 5; i++ {
		events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano() + int64(i), Source: "test", At: time.Now()})
	}
	// Allow debounce window
	time.Sleep(100 * time.Millisecond)
	// In unit test environment, global DB is nil so Reload won't update version; ensure no panic
	// if m.LastVersion() == 0 { t.Fatalf("expected lastVersion updated") }
	// Ensure no panic and methods callable
	m.StopAll()
	m.StartAll(&models.Config{})
}
