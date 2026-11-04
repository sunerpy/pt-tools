package subscribe

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// fakeReviewer 是假的 AI 复核：answer 按种子标题回答，记下每次问的内容。
type fakeReviewer struct {
	mu     sync.Mutex
	calls  []ReviewInput
	answer func(in ReviewInput) (ReviewResult, error)
}

func (f *fakeReviewer) review(_ context.Context, in ReviewInput) (ReviewResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, in)
	answer := f.answer
	f.mu.Unlock()
	if answer == nil {
		return ReviewResult{Match: true}, nil
	}
	return answer(in)
}

func (f *fakeReviewer) titles() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.calls))
	for _, c := range f.calls {
		out = append(out, c.TorrentTitle)
	}
	return out
}

// reviewEnv 打开 AI 复核（scope 空的按名字）并接上假的复核：no 里的种子标题回答「不是」。
func reviewEnv(t *testing.T, scope string, no ...string) (*env, *fakeReviewer) {
	t.Helper()
	e := newEnv(t)
	e.enable(func(s *Settings) { s.AIReviewEnabled, s.AIReviewScope = true, scope })
	f := &fakeReviewer{answer: func(in ReviewInput) (ReviewResult, error) {
		for _, n := range no {
			if in.TorrentTitle == n {
				return ReviewResult{Reason: "这是 1984 年的那部", Tokens: 30}, nil
			}
		}
		return ReviewResult{Match: true, Reason: "名字和年份对得上", Tokens: 28}, nil
	}}
	e.svc.SetReviewer(f.review)
	return e, f
}

func TestReviewSettings(t *testing.T) {
	e := newEnv(t)
	set := e.enable(nil)
	assert.False(t, set.AIReviewEnabled, "默认关")
	assert.Equal(t, ReviewByName, set.AIReviewScope, "默认只复核按名字对上的")
	set = e.enable(func(s *Settings) { s.AIReviewEnabled, s.AIReviewScope = true, ReviewAll })
	assert.True(t, set.AIReviewEnabled)
	assert.Equal(t, ReviewAll, set.AIReviewScope)
	_, err := e.svc.SaveSettings(e.ctx, Settings{AIReviewScope: "some"})
	require.ErrorIs(t, err, ErrInvalid)
	got, err := e.svc.Settings(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, ReviewAll, got.AIReviewScope, "写错的没保存")
}

// 电影：最好的候选 AI 说不是，接着看下一个；问的是订阅的条目与种子的信息；结论记 24 小时。
func TestReviewMovie(t *testing.T) {
	bad := "Dune.Part.Two.2024.2160p.WEB-DL.H265-BAD"
	e, f := reviewEnv(t, "", bad)
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	e.search.set(
		e.item("hdsky", "71", bad, "沙丘 1984", 20, 50),
		e.item("hdsky", "72", "Dune.Part.Two.2024.1080p.WEB-DL.H264-GOOD", "", 8, 50),
	)
	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, "下载了 Dune.Part.Two.2024.1080p.WEB-DL.H264-GOOD", msg)
	require.Len(t, e.gotPushes(), 1)
	assert.Equal(t, "72", e.gotPushes()[0].TorrentID)
	assert.Equal(t, []string{bad, "Dune.Part.Two.2024.1080p.WEB-DL.H264-GOOD"}, f.titles())
	first := f.calls[0]
	assert.Equal(t, "movie", first.Kind)
	assert.Equal(t, "沙丘2", first.Title)
	assert.Equal(t, "Dune: Part Two", first.OriginalTitle)
	assert.Equal(t, 2024, first.Year)
	assert.Zero(t, first.Season)
	assert.Equal(t, "沙丘 1984", first.TorrentSubtitle)
	assert.Equal(t, "中文名 沙丘，英文名 Dune Part Two，年份 2024，2160p", first.Parsed, "副标题里的中文名也解析出来了")
	steps := runSteps(e, sub.ID)
	assert.Contains(t, steps, "reject:"+bad+"：AI 复核：不是，这是 1984 年的那部（30 tokens）\n")
	assert.Contains(t, steps, "info:Dune.Part.Two.2024.1080p.WEB-DL.H264-GOOD：AI 复核：是，名字和年份对得上（28 tokens）\n")
}

