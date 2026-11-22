package scheduler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	_ "github.com/sunerpy/pt-tools/site/v2/definitions"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 测试用的读列表能力：与 HDSky、PTTime 站点定义里的写法一样（定义本身由 site/v2/definitions 的测试守着）。
var (
	hdskyLists = &v2.BrushListConfig{
		DiscountFilters: map[v2.DiscountLevel]string{v2.DiscountFree: "2", v2.Discount2xFree: "4"},
		Paging:          true,
		MinInterval:     61 * time.Second,
	}
	pagingOnly = &v2.BrushListConfig{Paging: true}
)

func TestBrushListPlan(t *testing.T) {
	free := brushList{name: "免费列表", query: v2.SearchQuery{BrushDiscount: v2.DiscountFree}, pageable: true}
	twoX := brushList{name: "2x 免费列表", query: v2.SearchQuery{BrushDiscount: v2.Discount2xFree}, pageable: true}
	cases := []struct {
		name      string
		discounts string
		kind      v2.SiteKind
		spec      *v2.BrushListConfig
		want      []brushList
	}{
		{"按优惠类型：默认是免费、2X 免费各一张", "", v2.SiteNexusPHP, hdskyLists, []brushList{free, twoX}},
		{"按优惠类型：勾了两种", "FREE,2XFREE", v2.SiteNexusPHP, hdskyLists, []brushList{free, twoX}},
		{"按优惠类型：顺序固定，免费在前", "2XFREE,FREE", v2.SiteNexusPHP, hdskyLists, []brushList{free, twoX}},
		{"按优惠类型：只勾免费", "FREE", v2.SiteNexusPHP, hdskyLists, []brushList{free}},
		{"按优惠类型：只勾 2X 免费", "2XFREE", v2.SiteNexusPHP, hdskyLists, []brushList{twoX}},
		{
			"有一种没有筛选值：读全部列表、在本地挑", "FREE,PERCENT_50", v2.SiteNexusPHP, hdskyLists,
			[]brushList{{name: "全部种子列表", local: true, pageable: true}},
		},
		{"只能翻页的站点：全部列表", "", v2.SiteNexusPHP, pagingOnly, []brushList{{name: "全部种子列表", local: true, pageable: true}}},
		{
			"只能翻页的站点只勾免费：免费列表", "FREE", v2.SiteNexusPHP, pagingOnly,
			[]brushList{{name: "免费种子列表", query: v2.SearchQuery{FreeOnly: true}, pageable: true}},
		},
		{"没写的站点：全部列表第 1 页", "", v2.SiteNexusPHP, nil, []brushList{{name: "全部种子列表", local: true}}},
		{"没写的站点只勾免费：免费列表第 1 页", "FREE", v2.SiteNexusPHP, nil, []brushList{{name: "免费种子列表", query: v2.SearchQuery{FreeOnly: true}}}},
		{
			"M-Team 按分区读、不补页", "", v2.SiteMTorrent, hdskyLists,
			[]brushList{{name: "综合区", query: v2.SearchQuery{Section: BrushSectionNormal}}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, brushListPlan(models.BrushTask{Discounts: c.discounts}, c.kind, c.spec))
		})
	}

	both := brushListPlan(models.BrushTask{Section: BrushSectionAll, Discounts: "FREE"}, v2.SiteMTorrent, nil)
	assert.Equal(t, []brushList{
		{name: "综合区", query: v2.SearchQuery{FreeOnly: true, Section: BrushSectionNormal}},
		{name: "成人区", query: v2.SearchQuery{FreeOnly: true, Section: BrushSectionAdult}},
	}, both, "M-Team 的查询和以前一样")

	def, ok := v2.GetDefinitionRegistry().Get("hdsky")
	require.True(t, ok)
	assert.Equal(t, []brushList{free, twoX}, brushListPlan(models.BrushTask{}, v2.SiteNexusPHP, def.BrushList), "HDSky 的站点定义")
}

// listSite 是按列表与页码回种子的假站点，实现 v2.BrushListSource。pages 的键见 listKey，值按页码（从 1 起）排；
// errs 的键是「键#页码」。请求与种子文件下载都记在 fakeBrushSite 里。
type listSite struct {
	*fakeBrushSite
	spec  *v2.BrushListConfig
	pages map[string][][]v2.TorrentItem
	errs  map[string]error
	// deadlines 是每次读列表拿到的 ctx 的期限
	deadlines []time.Time
}

// listKey 是一次列表请求读的是哪张列表：按优惠类型读的是优惠类型，免费列表是 free，全部列表是 all。
func listKey(q v2.SearchQuery) string {
	switch {
	case q.BrushDiscount != "":
		return string(q.BrushDiscount)
	case q.FreeOnly:
		return "free"
	}
	return "all"
}

func (s *listSite) BrushList() *v2.BrushListConfig { return s.spec }

