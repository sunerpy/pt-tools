package subscribe

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 追剧推整季包（或一个种子里有几集）补缺的集时只下缺的集：库里已有的集在下载器里设成不下载
// （2026-10-25 用户：「我只要后四集 却下载了一整个合集」）。

const nhPack = "Natural.High.S04.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB"

func nhFile(n int) string {
	return fmt.Sprintf("Natural.High.S04E%02d.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB.mkv", n)
}

// packEnv：TMDB 上第 4 季 4 集都播完了，库里有 have 这几集；站点上有整季包 801，文件是 files（默认第 1–4 集、
// 第 3 集的字幕与一个 nfo）；下载器里列出同样的文件（带种子的根目录）。
func packEnv(t *testing.T, have []int, files ...string) (*env, *models.MediaSubscription, string) {
	t.Helper()
	e := newEnv(t)
	e.tmdb.set("/3/tv/231620/season/4", `{"season_number":4,"episodes":[
		{"episode_number":1,"name":"第1期上","air_date":"2026-10-01"},{"episode_number":2,"name":"第1期下","air_date":"2026-10-02"},
		{"episode_number":3,"name":"第2期上","air_date":"2026-10-03"},{"episode_number":4,"name":"第2期下","air_date":"2026-10-04"}]}`)
	sub := e.follow(func(in *SubscriptionInput) { in.LockVersion = off() })
	e.enable(func(s *Settings) { s.NotifyChannels = []uint{3} })
	for _, n := range have {
		e.organized(fmt.Sprintf("h%d", n), models.MediaKindTV, 231620, 4, n, fmt.Sprintf("/lib/nh/E%02d.mkv", n))
	}
	if len(files) == 0 {
		files = []string{nhFile(1), nhFile(2), nhFile(3), strings.TrimSuffix(nhFile(3), ".mkv") + ".chs.srt", nhFile(4), "HHWEB.nfo"}
	}
	pack := e.item("mteam", "801", nhPack, "", 8, 10)
	e.site.files["801"] = packBytes(nhPack, files...)
	hash := e.hashOf("801")
	list := make([]downloader.TorrentFile, 0, len(files))
	for i, f := range files {
		list = append(list, downloader.TorrentFile{Index: i, Name: nhPack + "/" + f, Size: 1 << 30, Priority: 1})
	}
	e.dl.setFiles(hash, list...)
	e.search.set(pack)
	return e, sub, hash
}

func runSteps(e *env, id uint) string {
	runs := e.runs(id)
	if len(runs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, st := range runs[0].Steps {
		b.WriteString(st.Kind + ":" + st.Text + "\n")
	}
	return b.String()
}

func TestPackDownloadsOnlyMissingEpisodes(t *testing.T) {
	e, sub, hash := packEnv(t, []int{1, 2})
	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"801"}, pushedIDs(e), msg)
	assert.Contains(t, msg, "下载了 "+nhPack)
	assert.Contains(t, msg, "只下 E03、E04")
	// 第 1、2 集的视频设成不下载；第 3 集的字幕、nfo 这些对不出集或是要的，不动
	assert.Equal(t, []prioCall{{ID: hash, Indexes: []int{0, 1}, Priority: 0}}, e.dl.prioCalls())
	assert.Contains(t, runSteps(e, sub.ID), "种子里另外 2 个视频文件（E01、E02）设成了不下载")

	linked := e.linked(sub.ID)
	require.Len(t, linked, 1)
	assert.True(t, linked[0].Complete)
	assert.Equal(t, "[3,4]", linked[0].CoveredEpisodes, "只记实际要下的集")
	assert.Empty(t, e.ledger(sub).missing)

	found := e.noticesOf(NoticeFollowFound)
	require.Len(t, found, 1)
	assert.Contains(t, found[0].Body, "第 3–4 集（整季包里只下这几集）")
	assert.NotContains(t, found[0].Body, "整季\n")
	assert.Equal(t, "第 3–4 集（整季包里只下这几集）", found[0].Event.Fields["episodes"])
}