// 都说不是：不推；24 小时内不再问（运行记录写问过），过了再问。
func TestReviewCache(t *testing.T) {
	bad := "Dune.Part.Two.2024.2160p.WEB-DL.H265-BAD"
	e, f := reviewEnv(t, "", bad)
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	e.search.set(e.item("hdsky", "71", bad, "", 20, 50))
	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, "找到 1 个，AI 复核都说不是", msg)
	assert.Empty(t, e.gotPushes())
	asked := e.Now()

	e.advance(23 * time.Hour)
	msg, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, "找到 1 个，AI 复核都说不是", msg)
	assert.Len(t, f.titles(), 1, "24 小时内不再问")
	assert.Contains(t, runSteps(e, sub.ID), fmt.Sprintf("reject:%s：AI 复核：不是，这是 1984 年的那部（%s 问过）\n", bad, asked.In(time.Local).Format("01-02 15:04")))

	e.advance(2 * time.Hour)
	_, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Len(t, f.titles(), 2, "过了 24 小时再问")
}

// 范围：按名字（默认）时两边都有 IMDb 编号、按编号对上的不问；全部时也问。没开时不问。
func TestReviewScope(t *testing.T) {
	byID := func(e *env) {
		it := e.item("hdsky", "81", "Dune.Part.Two.2024.2160p.WEB-DL.H265-X", "", 20, 50)
		it.IMDbID = "tt15239678"
		e.search.set(it)
	}
	e, f := reviewEnv(t, "")
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	byID(e)
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Len(t, e.gotPushes(), 1)
	assert.Empty(t, f.titles(), "按 IMDb 编号对上的不问")

	e, f = reviewEnv(t, ReviewAll)
	sub = e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	byID(e)
	_, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Len(t, e.gotPushes(), 1)
	assert.Len(t, f.titles(), 1, "全部时也问")

	e = newEnv(t)
	e.enable(nil)
	f = &fakeReviewer{}
	e.svc.SetReviewer(f.review)
	sub = e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	e.search.set(e.item("hdsky", "82", "Dune.Part.Two.2024.2160p.WEB-DL.H265-X", "", 20, 50))
	_, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Len(t, e.gotPushes(), 1)
	assert.Empty(t, f.titles(), "没开时不问")
}

// 调用失败：照常推送，这一轮后面的候选不再复核；看不懂回答的只是这一个没复核成；没接上助手时也照常推送。
func TestReviewFailures(t *testing.T) {
	t.Run("调用失败", func(t *testing.T) {
		e := newEnv(t)
		e.enable(func(s *Settings) { s.AIReviewEnabled = true })
		f := &fakeReviewer{answer: func(ReviewInput) (ReviewResult, error) {
			return ReviewResult{}, errors.New("模型出错: 502 Bad Gateway, key=sk-0123456789abcdef0123456789abcdef")
		}}
		e.svc.SetReviewer(f.review)
		e.advance(60 * 24 * time.Hour)
		tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
		e.organized("e1hash", models.MediaKindTV, 100088, 2, 1, "/lib/tlou/s02e01.mkv")
		e.search.set(
			e.item("hdsky", "91", "The.Last.of.Us.S02E02.1080p.WEB-DL.H264-X", "", 3, 10),
			e.item("hdsky", "92", "The.Last.of.Us.S02E03.1080p.WEB-DL.H264-X", "", 3, 10),
		)
		_, err := e.svc.SearchNow(e.ctx, tv.ID)
		require.NoError(t, err)
		assert.Len(t, e.gotPushes(), 2, "照常推送")
		assert.Len(t, f.titles(), 1, "这一轮后面的候选不再复核")
		steps := runSteps(e, tv.ID)
		assert.Contains(t, steps, "info:The.Last.of.Us.S02E02.1080p.WEB-DL.H264-X：AI 复核没做成：模型出错: 502 Bad Gateway")
		assert.Contains(t, steps, "照常推送（这一轮后面的候选不再复核）\n")
		assert.NotContains(t, steps, "0123456789abcdef0123456789abcdef", "错误里的密钥遮掉")
	})
	t.Run("看不懂回答", func(t *testing.T) {
		e := newEnv(t)
		e.enable(func(s *Settings) { s.AIReviewEnabled = true })
		f := &fakeReviewer{answer: func(ReviewInput) (ReviewResult, error) { return ReviewResult{}, ErrReviewUnclear }}
		e.svc.SetReviewer(f.review)
		e.advance(60 * 24 * time.Hour)
		tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
		e.organized("e1hash", models.MediaKindTV, 100088, 2, 1, "/lib/tlou/s02e01.mkv")
		e.search.set(
			e.item("hdsky", "91", "The.Last.of.Us.S02E02.1080p.WEB-DL.H264-X", "", 3, 10),
			e.item("hdsky", "92", "The.Last.of.Us.S02E03.1080p.WEB-DL.H264-X", "", 3, 10),
		)
		_, err := e.svc.SearchNow(e.ctx, tv.ID)
		require.NoError(t, err)
		assert.Len(t, e.gotPushes(), 2)
		assert.Len(t, f.titles(), 2, "后面的候选照常复核")
		assert.Contains(t, runSteps(e, tv.ID), "info:The.Last.of.Us.S02E03.1080p.WEB-DL.H264-X：AI 复核没做成：看不出模型回答的是还是不是，照常推送\n")
	})
	t.Run("没接上助手", func(t *testing.T) {
		e := newEnv(t)
		e.enable(func(s *Settings) { s.AIReviewEnabled = true })
		sub := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
		e.search.set(e.item("hdsky", "82", "Dune.Part.Two.2024.2160p.WEB-DL.H265-X", "", 20, 50))
		_, err := e.svc.SearchNow(e.ctx, sub.ID)
		require.NoError(t, err)
		assert.Len(t, e.gotPushes(), 1)
		assert.Contains(t, runSteps(e, sub.ID), "info:Dune.Part.Two.2024.2160p.WEB-DL.H265-X：AI 复核没做成：AI 助手没有启动，照常推送\n")
	})
}

