package transfer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestNormalizeRule(t *testing.T) {
	r := models.TransferRule{Name: " r ", SourceDownloaderID: 1, TargetDownloaderID: 2, Tag: " keep "}
	require.NoError(t, NormalizeRule(&r))
	assert.Equal(t, "r", r.Name)
	assert.Equal(t, "keep", r.Tag)
	assert.Equal(t, models.TransferRuleDefaultMaxPerRun, r.MaxPerRun)
	assert.Equal(t, models.TransferRuleDefaultIntervalMin, r.IntervalMin)
	for _, bad := range []models.TransferRule{
		{SourceDownloaderID: 1, TargetDownloaderID: 2},
		{Name: "x", TargetDownloaderID: 2},
		{Name: "x", SourceDownloaderID: 1, TargetDownloaderID: 1},
		{Name: "x", SourceDownloaderID: 1, TargetDownloaderID: 2, MinSeedingHours: -1},
		{Name: "x", SourceDownloaderID: 1, TargetDownloaderID: 2, MaxPerRun: models.TransferRuleMaxPerRun + 1},
		{Name: "x", SourceDownloaderID: 1, TargetDownloaderID: 2, IntervalMin: 5},
	} {
		assert.ErrorIs(t, NormalizeRule(&bad), ErrRuleInvalid, "%+v", bad)
	}
}

func TestRuleDue(t *testing.T) {
	now := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	r := models.TransferRule{Enabled: true, IntervalMin: 60}
	assert.True(t, RuleDue(r, now), "没跑过")
	last := now.Add(-30 * time.Minute)
	r.LastRunAt = &last
	assert.False(t, RuleDue(r, now))
	last = now.Add(-60 * time.Minute)
	assert.True(t, RuleDue(r, now))
	r.Enabled = false
	assert.False(t, RuleDue(r, now))
	r = models.TransferRule{Enabled: true, LastRunAt: &last}
	assert.True(t, RuleDue(r, now), "0 按默认 60 分钟")
}

// 规则只挑已下完、不是刷流、条件都符合、目标里没有、也没有进行中任务的种子；按添加时间从早到晚，最多 MaxPerRun 个。
func TestRunRule(t *testing.T) {
	e := newEnv(t)
	mk := func(name string, added int64, mut func(t *downloader.Torrent)) string {
		h := e.seed(e.src.fakeDL, name, 1<<30, true)
		e.src.set(h, func(t *downloader.Torrent) {
			t.DateAdded, t.SeedingTime = added, 100*3600
			if mut != nil {
				mut(t)
			}
		})
		return h
	}
	oldest := mk("A.oldest", 1, nil)
	second := mk("B.second", 2, nil)
	third := mk("C.third", 3, nil)
	mk("D.partial", 0, func(t *downloader.Torrent) { t.Progress, t.IsCompleted = 0.5, false })
	mk("E.brush", 0, func(t *downloader.Torrent) { t.Tags = "hdsky," + models.BrushTagAll })
	mk("F.othercat", 0, func(t *downloader.Torrent) { t.Category = "tv" })
	mk("G.notag", 0, func(t *downloader.Torrent) { t.Tags = "hdsky" })
	mk("H.othersite", 0, func(t *downloader.Torrent) { t.Tracker = "https://tracker.other.org/a" })
	mk("I.young", 0, func(t *downloader.Torrent) { t.SeedingTime = 3600 })
	inTarget := mk("J.intarget", 0, nil)
	e.dst.put(downloader.Torrent{InfoHash: inTarget})
	busy := mk("K.busy", 0, nil)
	require.NoError(t, e.db.Create(&models.TorrentTransferJob{InfoHash: busy, State: models.TransferPending}).Error)

	rule := models.TransferRule{
		ID: 5, Name: "r", Enabled: true, SourceDownloaderID: srcID, TargetDownloaderID: dstID,
		Category: "MOVIES", Tag: "4K", SiteName: "hdsky", MinSeedingHours: 48, MaxPerRun: 2,
	}
	res, err := e.svc.RunRule(context.Background(), rule)
	require.NoError(t, err)
	assert.Equal(t, 5, res.Matched, "A、B、C、J、K 符合条件")
	assert.Equal(t, 2, res.Created)
	var jobs []models.TorrentTransferJob
	require.NoError(t, e.db.Where("rule_id = ?", 5).Order("id").Find(&jobs).Error)
	require.Len(t, jobs, 2)
	assert.Equal(t, oldest, jobs[0].InfoHash)
	assert.Equal(t, second, jobs[1].InfoHash)
	assert.Contains(t, res.Summary(), "建了 2 个任务")

	res, err = e.svc.RunRule(context.Background(), rule)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created, "已经建过的不再建")
	require.NoError(t, e.db.Where("rule_id = ?", 5).Order("id").Find(&jobs).Error)
	assert.Equal(t, third, jobs[2].InfoHash)

	_, err = e.svc.RunRule(context.Background(), models.TransferRule{SourceDownloaderID: srcID, TargetDownloaderID: srcID})
	assert.ErrorIs(t, err, ErrRuleInvalid)
	_, err = e.svc.RunRule(context.Background(), models.TransferRule{SourceDownloaderID: offID, TargetDownloaderID: dstID})
	assert.ErrorContains(t, err, "源下载器不可用")
}

func TestHasTag(t *testing.T) {
	assert.True(t, hasTag("a, B ,c", "b"))
	assert.False(t, hasTag("ab", "a"))
	assert.False(t, hasTag("", "a"))
}

func TestRuleResultSummary(t *testing.T) {
	assert.Equal(t, "符合条件 3 个，建了 1 个任务，跳过 1 个（X：已经有进行中的转移任务）",
		RuleResult{Matched: 3, Created: 1, Skipped: []string{"X：已经有进行中的转移任务"}}.Summary())
}
