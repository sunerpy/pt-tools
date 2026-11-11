package subscribe

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 推送一个有几集的种子时设成不下载的集（当时在别处下载），之后又缺了：再推一次，只下这几集。
// 照生产上的第 44 集写的：认领的刷流种子被删了，LongWeb E44-E45 只下了 E45，这一集一直等。

const nhRange = "Natural.High.S04E01-E02.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB"

// organizedAt 记一条 at 时整理好的记录（e.organized 的整理时间是真的现在，和测试里的时钟对不上）。
func (e *env) organizedAt(hash string, episode int, at time.Time) models.MediaTransferHistory {
	e.t.Helper()
	row := e.organized(hash, models.MediaKindTV, 231620, 4, episode, "/lib/nh/"+nhFile(episode))
	require.NoError(e.t, e.db.Model(&models.MediaTransferHistory{}).Where("id = ?", row.ID).UpdateColumn("created_at", at).Error)
	return row
}

// repushEnv：现在就出发第 4 季播完了 4 集。下载器里有别处加的第 1 集（追剧认领它），站点上有第 1–2 集的种子 803：
// 推送时第 1 集在下载，只下了第 2 集；803 留在下载器里，第 2 集整理入库。返回订阅、803 的 info hash 与下载器里的 803。
func repushEnv(t *testing.T) (*env, *models.MediaSubscription, string, downloader.Torrent) {
	t.Helper()
	e := newEnv(t)
	e.tmdb.set("/3/tv/231620/season/4", `{"season_number":4,"episodes":[
		{"episode_number":1,"name":"第1期上","air_date":"2026-10-01"},{"episode_number":2,"name":"第1期下","air_date":"2026-10-02"},
		{"episode_number":3,"name":"第2期上","air_date":"2026-10-03"},{"episode_number":4,"name":"第2期下","air_date":"2026-10-04"}]}`)
	sub := e.follow(func(in *SubscriptionInput) { in.LockVersion = off() })
	e.enable(func(s *Settings) { s.NotifyChannels = []uint{3} })
	elsewhere := downloader.Torrent{ID: "e01", InfoHash: "eeee0000000000000000000000000000000000e1", Name: strings.TrimSuffix(nhFile(1), ".mkv"), DateAdded: 100}
	rng := e.item("mteam", "803", nhRange, "", 2, 10)
	e.site.files["803"] = packBytes(nhRange, nhFile(1), nhFile(2))
	rh := e.hashOf("803")
	e.dl.setFiles(rh,
		downloader.TorrentFile{Index: 0, Name: nhRange + "/" + nhFile(1), Size: 1 << 30, Priority: 1},
		downloader.TorrentFile{Index: 1, Name: nhRange + "/" + nhFile(2), Size: 1 << 30, Priority: 1})
	e.dl.setTorrents(elsewhere)
	e.search.set(rng)
	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"803"}, pushedIDs(e), msg)
	require.Equal(t, []prioCall{{ID: rh, Indexes: []int{0}, Priority: 0}}, e.dl.prioCalls(), "第 1 集在别处下载，设成不下载")
	row := e.linkOf(sub.ID, "803")
	require.Equal(t, "[2]", row.CoveredEpisodes)
	require.Equal(t, "[1]", row.SkippedEpisodes, "设成不下载的集记下来")

	inDL := downloader.Torrent{ID: rh, InfoHash: rh, Name: nhRange, Progress: 1}
	e.dl.setTorrents(elsewhere, inDL)
	e.organizedAt(rh, 2, e.Now())
	e.advance(2 * time.Minute)
	e.svc.refresh(e.ctx)
	require.Equal(t, models.MediaSubTorrentDone, e.linkOf(sub.ID, "803").Status)
	return e, sub, rh, inDL
}

// dropElsewhere 把别处加的第 1 集从下载器里删掉（keep 是留在下载器里的种子），等到认领的种子判成失败。
func dropElsewhere(t *testing.T, e *env, sub *models.MediaSubscription, keep ...downloader.Torrent) {
	t.Helper()
	e.relist(append([]downloader.Torrent{otherTorrent}, keep...)...)
	e.advance(11 * time.Minute)
	e.svc.refresh(e.ctx)
	adopted := e.linkOf(sub.ID, "eeee0000000000000000000000000000000000e1")
	require.Equal(t, models.MediaSubTorrentFailed, adopted.Status)
	runs := e.runs(sub.ID)
	require.NotEmpty(t, runs)
	assert.Contains(t, runs[0].Summary, "认领的「"+strings.TrimSuffix(nhFile(1), ".mkv")+"」已经不在下载器里了")
	assert.Contains(t, e.ledger(sub).missing, 1, "第 1 集重新算缺的")
}

func (e *env) torrentInfo(site, id string) models.TorrentInfo {
	e.t.Helper()
	var info models.TorrentInfo
	require.NoError(e.t, e.db.Where("site_name = ? AND torrent_id = ?", site, id).First(&info).Error)
	return info
}