// 每次运行最多问 10 个：问满了不推剩下的，下次再看（提前再搜一次）；下一轮问过的不再问。
func TestReviewPerRunLimit(t *testing.T) {
	var titles []string
	for i := range 12 {
		titles = append(titles, fmt.Sprintf("Dune.Part.Two.2024.1080p.WEB-DL.H264-G%02d", i))
	}
	e, f := reviewEnv(t, "", titles...)
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	var items []v2.TorrentItem
	for i, title := range titles {
		items = append(items, e.item("hdsky", fmt.Sprintf("1%02d", i), title, "", 8, 50-i))
	}
	e.search.set(items...)
	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, "AI 复核说 10 个不是，这一轮问满了 10 个，剩下的下次再看", msg)
	assert.Len(t, f.titles(), 10)
	assert.Empty(t, e.gotPushes())
	runs := e.runs(sub.ID)
	require.NotEmpty(t, runs)
	assert.Equal(t, models.MediaRunWaiting, runs[0].Status)
	steps := runSteps(e, sub.ID)
	assert.Contains(t, steps, "wait:这一轮已经让 AI 复核了 10 个，剩下的候选下次再看\n")
	row := e.subRow(sub.ID)
	require.NotNil(t, row.NextSearchAt)
	assert.WithinDuration(t, e.Now().Add(retrySoonBase), *row.NextSearchAt, time.Minute, "提前再搜一次")

	msg, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, "找到 12 个，AI 复核都说不是", msg)
	assert.Len(t, f.titles(), 12, "问过的不再问，只问剩下的两个")
}

// 剧集：整季包都说不是时一集一集补；单集说不是的跳过，接着看同一集的下一个；告诉模型对到了第几季第几集。
func TestReviewTV(t *testing.T) {
	badPack := "The.Last.of.Us.S02.2160p.WEB-DL.H265-BAD"
	badEp := "The.Last.of.Us.S02E02.2160p.WEB-DL.H265-BAD"
	e, f := reviewEnv(t, "", badPack, badEp)
	e.advance(60 * 24 * time.Hour)
	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	e.organized("e1hash", models.MediaKindTV, 100088, 2, 1, "/lib/tlou/s02e01.mkv")
	e.search.set(
		e.item("hdsky", "51", badPack, "", 30, 10),
		e.item("hdsky", "52", badEp, "", 8, 10),
		e.item("hdsky", "53", "The.Last.of.Us.S02E02.1080p.WEB-DL.H264-X", "", 3, 10),
		e.item("hdsky", "54", "The.Last.of.Us.S02E03.1080p.WEB-DL.H264-X", "", 3, 10),
	)
	msg, err := e.svc.SearchNow(e.ctx, tv.ID)
	require.NoError(t, err)
	assert.Equal(t, "下载了 The.Last.of.Us.S02E02.1080p.WEB-DL.H264-X；下载了 The.Last.of.Us.S02E03.1080p.WEB-DL.H264-X", msg)
	assert.Equal(t, []string{badPack, badEp, "The.Last.of.Us.S02E02.1080p.WEB-DL.H264-X", "The.Last.of.Us.S02E03.1080p.WEB-DL.H264-X"}, f.titles())
	assert.Equal(t, "tv", f.calls[0].Kind)
	assert.Equal(t, 2, f.calls[0].Season)
	assert.Equal(t, []string{"The Last of Us", "TLOU"}, f.calls[0].Aliases, "订阅的别名原样交过去（和名字一样的由助手去掉）")
	assert.Equal(t, "英文名 The Last of Us，第 2 季，2160p；按集号对到这部剧的第 2 季整季", f.calls[0].Parsed)
	assert.Equal(t, "英文名 The Last of Us，第 2 季，第 2 集，2160p；按集号对到这部剧的第 2 季第 2 集", f.calls[1].Parsed)

	// 缺的集只有说不是的：写明
	e, _ = reviewEnv(t, "", badEp)
	tv = e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	e.organized("e1hash", models.MediaKindTV, 100088, 2, 1, "/lib/tlou/s02e01.mkv")
	e.search.set(e.item("hdsky", "52", badEp, "", 8, 10))
	msg, err = e.svc.SearchNow(e.ctx, tv.ID)
	require.NoError(t, err)
	assert.Equal(t, "缺 1 集，找到的 1 个 AI 复核都说不是", msg)
	assert.Empty(t, e.gotPushes())
}

