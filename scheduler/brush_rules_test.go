package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

var brushNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func brushItem(mod func(*v2.TorrentItem)) v2.TorrentItem {
	it := v2.TorrentItem{
		ID: "100", Title: "Some.Movie.2026.1080p.WEB-DL", SizeBytes: 10 * gib,
		Seeders: 3, Leechers: 40, UploadedAt: brushNow.Add(-20 * time.Minute).Unix(),
		DiscountLevel: v2.DiscountFree, DiscountEndTime: brushNow.Add(10 * time.Hour),
	}
	if mod != nil {
		mod(&it)
	}
	return it
}

func TestAdmitBrushItem(t *testing.T) {
	base := models.BrushTask{ExcludeHR: true}
	cases := []struct {
		name   string
		task   func(*models.BrushTask)
		item   func(*v2.TorrentItem)
		siteHR bool
		ok     bool
		reason string
	}{
		{name: "默认只收免费：FREE 通过", ok: true},
		{name: "2XFREE 也算免费", item: func(i *v2.TorrentItem) { i.DiscountLevel = v2.Discount2xFree }, ok: true},
		// M-Team 的搜索忽略 FreeOnly，列表里会混入非免费种：在客户端按优惠类型拦下
		{name: "M-Team 混入的普通种被拦", item: func(i *v2.TorrentItem) { i.DiscountLevel = v2.DiscountNone }, reason: "优惠类型 NONE 不在允许范围"},
		{name: "M-Team 混入的 50% 被拦", item: func(i *v2.TorrentItem) { i.DiscountLevel = v2.DiscountPercent50 }, reason: "不在允许范围"},
		{
			name: "配置了 50% 时放行", task: func(t *models.BrushTask) { t.Discounts = "FREE, percent_50" },
			item: func(i *v2.TorrentItem) { i.DiscountLevel = v2.DiscountPercent50 }, ok: true,
		},
		{name: "优惠已到期", item: func(i *v2.TorrentItem) { i.DiscountEndTime = brushNow.Add(-time.Minute) }, reason: "优惠已到期"},
		{name: "永久免费（没有到期时间）不受剩余时间限制", task: func(t *models.BrushTask) { t.MinFreeRemainMin = 600 }, item: func(i *v2.TorrentItem) { i.DiscountEndTime = time.Time{} }, ok: true},
		{name: "免费剩余不足", task: func(t *models.BrushTask) { t.MinFreeRemainMin = 11 * 60 }, reason: "优惠剩余不足 660 分钟"},
		{name: "体积下限", task: func(t *models.BrushTask) { t.MinSizeGB = 20 }, reason: "小于 20.00 GB"},
		{name: "体积上限", task: func(t *models.BrushTask) { t.MaxSizeGB = 5 }, reason: "大于 5.00 GB"},
		{name: "设了体积限制但体积未知", task: func(t *models.BrushTask) { t.MaxSizeGB = 50 }, item: func(i *v2.TorrentItem) { i.SizeBytes = 0 }, reason: "体积未知"},
		{name: "做种人数上限", task: func(t *models.BrushTask) { t.MaxSeeders = 2 }, reason: "做种 3 人多于 2"},
		{name: "下载人数下限", task: func(t *models.BrushTask) { t.MinLeechers = 50 }, reason: "下载 40 人少于 50"},
		{name: "发布时长上限", task: func(t *models.BrushTask) { t.MaxPublishAgeMin = 10 }, reason: "超过 10 分钟"},
		{name: "设了发布时长但时间未知", task: func(t *models.BrushTask) { t.MaxPublishAgeMin = 60 }, item: func(i *v2.TorrentItem) { i.UploadedAt = 0 }, reason: "发布时间未知"},
		{name: "种子带 H&R", item: func(i *v2.TorrentItem) { i.HasHR = true }, reason: "有 H&R"},
		// M-Team 列表不填 HasHR：站点级开启 H&R 时整站视为 H&R，与 RSS 路径同一规则
		{name: "HasHR 为空但站点级 H&R", siteHR: true, reason: "有 H&R"},
		{name: "不排除 H&R 时放行", task: func(t *models.BrushTask) { t.ExcludeHR = false }, siteHR: true, ok: true},
		{name: "包含词命中副标题（大小写不敏感）", task: func(t *models.BrushTask) { t.IncludeKeywords = "国语\nREMUX" }, item: func(i *v2.TorrentItem) { i.Subtitle = "国语中字" }, ok: true},
		{name: "包含词都不命中", task: func(t *models.BrushTask) { t.IncludeKeywords = "remux, 2160p" }, reason: "标题不含任何包含词"},
		{name: "排除词命中", task: func(t *models.BrushTask) { t.ExcludeKeywords = "web-dl" }, reason: "标题含排除词 web-dl"},
		{name: "没有种子 ID", item: func(i *v2.TorrentItem) { i.ID = "" }, reason: "没有种子 ID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task := base
			if tc.task != nil {
				tc.task(&task)
			}
			ok, reason := admitBrushItem(task, brushItem(tc.item), tc.siteHR, brushNow)
			assert.Equal(t, tc.ok, ok, reason)
			if !tc.ok {
				assert.Contains(t, reason, tc.reason)
			}
		})
	}
}

