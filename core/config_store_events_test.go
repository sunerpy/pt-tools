package core

import (
	"testing"
	"time"

	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
)

// Verify that Save* publishes ConfigChanged events
func TestPublishOnSave(t *testing.T) {
	db := newTempDB(t)
	store := NewConfigStore(db)
	_, ch, cancel := events.Subscribe(4)
	defer cancel()
	if err := store.SaveGlobal(models.SettingsGlobal{DefaultIntervalMinutes: 20, DownloadDir: "download"}); err != nil {
		t.Fatalf("save global: %v", err)
	}
	select {
	case e := <-ch:
		if e.Type != events.ConfigChanged {
			t.Fatalf("expected ConfigChanged")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("no event received on SaveGlobal")
	}
}