// 整季包按集号对到的范围（多集），以及剧集的整季包都说不是、又没有单集可补时写明。
func TestReviewInputSpan(t *testing.T) {
	sub := &models.MediaSubscription{MediaType: models.MediaKindTV, Title: "最后生还者", Season: 2}
	got := reviewInput(sub, scored{sp: span{Start: 2, End: 3}})
	assert.Equal(t, "按集号对到这部剧的第 2 季第 2–3 集", got.Parsed, "解析不出名字时只写对到的集")
	got = reviewInput(sub, scored{c: Candidate{Meta: meta.Parse("The.Last.of.Us.S01-S02.1080p", "")}, sp: span{Whole: true}})
	assert.Equal(t, "英文名 The Last of Us，第 1–2 季，1080p；按集号对到这部剧的第 2 季整季", got.Parsed)
	got = reviewInput(sub, scored{c: Candidate{Meta: meta.Parse("The.Last.of.Us.S02E02-E03.1080p", "")}, sp: span{Start: 2, End: 3}})
	assert.Equal(t, "英文名 The Last of Us，第 2 季，第 2–3 集，1080p；按集号对到这部剧的第 2 季第 2–3 集", got.Parsed)
	movie := reviewInput(&models.MediaSubscription{MediaType: models.MediaKindMovie, Title: "沙丘2", Season: 9}, scored{})
	assert.Zero(t, movie.Season, "电影不写季")
	assert.Empty(t, movie.Parsed)
}

// 洗版：只复核比现在好的（整季包洗版见 TestReviewUpgradePack）。
func TestReviewUpgrade(t *testing.T) {
	e, f := reviewEnv(t, "", "Dune.Part.Two.2024.2160p.UHD.BluRay.x265-BAD")
	p, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "4K 原盘", Resolutions: []string{"2160p", "1080p"}, Sources: []string{"UHD BluRay", "WEB-DL"}})
	require.NoError(t, err)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: p.ID, Upgrade: true})
	e.search.set(e.item("hdsky", "61", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "", 8, 50))
	_, err = e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	require.Len(t, e.gotPushes(), 1)
	e.organized(e.hashOf("61"), models.MediaKindMovie, 693134, 0, 0, "/lib/dune2/1080p.mkv")
	e.svc.refresh(e.ctx)

	e.search.set(
		e.item("hdsky", "61", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "", 8, 50),
		e.item("hdsky", "62", "Dune.Part.Two.2024.1080p.WEB-DL.H264-Y", "", 8, 80),
		e.item("hdsky", "63", "Dune.Part.Two.2024.2160p.UHD.BluRay.x265-BAD", "", 60, 20),
		e.item("hdsky", "64", "Dune.Part.Two.2024.2160p.UHD.BluRay.x265-FRDS", "", 60, 10),
	)
	msg, err := e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, "下载了 Dune.Part.Two.2024.2160p.UHD.BluRay.x265-FRDS", msg)
	assert.Equal(t, []string{"Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "Dune.Part.Two.2024.2160p.UHD.BluRay.x265-BAD", "Dune.Part.Two.2024.2160p.UHD.BluRay.x265-FRDS"},
		f.titles(), "第一版问过一次；洗版时不如现在的 1080p 不问")
}