// 803 还在下载器里：把第 1 集的文件改回下载，种子记录改回没下完（下完时再发完成事件），按这次的策略记账；
// 以前整理的第 2 集不算这一次入库，整理出第 1 集才算；补上以后不再推。
func TestRepushSkippedEpisodeStillInDownloader(t *testing.T) {
	e, sub, rh, inDL := repushEnv(t)
	dropElsewhere(t, e, sub, inDL)
	e.dl.setFiles(rh,
		downloader.TorrentFile{Index: 0, Name: nhRange + "/" + nhFile(1), Size: 1 << 30, Priority: 0},
		downloader.TorrentFile{Index: 1, Name: nhRange + "/" + nhFile(2), Size: 1 << 30, Priority: 1})
	require.NoError(t, e.db.Model(&models.TorrentInfo{}).Where("site_name = ? AND torrent_id = ?", "mteam", "803").
		Updates(map[string]any{"is_completed": true, "completed_at": e.Now()}).Error)
	e.mu.Lock()
	e.pushRes = &ptinternal.PushTorrentResult{Success: true, Skipped: true}
	e.mu.Unlock()
	calls, pushes := len(e.dl.prioCalls()), len(e.gotPushes())

	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "下载了 "+nhRange+"（再下一次，只下 E01）")
	require.Len(t, e.gotPushes(), pushes+1)
	assert.True(t, e.gotPushes()[pushes].AdoptExisting, "下载器里还有：按这次的策略与优惠记账")
	assert.Equal(t, []prioCall{{ID: rh, Indexes: []int{0}, Priority: 1}}, e.dl.prioCalls()[calls:])
	assert.Contains(t, runSteps(e, sub.ID), nhRange+" 还在下载器里：把 E01 的文件改回下载")
	row := e.linkOf(sub.ID, "803")
	assert.Equal(t, models.MediaSubTorrentDownloading, row.Status)
	assert.Equal(t, "[1]", row.CoveredEpisodes)
	assert.Equal(t, "[]", row.SkippedEpisodes)
	require.NotNil(t, row.RepushedAt)
	assert.Equal(t, "再下一次，补上当时没下的 E01", row.Message)
	info := e.torrentInfo("mteam", "803")
	assert.False(t, info.IsCompleted, "改回没下完：下完时再发完成事件")
	assert.Nil(t, info.CompletedAt)
	found := e.noticesOf(NoticeFollowFound)
	require.NotEmpty(t, found)
	assert.Contains(t, found[len(found)-1].Body, "第 1 集")

	// 以前整理的第 2 集不算：还在下载
	e.advance(2 * time.Minute)
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaSubTorrentDownloading, e.linkOf(sub.ID, "803").Status)
	assert.Equal(t, models.MediaEpisodeDownloading, e.state(sub, 1))

	// 整理出第 1 集：已入库；不再推
	e.organizedAt(rh, 1, e.Now())
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaSubTorrentDone, e.linkOf(sub.ID, "803").Status)
	assert.Equal(t, models.MediaEpisodeLibrary, e.state(sub, 1))
	pushes = len(e.gotPushes())
	_, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Len(t, e.gotPushes(), pushes)
}

// 803 不在下载器里了：重新加进去，这次只下第 1 集（第 2 集已经入库，设成不下载、记下来）。
func TestRepushSkippedEpisodeTorrentGone(t *testing.T) {
	e, sub, rh, _ := repushEnv(t)
	dropElsewhere(t, e, sub)
	calls := len(e.dl.prioCalls())

	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "下载了 "+nhRange+"（再下一次，只下 E01）")
	assert.Equal(t, []prioCall{{ID: rh, Indexes: []int{1}, Priority: 0}}, e.dl.prioCalls()[calls:], "第 2 集已经入库，设成不下载")
	row := e.linkOf(sub.ID, "803")
	assert.Equal(t, models.MediaSubTorrentDownloading, row.Status)
	assert.Equal(t, "[1]", row.CoveredEpisodes)
	assert.Equal(t, "[2]", row.SkippedEpisodes)
	require.NotNil(t, row.RepushedAt)
	assert.Len(t, e.linked(sub.ID), 2, "还是同一条记录（另一条是认领的）")
}

// 设成不下载的集还没缺（在别处下载、库里有）：订阅已经下载过的种子照旧不再推。
func TestRepushOnlyWhenSkippedEpisodeMissing(t *testing.T) {
	e, sub, _, _ := repushEnv(t)
	pushes := len(e.gotPushes())
	e.advance(2 * time.Minute)
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Len(t, e.gotPushes(), pushes, "第 1 集还在别处下载")
}

