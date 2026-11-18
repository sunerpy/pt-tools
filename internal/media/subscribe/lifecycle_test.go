package subscribe

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 后台：RSS 交来的种子由后台处理（总开关关着时丢掉），每一拍做到期的主动搜索；重复启动、重复停止没有副作用
func TestServiceBackground(t *testing.T) {
	old := tickInterval
	tickInterval = 20 * time.Millisecond
	t.Cleanup(func() { tickInterval = old })
	e := newEnv(t)
	e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, Sites: []string{"hdsky"}})
	ctx, cancel := context.WithCancel(e.ctx)
	defer cancel()
	e.svc.Start(ctx)
	e.svc.Start(ctx)

	e.svc.Offer("hdsky", e.item("hdsky", "o1", "Dune.Part.Two.2024.2160p.WEB-DL.H265-X", "", 20, 0))
	// 后台一个个处理：对不上的哨兵被取走时，o1 已经处理完（总开关关着，丢掉）
	e.svc.Offer("other", v2.TorrentItem{ID: "sentinel", Title: "Nothing.At.All"})
	require.Eventually(t, func() bool { return len(e.svc.offers) == 0 }, 10*time.Second, 5*time.Millisecond)
	e.enable(nil)
	e.svc.Offer("hdsky", e.item("hdsky", "o2", "Dune.Part.Two.2024.2160p.WEB-DL.H265-Y", "", 20, 0))
	require.Eventually(t, func() bool { return len(e.gotPushes()) == 1 }, 10*time.Second, 10*time.Millisecond)
	assert.Equal(t, "o2", e.gotPushes()[0].TorrentID, "总开关关着时交来的种子丢掉了")

	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	require.Eventually(t, func() bool {
		row, err := e.svc.subRow(e.ctx, tv.ID)
		return err == nil && row.LastSearchAt != nil
	}, 10*time.Second, 10*time.Millisecond, "每一拍做到期的主动搜索")

	e.svc.Stop()
	e.svc.Stop()
	// 停掉以后交来的种子不处理，也不阻塞
	for i := range offerQueue + 5 {
		e.svc.Offer("hdsky", v2.TorrentItem{ID: fmt.Sprint(i), Title: "x"})
	}
	assert.Len(t, e.gotPushes(), 1)
}

// 输入不对时拒绝（ErrInvalid），找不到时返回 ErrNotFound
func TestInputValidation(t *testing.T) {
	e := newEnv(t)
	p := e.profile4K()
	long := strings.Repeat("长", 300)
	many := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("s%d", i)
		}
		return out
	}

	for i, in := range []Settings{
		{DefaultProfileID: 999},
		{NotifyChannels: make([]uint, 21)},
		{SearchSkipSites: []string{strings.Repeat("x", 65)}},
		{SearchSkipSites: many(201)},
	} {
		_, err := e.svc.SaveSettings(e.ctx, in)
		require.ErrorIs(t, err, ErrInvalid, "设置 %d", i)
	}

	for i, in := range []ProfileInput{
		{Name: "g", Groups: []string{strings.Repeat("组", 65)}},
		{Name: "g", Groups: many(51)},
		{Name: "g", MaxSizeGB: 20000},
		{Name: "g", Remux: "maybe"},
		{Name: "g", Free: models.MediaPrefAvoid},
		{Name: p.Name},
	} {
		_, perr := e.svc.SaveProfile(e.ctx, 0, in)
		require.ErrorIs(t, perr, ErrInvalid, "档案 %d", i)
	}
	_, err := e.svc.SaveProfile(e.ctx, 999, ProfileInput{Name: "x"})
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, e.svc.DeleteProfile(e.ctx, 999), ErrNotFound)

	for i, in := range []SubscriptionInput{
		{MediaType: models.MediaKindMovie},
		{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 201},
		{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: 999},
		{MediaType: models.MediaKindMovie, TMDBID: 693134, Category: long},
		{MediaType: models.MediaKindMovie, TMDBID: 693134, SavePath: strings.Repeat("x", 1100)},
		{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 9},
	} {
		_, serr := e.svc.CreateSubscription(e.ctx, in, models.MediaSubFromManual)
		require.ErrorIs(t, serr, ErrInvalid, "订阅 %d", i)
	}
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: p.ID})
	_, err = e.svc.UpdateSubscription(e.ctx, 999, SubscriptionInput{})
	require.ErrorIs(t, err, ErrNotFound)
	_, err = e.svc.UpdateSubscription(e.ctx, m.ID, SubscriptionInput{ProfileID: 999})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.SetStatus(e.ctx, m.ID, "weird")
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.SetStatus(e.ctx, 999, models.MediaSubPaused)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = e.svc.Subscriptions(e.ctx, SubscriptionQuery{Status: "weird"})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.Subscriptions(e.ctx, SubscriptionQuery{Keyword: long})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.Subscription(e.ctx, 999)
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, e.svc.DeleteSubscription(e.ctx, 999), ErrNotFound)
	_, err = e.svc.SearchNow(e.ctx, 999)
	require.ErrorIs(t, err, ErrNotFound)

	for i, in := range []DoubanSourceInput{
		{UserID: ""},
		{UserID: "qa/x"},
		{UserID: "ok", Name: strings.Repeat("名", 65)},
		{UserID: "ok", ProfileID: 999},
	} {
		_, derr := e.svc.SaveDoubanSource(e.ctx, 0, in)
		require.ErrorIs(t, derr, ErrInvalid, "豆瓣来源 %d", i)
	}
	src, err := e.svc.SaveDoubanSource(e.ctx, 0, DoubanSourceInput{UserID: "qa", ProfileID: p.ID})
	require.NoError(t, err)
	_, err = e.svc.SaveDoubanSource(e.ctx, 0, DoubanSourceInput{UserID: "qa"})
	require.ErrorIs(t, err, ErrInvalid, "同一个用户只加一次")
	_, err = e.svc.SaveDoubanSource(e.ctx, 999, DoubanSourceInput{UserID: "other"})
	require.ErrorIs(t, err, ErrNotFound)
	upd, err := e.svc.SaveDoubanSource(e.ctx, src.ID, DoubanSourceInput{UserID: "qa", Name: "改名", Enabled: true})
	require.NoError(t, err)
	assert.Equal(t, "改名", upd.Name)
	require.ErrorIs(t, e.svc.DeleteDoubanSource(e.ctx, 999), ErrNotFound)
	_, err = e.svc.DoubanItems(e.ctx, 999)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = e.svc.FetchDouban(e.ctx, 999)
	require.ErrorIs(t, err, ErrNotFound)

	// 档案被用着时不能删：订阅在用、豆瓣来源在用、是默认档案
	require.ErrorIs(t, e.svc.DeleteProfile(e.ctx, p.ID), ErrInvalid, "订阅在用")
	require.NoError(t, e.svc.DeleteSubscription(e.ctx, m.ID))
	_, err = e.svc.SaveDoubanSource(e.ctx, src.ID, DoubanSourceInput{UserID: "qa", ProfileID: p.ID})
	require.NoError(t, err)
	require.ErrorIs(t, e.svc.DeleteProfile(e.ctx, p.ID), ErrInvalid, "豆瓣来源在用")
	require.NoError(t, e.svc.DeleteDoubanSource(e.ctx, src.ID))
	e.enable(func(s *Settings) { s.DefaultProfileID = p.ID })
	require.ErrorIs(t, e.svc.DeleteProfile(e.ctx, p.ID), ErrInvalid, "默认档案")
	e.enable(nil)
	require.NoError(t, e.svc.DeleteProfile(e.ctx, p.ID))

	_, err = e.svc.Explore(e.ctx, "music", "trending", 1, "")
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.Explore(e.ctx, models.MediaKindMovie, ExploreSearch, 1, "")
	require.ErrorIs(t, err, ErrInvalid, "搜索要填名字")
	_, err = e.svc.Explore(e.ctx, models.MediaKindMovie, "weird", 1, "")
	require.ErrorIs(t, err, ErrInvalid)
}