func (s *listSite) Search(ctx context.Context, q v2.SearchQuery) ([]v2.TorrentItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.searches = append(s.searches, q)
	deadline, _ := ctx.Deadline()
	s.deadlines = append(s.deadlines, deadline)
	if s.onSearch != nil {
		s.onSearch()
	}
	page := max(q.Page, 1)
	if err := s.errs[fmt.Sprintf("%s#%d", listKey(q), page)]; err != nil {
		return nil, err
	}
	pages := s.pages[listKey(q)]
	if page > len(pages) {
		return nil, nil
	}
	return pages[page-1], nil
}

// listRig 是站点换成 listSite 的刷流环境；两次读列表之间的等待只推进假时钟，等了多久记在返回的切片里。
func listRig(t *testing.T, spec *v2.BrushListConfig) (*brushRig, *listSite, *[]time.Duration) {
	t.Helper()
	r := newBrushRig(t)
	ls := &listSite{fakeBrushSite: r.site, spec: spec, pages: map[string][][]v2.TorrentItem{}, errs: map[string]error{}}
	sleeps := &[]time.Duration{}
	r.mon.cfg.Sites = BrushSitesFunc(func(name string) (v2.Site, bool) { return ls, name == r.siteName })
	r.mon.cfg.Sleep = func(_ context.Context, d time.Duration) error {
		*sleeps = append(*sleeps, d)
		r.clock.Advance(d)
		return nil
	}
	return r, ls, sleeps
}

// listItems 造一页种子（默认免费、12 小时后到期、4 GB），种子文件放进假站点。
func (r *brushRig) listItems(t *testing.T, mod func(*v2.TorrentItem), ids ...string) []v2.TorrentItem {
	t.Helper()
	out := make([]v2.TorrentItem, 0, len(ids))
	for _, id := range ids {
		if _, ok := r.site.files[id]; !ok {
			data, _ := brushTorrentBytes(t, "t-"+id, 4*gib)
			r.site.files[id] = data
		}
		it := v2.TorrentItem{
			ID: id, Title: "Title " + id, SizeBytes: 4 * gib, Seeders: 2, Leechers: 30,
			UploadedAt: r.clock.Now().Add(-10 * time.Minute).Unix(), DiscountLevel: v2.DiscountFree,
			DiscountEndTime: r.clock.Now().Add(12 * time.Hour),
		}
		if mod != nil {
			mod(&it)
		}
		out = append(out, it)
	}
	return out
}

// noEnd 让种子看不出免费到什么时候：只收免费的任务不收（「免费结束时间未知」）。
func noEnd(i *v2.TorrentItem) { i.DiscountEndTime = time.Time{} }

func twoXFree(i *v2.TorrentItem) { i.DiscountLevel = v2.Discount2xFree }

// searchedPages 是这一轮读了哪些列表的第几页（「键#页码」）。
func searchedPages(r *brushRig) []string {
	out := make([]string, 0, len(r.site.searches))
	for _, q := range r.site.searches {
		out = append(out, fmt.Sprintf("%s#%d", listKey(q), max(q.Page, 1)))
	}
	return out
}

func pushedIDs(r *brushRig) []string {
	out := make([]string, 0, len(r.pushes))
	for _, p := range r.pushes {
		out = append(out, p.TorrentID)
	}
	return out
}

// 站点定义给全了任务允许的优惠类型：每种读一张列表的第 1 页（免费在前），两次之间等够站点要求的间隔；候选够了不补页。
func TestBrushMonitor_ReadsOneListPerDiscount(t *testing.T) {
	r, ls, sleeps := listRig(t, hdskyLists)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 2 })
	ls.pages["FREE"] = [][]v2.TorrentItem{r.listItems(t, nil, "1", "2")}
	ls.pages["2XFREE"] = [][]v2.TorrentItem{r.listItems(t, twoXFree, "3")}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"FREE#1", "2XFREE#1"}, searchedPages(r))
	for _, q := range r.site.searches {
		assert.False(t, q.FreeOnly, "按优惠类型读的列表只用 BrushDiscount")
	}
	assert.Equal(t, []time.Duration{61 * time.Second}, *sleeps, "两次读列表之间等站点要求的 61 秒")
	assert.Equal(t, 3, res.Listed)
	assert.Equal(t, 3, res.Eligible)
	assert.Equal(t, 2, res.Added)

	run := onlyRun(t, r, task.ID)
	assert.Equal(t, models.BrushRunOK, run.Status)
	assert.Equal(t, []string{
		"读取 hdsky 的免费列表第 1 页：2 个",
		"读取 hdsky 的 2x 免费列表第 1 页：1 个",
		"去重后共 3 个",
	}, stepTexts(run, brushStepList))
	assert.Contains(t, stepTexts(run, brushStepInfo), "等 61 秒再读（hdsky 两次读列表至少隔 61 秒）")
}

