package commands

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func run(t *testing.T, name string, args ...string) chatops.Reply {
	t.Helper()
	spec, ok := chatops.DefaultRegistry().Get(name)
	require.True(t, ok, name)
	r, err := spec.Handler(context.Background(), args, chatops.Source{ReplyLang: "zh", IsAdmin: true, PtAdmin: true})
	require.NoError(t, err)
	return r
}

// eventOf 断言回复带着 typ 这种事件，返回规范化以后的变量。
func eventOf(t *testing.T, r chatops.Reply, typ string) map[string]any {
	t.Helper()
	require.NotNil(t, r.Event, "回复没有带事件")
	assert.Equal(t, typ, r.Event.Type)
	return r.Event.Fields
}

func TestStatusReplyEvent(t *testing.T) {
	setupServices(t, &Services{
		Task: &mockTaskService{jobs: []app.JobStatusDTO{{Running: true}, {Running: false}, {Running: true}}},
		Downloader: &mockDownloaderStatus{statuses: []downloader.DownloaderStatus{
			{Name: "qb1", Type: "qbittorrent", IsHealthy: true},
			{Name: "tr1", Type: "transmission"},
		}},
	})
	r := run(t, "status")
	f := eventOf(t, r, notify.EventReplyStatus)
	assert.Equal(t, int64(3), f["tasks"])
	assert.Equal(t, int64(2), f["running"])
	assert.Equal(t, []any{
		map[string]any{"name": "qb1", "type": "qbittorrent", "healthy": true},
		map[string]any{"name": "tr1", "type": "transmission", "healthy": false},
	}, f["items"])
	assert.Equal(t, "状态概览\n任务: 3 总计 / 2 运行中\n下载器: 2\n  ✓ qb1 (qbittorrent)\n  ✗ tr1 (transmission)", r.Text, "内置文字不变")
}

func TestTorrentsReplyEvent(t *testing.T) {
	setupServices(t, &Services{Torrent: &mockTorrentService{
		listResult: []app.TorrentDTO{{Name: "A", State: "seeding", Progress: 1, Size: 1 << 30}, {Name: "B", State: "downloading", Progress: 0.455, Size: 2048}},
		listTotal:  42,
	}})
	r := run(t, "torrents", "qb1")
	f := eventOf(t, r, notify.EventReplyTorrents)
	assert.Equal(t, "qb1", f["downloader"])
	assert.Equal(t, int64(42), f["total"])
	assert.Equal(t, int64(40), f["items_more"])
	items := f["items"].([]any)
	require.Len(t, items, 2)
	assert.Equal(t, map[string]any{"name": "A", "state": "seeding", "progress": float64(100), "size": "1.00 GiB", "size_bytes": int64(1 << 30)}, items[0])
	assert.InDelta(t, 45.5, items[1].(map[string]any)["progress"], 1e-9)
	assert.Equal(t, "```\nqb1 种子（42 总计，显示前 2）\n[seeding] A 100.0% 1.00 GiB\n[downloading] B 45.5% 2.00 KiB\n```", r.Text)

	setupServices(t, &Services{Torrent: &mockTorrentService{}})
	r = run(t, "torrents", "qb1")
	f = eventOf(t, r, notify.EventReplyTorrents)
	assert.Equal(t, "无种子", r.Text)
	assert.Equal(t, []any{}, f["items"])
	assert.Equal(t, int64(0), f["total"])
}

func TestSitesReplyEvents(t *testing.T) {
	setupServices(t, &Services{Site: &mockSiteService{sites: []app.SiteSummaryDTO{
		{Name: "hdtime", Enabled: true, Status: "enabled"}, {Name: "hdsky", Status: "disabled"},
	}}})
	r := run(t, "sites")
	f := eventOf(t, r, notify.EventReplySites)
	assert.Equal(t, int64(2), f["count"])
	assert.Equal(t, []any{
		map[string]any{"name": "hdtime", "status": "enabled", "enabled": true},
		map[string]any{"name": "hdsky", "status": "disabled", "enabled": false},
	}, f["items"])
	assert.Equal(t, "站点（2）\n[enabled] hdtime\n[disabled] hdsky", r.Text)

	setupServices(t, &Services{Site: &siteInfoService{info: app.UserInfoDTO{
		SiteName: "hdtime", Username: "me", Uploaded: "1.00 TiB", Downloaded: "512.00 GiB", Ratio: "2.000", Bonus: "1235", Class: "VIP",
		UploadedBytes: 1 << 40, DownloadedBytes: 1 << 39, RatioValue: 2, BonusValue: 1234.6,
	}}})
	r = run(t, "sites", "hdtime")
	f = eventOf(t, r, notify.EventReplySiteInfo)
	assert.Equal(t, map[string]any{
		"site": "hdtime", "user": "me", "uploaded": "1.00 TiB", "downloaded": "512.00 GiB",
		"uploaded_bytes": int64(1 << 40), "downloaded_bytes": int64(1 << 39), "ratio": float64(2), "bonus": 1234.6, "class": "VIP",
	}, f)
	assert.Contains(t, r.Text, "Bonus: 1235")
	assert.Contains(t, r.Text, "Downloaded (billed): 512.00 GiB", "站点报的下载量写明是计费口径")
}