func TestSortBrushCandidates(t *testing.T) {
	items := []v2.TorrentItem{
		{ID: "a", Leechers: 5, Seeders: 1},
		{ID: "b", Leechers: 50, Seeders: 9},
		{ID: "c", Leechers: 50, Seeders: 2, UploadedAt: 1},
		{ID: "d", Leechers: 50, Seeders: 2, UploadedAt: 9},
	}
	sortBrushCandidates(items)
	got := []string{}
	for _, it := range items {
		got = append(got, it.ID)
	}
	assert.Equal(t, []string{"d", "c", "b", "a"}, got)
}

func TestBrushRemoval(t *testing.T) {
	freeEnd := brushNow.Add(5 * time.Minute)
	later := brushNow.Add(5 * time.Hour)
	idle := brushNow.Add(-7 * time.Hour)
	task := models.BrushTask{IntervalMin: 10, RemoveFreeExpiredIncomplete: true}
	cases := []struct {
		name    string
		task    func(*models.BrushTask)
		bt      models.BrushTorrent
		tor     downloader.Torrent
		samples []models.BrushTorrentSample
		remove  bool
		reason  string
	}{
		{name: "免费在下一轮之前到期且没下完：删", bt: models.BrushTorrent{FreeEndAt: &freeEnd}, tor: downloader.Torrent{Progress: 0.4}, remove: true, reason: "免费即将到期"},
		{name: "免费还早：不删", bt: models.BrushTorrent{FreeEndAt: &later}, tor: downloader.Torrent{Progress: 0.4}},
		{name: "下完了就不按免费到期删", bt: models.BrushTorrent{FreeEndAt: &freeEnd}, tor: downloader.Torrent{Progress: 1}},
		{name: "关掉免费到期规则", task: func(t *models.BrushTask) { t.RemoveFreeExpiredIncomplete = false }, bt: models.BrushTorrent{FreeEndAt: &freeEnd}, tor: downloader.Torrent{Progress: 0.4}},
		{
			name: "H&R 未满做种时长：免费到期也不删", bt: models.BrushTorrent{FreeEndAt: &freeEnd, HasHR: true, HRSeedTimeH: 72},
			tor: downloader.Torrent{Progress: 0.4},
		},
		{
			name: "H&R 满了做种时长之后照常按规则删", task: func(t *models.BrushTask) { t.RemoveRatio = 1 },
			bt: models.BrushTorrent{HasHR: true, HRSeedTimeH: 1}, tor: downloader.Torrent{Progress: 1, SeedingTime: 7200, Ratio: 1.5}, remove: true, reason: "分享率 1.50，达到 1.00",
		},
		{name: "做种时长", task: func(t *models.BrushTask) { t.RemoveSeedTimeH = 2 }, tor: downloader.Torrent{Progress: 1, SeedingTime: 7300}, remove: true, reason: "做种 2.0 小时，达到 2.0 小时"},
		{name: "做种时长没到", task: func(t *models.BrushTask) { t.RemoveSeedTimeH = 2 }, tor: downloader.Torrent{Progress: 1, SeedingTime: 3600}},
		{name: "分享率", task: func(t *models.BrushTask) { t.RemoveRatio = 3 }, tor: downloader.Torrent{Progress: 1, Ratio: 3.2}, remove: true, reason: "分享率 3.20"},
		{
			name: "最近 30 分钟平均上传太慢", task: func(t *models.BrushTask) { t.RemoveLowSpeedKBs = 100; t.RemoveLowSpeedWindowMin = 30 },
			tor:     downloader.Torrent{Progress: 1, SeedingTime: 3600, TotalUploaded: 1024 * 1024 * 60},
			samples: []models.BrushTorrentSample{{At: brushNow.Add(-31 * time.Minute), Uploaded: 1024 * 1024 * 50}},
			remove:  true, reason: "最近 30 分钟平均上传",
		},
		{
			name: "上传够快", task: func(t *models.BrushTask) { t.RemoveLowSpeedKBs = 100; t.RemoveLowSpeedWindowMin = 30 },
			tor:     downloader.Torrent{Progress: 1, SeedingTime: 3600, TotalUploaded: 1024 * 1024 * 500},
			samples: []models.BrushTorrentSample{{At: brushNow.Add(-31 * time.Minute), Uploaded: 0}},
		},
		{
			name: "采样还没覆盖整个窗口：不判", task: func(t *models.BrushTask) { t.RemoveLowSpeedKBs = 100; t.RemoveLowSpeedWindowMin = 30 },
			tor:     downloader.Torrent{Progress: 1, SeedingTime: 3600, TotalUploaded: 1},
			samples: []models.BrushTorrentSample{{At: brushNow.Add(-10 * time.Minute)}},
		},
		{
			name: "刚下完、做种时长不足一个窗口：不判", task: func(t *models.BrushTask) { t.RemoveLowSpeedKBs = 100; t.RemoveLowSpeedWindowMin = 30 },
			tor:     downloader.Torrent{Progress: 1, SeedingTime: 600, TotalUploaded: 1},
			samples: []models.BrushTorrentSample{{At: brushNow.Add(-40 * time.Minute)}},
		},
		{
			name: "还在下载：低速规则不管", task: func(t *models.BrushTask) { t.RemoveLowSpeedKBs = 100; t.RemoveLowSpeedWindowMin = 30 },
			tor:     downloader.Torrent{Progress: 0.5, SeedingTime: 0, TotalUploaded: 1},
			samples: []models.BrushTorrentSample{{At: brushNow.Add(-40 * time.Minute)}},
		},
		{name: "长时间没有活动", task: func(t *models.BrushTask) { t.RemoveInactiveH = 6 }, bt: models.BrushTorrent{LastActivityAt: &idle}, tor: downloader.Torrent{Progress: 1}, remove: true, reason: "已 7.0 小时没有上传或下载"},
		{name: "没开任何规则：不删", tor: downloader.Torrent{Progress: 1, Ratio: 99, SeedingTime: 1 << 30}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tk := task
			if tc.task != nil {
				tc.task(&tk)
			}
			remove, reason := brushRemoval(tk, tc.bt, tc.tor, tc.samples, brushNow)
			assert.Equal(t, tc.remove, remove, reason)
			if tc.remove {
				assert.Contains(t, reason, tc.reason)
			}
		})
	}
}

func TestHasTag(t *testing.T) {
	assert.True(t, hasTag(downloader.Torrent{Tags: "hdsky, PT-Brush-3"}, "pt-brush-3"))
	assert.True(t, hasTag(downloader.Torrent{Label: "pt-brush-3"}, "pt-brush-3"))
	assert.False(t, hasTag(downloader.Torrent{Tags: "pt-brush-30"}, "pt-brush-3"), "不能前缀匹配")
}