// 第 1 页的候选不够（全被拒、以前加过）：按顺序给第 1 页不是空的列表补读第 2 页，够了就停。
func TestBrushMonitor_ReadsSecondPageWhenShort(t *testing.T) {
	r, ls, sleeps := listRig(t, hdskyLists)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 4 })
	repo := models.NewBrushRepository(r.db.DB)
	require.NoError(t, repo.RecordAdded(&models.BrushTorrent{
		TaskID: task.ID, InfoHash: "old", SiteName: "hdsky", TorrentID: "seen", AddedAt: r.clock.Now(), State: models.BrushTorrentRemoved,
	}, "2026-10-05"))
	ls.pages["FREE"] = [][]v2.TorrentItem{
		append(r.listItems(t, noEnd, "1", "2"), r.listItems(t, nil, "seen")...),
		r.listItems(t, nil, "3", "4"),
	}
	ls.pages["2XFREE"] = [][]v2.TorrentItem{{}}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"FREE#1", "2XFREE#1", "FREE#2"}, searchedPages(r), "2x 免费列表的第 1 页是空的，不补页")
	assert.Equal(t, 2, r.site.searches[2].Page, "第 2 页")
	assert.Equal(t, []time.Duration{61 * time.Second, 61 * time.Second}, *sleeps)
	assert.Equal(t, 5, res.Listed)
	assert.Equal(t, []string{"3", "4"}, pushedIDs(r))

	run := onlyRun(t, r, task.ID)
	assert.Equal(t, []string{
		"读取 hdsky 的免费列表第 1 页：3 个",
		"读取 hdsky 的 2x 免费列表第 1 页：0 个",
		"候选不够（0 个，还能下 4 个），读取 hdsky 的免费列表第 2 页：2 个",
		"去重后共 5 个",
	}, stepTexts(run, brushStepList))
	assert.Contains(t, stepTexts(run, brushStepInfo), "以前加过的 1 个、不符合条件 2 个、符合条件 2 个")
}

// 读完一页候选就够了：不再往后读。
func TestBrushMonitor_StopsPagingWhenEnough(t *testing.T) {
	r, ls, _ := listRig(t, hdskyLists)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 2 })
	ls.pages["FREE"] = [][]v2.TorrentItem{r.listItems(t, nil, "1"), r.listItems(t, nil, "2", "3")}
	ls.pages["2XFREE"] = [][]v2.TorrentItem{r.listItems(t, noEnd, "4"), r.listItems(t, twoXFree, "5")}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"FREE#1", "2XFREE#1", "FREE#2"}, searchedPages(r))
	assert.Equal(t, 2, res.Added)
}

// 没设同时下载上限（老数据）：候选为 0 才补页。
func TestBrushMonitor_NoDownloadLimitPagesOnlyWithoutCandidates(t *testing.T) {
	r, ls, _ := listRig(t, hdskyLists)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 0; bt.Discounts = "FREE" })
	ls.pages["FREE"] = [][]v2.TorrentItem{r.listItems(t, nil, "1"), r.listItems(t, nil, "2")}
	_, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"FREE#1"}, searchedPages(r), "有一个候选就不补")

	r, ls, _ = listRig(t, hdskyLists)
	task = r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 0; bt.Discounts = "FREE" })
	ls.pages["FREE"] = [][]v2.TorrentItem{r.listItems(t, noEnd, "1"), r.listItems(t, nil, "2")}
	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"FREE#1", "FREE#2"}, searchedPages(r))
	assert.Equal(t, 1, res.Added)
	assert.Contains(t, stepTexts(onlyRun(t, r, task.ID), brushStepList), "没有候选，读取 hdsky 的免费列表第 2 页：1 个")
}

// 同时下载的名额已满：一张列表都不读。
func TestBrushMonitor_NoListReadWhenSlotsFull(t *testing.T) {
	r, ls, _ := listRig(t, hdskyLists)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 1 })
	require.NoError(t, models.NewBrushRepository(r.db.DB).RecordAdded(&models.BrushTorrent{
		TaskID: task.ID, InfoHash: "h1", SiteName: "hdsky", TorrentID: "9", Title: "T9",
		AddedAt: r.clock.Now(), State: models.BrushTorrentActive, DownloaderID: 1,
	}, "2026-10-06"))
	r.dl.torrents = []downloader.Torrent{{ID: "h1", InfoHash: "h1", Tags: models.BrushTaskTag(task.ID), Progress: 0.3}}
	ls.pages["FREE"] = [][]v2.TorrentItem{r.listItems(t, nil, "1")}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Empty(t, r.site.searches)
	assert.Contains(t, res.Stopped, "达到上限")
}

