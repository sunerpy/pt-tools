package subscribe

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// detailSite 在 fakeSite 上加读详情（推送前重新验证免费资格用）。
type detailSite struct {
	*fakeSite
	detail func(id string) (*v2.TorrentItem, error)
	calls  int
}

func (d *detailSite) GetDetailFetcher() v2.TorrentDetailFetcher { return d }

func (d *detailSite) GetTorrentDetail(ctx context.Context, guid, _, _ string) (*v2.TorrentItem, error) {
	d.calls++
	if _, ok := v2.FreshAfterFrom(ctx); !ok {
		return nil, errors.New("没有要求新鲜数据")
	}
	return d.detail(guid)
}

func freeProfileEnv(t *testing.T, free string) (*env, *models.MediaSubscription, *detailSite) {
	t.Helper()
	e := newEnv(t)
	e.enable(nil)
	e.svc.cfg.SiteNames = func() []string { return []string{"hdsky"} }
	ds := &detailSite{fakeSite: e.site}
	e.svc.cfg.Sites = func(name string) (v2.Site, bool) { return ds, name == "hdsky" }
	p, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "免费", Free: free})
	require.NoError(t, err)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: p.ID, Category: "movies"})
	it := e.item("hdsky", "11", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "沙丘2", 8, 50)
	it.DiscountLevel = v2.DiscountFree // 列表（可能来自搜索缓存）说是免费
	e.search.set(it)
	return e, m, ds
}

// 档案要求免费：推送要求免费，资格来自推送前重新读的详情（搜索结果可能是缓存的），不用列表里的。
func TestSubscribe_RequireFreeUsesFreshDetail(t *testing.T) {
	e, m, ds := freeProfileEnv(t, models.MediaPrefRequire)
	end := e.Now().Add(24 * time.Hour)
	ds.detail = func(id string) (*v2.TorrentItem, error) {
		return &v2.TorrentItem{ID: id, DiscountLevel: v2.DiscountFree, DiscountEndTime: end, FetchedAt: e.Now()}, nil
	}

	_, err := e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	require.Equal(t, 1, ds.calls, "推送前读了详情")
	pushes := e.gotPushes()
	require.Len(t, pushes, 1)
	p := pushes[0]
	assert.True(t, p.Policy.RequireFree)
	assert.Equal(t, ptinternal.SourceKindSubscribe, p.Policy.SourceKind)
	require.NotNil(t, p.Snapshot)
	assert.Equal(t, v2.FreeUntil, p.Snapshot.Validity)
	require.NotNil(t, p.Snapshot.EndsAt)
	assert.True(t, p.Snapshot.EndsAt.Equal(end))
	assert.False(t, p.Snapshot.VerifiedAt.Before(p.Policy.AttemptStart))
	assert.Equal(t, e.Now(), p.Policy.Now(), "门禁用订阅的时钟")
}

// 档案要求免费但读不到详情：没有本次读到的资格（门禁会拒绝），不拿列表里的免费充数。
func TestSubscribe_RequireFreeDetailFailureHasNoSnapshot(t *testing.T) {
	e, m, ds := freeProfileEnv(t, models.MediaPrefRequire)
	ds.detail = func(string) (*v2.TorrentItem, error) { return nil, errors.New("站点返回 500") }
	e.pushRes = &ptinternal.PushTorrentResult{Success: false, Status: ptinternal.PushStatusRejectedStale, Message: "没有本次读取的免费资格，不推送"}

	msg, err := e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	pushes := e.gotPushes()
	require.Len(t, pushes, 1)
	assert.True(t, pushes[0].Policy.RequireFree)
	assert.Nil(t, pushes[0].Snapshot)
	assert.Contains(t, msg, "免费资格")
	assert.Empty(t, e.linked(m.ID), "没推成不记下，下次还能再试")
}

// 档案不要求免费：不要求免费，也不为它多读一次详情。
func TestSubscribe_NotRequireFreeSkipsDetail(t *testing.T) {
	e, m, ds := freeProfileEnv(t, "")
	ds.detail = func(string) (*v2.TorrentItem, error) { return nil, errors.New("不该读") }

	_, err := e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Zero(t, ds.calls)
	pushes := e.gotPushes()
	require.Len(t, pushes, 1)
	assert.False(t, pushes[0].Policy.RequireFree)
	assert.Equal(t, ptinternal.SourceKindSubscribe, pushes[0].Policy.SourceKind)
}
