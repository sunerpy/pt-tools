package subscribe

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
)

// libraryFile 记一条库里已有的文件（不是订阅下载的：只有种子名）。
func (e *env) libraryFile(hash, name, kind string, tmdbID, season, episode, episodeEnd int, target string) models.MediaTransferHistory {
	e.t.Helper()
	row := models.MediaTransferHistory{
		InfoHash: hash, TorrentName: name, SourcePath: fmt.Sprintf("/dl/%s/%d", hash, episode), TargetPath: target, MediaType: kind,
		TMDBID: tmdbID, Season: season, Episode: episode, EpisodeEnd: episodeEnd, Status: models.MediaTransferDone, Mode: models.MediaModeHardlink,
	}
	require.NoError(e.t, e.db.Create(&row).Error)
	return row
}

func (e *env) linkOf(subID uint, torrentID string) models.MediaSubscriptionTorrent {
	e.t.Helper()
	for _, l := range e.linked(subID) {
		if l.TorrentID == torrentID {
			return l
		}
	}
	e.t.Fatalf("订阅 %d 没有种子 %s", subID, torrentID)
	return models.MediaSubscriptionTorrent{}
}

func (e *env) profile4K() Profile {
	e.t.Helper()
	p, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "4K", Resolutions: []string{"2160p", "1080p"}})
	require.NoError(e.t, err)
	return p
}

// 整季包只整理成一部分：还在自动重试时先不换；重试用完以后只换整理成的集，旧整季包还有集在库里时不算换下来、不删
func TestUpgradePartialPackReplacesOnlyOrganizedEpisodes(t *testing.T) {
	e := newEnv(t)
	e.enable(func(s *Settings) { s.UpgradeOld = models.MediaUpgradeDelete })
	e.advance(60 * 24 * time.Hour) // 3 集都播完了
	p := e.profile4K()
	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, ProfileID: p.ID, Upgrade: true})
	e.search.set(e.item("hdsky", "a1", "The.Last.of.Us.S02.1080p.WEB-DL.H264-X", "", 12, 10))
	msg, err := e.svc.SearchNow(e.ctx, tv.ID)
	require.NoError(t, err)
	require.Contains(t, msg, "下载了 The.Last.of.Us.S02.1080p")
	var olds []models.MediaTransferHistory
	for ep := 1; ep <= 3; ep++ {
		olds = append(olds, e.organized(e.hashOf("a1"), models.MediaKindTV, 100088, 2, ep, fmt.Sprintf("/lib/tlou/s02e0%d.1080p.mkv", ep)))
	}
	e.svc.refresh(e.ctx)
	require.Equal(t, models.MediaSubTorrentDone, e.linkOf(tv.ID, "a1").Status)

	e.search.set(e.item("hdsky", "b1", "The.Last.of.Us.S02.2160p.WEB-DL.H265-X", "", 30, 10))
	msg, err = e.svc.SearchNow(e.ctx, tv.ID)
	require.NoError(t, err)
	require.Contains(t, msg, "下载了 The.Last.of.Us.S02.2160p")
	nh := e.hashOf("b1")
	e.organized(nh, models.MediaKindTV, 100088, 2, 1, "/lib/tlou/s02e01.2160p.mkv")
	e.organized(nh, models.MediaKindTV, 100088, 2, 3, "/lib/tlou/s02e03.2160p.mkv")
	retryAt := e.Now().Add(time.Hour)
	failed := models.MediaTransferHistory{
		InfoHash: nh, SourcePath: "/dl/b1/2", MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episode: 2,
		Status: models.MediaTransferFailed, NextRetryAt: &retryAt,
	}
	require.NoError(t, e.db.Create(&failed).Error)

	e.svc.refresh(e.ctx)
	assert.Empty(t, e.org.retired, "第 2 集还在自动重试，先不换")
	assert.Equal(t, models.MediaSubTorrentDownloading, e.linkOf(tv.ID, "b1").Status)

	// 到期重试排上了队：记录里的重试时间先清掉了，整理服务知道它还在重试
	require.NoError(t, e.db.Model(&failed).Update("next_retry_at", nil).Error)
	e.org.setRetrying(failed.ID, true)
	e.svc.refresh(e.ctx)
	assert.Empty(t, e.org.retired, "排上队的重试还没整理完，先不换")
	assert.Equal(t, models.MediaSubTorrentDownloading, e.linkOf(tv.ID, "b1").Status)

	e.org.setRetrying(failed.ID, false)
	e.svc.refresh(e.ctx)
	assert.ElementsMatch(t, []uint{olds[0].ID, olds[2].ID}, e.org.retired, "只换整理成的第 1、3 集")
	nb := e.linkOf(tv.ID, "b1")
	assert.Equal(t, models.MediaSubTorrentDone, nb.Status)
	assert.Contains(t, nb.Message, "1 个文件没整理成")
	assert.Equal(t, models.MediaSubTorrentDone, e.linkOf(tv.ID, "a1").Status, "旧整季包的第 2 集还在库里")
	assert.Empty(t, e.dl.removed, "旧种子不删")
}