// 补不补页按「还能下几个」算：任务名下在下的种子占着名额；下载器里已有的种子不算候选。
func TestBrushMonitor_PagingCountsFreeSlotsAndSkipsInDownloader(t *testing.T) {
	// 同时下载上限 3、已经在下 1 个：还能下 2 个，第 1 页有 2 个候选就不补页
	r, ls, _ := listRig(t, hdskyLists)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 3 })
	require.NoError(t, models.NewBrushRepository(r.db.DB).RecordAdded(&models.BrushTorrent{
		TaskID: task.ID, InfoHash: "h1", SiteName: "hdsky", TorrentID: "9", Title: "T9",
		AddedAt: r.clock.Now(), State: models.BrushTorrentActive, DownloaderID: 1,
	}, "2026-10-06"))
	r.dl.torrents = []downloader.Torrent{{ID: "h1", InfoHash: "h1", Tags: models.BrushTaskTag(task.ID), Progress: 0.3}}
	ls.pages["FREE"] = [][]v2.TorrentItem{r.listItems(t, nil, "1", "2"), r.listItems(t, nil, "3")}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"FREE#1", "2XFREE#1"}, searchedPages(r), "2 个候选、还能下 2 个：不补页")
	assert.Equal(t, 2, res.Added)

	// 第 1 页的两个种子下载器里已经有了：不算候选，补读第 2 页
	r, ls, _ = listRig(t, hdskyLists)
	task = r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 2 })
	first := r.listItems(t, nil, "1", "2")
	first[0].InfoHash, first[1].InfoHash = "AAA", "BBB"
	r.dl.torrents = []downloader.Torrent{{ID: "aaa", InfoHash: "aaa"}, {ID: "bbb", InfoHash: "bbb"}}
	ls.pages["FREE"] = [][]v2.TorrentItem{first, r.listItems(t, nil, "3")}

	_, err = r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"FREE#1", "2XFREE#1", "FREE#2"}, searchedPages(r))
	assert.Equal(t, []string{"3"}, pushedIDs(r))
	assert.Contains(t, stepTexts(onlyRun(t, r, task.ID), brushStepList),
		"候选不够（0 个，还能下 2 个），读取 hdsky 的免费列表第 2 页：1 个")
}

// 读列表在加种锁外、可能要几分钟：锁里重新读「以前加过的」，这期间别的任务加了同一个种子时，这一轮不再推送它。
func TestBrushMonitor_RereadsSeenUnderLockAfterListing(t *testing.T) {
	r, ls, _ := listRig(t, hdskyLists)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 2 })
	other := r.task(t, func(bt *models.BrushTask) { bt.Name = "另一个"; bt.Enabled = false })
	ls.pages["FREE"] = [][]v2.TorrentItem{r.listItems(t, nil, "1", "2")}
	reads := 0
	r.site.onSearch = func() {
		reads++
		if reads == 2 { // 读 2x 免费列表的时候，别的任务加了种子 1
			require.NoError(t, models.NewBrushRepository(r.db.DB).RecordAdded(&models.BrushTorrent{
				TaskID: other.ID, InfoHash: "x1", SiteName: "hdsky", TorrentID: "1", Title: "Title 1",
				AddedAt: r.clock.Now(), State: models.BrushTorrentActive, DownloaderID: 1,
			}, "2026-10-06"))
		}
	}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"FREE#1", "2XFREE#1"}, searchedPages(r))
	assert.Equal(t, []string{"2"}, pushedIDs(r), "种子 1 已由别的任务加入，不再推送")
	assert.Equal(t, 1, res.Added)
}

// 两张列表里重复的种子只算一次，留最近读到的那份。
func TestBrushMonitor_DuplicateIDsAcrossListsCountOnce(t *testing.T) {
	r, ls, _ := listRig(t, hdskyLists)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 5 })
	ls.pages["FREE"] = [][]v2.TorrentItem{r.listItems(t, nil, "1", "2")}
	ls.pages["2XFREE"] = [][]v2.TorrentItem{r.listItems(t, twoXFree, "2", "3")}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, res.Listed)
	assert.ElementsMatch(t, []string{"1", "2", "3"}, pushedIDs(r))
	for _, p := range r.pushes {
		if p.TorrentID == "2" {
			assert.Equal(t, string(v2.Discount2xFree), p.Meta.FreeLevel, "后读到的 2x 免费列表那份")
		}
	}
	assert.Contains(t, stepTexts(onlyRun(t, r, task.ID), brushStepList), "去重后共 3 个")
}

// 一轮最多读 4 次列表：第 1 页先读完，补页读到 4 次就停。
func TestBrushMonitor_AtMostFourListReads(t *testing.T) {
	spec := &v2.BrushListConfig{
		DiscountFilters: map[v2.DiscountLevel]string{v2.DiscountFree: "2", v2.Discount2xFree: "4", v2.DiscountPercent50: "5"},
		Paging:          true,
	}
	r, ls, _ := listRig(t, spec)
	task := r.task(t, func(bt *models.BrushTask) {
		bt.MaxDownloading = 5
		bt.Discounts = "FREE,2XFREE,PERCENT_50"
		bt.MinLeechers = 100 // 一个都不符合
	})
	for i, key := range []string{"FREE", "2XFREE", "PERCENT_50"} {
		ls.pages[key] = [][]v2.TorrentItem{r.listItems(t, nil, fmt.Sprint(i*10+1)), r.listItems(t, nil, fmt.Sprint(i*10+2))}
	}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"FREE#1", "2XFREE#1", "PERCENT_50#1", "FREE#2"}, searchedPages(r))
	assert.Equal(t, 4, res.Listed)
	assert.Contains(t, stepTexts(onlyRun(t, r, task.ID), brushStepInfo), "这一轮已经读了 4 次列表，不再读 hdsky 的 2x 免费列表第 2 页")
}

