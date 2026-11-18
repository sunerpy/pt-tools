package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// RSS 的两种通知带上事件，内置的标题与正文和原来逐字节相同
func TestRSSPayloadsCarryEvents(t *testing.T) {
	pub := time.Date(2026, 10, 12, 8, 0, 0, 0, time.Local)
	got := renderAllPayload(RSSItemEvent{
		RSS: &models.RSSConfig{Name: "HDTime 电影"}, SiteName: "hdtime", TorrentID: "300101",
		FeedItem: &gofeed.Item{Title: "Fake.Movie", PublishedParsed: &pub, Link: "https://hdtime.org/details.php?id=300101"},
	})
	assert.Equal(t, "Fake.Movie", got.Title)
	assert.Equal(t, "🆕 [hdtime] Fake.Movie\n\n📅 2026-10-12 08:00\n🔗 https://hdtime.org/details.php?id=300101", got.Text)
	require.NotNil(t, got.Event)
	assert.Equal(t, notify.EventRSSNew, got.Event.Type)
	assert.Equal(t, map[string]any{
		"site": "hdtime", "rss": "HDTime 电影", "torrent_title": "Fake.Movie", "torrent_id": "300101",
		"link": "https://hdtime.org/details.php?id=300101", "published": "2026-10-12 08:00",
	}, got.Event.Fields)

	end := time.Now().Add(3*time.Hour + 30*time.Second)
	m := renderFilteredPayload(RSSFilteredEvent{
		RSS: &models.RSSConfig{Name: "HDTime 电影"}, SiteName: "hdtime", TorrentID: "300101",
		Rule: &models.FilterRule{Name: "只要免费"},
		Torrent: &v2.TorrentItem{
			Title: "Fake.Movie", Subtitle: "假电影", URL: "https://hdtime.org/details.php?id=300101",
			SizeBytes: 2 << 30, Seeders: 12, Leechers: 3, HasHR: true,
			DiscountLevel: v2.DiscountFree, DiscountEndTime: end,
		},
	})
	assert.Equal(t, "🎯 [hdtime] Fake.Movie\n\n📦 2.00 GB\n🆓 免费 (剩余 3h0min)\n📌 匹配规则：只要免费\n🔗 https://hdtime.org/details.php?id=300101", m.Text)
	require.NotNil(t, m.Event)
	f := m.Event.Fields
	assert.Equal(t, notify.EventRSSMatched, m.Event.Type)
	assert.Equal(t, "2.00 GB", f["size"])
	assert.Equal(t, int64(2<<30), f["size_bytes"])
	assert.Equal(t, true, f["free"])
	assert.Equal(t, end.Local().Format(time.DateTime), f["free_end"])
	assert.Equal(t, "3h0min", f["free_remaining"])
	assert.Equal(t, "只要免费", f["rule"])
	assert.Equal(t, "假电影", f["subtitle"])
	assert.Equal(t, int64(12), f["seeders"])
	assert.Equal(t, int64(3), f["leechers"])
	assert.Equal(t, true, f["has_hr"])
	assert.Equal(t, "https://hdtime.org/details.php?id=300101", f["link"])
}

type capturePush struct {
	mu  sync.Mutex
	got []Notification
}

func (c *capturePush) Push(_ context.Context, n Notification) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.got = append(c.got, n)
	return nil
}

// 合并发送：一条照它自己的事件；多条时是 rss.digest，各条的站点与种子取自它们自己的事件
func TestDigestFlushCarriesEvents(t *testing.T) {
	db := setupRSSNotifierDB(t)
	mk := func(id string) models.RSSNotificationLog {
		row := models.RSSNotificationLog{RSSID: 1, SiteName: "hdtime", TorrentID: id, NotifyKind: "all", NotificationConfID: 7, Result: "pending", CreatedAt: time.Now(), UpdatedAt: time.Now()}
		require.NoError(t, db.Create(&row).Error)
		return row
	}
	a, b := mk("1"), mk("2")
	evA := renderAllPayload(RSSItemEvent{SiteName: "hdtime", TorrentID: "1", FeedItem: &gofeed.Item{Title: "A"}})
	evB := renderAllPayload(RSSItemEvent{SiteName: "hdtime", TorrentID: "2", FeedItem: &gofeed.Item{Title: "B"}})
	push := &capturePush{}
	NewRSSDigestFlush(db, push, nil)(context.Background(), 7, []notify.DigestItem{
		{LogID: a.ID, Title: evA.Title, Text: evA.Text, Event: evA.Event},
		{LogID: b.ID, Title: evB.Title, Text: evB.Text, Event: evB.Event},
	})
	require.Len(t, push.got, 1)
	ev := push.got[0].Event
	require.NotNil(t, ev)
	assert.Equal(t, notify.EventRSSDigest, ev.Type)
	assert.Equal(t, int64(2), ev.Fields["count"])
	items := ev.Fields["items"].([]any)
	require.Len(t, items, 2)
	assert.Equal(t, map[string]any{"title": "A", "site": "hdtime", "torrent_title": "A", "torrent_id": "1"}, items[0])

	c := mk("3")
	evC := renderAllPayload(RSSItemEvent{SiteName: "hdtime", TorrentID: "3", FeedItem: &gofeed.Item{Title: "C"}})
	NewRSSDigestFlush(db, push, nil)(context.Background(), 7, []notify.DigestItem{{LogID: c.ID, Title: evC.Title, Text: evC.Text, Event: evC.Event}})
	require.Len(t, push.got, 2)
	assert.Equal(t, evC.Event, push.got[1].Event, "只有一条时照它自己的事件")
}

// RSS 重试：载荷里的事件经 JSON 往返以后照样交给发送，类型不变
func TestRSSRetryKeepsEvent(t *testing.T) {
	db := setupRetryWorkerDB(t)
	past := time.Now().Add(-time.Minute)
	payload := renderFilteredPayload(RSSFilteredEvent{SiteName: "s", TorrentID: "9", Torrent: &v2.TorrentItem{Title: "X", SizeBytes: 1 << 40, Seeders: 5}})
	row := models.RSSNotificationLog{
		RSSID: 1, SiteName: "s", TorrentID: "9", NotifyKind: "filtered", NotificationConfID: 1, Result: "pending",
		NextRetryAt: &past, PayloadJSON: mustJSON(t, payload), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&row).Error)
	push := &capturePush{}
	require.NoError(t, NewRSSRetryWorker(db, push).drainOnce(context.Background()))
	require.Len(t, push.got, 1)
	ev := push.got[0].Event
	require.NotNil(t, ev)
	assert.Equal(t, notify.EventRSSMatched, ev.Type)
	assert.Equal(t, payload.Event.Fields, ev.Fields)
	assert.Equal(t, int64(1<<40), ev.Fields["size_bytes"])
}