// 旧文件是 E02–E03 的合集，新版本只有第 2 集：旧文件不删
func TestUpgradeKeepsMultiEpisodeFileNotFullyCovered(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	e.advance(60 * 24 * time.Hour)
	p := e.profile4K()
	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, ProfileID: p.ID, Upgrade: true})
	e.libraryFile("e1old", "The.Last.of.Us.S02E01.1080p.WEB-DL.H264-OLD", models.MediaKindTV, 100088, 2, 1, 0, "/lib/tlou/s02e01.mkv")
	multi := e.libraryFile("e23old", "The.Last.of.Us.S02E02-E03.1080p.WEB-DL.H264-OLD", models.MediaKindTV, 100088, 2, 2, 3, "/lib/tlou/s02e02-e03.mkv")
	e.search.set(e.item("hdsky", "c2", "The.Last.of.Us.S02.2160p.WEB-DL.H265-X", "", 30, 10))
	msg, err := e.svc.SearchNow(e.ctx, tv.ID)
	require.NoError(t, err)
	require.Contains(t, msg, "下载了 The.Last.of.Us.S02.2160p", "都在库里但分数更高的整季包")
	e.organized(e.hashOf("c2"), models.MediaKindTV, 100088, 2, 2, "/lib/tlou/s02e02.2160p.mkv")
	e.svc.refresh(e.ctx)
	assert.NotContains(t, e.org.retired, multi.ID, "合集里的第 3 集新版本没有")
	assert.Empty(t, e.org.retired, "第 1 集新版本也没整理进来")
}

// 库里已有的版本（不是订阅下载的）也算现在的版本：更好的不被差的换掉；差的被更好的换掉；不洗版时不再下
func TestUpgradeBaselineIncludesLibrary(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	p := e.profile4K()
	better := e.libraryFile("uhd", "Dune.Part.Two.2024.2160p.UHD.BluRay.x265-OLD", models.MediaKindMovie, 693134, 0, 0, 0, "/lib/dune2/2160p.mkv")
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: p.ID, Upgrade: true})
	e.search.set(e.item("hdsky", "d1", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "", 8, 50))
	msg, err := e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, "洗版：没有比现在更好的版本", msg)
	assert.Empty(t, e.gotPushes())

	// 库里的是 1080p 时 2160p 是更好的版本：下载，入库以后换掉库里的 1080p
	require.NoError(t, e.db.Model(&better).Updates(map[string]any{"torrent_name": "Dune.Part.Two.2024.1080p.WEB-DL.H264-OLD"}).Error)
	e.search.set(e.item("hdsky", "d2", "Dune.Part.Two.2024.2160p.WEB-DL.H265-X", "", 20, 50))
	msg, err = e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	require.Contains(t, msg, "下载了 Dune.Part.Two.2024.2160p")
	e.organized(e.hashOf("d2"), models.MediaKindMovie, 693134, 0, 0, "/lib/dune2/2160p-new.mkv")
	e.svc.refresh(e.ctx)
	assert.Equal(t, []uint{better.ID}, e.org.retired)

	// 不洗版的订阅：库里已经有了就不下
	e2 := newEnv(t)
	e2.enable(nil)
	e2.libraryFile("x", "Dune.Part.Two.2024.720p.WEB-DL.H264-OLD", models.MediaKindMovie, 693134, 0, 0, 0, "/lib/dune2/720p.mkv")
	m2 := e2.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	e2.search.set(e2.item("hdsky", "d3", "Dune.Part.Two.2024.2160p.WEB-DL.H265-X", "", 20, 50))
	msg, err = e2.svc.SearchNow(e2.ctx, m2.ID)
	require.NoError(t, err)
	assert.Equal(t, "库里已经有了", msg)
	assert.Empty(t, e2.gotPushes())
}