// 第 2 页和第 1 页一样（站点不认翻页）：停掉这张列表，不重复计数；第 1 页是空的列表不补页。
func TestBrushMonitor_RepeatedPageStopsList(t *testing.T) {
	r, ls, _ := listRig(t, hdskyLists)
	task := r.task(t, nil)
	first := r.listItems(t, noEnd, "1", "2")
	ls.pages["FREE"] = [][]v2.TorrentItem{first, {first[1], first[0]}}
	ls.pages["2XFREE"] = [][]v2.TorrentItem{{}, r.listItems(t, twoXFree, "3")}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"FREE#1", "2XFREE#1", "FREE#2"}, searchedPages(r))
	assert.Equal(t, 2, res.Listed)
	assert.Contains(t, stepTexts(onlyRun(t, r, task.ID), brushStepList),
		"候选不够（0 个，还能下 2 个），读取 hdsky 的免费列表第 2 页：2 个，和第 1 页的一样")
}

// 只写了能翻页的站点（PTTime）：照旧读全部种子列表，候选不够时补第 2 页；没有读列表间隔，不用等。
func TestBrushMonitor_PagesAllListForPagingOnlySite(t *testing.T) {
	r, ls, sleeps := listRig(t, pagingOnly)
	r.siteName = "pttime"
	task := r.task(t, func(bt *models.BrushTask) { bt.SiteName = "pttime"; bt.Name = "pttime 刷流" })
	all := r.listItems(t, noEnd, "1", "2")
	ls.pages["all"] = [][]v2.TorrentItem{all, append([]v2.TorrentItem{all[1]}, r.listItems(t, nil, "3")...)}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"all#1", "all#2"}, searchedPages(r))
	for _, q := range r.site.searches {
		assert.False(t, q.FreeOnly)
		assert.Empty(t, q.BrushDiscount)
	}
	assert.Empty(t, *sleeps)
	assert.Equal(t, 3, res.Listed)
	assert.Equal(t, []string{"3"}, pushedIDs(r))
	assert.Equal(t, []string{
		"读取 pttime 的全部种子列表第 1 页（本地按优惠类型挑）：2 个",
		"候选不够（0 个，还能下 2 个），读取 pttime 的全部种子列表第 2 页（本地按优惠类型挑）：2 个",
		"去重后共 3 个",
	}, stepTexts(onlyRun(t, r, task.ID), brushStepList))
}

// 登录失效、质询、限流：这一轮不再读这个站点的别的列表，读到的照常用，记成部分失败。
func TestBrushMonitor_SiteErrorsStopReadingLists(t *testing.T) {
	limited := fmt.Errorf("parse search: %w", fmt.Errorf("%w：站点限制了访问频率（页面标题：403；开头：请求过于频繁，限制访问间隔 60 秒/次）", v2.ErrRateLimited))
	for name, failure := range map[string]error{
		"限流":   limited,
		"登录失效": fmt.Errorf("execute search: %w", v2.ErrSessionExpired),
		"质询":   fmt.Errorf("execute search: HTTP 403: %w", v2.ErrCloudflareChallenge),
	} {
		t.Run(name, func(t *testing.T) {
			r, ls, _ := listRig(t, hdskyLists)
			task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 3 })
			ls.pages["FREE"] = [][]v2.TorrentItem{r.listItems(t, nil, "1"), r.listItems(t, nil, "2")}
			ls.errs["2XFREE#1"] = failure

			res, err := r.mon.RunTask(context.Background(), task.ID)
			require.NoError(t, err)
			assert.Equal(t, []string{"FREE#1", "2XFREE#1"}, searchedPages(r), "不再补读免费列表第 2 页")
			assert.Equal(t, []string{"1"}, pushedIDs(r))
			require.Len(t, res.Errors, 1)
			assert.Contains(t, res.Errors[0], "2x 免费列表第 1 页")
			assert.Contains(t, res.Errors[0], "这一轮不再读这个站点的列表")
			assert.Equal(t, models.BrushRunPartial, onlyRun(t, r, task.ID).Status)
		})
	}

	// 第一张就限流：这一轮失败，别的列表都不读
	r, ls, _ := listRig(t, hdskyLists)
	task := r.task(t, nil)
	ls.errs["FREE#1"] = limited
	ls.pages["2XFREE"] = [][]v2.TorrentItem{r.listItems(t, twoXFree, "1")}
	_, err := r.mon.RunTask(context.Background(), task.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, v2.ErrRateLimited)
	assert.Contains(t, err.Error(), "请求过于频繁")
	assert.Equal(t, []string{"FREE#1"}, searchedPages(r))
}