// 一键追剧里用户自己选的种子不复核。
func TestReviewSkipsChosen(t *testing.T) {
	e, f := reviewEnv(t, "", "The.Last.of.Us.S02E02.1080p.WEB-DL.H264-X")
	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	e.organized("e1hash", models.MediaKindTV, 100088, 2, 1, "/lib/tlou/s02e01.mkv")
	it := e.item("hdsky", "53", "The.Last.of.Us.S02E02.1080p.WEB-DL.H264-X", "", 3, 10)
	set, err := e.svc.Settings(e.ctx)
	require.NoError(t, err)
	row := e.subRow(tv.ID)
	ctx := withRun(e.ctx, &runLog{})
	e.svc.mu.Lock()
	msg := e.svc.evaluate(ctx, &row, []Candidate{{Site: "hdsky", Item: it, Meta: meta.Parse(it.Title, ""), From: fromSearch, Chosen: true}}, set)
	e.svc.mu.Unlock()
	assert.Equal(t, "下载了 The.Last.of.Us.S02E02.1080p.WEB-DL.H264-X", msg)
	assert.Empty(t, f.titles())
}

// 记下的结论超过上限时先清掉过期的，还是多就全清掉。
func TestReviewCacheBound(t *testing.T) {
	s := &Service{}
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	for i := range reviewCacheMax {
		at := now
		if i%2 == 0 {
			at = now.Add(-25 * time.Hour)
		}
		s.remember(reviewKey{sub: 1, site: "hdsky", id: fmt.Sprint(i)}, ReviewResult{}, at)
	}
	s.remember(reviewKey{sub: 2, site: "hdsky", id: "x"}, ReviewResult{Match: true}, now)
	assert.Len(t, s.reviews, reviewCacheMax/2+1, "过期的清掉了")
	for i := len(s.reviews); i < reviewCacheMax; i++ {
		s.remember(reviewKey{sub: 3, site: "hdsky", id: fmt.Sprint(i)}, ReviewResult{}, now)
	}
	s.remember(reviewKey{sub: 4, site: "hdsky", id: "y"}, ReviewResult{}, now)
	assert.Len(t, s.reviews, 1, "都没过期时全清掉")
}

// 整季包洗版：比现在好的整季包也复核，说不是的换下一个。
func TestReviewUpgradePack(t *testing.T) {
	bad := "The.Last.of.Us.S02.2160p.WEB-DL.H265-BAD"
	e, f := reviewEnv(t, "", bad)
	e.advance(60 * 24 * time.Hour) // 3 集都播完了
	p := e.profile4K()
	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, ProfileID: p.ID, Upgrade: true})
	e.search.set(e.item("hdsky", "a1", "The.Last.of.Us.S02.1080p.WEB-DL.H264-X", "", 12, 10))
	_, err := e.svc.SearchNow(e.ctx, tv.ID)
	require.NoError(t, err)
	for ep := 1; ep <= 3; ep++ {
		e.organized(e.hashOf("a1"), models.MediaKindTV, 100088, 2, ep, fmt.Sprintf("/lib/tlou/s02e0%d.1080p.mkv", ep))
	}
	e.svc.refresh(e.ctx)

	e.search.set(
		e.item("hdsky", "b1", bad, "", 30, 50),
		e.item("hdsky", "b2", "The.Last.of.Us.S02.2160p.WEB-DL.H265-GOOD", "", 30, 10),
	)
	msg, err := e.svc.SearchNow(e.ctx, tv.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "下载了 The.Last.of.Us.S02.2160p.WEB-DL.H265-GOOD")
	assert.Equal(t, []string{"The.Last.of.Us.S02.1080p.WEB-DL.H264-X", bad, "The.Last.of.Us.S02.2160p.WEB-DL.H265-GOOD"}, f.titles())
}

// 模型给的理由照样遮掉像凭证的内容（不管接的是哪个复核）。
func TestReviewRedactsReason(t *testing.T) {
	e := newEnv(t)
	e.enable(func(s *Settings) { s.AIReviewEnabled = true })
	secret := "0123456789abcdef0123456789abcdef"
	e.svc.SetReviewer(func(context.Context, ReviewInput) (ReviewResult, error) {
		return ReviewResult{Reason: "种子页 https://tracker.example.test/download.php?id=1&passkey=" + secret + " 写的是花絮"}, nil
	})
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	e.search.set(e.item("hdsky", "71", "Dune.Part.Two.2024.2160p.WEB-DL.H265-X", "", 20, 50))
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	steps := runSteps(e, sub.ID)
	assert.Contains(t, steps, "AI 复核：不是，种子页 https://tracker.example.test/download.php")
	assert.Contains(t, steps, "写的是花絮")
	assert.NotContains(t, steps, secret)
}
