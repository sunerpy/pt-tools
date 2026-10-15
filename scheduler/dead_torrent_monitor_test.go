package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func deadRig(t *testing.T) (*DeadTorrentMonitor, *schedFakeDownloader, *bool, func() []models.MonitorNotificationLog) {
	t.Helper()
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	notifier, _, clock, db := newNotifierForTest(t, now, models.NotificationConf{ChannelType: "webhook", Name: "w", Enabled: true})
	var conf models.NotificationConf
	require.NoError(t, db.First(&conf).Error)
	dl := newSchedFakeDownloader("qb")
	dl.trackers = map[string][]downloader.TorrentTracker{}
	enabled := true
	mon := NewDeadTorrentMonitor(DeadTorrentMonitorConfig{
		Settings: func() (bool, time.Duration, []uint, error) { return enabled, 24 * time.Hour, []uint{conf.ID}, nil },
		Downloaders: func(context.Context) ([]DeadTorrentDownloader, []error, error) {
			return []DeadTorrentDownloader{{Name: "qb", DL: dl}}, []error{errors.New("tr 连不上")}, nil
		},
		Notifier: notifier,
		Resolver: func() *v2.TrackerResolver {
			return v2.NewTrackerResolverFrom(&v2.SiteDefinition{ID: "hdsky", Schema: v2.SchemaNexusPHP, URLs: []string{"https://hdsky.me/"}})
		},
		Clock: clock,
	})
	rows := func() []models.MonitorNotificationLog { return notifyRows(t, db) }
	t.Cleanup(func() { mon.Stop() })
	return mon, dl, &enabled, func() []models.MonitorNotificationLog {
		clock.Advance(25 * time.Hour)
		return rows()
	}
}

func deadTorrent(dl *schedFakeDownloader, hash string) {
	dl.torrents = append(dl.torrents, downloader.Torrent{ID: hash, InfoHash: hash, Name: "种子 " + hash, TotalSize: 1 << 30})
	dl.trackers[hash] = []downloader.TorrentTracker{{URL: "https://tracker.hdsky.me/announce.php?passkey=x", Status: 4, Message: "Unregistered torrent"}}
}

func TestDeadTorrentMonitor_NotifiesNewDeadTorrentsOnce(t *testing.T) {
	mon, dl, enabled, next := deadRig(t)
	deadTorrent(dl, "aa")
	dl.torrents = append(dl.torrents, downloader.Torrent{ID: "ok", InfoHash: "ok", Name: "正常"})
	dl.trackers["ok"] = []downloader.TorrentTracker{{URL: "https://tracker.hdsky.me/announce.php", Status: 2}}

	assert.Equal(t, 1, mon.RunOnce(context.Background()))
	rows := next()
	require.Len(t, rows, 1)
	assert.Equal(t, "downloader_assistant", rows[0].Source)
	assert.Contains(t, rows[0].PayloadJSON, "qb 里有 1 个种子")
	assert.Contains(t, rows[0].PayloadJSON, "种子 aa")
	assert.NotContains(t, rows[0].PayloadJSON, "正常")
	var p monitorNotifyPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].PayloadJSON), &p))
	require.NotNil(t, p.Event)
	assert.Equal(t, notify.EventDeadTorrents, p.Event.Type)
	assert.Equal(t, "qb", p.Event.Fields["downloader"])
	assert.Equal(t, int64(1), p.Event.Fields["count"])
	items := p.Event.Fields["items"].([]any)
	require.Len(t, items, 1)
	assert.Equal(t, "种子 aa", items[0].(map[string]any)["name"])

	assert.Equal(t, 0, mon.RunOnce(context.Background()), "同一批失效种子不再通知")
	assert.Len(t, next(), 1)

	deadTorrent(dl, "bb")
	assert.Equal(t, 1, mon.RunOnce(context.Background()), "出现新的失效种子再通知")
	rows = next()
	require.Len(t, rows, 2)
	assert.Contains(t, rows[1].PayloadJSON, "2 个种子")

	dl.torrents = dl.torrents[1:] // 用户删掉了 aa
	assert.Equal(t, 0, mon.RunOnce(context.Background()), "只是变少了不通知")

	*enabled = false
	deadTorrent(dl, "cc")
	assert.Equal(t, 0, mon.RunOnce(context.Background()), "关闭时不扫描")
}

func TestDeadTorrentMonitor_IntervalAndListLimit(t *testing.T) {
	mon, dl, _, next := deadRig(t)
	for _, h := range []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7"} {
		deadTorrent(dl, h)
	}
	assert.Equal(t, 1, mon.RunOnce(context.Background()))
	deadTorrent(dl, "a8")
	assert.Equal(t, 0, mon.RunOnce(context.Background()), "没到间隔不扫描")
	rows := next()
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0].PayloadJSON, "其余 2 个省略")

	assert.Equal(t, 1, mon.RunOnce(context.Background()), "过了间隔再扫描")
}

