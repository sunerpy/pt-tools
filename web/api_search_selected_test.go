package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// countingSearchSite 记下被搜了几次。
type countingSearchSite struct {
	fakeV2Site
	calls atomic.Int32
}

func (c *countingSearchSite) Search(_ context.Context, _ v2.SearchQuery) ([]v2.TorrentItem, error) {
	c.calls.Add(1)
	return nil, nil
}

// 指定了站点时只搜它们：指定的站点都没有启用时不能变成「搜所有站点」（空的站点列表在搜索里是全部），
// 没启用的写进 errors（2026-10-26 用户：「筛选了比如星陨阁时，其余站点不应参与搜索」）。
func TestApiMultiSiteSearch_RequestedSitesNotWidened(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "uid=1",
	}))
	hdsky := &countingSearchSite{fakeV2Site: fakeV2Site{id: "hdsky", name: "HDSky"}}
	xyg := &countingSearchSite{fakeV2Site: fakeV2Site{id: "xingyunge", name: "XingYunGe"}}
	withOrchestrator(t, hdsky, xyg)

	search := func(keyword string, sites ...string) MultiSiteSearchResponse {
		t.Helper()
		body, _ := json.Marshal(MultiSiteSearchRequest{Keyword: keyword, Sites: sites, TimeoutSecs: 5})
		w := httptest.NewRecorder()
		srv.apiMultiSiteSearch(w, httptest.NewRequest(http.MethodPost, "/api/v2/search/multi", bytes.NewReader(body)))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp MultiSiteSearchResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp
	}

	resp := search("天气之子", "xingyunge")
	assert.Empty(t, resp.Items)
	assert.Equal(t, []SearchErrorResponse{{Site: "xingyunge", Error: "站点没有启用，没有搜索"}}, resp.Errors)
	assert.Zero(t, hdsky.calls.Load(), "只选了没启用的星陨阁：别的站点不搜")
	assert.Zero(t, xyg.calls.Load())

	resp = search("天气之子", "hdsky", "xingyunge")
	assert.Equal(t, int32(1), hdsky.calls.Load())
	assert.Zero(t, xyg.calls.Load())
	assert.Equal(t, []SearchErrorResponse{{Site: "xingyunge", Error: "站点没有启用，没有搜索"}}, resp.Errors)

	// 换个关键词：同样的条件在缓存里
	search("你的名字")
	assert.Equal(t, int32(2), hdsky.calls.Load(), "不选站点：搜所有启用的站点")
	assert.Zero(t, xyg.calls.Load(), "没启用的不搜")
}