// 新版本入库时库里的版本按现在的档案不比它差（例如档案改过）：留着
func TestUpgradeReplaceKeepsBetterLibraryFile(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	p := e.profile4K()
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: p.ID, Upgrade: true})
	e.search.set(e.item("hdsky", "f1", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "", 8, 50))
	_, err := e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	require.Len(t, e.gotPushes(), 1)
	// 下载期间别处整理进来一个 2160p
	keep := e.libraryFile("uhd", "Dune.Part.Two.2024.2160p.UHD.BluRay.x265-OLD", models.MediaKindMovie, 693134, 0, 0, 0, "/lib/dune2/2160p.mkv")
	e.organized(e.hashOf("f1"), models.MediaKindMovie, 693134, 0, 0, "/lib/dune2/1080p.mkv")
	e.svc.refresh(e.ctx)
	assert.NotContains(t, e.org.retired, keep.ID)
}

// 失败的下载不算现在的版本；档案改过以后按现在的档案重算已有版本的分数
func TestUpgradeBaselineIgnoresFailedAndFollowsProfile(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	p := e.profile4K()
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: p.ID, Upgrade: true})
	e.search.set(e.item("hdsky", "g1", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "", 8, 50))
	_, err := e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	e.organized(e.hashOf("g1"), models.MediaKindMovie, 693134, 0, 0, "/lib/dune2/1080p.mkv")
	e.svc.refresh(e.ctx)
	e.search.set(e.item("hdsky", "g2", "Dune.Part.Two.2024.2160p.WEB-DL.H265-X", "", 20, 50))
	_, err = e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	e.advance(8 * 24 * time.Hour)
	e.svc.refresh(e.ctx)
	require.Equal(t, models.MediaSubTorrentFailed, e.linkOf(m.ID, "g2").Status)
	e.search.set(e.item("hdsky", "g3", "Dune.Part.Two.2024.2160p.WEB-DL.H265-Y", "", 20, 50))
	msg, err := e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "下载了 Dune.Part.Two.2024.2160p.WEB-DL.H265-Y", "失败的 g2 不挡着")

	// 档案原来偏好 1080p，下了 1080p WEB-DL（来源没到原盘，还在洗版）；改成偏好 2160p 以后 2160p WEB-DL 是更好的版本
	e2 := newEnv(t)
	e2.enable(nil)
	src := []string{"UHD BluRay", "WEB-DL"}
	p2, err := e2.svc.SaveProfile(e2.ctx, 0, ProfileInput{Name: "1080p 优先", Resolutions: []string{"1080p", "2160p"}, Sources: src})
	require.NoError(t, err)
	m2 := e2.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: p2.ID, Upgrade: true})
	e2.search.set(e2.item("hdsky", "h1", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "", 8, 50))
	_, err = e2.svc.SearchNow(e2.ctx, m2.ID)
	require.NoError(t, err)
	e2.organized(e2.hashOf("h1"), models.MediaKindMovie, 693134, 0, 0, "/lib/dune2/1080p.mkv")
	e2.svc.refresh(e2.ctx)
	require.Equal(t, models.MediaSubActive, e2.subRow(m2.ID).Status)
	_, err = e2.svc.SaveProfile(e2.ctx, p2.ID, ProfileInput{Name: "1080p 优先", Resolutions: []string{"2160p", "1080p"}, Sources: src})
	require.NoError(t, err)
	e2.search.set(e2.item("hdsky", "h2", "Dune.Part.Two.2024.2160p.WEB-DL.H265-X", "", 20, 50))
	msg, err = e2.svc.SearchNow(e2.ctx, m2.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "下载了 Dune.Part.Two.2024.2160p")
}

