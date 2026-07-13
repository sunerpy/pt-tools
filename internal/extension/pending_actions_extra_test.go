package extension

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNilDBGuards(t *testing.T) {
	assert.Error(t, Enqueue(nil, PendingAction{Type: ActionOpenTab, TargetURL: "https://x"}))
	assert.Error(t, AutoMigrate(nil))
	assert.Error(t, Ack(nil, 1))

	_, err := ListPending(nil, 0)
	assert.Error(t, err)

	_, err = PurgeExpired(nil, time.Now())
	assert.Error(t, err)
}

func TestTableName(t *testing.T) {
	assert.Equal(t, "extension_pending_actions", PendingAction{}.TableName())
}

func TestEnqueueRespectsExplicitTimestamps(t *testing.T) {
	db := newTestDB(t)
	created := time.Now().UTC().Add(-3 * time.Hour)
	expires := created.Add(48 * time.Hour)
	require.NoError(t, Enqueue(db, PendingAction{
		Type:      ActionOpenTab,
		TargetURL: "https://x.example/",
		SiteName:  "x",
		CreatedAt: created,
		ExpiresAt: expires,
	}))

	actions, err := ListPending(db, 0)
	require.NoError(t, err)
	require.Len(t, actions, 1)
	assert.WithinDuration(t, expires, actions[0].ExpiresAt, time.Second,
		"explicit ExpiresAt must not be overwritten by the default TTL")
}

func TestPurgeExpiredNothingToDelete(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, Enqueue(db, PendingAction{Type: ActionOpenTab, TargetURL: "https://fresh/"}))
	deleted, err := PurgeExpired(db, time.Now())
	require.NoError(t, err)
	assert.EqualValues(t, 0, deleted)
}
