package organize

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// tlouAbsolute 是假 TMDB 上最后生还者的绝对集号剧集组（见 newFakeTMDB）。
const tlouAbsolute = "5b11ba820e0a265a53000ce9"

const absHash = "cccccccccccccccccccccccccccccccccccccccc"

// absTorrent 是按绝对集号写的种子：第 3、4 集（是第 2 季的第 1、2 集）与剧集组里没有的第 9 集。
func (e *env) absTorrent() {
	name := "The.Last.of.Us.E03-E09.1080p.WEB-DL-GRP"
	e.addTorrent(absHash, name, map[string]int{
		name + "/The.Last.of.Us.E03.1080p.WEB-DL-GRP.mkv": 5,
		name + "/The.Last.of.Us.E04.1080p.WEB-DL-GRP.mkv": 5,
		name + "/The.Last.of.Us.E09.1080p.WEB-DL-GRP.mkv": 5,
	}, func(t *downloader.Torrent) { t.Category = "tv" })
}

// 追剧选了剧集组：文件名里的季集先换成 TMDB 默认的季集再命名；剧集组里没有的照文件名，写进记录与说明。
func TestOrganizeEpisodeGroup(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.absTorrent()
	e.svc.SetMediaFor(func(context.Context, uint, string) (MediaHint, bool, error) {
		return MediaHint{MediaType: models.MediaKindTV, TMDBID: 100088, EpisodeGroup: tlouAbsolute}, true, nil
	})

	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: absHash})
	require.NoError(t, err)
	require.Empty(t, p.Problem)
	targets := map[string]string{}
	for _, it := range p.Items {
		targets[filepath.Base(it.Rel)] = filepath.Base(it.Target)
	}
	assert.Equal(t, map[string]string{
		"The.Last.of.Us.E03.1080p.WEB-DL-GRP.mkv": "最后生还者 - S02E01.mkv",
		"The.Last.of.Us.E04.1080p.WEB-DL-GRP.mkv": "最后生还者 - S02E02.mkv",
		"The.Last.of.Us.E09.1080p.WEB-DL-GRP.mkv": "最后生还者 - S01E09.mkv",
	}, targets)
	assert.Contains(t, p.Notes, "The.Last.of.Us.E09.1080p.WEB-DL-GRP.mkv：剧集组「Absolute Order」里没有第 1 组第 9 集，按文件名里的季集整理")

	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: absHash})
	require.NoError(t, err)
	assert.Equal(t, 3, res.Created)
	byEp := map[int]models.MediaTransferHistory{}
	for _, r := range e.history() {
		byEp[r.Season*100+r.Episode] = r
	}
	require.Len(t, byEp, 3)
	assert.Equal(t, "剧集组「Absolute Order」的第 1 组第 3 集是 S02E01", byEp[201].Message)
	assert.Equal(t, "剧集组「Absolute Order」的第 1 组第 4 集是 S02E02", byEp[202].Message)
	assert.True(t, strings.HasPrefix(byEp[109].Message, "剧集组「Absolute Order」里没有第 1 组第 9 集"), byEp[109].Message)
	assert.True(t, exists(filepath.Join(e.tv, "最后生还者 (2023)", "Season 2", "最后生还者 - S02E01.mkv")))
}

// 读不到剧集组时不整理：TMDB 暂时不能访问的稍后重试，剧集组没了的写明怎么办；整理的条目不是追剧那部剧时不用剧集组。
func TestOrganizeEpisodeGroupUnavailable(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.absTorrent()
	hint := MediaHint{MediaType: models.MediaKindTV, TMDBID: 100088, EpisodeGroup: tlouAbsolute}
	e.svc.SetMediaFor(func(context.Context, uint, string) (MediaHint, bool, error) { return hint, true, nil })

	e.tmdb.mu.Lock()
	e.tmdb.groupDown = true
	e.tmdb.mu.Unlock()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: absHash})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "读不到追剧选的剧集组，稍后重试")
	rows := e.history()
	require.NotEmpty(t, rows)
	assert.Equal(t, models.MediaFailRecognize, rows[0].FailKind)
	assert.NotNil(t, rows[0].NextRetryAt, "可以重试")
	assert.Zero(t, res.Created)

	e.tmdb.mu.Lock()
	e.tmdb.groupDown = false
	e.tmdb.mu.Unlock()
	hint.EpisodeGroup = "5b11ba820e0a265a53000000"
	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: absHash})
	require.NoError(t, err)
	assert.Equal(t, "追剧选的剧集组在 TMDB 上找不到了：在追剧的「修改」里换一个剧集组或不用剧集组，再重新整理", p.Problem)

	// 手动指定了别的条目：不用追剧的剧集组
	p, err = e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: absHash, MediaType: models.MediaKindTV, TMDBID: 1396})
	require.NoError(t, err)
	require.Empty(t, p.Problem)
	for _, it := range p.Items {
		assert.Equal(t, 1, it.Season, it.Rel)
		assert.NotContains(t, it.Target, "S02", it.Rel)
	}
	assert.Empty(t, p.Notes)
}