// 修改订阅：站点规整，洗版打开时已完成的订阅接着找
func TestUpdateSubscription(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", m.ID).Update("status", models.MediaSubDone).Error)
	got, err := e.svc.UpdateSubscription(e.ctx, m.ID, SubscriptionInput{Sites: []string{" hdsky ", "hdsky", ""}, Category: "movies", Upgrade: true})
	require.NoError(t, err)
	assert.Equal(t, models.MediaSubActive, got.Status, "打开洗版：已完成的接着找")
	assert.Equal(t, "movies", got.Category)
	assert.Equal(t, []string{"hdsky"}, decodeList(got.Sites))
}

// 数据库出错时各个入口都返回错误，后台不 panic
func TestStoreFailuresSurface(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	sqlDB, err := e.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = e.svc.Settings(e.ctx)
	require.Error(t, err)
	_, err = e.svc.SaveSettings(e.ctx, Settings{})
	require.Error(t, err)
	_, err = e.svc.Profiles(e.ctx)
	require.Error(t, err)
	_, err = e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "x"})
	require.Error(t, err)
	require.Error(t, e.svc.DeleteProfile(e.ctx, 1))
	_, err = e.svc.Subscriptions(e.ctx, SubscriptionQuery{})
	require.Error(t, err)
	_, err = e.svc.Subscription(e.ctx, m.ID)
	require.Error(t, err)
	_, err = e.svc.CreateSubscription(e.ctx, SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2}, models.MediaSubFromManual)
	require.Error(t, err)
	_, err = e.svc.SetStatus(e.ctx, m.ID, models.MediaSubPaused)
	require.Error(t, err)
	require.Error(t, e.svc.DeleteSubscription(e.ctx, m.ID))
	_, err = e.svc.SearchNow(e.ctx, m.ID)
	require.Error(t, err)
	_, err = e.svc.DoubanSources(e.ctx)
	require.Error(t, err)
	_, err = e.svc.SaveDoubanSource(e.ctx, 0, DoubanSourceInput{UserID: "qa"})
	require.Error(t, err)
	_, err = e.svc.FetchDouban(e.ctx, 1)
	require.Error(t, err)
	_, err = e.svc.DoubanItems(e.ctx, 1)
	require.Error(t, err)
	e.svc.Tick(e.ctx)
	e.svc.refresh(e.ctx)
	e.svc.doubanDue(e.ctx)
	e.svc.searchDue(e.ctx, Settings{Enabled: true, SearchIntervalHours: 12})
	e.svc.considerOffer(e.ctx, Candidate{Site: "hdsky", Item: v2.TorrentItem{ID: "1", Title: "Dune.Part.Two.2024.1080p.WEB-DL-X"}, From: fromRSS})
}