// 再推的种子在下载器里下完了，过了 10 分钟还没有新的整理记录（完成事件丢了，或者在下载器算出新的进度以前就发了）：
// 订阅让整理入库按订阅的条目整理一次，一小时内不再重复；整理出记录以后标成已入库。
func TestRepushedTorrentOrganizedWhenEventMissed(t *testing.T) {
	e, sub, rh, inDL := repushEnv(t)
	dropElsewhere(t, e, sub, inDL)
	e.dl.setFiles(rh,
		downloader.TorrentFile{Index: 0, Name: nhRange + "/" + nhFile(1), Size: 1 << 30, Priority: 0},
		downloader.TorrentFile{Index: 1, Name: nhRange + "/" + nhFile(2), Size: 1 << 30, Priority: 1})
	e.mu.Lock()
	e.pushRes = &ptinternal.PushTorrentResult{Success: true, Skipped: true}
	e.mu.Unlock()
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, models.MediaSubTorrentDownloading, e.linkOf(sub.ID, "803").Status)
	organized := func() []organize.Request {
		e.org.mu.Lock()
		defer e.org.mu.Unlock()
		return append([]organize.Request(nil), e.org.organized...)
	}

	// 还在下第 1 集：不管
	e.relist(otherTorrent, downloader.Torrent{ID: rh, InfoHash: rh, Name: nhRange, Progress: 0.6})
	e.advance(20 * time.Minute)
	e.svc.refresh(e.ctx)
	assert.Empty(t, organized())

	// 下完了，没有新的整理记录：整理一次，一小时内不再重复
	e.relist(otherTorrent, inDL)
	require.Equal(t, []organize.Request{{DownloaderID: 1, Hash: rh, MediaType: models.MediaKindTV, TMDBID: 231620}}, organized())
	e.advance(30 * time.Minute)
	e.svc.refresh(e.ctx)
	assert.Len(t, organized(), 1)

	// 一小时以后再整理一次，这次整理出了第 1 集：标成已入库
	e.org.mu.Lock()
	e.org.onOrganize = func(organize.Request) { e.organizedAt(rh, 1, e.Now()) }
	e.org.mu.Unlock()
	e.advance(31 * time.Minute)
	e.svc.refresh(e.ctx)
	assert.Len(t, organized(), 2)
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaSubTorrentDone, e.linkOf(sub.ID, "803").Status)
	assert.Len(t, organized(), 2, "有了新的整理记录不再整理")
}

func TestSkippedOf(t *testing.T) {
	cases := []struct {
		name string
		row  models.MediaSubscriptionTorrent
		want []int
	}{
		{"记下的", models.MediaSubscriptionTorrent{Episode: 44, EpisodeEnd: 45, CoveredEpisodes: "[45]", SkippedEpisodes: "[44]"}, []int{44}},
		{"补上了", models.MediaSubscriptionTorrent{Episode: 44, EpisodeEnd: 45, CoveredEpisodes: "[44]", SkippedEpisodes: "[]"}, []int{}},
		{"升级前推送的：按种子名的集减去要下的", models.MediaSubscriptionTorrent{Episode: 44, EpisodeEnd: 46, CoveredEpisodes: "[45]"}, []int{44, 46}},
		{"整季包", models.MediaSubscriptionTorrent{Complete: true, CoveredEpisodes: "[1,2]"}, nil},
		{"单集", models.MediaSubscriptionTorrent{Episode: 3, CoveredEpisodes: "[3]"}, nil},
		{"没选过文件", models.MediaSubscriptionTorrent{Episode: 3, EpisodeEnd: 4}, nil},
		{"记下的读不出", models.MediaSubscriptionTorrent{Episode: 3, EpisodeEnd: 4, CoveredEpisodes: "[4]", SkippedEpisodes: "x"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := skippedOf(c.row)
			if c.want == nil {
				assert.Nil(t, got)
				return
			}
			assert.ElementsMatch(t, c.want, got)
		})
	}
}

func TestRepushable(t *testing.T) {
	lg := &ledger{missing: []int{44}}
	base := models.MediaSubscriptionTorrent{SiteName: "mteam", Status: models.MediaSubTorrentDone, Episode: 44, EpisodeEnd: 45, CoveredEpisodes: "[45]"}
	assert.True(t, repushable(base, lg))
	assert.False(t, repushable(base, nil), "没有台账")
	assert.False(t, repushable(base, &ledger{missing: []int{46}}), "设成不下载的集不缺")
	for name, mod := range map[string]func(*models.MediaSubscriptionTorrent){
		"还在下载": func(t *models.MediaSubscriptionTorrent) { t.Status = models.MediaSubTorrentDownloading },
		"失败了":  func(t *models.MediaSubscriptionTorrent) { t.Status = models.MediaSubTorrentFailed },
		"认领的":  func(t *models.MediaSubscriptionTorrent) { t.Adopted = true },
		"下载器里的": func(t *models.MediaSubscriptionTorrent) {
			t.SiteName = models.MediaAdoptedSitePrefix + "1"
		},
	} {
		row := base
		mod(&row)
		assert.False(t, repushable(row, lg), name)
	}
}