// 删旧种子要确认是订阅加进下载器的、没有 H&R：下载器里原来就有的、推送时就知道有 H&R 的都留着
func TestDeleteOldTorrentFailsClosed(t *testing.T) {
	e := newEnv(t)
	e.enable(func(s *Settings) { s.UpgradeOld = models.MediaUpgradeDelete })
	p, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "4K 原盘", Resolutions: []string{"2160p", "1080p"}, Sources: []string{"UHD BluRay", "WEB-DL"}})
	require.NoError(t, err)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: p.ID, Upgrade: true})
	e.pushRes = &ptinternal.PushTorrentResult{Success: true, Skipped: true, Message: "种子已存在于下载器中"}
	e.search.set(e.item("hdsky", "k1", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "", 8, 50))
	_, err = e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	e.pushRes = nil
	require.True(t, e.linkOf(m.ID, "k1").Adopted, "下载器里原来就有")
	e.organized(e.hashOf("k1"), models.MediaKindMovie, 693134, 0, 0, "/lib/dune2/1080p.mkv")
	e.svc.refresh(e.ctx)

	hr := e.item("hdsky", "k2", "Dune.Part.Two.2024.2160p.WEB-DL.H264-X", "", 20, 50)
	hr.HasHR = true
	e.search.set(hr)
	_, err = e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	require.True(t, e.linkOf(m.ID, "k2").HasHR)
	e.organized(e.hashOf("k2"), models.MediaKindMovie, 693134, 0, 0, "/lib/dune2/2160p-web.mkv")
	e.svc.refresh(e.ctx)
	old := e.linkOf(m.ID, "k1")
	assert.Equal(t, models.MediaSubTorrentReplaced, old.Status)
	assert.Contains(t, old.Message, "不是订阅加进下载器的")

	e.search.set(e.item("hdsky", "k3", "Dune.Part.Two.2024.2160p.UHD.BluRay.x265-X", "", 60, 50))
	_, err = e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	e.organized(e.hashOf("k3"), models.MediaKindMovie, 693134, 0, 0, "/lib/dune2/2160p-uhd.mkv")
	e.svc.refresh(e.ctx)
	hrOld := e.linkOf(m.ID, "k2")
	assert.Equal(t, models.MediaSubTorrentReplaced, hrOld.Status)
	assert.Contains(t, hrOld.Message, "H&R", "推送时记下的 H&R 留着")
	assert.Empty(t, e.dl.removed)

	// 读不到种子记录时不删：不知道有没有 H&R
	e3 := newEnv(t)
	e3.enable(func(s *Settings) { s.UpgradeOld = models.MediaUpgradeDelete })
	p3 := e3.profile4K()
	m3 := e3.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: p3.ID, Upgrade: true})
	e3.search.set(e3.item("hdsky", "n1", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "", 8, 50))
	_, err = e3.svc.SearchNow(e3.ctx, m3.ID)
	require.NoError(t, err)
	e3.organized(e3.hashOf("n1"), models.MediaKindMovie, 693134, 0, 0, "/lib/dune2/1080p.mkv")
	e3.svc.refresh(e3.ctx)
	require.NoError(t, e3.db.Where("site_name = ? AND torrent_id = ?", "hdsky", "n1").Delete(&models.TorrentInfo{}).Error)
	e3.search.set(e3.item("hdsky", "n2", "Dune.Part.Two.2024.2160p.WEB-DL.H265-X", "", 20, 50))
	_, err = e3.svc.SearchNow(e3.ctx, m3.ID)
	require.NoError(t, err)
	e3.organized(e3.hashOf("n2"), models.MediaKindMovie, 693134, 0, 0, "/lib/dune2/2160p.mkv")
	e3.svc.refresh(e3.ctx)
	gone := e3.linkOf(m3.ID, "n1")
	assert.Equal(t, models.MediaSubTorrentReplaced, gone.Status)
	assert.Contains(t, gone.Message, "找不到种子记录")
	assert.Empty(t, e3.dl.removed)
}

