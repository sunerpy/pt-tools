package organize

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

const (
	frierenID = 209867 // 假 TMDB 里的葬送的芙莉莲（剧集）
	bbID      = 1396   // 绝命毒师（剧集，只能按编号取）
	tlouID    = 100088 // 最后生还者（剧集）：lastOfUs 的种子名自动识别出的就是它
)

// pushedMedia 记一条推送时说明了条目的种子记录（按条目搜索、订阅推送时写的那三列）。
func (e *env) pushedMedia(hash, kind string, id, season int) {
	e.t.Helper()
	h := hash
	require.NoError(e.t, e.db.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: hash[:6], TorrentHash: &h, Title: "x", MediaType: kind, TMDBID: id, MediaSeason: season,
	}).Error)
}

func (e *env) follows(h MediaHint) {
	e.svc.SetMediaFor(func(context.Context, uint, string) (MediaHint, bool, error) { return h, true, nil })
}

// 条目从哪来，按这个顺序：手动指定 → 之前整理用过的 → 订阅的 → 推送时说明的 → 自动识别。
// 种子名自动识别是最后生还者，订阅说的是芙莉莲、推送时说的是绝命毒师：断言用的是排在前面的那个。
func TestRecognizeOrder(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.lastOfUs()
	preview := func(req Request) *Plan {
		t.Helper()
		req.DownloaderID, req.Hash = 1, tlouHash
		p, err := e.svc.Preview(e.ctx, req)
		require.NoError(t, err)
		require.Empty(t, p.Problem)
		require.NotNil(t, p.Match)
		return p
	}

	p := preview(Request{})
	assert.Equal(t, tlouID, p.Match.ID, "什么都没说：自动识别")
	assert.NotContains(t, []string{sourceManual, sourceHistory, sourceSubscription, sourcePush}, p.Source)

	e.pushedMedia(tlouHash, models.MediaKindTV, bbID, 1)
	p = preview(Request{})
	assert.Equal(t, bbID, p.Match.ID)
	assert.Equal(t, sourcePush, p.Source)
	for _, it := range p.Items {
		assert.Contains(t, it.Target, "绝命毒师", "按推送时说明的条目命名")
	}

	e.follows(MediaHint{MediaType: models.MediaKindTV, TMDBID: frierenID})
	p = preview(Request{})
	assert.Equal(t, frierenID, p.Match.ID)
	assert.Equal(t, sourceSubscription, p.Source, "订阅的条目排在推送时说明的前面")
	p = preview(Request{LibraryID: p.Library.ID})
	assert.Equal(t, sourceSubscription, p.Source, "只指定了媒体库时照样用订阅的条目")

	p = preview(Request{MediaType: models.MediaKindTV, TMDBID: tlouID})
	assert.Equal(t, tlouID, p.Match.ID)
	assert.Equal(t, sourceManual, p.Source)

	// 之前整理用过的条目（例如手动纠正过）排在订阅的前面
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash, MediaType: models.MediaKindTV, TMDBID: tlouID})
	require.NoError(t, err)
	require.Positive(t, res.Created)
	p = preview(Request{})
	assert.Equal(t, tlouID, p.Match.ID)
	assert.Equal(t, sourceHistory, p.Source)
}

// 订阅或推送时说明的条目在 TMDB 上不在了：退回自动识别，预览与整理记录里写明；TMDB 暂时出错时稍后重试，不改识别。
func TestRecognizeHintFallback(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.lastOfUs()
	e.pushedMedia(tlouHash, models.MediaKindTV, 999999, 1)

	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	require.Empty(t, p.Problem)
	assert.Equal(t, tlouID, p.Match.ID, "退回自动识别")
	require.NotEmpty(t, p.Notes)
	assert.Contains(t, p.Notes[0], "推送时说明的条目")
	assert.Contains(t, p.Notes[0], "找不到")

	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	require.Positive(t, res.Created)
	for _, r := range e.history() {
		assert.Contains(t, r.Message, "找不到", "整理记录里写明")
		assert.Equal(t, tlouID, r.TMDBID)
	}

	// 订阅的条目：TMDB 回 503，稍后重试
	e2 := newEnv(t)
	e2.defaultLibraries()
	e2.settings(SettingsInput{MinVideoMB: 1})
	e2.lastOfUs()
	e2.follows(MediaHint{MediaType: models.MediaKindTV, TMDBID: frierenID})
	e2.tmdb.setDown(true)
	res, err = e2.svc.Organize(e2.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "识别失败")
	rows := e2.history()
	require.NotEmpty(t, rows)
	assert.NotNil(t, rows[0].NextRetryAt, "可以重试")
}

// 整理记录记下条目从哪来；推送记录挪进归档表（完成两周后）以后照样按推送时说明的条目整理。
func TestRecognizeMatchSourceAndArchive(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.lastOfUs()
	h := tlouHash
	require.NoError(t, e.db.Create(&models.TorrentInfoArchive{
		SiteName: "hdsky", TorrentID: "1", TorrentHash: &h, Title: "最后生还者 第一季", MediaType: models.MediaKindTV, TMDBID: frierenID, MediaSeason: 1,
	}).Error)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	require.Empty(t, res.Plan.Problem)
	assert.Equal(t, frierenID, res.Plan.Match.ID)
	assert.Equal(t, sourcePush, res.Plan.Source)
	rows := e.history()
	require.NotEmpty(t, rows)
	for _, r := range rows {
		assert.Equal(t, sourcePush, r.MatchSource)
		assert.Equal(t, frierenID, r.TMDBID)
	}
}

// 不是电影、剧集的类型或编号不对的推送记录不算条目；订阅回的条目不完整时只用它的媒体库。
func TestRecognizeIgnoresInvalidHints(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.lastOfUs()
	e.pushedMedia(tlouHash, "book", frierenID, 0)
	e.follows(MediaHint{MediaType: models.MediaKindTV})
	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	require.NotNil(t, p.Match)
	assert.Equal(t, tlouID, p.Match.ID)
	assert.NotEqual(t, sourcePush, p.Source)
	assert.NotEqual(t, sourceSubscription, p.Source)
}
