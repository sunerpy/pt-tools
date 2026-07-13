package extension

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))
	return db
}

func TestEnqueueAndListPending(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, Enqueue(db, PendingAction{
		Type:      ActionOpenTab,
		TargetURL: "https://hdsky.me/",
		SiteName:  "hdsky",
		Reason:    "login refresh",
	}))
	require.NoError(t, Enqueue(db, PendingAction{
		Type:      ActionOpenTab,
		TargetURL: "https://springsunday.net/",
		SiteName:  "springsunday",
		Reason:    "login refresh",
	}))

	actions, err := ListPending(db, 0)
	require.NoError(t, err)
	require.Len(t, actions, 2)
	assert.Equal(t, "hdsky", actions[0].SiteName)
	assert.Equal(t, "springsunday", actions[1].SiteName)
	for _, a := range actions {
		assert.Equal(t, ActionOpenTab, a.Type)
		assert.False(t, a.CreatedAt.IsZero())
		assert.False(t, a.ExpiresAt.IsZero())
		assert.True(t, a.ExpiresAt.After(a.CreatedAt))
		assert.Nil(t, a.AckedAt)
	}
}

func TestEnqueueRejectsInvalid(t *testing.T) {
	db := newTestDB(t)
	require.Error(t, Enqueue(db, PendingAction{TargetURL: "https://x"}))
	require.Error(t, Enqueue(db, PendingAction{Type: ActionOpenTab}))
}

func TestListPendingSinceFilter(t *testing.T) {
	db := newTestDB(t)

	old := time.Now().UTC().Add(-2 * time.Hour)
	recent := time.Now().UTC().Add(-1 * time.Minute)

	require.NoError(t, Enqueue(db, PendingAction{
		Type:      ActionOpenTab,
		TargetURL: "https://a.example/",
		SiteName:  "a",
		CreatedAt: old,
		ExpiresAt: old.Add(DefaultTTL),
	}))
	require.NoError(t, Enqueue(db, PendingAction{
		Type:      ActionOpenTab,
		TargetURL: "https://b.example/",
		SiteName:  "b",
		CreatedAt: recent,
		ExpiresAt: recent.Add(DefaultTTL),
	}))

	all, err := ListPending(db, 0)
	require.NoError(t, err)
	require.Len(t, all, 2)

	cutoff := old.Add(time.Hour).Unix()
	filtered, err := ListPending(db, cutoff)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "b", filtered[0].SiteName)
}

func TestAckIdempotent(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, Enqueue(db, PendingAction{
		Type:      ActionOpenTab,
		TargetURL: "https://hdsky.me/",
		SiteName:  "hdsky",
	}))
	pending, err := ListPending(db, 0)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	id := pending[0].ID

	require.NoError(t, Ack(db, id))
	require.NoError(t, Ack(db, id))

	remaining, err := ListPending(db, 0)
	require.NoError(t, err)
	assert.Empty(t, remaining)

	var row PendingAction
	require.NoError(t, db.First(&row, id).Error)
	require.NotNil(t, row.AckedAt)
}

func TestAckUnknownID(t *testing.T) {
	db := newTestDB(t)
	require.Error(t, Ack(db, 9999))
	require.Error(t, Ack(db, 0))
}

func TestPurgeExpired(t *testing.T) {
	db := newTestDB(t)

	expired := time.Now().UTC().Add(-25 * time.Hour)
	require.NoError(t, Enqueue(db, PendingAction{
		Type:      ActionOpenTab,
		TargetURL: "https://old.example/",
		SiteName:  "old",
		CreatedAt: expired,
		ExpiresAt: expired.Add(DefaultTTL),
	}))
	require.NoError(t, Enqueue(db, PendingAction{
		Type:      ActionOpenTab,
		TargetURL: "https://fresh.example/",
		SiteName:  "fresh",
	}))

	beforePurge, err := ListPending(db, 0)
	require.NoError(t, err)
	require.Len(t, beforePurge, 1)

	deleted, err := PurgeExpired(db, time.Now())
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)

	var total int64
	require.NoError(t, db.Model(&PendingAction{}).Count(&total).Error)
	assert.EqualValues(t, 1, total)
}

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
