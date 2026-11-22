package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// hdskyRow 是 HDSky 种子列表里的一行。
type hdskyRow struct {
	id string
	// icon 是优惠图标的类名（pro_free、pro_free2up），空是没有优惠；end 是优惠结束时间（北京时间），空是没写
	icon, end string
}

// hdskyServer 按 HDSky 的页面结构回种子列表：lists 的键是 spstate（全部列表是空串），值按页排（page 参数从 0 起，与站点一样）；
// raw 按「spstate#page」整页替换（限流页）。记下每次列表请求的查询串。details.php、download.php 回种子文件（推送用）。
type hdskyServer struct {
	mu       sync.Mutex
	lists    map[string][][]hdskyRow
	raw      map[string]string
	files    map[string][]byte
	requests []string
}

func (s *hdskyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := r.URL.Query()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	switch r.URL.Path {
	case "/torrents.php":
		s.requests = append(s.requests, q.Encode())
		page, _ := strconv.Atoi(q.Get("page"))
		if body, ok := s.raw[fmt.Sprintf("%s#%d", q.Get("spstate"), page)]; ok {
			_, _ = w.Write([]byte(body))
			return
		}
		var rows []hdskyRow
		if pages := s.lists[q.Get("spstate")]; page < len(pages) {
			rows = pages[page]
		}
		_, _ = w.Write([]byte(hdskyListPage(rows)))
	case "/details.php":
		_, _ = fmt.Fprintf(w, `<html><body><table><tr><td class="rowhead">下载链接</td>
<td class="rowfollow"><a href="download.php?id=%s&amp;passkey=k">下载</a></td></tr></table></body></html>`, q.Get("id"))
	case "/download.php":
		data, ok := s.files[q.Get("id")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-bittorrent")
		_, _ = w.Write(data)
	default:
		http.NotFound(w, r)
	}
}

// hdskyListPage 是登录以后的 torrents.php：顶部有用户信息栏，列表每行一个种子（与 HDSky 的选择器对得上）。
func hdskyListPage(rows []hdskyRow) string {
	var b strings.Builder
	b.WriteString(`<html><head><title>HDSky :: 种子</title></head><body>
<div id="info_block"><a href="userdetails.php?id=1" class="User_Name">me</a></div>
<table class="torrents"><tbody><tr><td class="colhead">类型</td><td class="colhead">标题</td></tr>`)
	for _, row := range rows {
		icon := ""
		if row.icon != "" {
			tip := ""
			if row.end != "" {
				tip = fmt.Sprintf(` onmouseover="domTT_activate(this, event, 'content', '&lt;span title=&quot;%s&quot;&gt;1天&lt;/span&gt;', 'trail', false)"`, row.end)
			}
			icon = fmt.Sprintf(`<img class="%s" src="pic/trans.gif" alt="Free"%s />`, row.icon, tip)
		}
		fmt.Fprintf(&b, `<tr>
<td class="rowfollow"><img alt="Movie" /></td>
<td class="rowfollow"><table class="torrentname"><tr><td class="embedded"><a href="details.php?id=%[1]s&amp;hit=1">Title %[1]s</a>%[2]s</td></tr></table></td>
<td class="rowfollow"></td>
<td class="rowfollow"><span title="2026-10-06 11:50:00">10分钟前</span></td>
<td class="rowfollow">4.00 GB</td>
<td class="rowfollow">2</td>
<td class="rowfollow">30</td>
<td class="rowfollow">5</td>
</tr>`, row.id, icon)
	}
	b.WriteString(`</tbody></table></body></html>`)
	return b.String()
}

// hdskyRig 是用 HDSky 站点定义与真实 NexusPHP 驱动读假站点的刷流环境；两次读列表之间的等待只推进假时钟。
func hdskyRig(t *testing.T) (*brushRig, *hdskyServer, *[]time.Duration) {
	t.Helper()
	r := newBrushRig(t)
	srv := &hdskyServer{lists: map[string][][]hdskyRow{}, raw: map[string]string{}, files: map[string][]byte{}}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	def, ok := v2.GetDefinitionRegistry().Get("hdsky")
	require.True(t, ok)
	opts, err := json.Marshal(v2.NexusPHPOptions{Cookie: "c_secure_pass=1"})
	require.NoError(t, err)
	site, err := v2.CreateSiteFromDefinition(def, v2.SiteConfig{
		ID: "hdsky", Name: "HDSky", BaseURL: ts.URL, Options: opts, RateLimit: 1000, RateBurst: 1000,
	}, zap.NewNop())
	require.NoError(t, err)
	r.mon.cfg.Sites = BrushSitesFunc(func(name string) (v2.Site, bool) { return site, name == "hdsky" })
	sleeps := &[]time.Duration{}
	r.mon.cfg.Sleep = func(_ context.Context, d time.Duration) error {
		*sleeps = append(*sleeps, d)
		r.clock.Advance(d)
		return nil
	}
	return r, srv, sleeps
}

// rows 造几行种子并把种子文件放进假站点。
func (s *hdskyServer) rows(t *testing.T, icon, end string, ids ...string) []hdskyRow {
	t.Helper()
	out := make([]hdskyRow, 0, len(ids))
	for _, id := range ids {
		data, _ := brushTorrentBytes(t, "t-"+id, 4*gib)
		s.files[id] = data
		out = append(out, hdskyRow{id: id, icon: icon, end: end})
	}
	return out
}

// 优惠结束时间：比假时钟（2026-10-06 04:00 UTC）晚一天，按北京时间写。
const hdskyFreeEnd = "2026-10-07 12:00:00"

// HDSky：勾了免费与 2X 免费（或留空，默认这两种）时各读一张列表的第 1 页，请求 spstate=2、spstate=4；只勾一种时只读那一张；
// 勾了站点没有筛选的优惠（50%）时读全部种子列表。
func TestBrushMonitor_HDSkyRequestsListPerDiscount(t *testing.T) {
	cases := []struct {
		discounts  string
		want       []string
		wantSleeps int
	}{
		{"", []string{"spstate=2", "spstate=4"}, 1},
		{"FREE,2XFREE", []string{"spstate=2", "spstate=4"}, 1},
		{"FREE", []string{"spstate=2"}, 0},
		{"2XFREE", []string{"spstate=4"}, 0},
		{"FREE,PERCENT_50", []string{""}, 0},
	}
	for _, c := range cases {
		t.Run("discounts="+c.discounts, func(t *testing.T) {
			r, srv, sleeps := hdskyRig(t)
			srv.lists["2"] = [][]hdskyRow{srv.rows(t, "pro_free", hdskyFreeEnd, "11", "12")}
			srv.lists["4"] = [][]hdskyRow{srv.rows(t, "pro_free2up", hdskyFreeEnd, "21")}
			srv.lists[""] = [][]hdskyRow{append(srv.rows(t, "pro_free", hdskyFreeEnd, "11"), srv.rows(t, "", "", "31")...)}
			task := r.task(t, func(bt *models.BrushTask) { bt.Discounts = c.discounts; bt.MaxDownloading = 1 })

			res, err := r.mon.RunTask(context.Background(), task.ID)
			require.NoError(t, err)
			assert.Equal(t, c.want, srv.requests)
			assert.Len(t, *sleeps, c.wantSleeps)
			assert.Equal(t, 1, res.Added, res.Summary())
		})
	}

	// 两张列表的运行记录；2x 免费列表里的种子也加进来
	r, srv, _ := hdskyRig(t)
	srv.lists["2"] = [][]hdskyRow{srv.rows(t, "pro_free", hdskyFreeEnd, "11", "12")}
	srv.lists["4"] = [][]hdskyRow{srv.rows(t, "pro_free2up", hdskyFreeEnd, "21")}
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 3 })
	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, res.Listed)
	assert.Equal(t, []string{
		"读取 hdsky 的免费列表第 1 页：2 个",
		"读取 hdsky 的 2x 免费列表第 1 页：1 个",
		"去重后共 3 个",
	}, stepTexts(onlyRun(t, r, task.ID), brushStepList))
	require.Equal(t, []string{"11", "12", "21"}, pushedIDs(r))
	assert.Equal(t, string(v2.Discount2xFree), r.pushes[2].Meta.FreeLevel)
}

