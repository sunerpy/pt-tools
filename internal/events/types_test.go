package events

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventTypeStringForm validates that all ChatOps event constants follow dotted.snake_case format
// and are unique.
func TestEventTypeStringForm(t *testing.T) {
	// Pattern: lowercase letters + dot + lowercase letters/underscores (dotted.snake_case)
	pattern := regexp.MustCompile(`^[a-z]+\.[a-z_]+$`)

	eventTypes := []EventType{
		EvtTorrentAdded,
		EvtTorrentCompleted,
		EvtTorrentFailed,
		EvtFreeEndingSoon,
		EvtFreeEnded,
		EvtDiskLow,
		EvtCleanupTriggered,
		EvtSiteLoginExpired,
		EvtSiteScrapedDaily,
		EvtNotificationDelivered,
		EvtNotificationFailed,
	}

	// Check count
	assert.Equal(t, 11, len(eventTypes), "should have exactly 11 ChatOps event types")

	// Track for uniqueness
	seen := make(map[string]bool)
	for _, evt := range eventTypes {
		str := string(evt)

		// Check format
		assert.True(t, pattern.MatchString(str),
			"event type %q should match pattern ^[a-z]+\\.[a-z_]+$", str)

		// Check uniqueness
		assert.False(t, seen[str],
			"event type %q is duplicated", str)
		seen[str] = true
	}

	// Verify all 11 are unique
	assert.Equal(t, 11, len(seen), "all 11 event types should be unique")
}

// TestPayloadJSONShape validates that all payload structs marshal to JSON with snake_case field names.
func TestPayloadJSONShape(t *testing.T) {
	testCases := []struct {
		name         string
		payload      interface{}
		expectedJSON string
	}{
		{
			name: "TorrentAddedPayload",
			payload: TorrentAddedPayload{
				TorrentID:      "abc123",
				SiteName:       "MTeam",
				Title:          "Test.Torrent",
				Size:           1024,
				DownloaderName: "qb1",
			},
			expectedJSON: `{"torrent_id":"abc123","site_name":"MTeam","title":"Test.Torrent","size":1024,"downloader_name":"qb1"}`,
		},
		{
			name: "TorrentCompletedPayload",
			payload: TorrentCompletedPayload{
				TorrentID: "def456",
				SiteName:  "HDSKY",
				Title:     "Another.Torrent",
			},
			expectedJSON: `{"torrent_id":"def456","site_name":"HDSKY","title":"Another.Torrent"}`,
		},
		{
			name: "TorrentFailedPayload",
			payload: TorrentFailedPayload{
				TorrentID: "ghi789",
				ErrorMsg:  "Download failed",
			},
			expectedJSON: `{"torrent_id":"ghi789","error_msg":"Download failed"}`,
		},
		{
			name: "FreeEndingSoonPayload",
			payload: FreeEndingSoonPayload{
				TorrentID:  "jkl012",
				SiteName:   "PTHome",
				FreeEndsAt: 1234567890,
			},
			expectedJSON: `{"torrent_id":"jkl012","site_name":"PTHome","free_ends_at":1234567890}`,
		},
		{
			name: "FreeEndedPayload",
			payload: FreeEndedPayload{
				TorrentID: "mno345",
				SiteName:  "BitHD",
				Title:     "Expired.Free",
			},
			expectedJSON: `{"torrent_id":"mno345","site_name":"BitHD","title":"Expired.Free"}`,
		},
		{
			name: "DiskLowPayload",
			payload: DiskLowPayload{
				FreeSpaceGB:   10,
				MinRequiredGB: 50,
				Message:       "Disk space low",
			},
			expectedJSON: `{"free_space_gb":10,"min_required_gb":50,"message":"Disk space low"}`,
		},
		{
			name: "CleanupTriggeredPayload",
			payload: CleanupTriggeredPayload{
				RemovedCount: 5,
				FreedSpaceGB: 100,
			},
			expectedJSON: `{"removed_count":5,"freed_space_gb":100}`,
		},
		{
			name: "SiteLoginExpiredPayload",
			payload: SiteLoginExpiredPayload{
				SiteName: "CHDBits",
				Message:  "Cookie expired",
			},
			expectedJSON: `{"site_name":"CHDBits","message":"Cookie expired"}`,
		},
		{
			name: "SiteScrapedDailyPayload",
			payload: SiteScrapedDailyPayload{
				SiteName:      "Gainers",
				TorrentsCount: 150,
			},
			expectedJSON: `{"site_name":"Gainers","torrents_count":150}`,
		},
		{
			name: "NotificationDeliveredPayload",
			payload: NotificationDeliveredPayload{
				NotifID:   "notif123",
				Channel:   "telegram",
				Recipient: "user123",
			},
			expectedJSON: `{"notif_id":"notif123","channel":"telegram","recipient":"user123"}`,
		},
		{
			name: "NotificationFailedPayload",
			payload: NotificationFailedPayload{
				NotifID:  "notif456",
				Channel:  "qq",
				ErrorMsg: "User not found",
			},
			expectedJSON: `{"notif_id":"notif456","channel":"qq","error_msg":"User not found"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.payload)
			require.NoError(t, err, "should marshal without error")

			actual := string(data)
			assert.Equal(t, tc.expectedJSON, actual,
				"marshaled JSON should match expected format with snake_case field names")
		})
	}
}
