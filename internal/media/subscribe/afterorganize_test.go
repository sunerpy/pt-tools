package subscribe

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/models"
)

// followOneEpisode 建一个追剧、推送第 1 集，返回订阅与这个种子（下载中）。
func followOneEpisode(t *testing.T, e *env) (*models.MediaSubscription, models.MediaSubscriptionTorrent) {
	t.Helper()
	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	e.search.set(e.item("hdsky", "91", "The.Last.of.Us.S02E01.1080p.WEB-DL.H264-X", "", 3, 10))
	_, err := e.svc.SearchNow(e.ctx, tv.ID)
	require.NoError(t, err)
	tor := e.linked(tv.ID)[0]
	require.Equal(t, models.MediaSubTorrentDownloading, tor.Status)
	return tv, tor
}

// epState 是追剧台账里第 n 集的状态（订阅详情里看到的）；读不到时是空的。不调用 require：在 Eventually 里用。
func epState(e *env, subID uint, n int) string {
	d, err := e.svc.Subscription(e.ctx, subID)
	if err != nil || d.Progress == nil {
		return ""
	}
	for _, ep := range d.Progress.Episodes {
		if ep.Number == n {
			return ep.State
		}
	}
	return ""
}

// waitLibrary 等种子标成已入库、台账里第 1 集是已入库（回调在后台循环里处理：先改种子，再重建台账）。
func waitLibrary(t *testing.T, e *env, subID uint) {
	t.Helper()
	require.Eventually(t, func() bool {
		rows, err := e.svc.torrents(e.ctx, subID)
		return err == nil && len(rows) == 1 && rows[0].Status == models.MediaSubTorrentDone && epState(e, subID, 1) == models.MediaEpisodeLibrary
	}, 5*time.Second, 20*time.Millisecond)
}

// 整理完成的回调到了：不等下一拍，种子马上标成已入库、台账里这一集是已入库。
func TestAfterOrganizeRefreshesNow(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	tv, tor := followOneEpisode(t, e)
	e.organized(tor.InfoHash, models.MediaKindTV, 100088, 2, 1, "/lib/S02E01.mkv")
	e.svc.Start(e.ctx)
	t.Cleanup(e.svc.Stop)

	e.svc.AfterOrganize(tor.InfoHash)
	waitLibrary(t, e, tv.ID)
}

// 总开关关着时回调不动订阅（和后台刷新一样）。
func TestAfterOrganizeIgnoredWhenDisabled(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	tv, tor := followOneEpisode(t, e)
	e.organized(tor.InfoHash, models.MediaKindTV, 100088, 2, 1, "/lib/S02E01.mkv")
	_, err := e.svc.SaveSettings(e.ctx, Settings{Enabled: false})
	require.NoError(t, err)
	e.svc.refreshHash(e.ctx, tor.InfoHash)
	assert.Equal(t, models.MediaSubTorrentDownloading, e.linked(tv.ID)[0].Status)
}

// 回调只是放进队列：后台循环没在跑、队列满了也马上返回（多出来的丢掉，下一拍的刷新兜底）。
func TestAfterOrganizeNeverBlocks(t *testing.T) {
	e := newEnv(t)
	done := make(chan struct{})
	go func() {
		for range 3 * afterOrganizeQueue {
			e.svc.AfterOrganize("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("队列满了以后 AfterOrganize 阻塞了")
	}
	assert.Len(t, e.svc.after, afterOrganizeQueue)
}

// upgradeMovie 先下 1080p 并入库，再下 2160p（下载中）：返回订阅、旧版本的整理记录与新版本的种子。
func upgradeMovie(t *testing.T, e *env) (*models.MediaSubscription, models.MediaTransferHistory, models.MediaSubscriptionTorrent) {
	t.Helper()
	e.enable(nil)
	p, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "4K 原盘", Resolutions: []string{"2160p", "1080p"}, Sources: []string{"UHD BluRay", "WEB-DL"}})
	require.NoError(t, err)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: p.ID, Upgrade: true})
	e.search.set(e.item("hdsky", "61", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "", 8, 50))
	_, err = e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	old := e.organized(e.hashOf("61"), models.MediaKindMovie, 693134, 0, 0, "/lib/dune2/1080p.mkv")
	e.svc.refresh(e.ctx)
	e.search.set(
		e.item("hdsky", "61", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "", 8, 50),
		e.item("hdsky", "63", "Dune.Part.Two.2024.2160p.UHD.BluRay.x265-FRDS", "", 60, 20),
	)
	_, err = e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	tor := e.linkOf(m.ID, "63")
	require.Equal(t, models.MediaSubTorrentDownloading, tor.Status)
	require.Empty(t, e.org.retired)
	return m, old, tor
}