func TestDeadTorrentMonitor_Lifecycle(t *testing.T) {
	var nilMon *DeadTorrentMonitor
	nilMon.Start()
	nilMon.Stop()
	assert.Zero(t, nilMon.RunOnce(context.Background()))

	m := NewDeadTorrentMonitor(DeadTorrentMonitorConfig{
		Settings:    func() (bool, time.Duration, []uint, error) { return false, 0, nil, errors.New("db down") },
		Downloaders: func(context.Context) ([]DeadTorrentDownloader, []error, error) { return nil, nil, nil },
	})
	assert.Zero(t, m.RunOnce(context.Background()), "读不到设置时不扫描")
	m.Start()
	m.Start()
	m.Stop()
	m.Stop()

	mgr := &Manager{}
	mgr.SetDeadTorrentMonitor(m)
	other := NewDeadTorrentMonitor(DeadTorrentMonitorConfig{})
	other.Start()
	mgr.SetDeadTorrentMonitor(other)
	mgr.SetDeadTorrentMonitor(other)
	mgr.StopAll()
	assert.Nil(t, mgr.deadTorrentMonitor)
}

// 通道都停用、或者写投递表失败时不记作已通知：下一轮还会再试。
func TestDeadTorrentMonitor_RetriesWhenNotDelivered(t *testing.T) {
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	notifier, _, clock, db := newNotifierForTest(t, now, models.NotificationConf{ChannelType: "webhook", Name: "w", Enabled: true})
	var conf models.NotificationConf
	require.NoError(t, db.First(&conf).Error)
	require.NoError(t, db.Model(&conf).Update("enabled", false).Error)
	dl := newSchedFakeDownloader("qb")
	dl.trackers = map[string][]downloader.TorrentTracker{}
	deadTorrent(dl, "aa")
	mon := NewDeadTorrentMonitor(DeadTorrentMonitorConfig{
		Settings: func() (bool, time.Duration, []uint, error) { return true, time.Hour, []uint{conf.ID}, nil },
		Downloaders: func(context.Context) ([]DeadTorrentDownloader, []error, error) {
			return []DeadTorrentDownloader{{Name: "qb", DL: dl}}, nil, nil
		},
		Notifier: notifier,
		Resolver: func() *v2.TrackerResolver { return v2.NewTrackerResolverFrom() },
		Clock:    clock,
	})
	assert.Zero(t, mon.RunOnce(context.Background()), "通道停用：没有发出")
	assert.Empty(t, notifyRows(t, db))

	require.NoError(t, db.Model(&conf).Update("enabled", true).Error)
	clock.Advance(2 * time.Hour)
	assert.Equal(t, 1, mon.RunOnce(context.Background()), "通道恢复后同一批还会通知")
	assert.Len(t, notifyRows(t, db), 1)
	assert.True(t, notifier.Logged(context.Background(), "downloader_assistant", "qb", "dead_torrents", notifyRows(t, db)[0].EventKey))

	// 重启后（内存里的记录没了）同一批不会重复写
	mon2 := NewDeadTorrentMonitor(mon.cfg)
	assert.Zero(t, mon2.RunOnce(context.Background()))
	assert.Len(t, notifyRows(t, db), 1)

	deadTorrent(dl, "bb")
	require.NoError(t, db.Migrator().DropTable(&models.MonitorNotificationLog{}))
	clock.Advance(2 * time.Hour)
	assert.Zero(t, mon2.RunOnce(context.Background()), "写不进投递表：这次不算")
	require.NoError(t, db.AutoMigrate(&models.MonitorNotificationLog{}))
	clock.Advance(2 * time.Hour)
	assert.Equal(t, 1, mon2.RunOnce(context.Background()), "下一轮重试成功")
}

// 上一轮的时间与结果：每台下载器有几个失效种子、连不上的台数、通知了几台
func TestDeadTorrentMonitor_Status(t *testing.T) {
	mon, dl, _, _ := deadRig(t)
	at, res := mon.Status()
	assert.True(t, at.IsZero())
	assert.Empty(t, res)
	deadTorrent(dl, "aa")
	require.Equal(t, 1, mon.RunOnce(context.Background()))
	at, res = mon.Status()
	assert.False(t, at.IsZero())
	assert.Equal(t, "1 台下载器连不上；qb 有 1 个失效种子；通知了 1 台的新失效种子", res)

	// 连下载器列表都没读到：写明这一轮没有扫描，不当成「几台连不上」，也不发通知
	broken := NewDeadTorrentMonitor(DeadTorrentMonitorConfig{
		Settings: mon.cfg.Settings,
		Downloaders: func(context.Context) ([]DeadTorrentDownloader, []error, error) {
			return nil, nil, errors.New("database is locked")
		},
		Notifier: mon.cfg.Notifier,
		Clock:    mon.cfg.Clock,
	})
	assert.Zero(t, broken.RunOnce(context.Background()))
	at, res = broken.Status()
	assert.False(t, at.IsZero())
	assert.Equal(t, "读取下载器列表失败，这一轮没有扫描", res)
	var nilMon *DeadTorrentMonitor
	at, res = nilMon.Status()
	assert.True(t, at.IsZero())
	assert.Empty(t, res)
}