// 普通的读失败（网络、5xx）：记下是哪张列表第几页，别的列表照常读、照常补页。
func TestBrushMonitor_ListErrorKeepsOtherLists(t *testing.T) {
	r, ls, _ := listRig(t, hdskyLists)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 3 })
	ls.errs["FREE#1"] = errors.New("HTTP 502")
	ls.pages["2XFREE"] = [][]v2.TorrentItem{r.listItems(t, twoXFree, "1"), r.listItems(t, twoXFree, "2")}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"FREE#1", "2XFREE#1", "2XFREE#2"}, searchedPages(r), "读失败的免费列表不补页")
	assert.Equal(t, 2, res.Added)
	require.Len(t, res.Errors, 1)
	assert.Equal(t, "读取 hdsky 的种子列表失败（免费列表第 1 页: HTTP 502），这一轮只用读到的列表", res.Errors[0])
}

// 每张列表都读不到：这一轮失败，错误里写明每张列表第几页。
func TestBrushMonitor_AllListReadsFail(t *testing.T) {
	r, ls, _ := listRig(t, hdskyLists)
	task := r.task(t, nil)
	ls.errs["FREE#1"] = errors.New("HTTP 502")
	ls.errs["2XFREE#1"] = errors.New("HTTP 500")

	_, err := r.mon.RunTask(context.Background(), task.ID)
	require.Error(t, err)
	assert.Equal(t, "读取 hdsky 的种子列表失败: 免费列表第 1 页: HTTP 502；2x 免费列表第 1 页: HTTP 500", err.Error())
	assert.Empty(t, r.pushes)
	assert.Equal(t, models.BrushRunFailed, onlyRun(t, r, task.ID).Status)
}

// 两次读列表之间按站点的间隔等：从上一次读完算起，等待经注入的 Sleep（测试里只推进假时钟）。
func TestBrushMonitor_WaitsBetweenListReads(t *testing.T) {
	r, ls, sleeps := listRig(t, hdskyLists)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 5 })
	var starts []time.Time
	r.site.onSearch = func() {
		starts = append(starts, r.clock.Now())
		r.clock.Advance(5 * time.Second) // 一次读列表用 5 秒
	}
	ls.pages["FREE"] = [][]v2.TorrentItem{r.listItems(t, noEnd, "1"), r.listItems(t, noEnd, "2")}
	ls.pages["2XFREE"] = [][]v2.TorrentItem{r.listItems(t, noEnd, "3"), r.listItems(t, noEnd, "4")}

	_, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	require.Len(t, starts, 4)
	for i := 1; i < len(starts); i++ {
		assert.Equal(t, 66*time.Second, starts[i].Sub(starts[i-1]), "上一次读完以后再等 61 秒")
	}
	assert.Equal(t, []time.Duration{61 * time.Second, 61 * time.Second, 61 * time.Second}, *sleeps)
}

// 时间不够就不再读：整轮剩下的时间要够等完间隔、读一页、再留出加种的时间；读列表一共最多 3 分钟（不算等的）。
func TestBrushMonitor_ListReadsStopWhenTimeRunsOut(t *testing.T) {
	r, ls, sleeps := listRig(t, hdskyLists)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 3 })
	ls.pages["FREE"] = [][]v2.TorrentItem{r.listItems(t, nil, "1")}
	ls.pages["2XFREE"] = [][]v2.TorrentItem{r.listItems(t, twoXFree, "2")}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	res, err := r.mon.RunTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"FREE#1"}, searchedPages(r), "等 61 秒、读一页、留 1 分钟加种，100 秒不够")
	assert.Empty(t, *sleeps)
	assert.Equal(t, 1, res.Added)
	assert.Contains(t, stepTexts(onlyRun(t, r, task.ID), brushStepInfo), "这一轮剩下的时间不够，不再读 hdsky 的 2x 免费列表第 1 页")

	// 读列表本身用掉了 2 分 40 秒（假时钟）：剩下的不够一次像样的请求
	r, ls, _ = listRig(t, pagingOnly)
	task = r.task(t, nil)
	r.site.onSearch = func() { r.clock.Advance(160 * time.Second) }
	ls.pages["all"] = [][]v2.TorrentItem{r.listItems(t, noEnd, "1"), r.listItems(t, nil, "2")}
	_, err = r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"all#1"}, searchedPages(r))
	assert.Contains(t, stepTexts(onlyRun(t, r, task.ID), brushStepInfo), "这一轮剩下的时间不够，不再读 hdsky 的全部种子列表第 2 页")
}