type siteInfoService struct {
	mockSiteService
	info app.UserInfoDTO
}

func (s *siteInfoService) GetSiteUserInfo(context.Context, string) (app.UserInfoDTO, error) {
	return s.info, nil
}

func TestTasksReplyEvent(t *testing.T) {
	setupServices(t, &Services{Task: &mockTaskService{jobs: []app.JobStatusDTO{
		{SiteName: "hdtime", RSSName: "电影", Running: true}, {SiteName: "hdsky", RSSName: "剧集"},
	}}})
	r := run(t, "tasks")
	f := eventOf(t, r, notify.EventReplyTasks)
	assert.Equal(t, int64(2), f["count"])
	assert.Equal(t, int64(1), f["running"])
	assert.Equal(t, []any{
		map[string]any{"site": "hdtime", "rss": "电影", "state": "running"},
		map[string]any{"site": "hdsky", "rss": "剧集", "state": "stopped"},
	}, f["items"])
	assert.Equal(t, "RSS 任务（2）\n[running] hdtime/电影\n[stopped] hdsky/剧集", r.Text)
}

func TestSigninReplyEvent(t *testing.T) {
	setupServices(t, &Services{Attendance: &fakeAttendance{
		one: AttendanceOutcome{Site: "hdtime", Status: "signed", Message: "获得 10 魔力"},
		all: []AttendanceOutcome{
			{Site: "hdtime", Status: "already"},
			{Site: "hdsky", Status: "failed", Message: "忽略", Error: "站点超时"},
			{Site: "x", Status: "unsupported"},
		},
	}})
	r := run(t, "signin")
	f := eventOf(t, r, notify.EventReplySignin)
	assert.Equal(t, int64(1), f["signed"])
	assert.Equal(t, int64(1), f["failed"])
	items := f["items"].([]any)
	require.Len(t, items, 3)
	assert.Equal(t, map[string]any{"site": "hdsky", "status": "failed", "status_label": "签到失败", "message": "站点超时"}, items[1])
	assert.Equal(t, "hdtime: 今天已签到\nhdsky: 签到失败 (站点超时)\nx: 不支持自动签到", r.Text)

	r = run(t, "signin", "hdtime")
	f = eventOf(t, r, notify.EventReplySignin)
	assert.Equal(t, int64(1), f["signed"])
	assert.Equal(t, []any{map[string]any{"site": "hdtime", "status": "signed", "status_label": "签到成功", "message": "获得 10 魔力"}}, f["items"])
}

func TestVersionAndHelpReplyEvents(t *testing.T) {
	setupServices(t, &Services{})
	r := run(t, "version")
	f := eventOf(t, r, notify.EventReplyVersion)
	assert.Contains(t, r.Text, fmt.Sprintf("Version: %s", f["version"]))
	assert.IsType(t, false, f["update_checked"])

	r = run(t, "help")
	f = eventOf(t, r, notify.EventReplyHelp)
	items := f["items"].([]any)
	require.NotEmpty(t, items)
	found := false
	for _, it := range items {
		m := it.(map[string]any)
		if m["name"] == "signin" {
			found = true
			assert.Equal(t, true, m["admin"])
			assert.NotEmpty(t, m["description"])
		}
	}
	assert.True(t, found, "管理员看得到 /signin")
}

func TestSubsReplyEvent(t *testing.T) {
	lines := make([]SubscribeLine, 0, 22)
	for i := range 22 {
		lines = append(lines, SubscribeLine{Title: fmt.Sprintf("片%d", i), Status: "在找"})
	}
	lines[0].Progress = "已入库 3/7"
	setupSubscribe(t, &fakeSubscribe{lines: lines})
	r := run(t, "subs")
	f := eventOf(t, r, notify.EventReplySubs)
	assert.Equal(t, int64(22), f["count"])
	assert.Equal(t, int64(2), f["items_more"])
	items := f["items"].([]any)
	require.Len(t, items, maxSubLines)
	assert.Equal(t, map[string]any{"title": "片0", "status": "在找", "progress": "已入库 3/7"}, items[0])
	assert.Contains(t, r.Text, "……还有 2 个")

	setupSubscribe(t, &fakeSubscribe{})
	r = run(t, "subs")
	f = eventOf(t, r, notify.EventReplySubs)
	assert.Equal(t, int64(0), f["count"])
}

// 没做成的回复（errReply）不带事件：由 MessageChain 按 reply.other、ok=false 套用。
func TestFailedRepliesHaveNoEvent(t *testing.T) {
	setupServices(t, &Services{})
	r := run(t, "torrents", "qb1")
	assert.True(t, r.Failed)
	assert.Nil(t, r.Event)
}