// 剧集的版本有好有差（第 1 集已经是 2160p、后两集是 1080p）：整季包只要能让一集变好就下，入库以后每一集都达到目标、订阅完成
func TestUpgradeMixedSeasonTakesPack(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	e.advance(60 * 24 * time.Hour)
	p := e.profile4K()
	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, ProfileID: p.ID, Upgrade: true})
	e1 := e.libraryFile("e1", "The.Last.of.Us.S02E01.2160p.WEB-DL.H265-OLD", models.MediaKindTV, 100088, 2, 1, 0, "/lib/tlou/s02e01.old.mkv")
	e2 := e.libraryFile("e2", "The.Last.of.Us.S02E02.1080p.WEB-DL.H265-OLD", models.MediaKindTV, 100088, 2, 2, 0, "/lib/tlou/s02e02.old.mkv")
	e3 := e.libraryFile("e3", "The.Last.of.Us.S02E03.1080p.WEB-DL.H265-OLD", models.MediaKindTV, 100088, 2, 3, 0, "/lib/tlou/s02e03.old.mkv")
	e.search.set(e.item("hdsky", "m1", "The.Last.of.Us.S02.2160p.WEB-DL.H265-X", "", 30, 10))
	msg, err := e.svc.SearchNow(e.ctx, tv.ID)
	require.NoError(t, err)
	require.Contains(t, msg, "下载了 The.Last.of.Us.S02.2160p", "后两集会变好")
	for ep := 1; ep <= 3; ep++ {
		e.organized(e.hashOf("m1"), models.MediaKindTV, 100088, 2, ep, fmt.Sprintf("/lib/tlou/s02e0%d.mkv", ep))
	}
	e.svc.refresh(e.ctx)
	assert.ElementsMatch(t, []uint{e1.ID, e2.ID, e3.ID}, e.org.retired, "同分的第 1 集也换成新的，库里不留两份")
	row := e.subRow(tv.ID)
	assert.Equal(t, models.MediaSubDone, row.Status)
	assert.Contains(t, row.Message, "达到了洗版的目标")

	// 都已经是 2160p 时同分的整季包不下
	e2nd := newEnv(t)
	e2nd.enable(nil)
	e2nd.advance(60 * 24 * time.Hour)
	p2 := e2nd.profile4K()
	tv2 := e2nd.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, ProfileID: p2.ID, Upgrade: true})
	for ep := 1; ep <= 3; ep++ {
		e2nd.libraryFile(fmt.Sprintf("u%d", ep), fmt.Sprintf("The.Last.of.Us.S02E0%d.2160p.WEB-DL.H265-OLD", ep), models.MediaKindTV, 100088, 2, ep, 0, fmt.Sprintf("/lib/tlou/u%d.mkv", ep))
	}
	e2nd.search.set(e2nd.item("hdsky", "m2", "The.Last.of.Us.S02.2160p.WEB-DL.H265-X", "", 30, 10))
	msg, err = e2nd.svc.SearchNow(e2nd.ctx, tv2.ID)
	require.NoError(t, err)
	assert.Equal(t, "洗版：没有比现在更好的整季包", msg)
}