// 第一次以后的每次读列表，时限都给加种留出 brushAdmitReserve（还要先等完站点要求的间隔）；第一次的时限与以前一样。
func TestBrushMonitor_ListReadTimeoutLeavesTimeToAdd(t *testing.T) {
	r, ls, _ := listRig(t, hdskyLists)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 5 })
	ls.pages["FREE"] = [][]v2.TorrentItem{r.listItems(t, nil, "1"), r.listItems(t, nil, "2")}
	ls.pages["2XFREE"] = [][]v2.TorrentItem{r.listItems(t, twoXFree, "3"), r.listItems(t, twoXFree, "4")}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()
	round, _ := ctx.Deadline()

	_, err := r.mon.RunTask(ctx, task.ID)
	require.NoError(t, err)
	require.Len(t, ls.deadlines, 4)
	assert.WithinDuration(t, time.Now().Add(brushSearchTimeout), ls.deadlines[0], 5*time.Second, "第一次读：2 分钟")
	for _, d := range ls.deadlines[1:] {
		assert.GreaterOrEqual(t, round.Sub(d), 61*time.Second+brushAdmitReserve-time.Second, "等的 61 秒与留给加种的 1 分钟不给读列表")
	}
}

// 这一轮已经结束（ctx 取消了），或者等间隔时被打断：不再读后面的列表，读到的照常处理。
func TestBrushMonitor_StopsListingWhenRoundEnds(t *testing.T) {
	r, ls, _ := listRig(t, hdskyLists)
	task := r.task(t, nil)
	ls.pages["FREE"] = [][]v2.TorrentItem{r.listItems(t, nil, "1")}
	ls.pages["2XFREE"] = [][]v2.TorrentItem{r.listItems(t, twoXFree, "2")}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := r.mon.RunTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"FREE#1"}, searchedPages(r))
	assert.Equal(t, "本轮时间用完", res.Stopped)

	r, ls, _ = listRig(t, hdskyLists)
	task = r.task(t, nil)
	ls.pages["FREE"] = [][]v2.TorrentItem{r.listItems(t, nil, "1")}
	ls.pages["2XFREE"] = [][]v2.TorrentItem{r.listItems(t, twoXFree, "2")}
	r.mon.cfg.Sleep = func(context.Context, time.Duration) error { return context.Canceled }
	res, err = r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"FREE#1"}, searchedPages(r), "等间隔时被打断")
	assert.Equal(t, 1, res.Added)
}

// 要补页时先读「以前加过的」：读不出来这一轮失败，列表一张都不读。
func TestBrushMonitor_SeenReadFailureBeforeListing(t *testing.T) {
	r, ls, _ := listRig(t, hdskyLists)
	task := r.task(t, nil)
	ls.pages["FREE"] = [][]v2.TorrentItem{r.listItems(t, nil, "1")}
	downloaders := r.mon.cfg.Downloaders
	r.mon.cfg.Downloaders = BrushDownloadersFunc(func(id uint) (downloader.Downloader, string, error) {
		// 采样、删种读完记录以后、加种读「以前加过的」之前，刷流记录表没了
		if r.db.DB.Migrator().HasTable(&models.BrushTorrent{}) {
			require.NoError(t, r.db.DB.Migrator().DropTable(&models.BrushTorrent{}))
		}
		return downloaders.BrushDownloader(id)
	})
	_, err := r.mon.RunTask(context.Background(), task.ID)
	require.Error(t, err)
	assert.Empty(t, r.site.searches)
}

// M-Team 的综合区限流：这一轮不再读成人区。
func TestBrushMonitor_MTeamRateLimitSkipsOtherSection(t *testing.T) {
	r := mteamRig(t, map[string][]string{"adult": {"2"}})
	r.site.sectionErr = map[string]error{"normal": fmt.Errorf("execute search: HTTP 429: %w", v2.ErrRateLimited)}
	task := mteamTask(t, r, BrushSectionAll)
	_, err := r.mon.RunTask(context.Background(), task.ID)
	require.Error(t, err)
	assert.Equal(t, []string{"normal"}, searchedSections(r))
	assert.Contains(t, err.Error(), "综合区: execute search: HTTP 429")
}