// HDSky 免费列表第 1 页一个都不符合（没写结束时间）、2x 免费列表是空的：补读免费列表第 2 页（请求 page=1），加入那里的种子。
func TestBrushMonitor_HDSkyReadsSecondPage(t *testing.T) {
	r, srv, sleeps := hdskyRig(t)
	srv.lists["2"] = [][]hdskyRow{
		srv.rows(t, "pro_free", "", "11", "12"),
		srv.rows(t, "pro_free", hdskyFreeEnd, "13", "14"),
	}
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 4 })

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"spstate=2", "spstate=4", "page=1&spstate=2"}, srv.requests)
	assert.Equal(t, []time.Duration{61 * time.Second, 61 * time.Second}, *sleeps)
	assert.Equal(t, 4, res.Listed)
	assert.Equal(t, []string{"13", "14"}, pushedIDs(r))
	assert.Equal(t, []string{
		"读取 hdsky 的免费列表第 1 页：2 个",
		"读取 hdsky 的 2x 免费列表第 1 页：0 个",
		"候选不够（0 个，还能下 4 个），读取 hdsky 的免费列表第 2 页：2 个",
		"去重后共 4 个",
	}, stepTexts(onlyRun(t, r, task.ID), brushStepList))
}

// HDSky 回了「请求过于频繁」页：认成限流，不再读后面的列表与页，已经读到的照常加种，记成部分失败。
func TestBrushMonitor_HDSkyRateLimitPage(t *testing.T) {
	r, srv, _ := hdskyRig(t)
	srv.lists["2"] = [][]hdskyRow{srv.rows(t, "pro_free", hdskyFreeEnd, "11"), srv.rows(t, "pro_free", hdskyFreeEnd, "12")}
	srv.raw["4#0"] = `<html><head><title>403</title></head><body>请求过于频繁，限制访问间隔 60 秒/次</body></html>`
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 3 })

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"spstate=2", "spstate=4"}, srv.requests, "限流以后不再补读免费列表第 2 页")
	assert.Equal(t, []string{"11"}, pushedIDs(r))
	require.Len(t, res.Errors, 1)
	assert.Contains(t, res.Errors[0], "2x 免费列表第 1 页")
	assert.Contains(t, res.Errors[0], "rate limited")
	assert.Contains(t, res.Errors[0], "请求过于频繁，限制访问间隔 60 秒/次")
	assert.Equal(t, models.BrushRunPartial, onlyRun(t, r, task.ID).Status)
}