// 「下载中 → 已入库」只转换一次：拿着同一份旧行刷新两次，洗版替换只跑一次。
func TestRefreshTorrentClaimsOnce(t *testing.T) {
	e := newEnv(t)
	_, old, tor := upgradeMovie(t, e)
	e.organized(tor.InfoHash, models.MediaKindMovie, 693134, 0, 0, "/lib/dune2/2160p.mkv")
	set, err := e.svc.Settings(e.ctx)
	require.NoError(t, err)
	stale := tor
	assert.True(t, e.svc.refreshTorrent(e.ctx, &stale, set), "第一次由它转换")
	stale = tor
	assert.False(t, e.svc.refreshTorrent(e.ctx, &stale, set), "第二次已经转换过了")
	assert.Equal(t, []uint{old.ID}, e.org.retired, "旧版本只换一次")
}

// 「重新整理」与整理完成的回调都来一次（两种先后顺序）：洗版替换也只跑一次。
func TestOrganizeTorrentAndHookReplaceOnce(t *testing.T) {
	for _, hookFirst := range []bool{false, true} {
		name := "重新整理先"
		if hookFirst {
			name = "回调先"
		}
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			m, old, tor := upgradeMovie(t, e)
			e.org.onOrganize = func(organize.Request) {
				e.organized(tor.InfoHash, models.MediaKindMovie, 693134, 0, 0, "/lib/dune2/2160p.mkv")
				if hookFirst {
					e.svc.refreshHash(e.ctx, tor.InfoHash)
				}
			}
			_, err := e.svc.OrganizeTorrent(e.ctx, m.ID, tor.ID)
			require.NoError(t, err)
			if !hookFirst {
				e.svc.refreshHash(e.ctx, tor.InfoHash)
			}
			assert.Equal(t, []uint{old.ID}, e.org.retired)
			assert.Equal(t, models.MediaSubTorrentDone, e.linkOf(m.ID, "63").Status)
		})
	}
}

// 7 天没入库标成失败的种子点「重新整理」，请求在整理期间取消了：种子照样改回等整理，整理在后台做完、回调到了以后标成已入库。
// 以前改回等整理用的是请求的 context，请求一取消就改不成，种子一直留在失败。
func TestOrganizeFailedTorrentRequestCancelled(t *testing.T) {
	cases := []struct {
		name string
		// finishedBeforeCancel 为真时整理在请求取消以前做完（调用方拿到了结果）；否则调用方先拿到 Queued，整理之后才做完
		finishedBeforeCancel bool
	}{
		{"整理等待中取消", false},
		{"整理做完以后取消", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			e.enable(nil)
			tv, tor := followOneEpisode(t, e)
			e.advance(8 * 24 * time.Hour)
			e.svc.refresh(e.ctx)
			require.Equal(t, models.MediaSubTorrentFailed, e.linked(tv.ID)[0].Status)
			e.svc.Start(e.ctx)
			t.Cleanup(e.svc.Stop)

			ctx, cancel := context.WithCancel(e.ctx)
			defer cancel()
			e.org.queuedOnCancel = true
			e.org.onOrganize = func(organize.Request) {
				if c.finishedBeforeCancel {
					e.organized(tor.InfoHash, models.MediaKindTV, 100088, 2, 1, "/lib/S02E01.mkv")
					e.org.queuedOnCancel = false // 结果在取消以前送到了
				}
				cancel()
			}
			res, err := e.svc.OrganizeTorrent(ctx, tv.ID, tor.ID)
			require.NoError(t, err)
			assert.Equal(t, !c.finishedBeforeCancel, res.Queued)
			if !c.finishedBeforeCancel {
				assert.Equal(t, models.MediaSubTorrentDownloading, e.linked(tv.ID)[0].Status, "请求取消了也改回等整理")
				// 整理在后台做完，回调到了
				e.organized(tor.InfoHash, models.MediaKindTV, 100088, 2, 1, "/lib/S02E01.mkv")
			}
			e.svc.AfterOrganize(tor.InfoHash)
			waitLibrary(t, e, tv.ID)
			assert.Empty(t, e.org.retired)
		})
	}
}