// 单个种子放不下（推送返回 RejectedTooLarge）：跳过它、接着试后面的候选，不小于它的不再下种子文件；
// 有效空间到了保底线、读不到空间、站点容量满了照旧结束这一轮的加种。
func TestBrushMonitor_TooLargeSkipsToSmallerTorrents(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 5 })
	r.addItem(t, "big", func(i *v2.TorrentItem) { i.SizeBytes = 15 * gib; i.Leechers = 90 })
	r.addItem(t, "bigger", func(i *v2.TorrentItem) { i.SizeBytes = 20 * gib; i.Leechers = 80 })
	r.addItem(t, "small", func(i *v2.TorrentItem) { i.SizeBytes = 4 * gib; i.Leechers = 70 })
	push := r.mon.cfg.Push
	r.mon.cfg.Push = func(ctx context.Context, req ptinternal.PushTorrentRequest) (*ptinternal.PushTorrentResult, error) {
		if req.Meta.SizeBytes > 10*gib {
			r.pushes = append(r.pushes, req)
			return &ptinternal.PushTorrentResult{Status: ptinternal.PushStatusRejectedTooLarge, Message: "种子体积 15.0 GB 超过当前可用空间 12.0 GB，已跳过"}, nil
		}
		return push(ctx, req)
	}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Added)
	assert.Empty(t, res.Stopped)
	assert.Equal(t, []string{"big", "small"}, r.site.downloads, "不小于放不下的那个的，不再下种子文件")
	assert.Equal(t, []string{"big", "small"}, pushedIDs(r))
	assert.Equal(t, []string{
		"Title big：种子体积 15.0 GB 超过当前可用空间 12.0 GB，已跳过",
		"Title bigger：体积 20.00 GB，不小于刚才放不下的 15.00 GB，跳过",
	}, res.Skipped)

	for _, status := range []ptinternal.PushStatus{ptinternal.PushStatusRejectedDisk, ptinternal.PushStatusRejectedCapacity} {
		r := newBrushRig(t)
		task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 5 })
		r.addItem(t, "1", nil)
		r.addItem(t, "2", nil)
		r.pushResult = &ptinternal.PushTorrentResult{Status: status, Message: "拒绝：" + string(status)}
		res, err := r.mon.RunTask(context.Background(), task.ID)
		require.NoError(t, err)
		assert.Len(t, r.pushes, 1, status)
		assert.Equal(t, "推送被拒绝：拒绝："+string(status), res.Stopped)
	}
}

// 真实推送入口的磁盘保护：大种放不下（有效空间减去它低于保底线）跳过、小种加入；有效空间到了保底线时这一轮停止加种。
func TestBrushMonitor_RealDiskGateSkipsTooLarge(t *testing.T) {
	r := newBrushRig(t)
	dl := &addingDownloader{schedFakeDownloader: r.dl}
	dl.diskInfo.FreeSpace = 20 * gib
	ds := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, r.db.DB.Create(&ds).Error)
	require.NoError(t, r.db.DB.Create(&models.SettingsGlobal{DownloadDir: t.TempDir(), CleanupDiskProtect: true, CleanupMinDiskSpaceGB: 10}).Error)
	restore := ptinternal.SwapPushDownloaderFactory(func(models.DownloaderSetting) (downloader.Downloader, error) { return dl, nil })
	t.Cleanup(restore)
	ptinternal.GetDiskBudget().Reset()
	t.Cleanup(func() { ptinternal.GetDiskBudget().Reset() })
	r.mon.cfg.Push = ptinternal.PushTorrentToDownloader
	r.mon.cfg.Downloaders = BrushDownloadersFunc(func(uint) (downloader.Downloader, string, error) { return dl, "qb", nil })
	task := r.task(t, func(bt *models.BrushTask) { bt.DownloaderID = ds.ID; bt.MaxDownloading = 5 })
	sized := func(id string, size int64, leechers int) {
		data, _ := brushTorrentBytes(t, "t-"+id, size)
		r.site.files[id] = data
		r.site.items = append(r.site.items, v2.TorrentItem{
			ID: id, Title: "Title " + id, SizeBytes: size, Seeders: 2, Leechers: leechers, DiscountLevel: v2.DiscountFree,
			UploadedAt: r.clock.Now().Add(-10 * time.Minute).Unix(), DiscountEndTime: r.clock.Now().Add(12 * time.Hour),
		})
	}
	sized("big", 15*gib, 90)
	sized("small", 4*gib, 50)

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Added)
	assert.Empty(t, res.Stopped)
	require.Len(t, res.Skipped, 1)
	assert.Contains(t, res.Skipped[0], "Title big：种子体积 15.0 GB 超过当前可用空间")
	require.Len(t, dl.opts, 1, "小种加进了下载器")

	// 有效空间到了保底线：第一个就被拒，这一轮停止加种
	dl.diskInfo.FreeSpace = 5 * gib
	sized("small2", 2*gib, 40)
	r.clock.Advance(10 * time.Minute)
	res, err = r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Contains(t, res.Stopped, "磁盘空间不足")
	assert.Zero(t, res.Added)
	assert.Len(t, dl.opts, 1)
}

func TestBrushSleep(t *testing.T) {
	assert.NotNil(t, NewBrushMonitor(BrushMonitorConfig{}).cfg.Sleep, "默认按真实时间等")
	require.NoError(t, brushSleep(context.Background(), time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, brushSleep(ctx, time.Hour), context.Canceled)
}

func TestBrushStopsSite(t *testing.T) {
	for _, err := range []error{v2.ErrRateLimited, v2.ErrSessionExpired, v2.ErrInvalidCredentials, v2.ErrAuthFailed, v2.ErrCloudflareChallenge, v2.ErrCircuitOpen} {
		assert.True(t, brushStopsSite(fmt.Errorf("x: %w", err)), err)
	}
	for _, err := range []error{errors.New("HTTP 502"), v2.ErrNetworkError, v2.ErrUnexpectedPage, context.DeadlineExceeded} {
		assert.False(t, brushStopsSite(err), err)
	}
}