// 一个种子里有几集（不是整季包）也一样：库里已有的那集不下。
func TestMultiEpisodeTorrentDownloadsOnlyMissing(t *testing.T) {
	e, sub, _ := packEnv(t, []int{1}, nhFile(4))
	rangeTitle := "Natural.High.S04E01-E03.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB"
	rng := e.item("mteam", "803", rangeTitle, "", 3, 10)
	e.site.files["803"] = packBytes(rangeTitle, nhFile(1), nhFile(2), nhFile(3))
	rh := e.hashOf("803")
	e.dl.setFiles(rh,
		downloader.TorrentFile{Index: 0, Name: rangeTitle + "/" + nhFile(1), Priority: 1},
		downloader.TorrentFile{Index: 1, Name: rangeTitle + "/" + nhFile(2), Priority: 1},
		downloader.TorrentFile{Index: 2, Name: rangeTitle + "/" + nhFile(3), Priority: 1})
	single := e.item("mteam", "804", strings.TrimSuffix(nhFile(4), ".mkv"), "", 1, 10)
	e.search.set(rng, single)

	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"803", "804"}, pushedIDs(e), msg)
	assert.Equal(t, []prioCall{{ID: rh, Indexes: []int{0}, Priority: 0}}, e.dl.prioCalls())
	for _, l := range e.linked(sub.ID) {
		if l.TorrentID == "803" {
			assert.Equal(t, "[2,3]", l.CoveredEpisodes)
		}
	}
}

// 种子里的视频都对得出集、却没有一集是缺的：不推（以前会把整个包下下来）。
func TestPackWithoutMissingEpisodesNotPushed(t *testing.T) {
	e, sub, _ := packEnv(t, []int{1, 2}, nhFile(1), nhFile(2), "HHWEB.nfo")
	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Empty(t, pushedIDs(e), msg)
	assert.Contains(t, msg, "种子里没有缺的集（种子里是 E01、E02）")
	assert.Empty(t, e.dl.prioCalls())
	assert.Empty(t, e.linked(sub.ID))
	assert.NotContains(t, runSteps(e, sub.ID), stepFail+":", "不是推送失败")
}

// 下载器不支持只下部分文件、设置失败、下载器里本来就有这个种子：整个种子照常下，写明原因；本来就有的不去改它。
func TestPackSelectFallsBackToWholeTorrent(t *testing.T) {
	t.Run("下载器不支持", func(t *testing.T) {
		e, sub, _ := packEnv(t, []int{1, 2})
		e.dl.noSelect = true
		msg, err := e.svc.SearchNow(e.ctx, sub.ID)
		require.NoError(t, err)
		assert.Equal(t, []string{"801"}, pushedIDs(e))
		assert.NotContains(t, msg, "只下")
		assert.Contains(t, runSteps(e, sub.ID), "下载器不支持只下种子里的部分文件，整个种子都会下载")
		assert.Equal(t, "[1,2,3,4]", e.linked(sub.ID)[0].CoveredEpisodes)
	})
	t.Run("设置失败", func(t *testing.T) {
		e, sub, _ := packEnv(t, []int{1, 2})
		e.dl.prioErr = errors.New("HTTP 500")
		msg, err := e.svc.SearchNow(e.ctx, sub.ID)
		require.NoError(t, err)
		assert.Contains(t, msg, "下载了 "+nhPack)
		assert.NotContains(t, msg, "只下")
		assert.Contains(t, runSteps(e, sub.ID), "没能把不缺的集设成不下载（HTTP 500），整个种子都会下载")
		assert.Equal(t, "[1,2,3,4]", e.linked(sub.ID)[0].CoveredEpisodes)
	})
	t.Run("下载器里本来就有", func(t *testing.T) {
		e, sub, _ := packEnv(t, []int{1, 2})
		e.pushRes = &ptinternal.PushTorrentResult{Success: true, Skipped: true}
		_, err := e.svc.SearchNow(e.ctx, sub.ID)
		require.NoError(t, err)
		assert.Equal(t, []string{"801"}, pushedIDs(e))
		assert.Empty(t, e.dl.prioCalls(), "别处加的种子不改它选的文件")
	})
}
