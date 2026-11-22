package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// brushListDef 是写了刷流读列表能力的 NexusPHP 站点定义（与 HDSky 的写法一样）。
func brushListDef() *SiteDefinition {
	return &SiteDefinition{
		ID: "brushsite", Name: "BrushSite", Schema: SchemaNexusPHP, URLs: []string{"https://brush.example/"},
		BrushList: &BrushListConfig{
			DiscountFilters: map[DiscountLevel]string{DiscountFree: "2", Discount2xFree: "4"},
			Paging:          true,
			MinInterval:     61 * time.Second,
		},
	}
}

func TestBrushListConfig_DiscountFilter(t *testing.T) {
	c := brushListDef().BrushList
	v, ok := c.DiscountFilter(DiscountFree)
	assert.True(t, ok)
	assert.Equal(t, "2", v)
	v, ok = c.DiscountFilter(Discount2xFree)
	assert.True(t, ok)
	assert.Equal(t, "4", v)
	_, ok = c.DiscountFilter(DiscountPercent50)
	assert.False(t, ok, "没写的优惠类型")
	_, ok = c.DiscountFilter("")
	assert.False(t, ok)

	var none *BrushListConfig
	_, ok = none.DiscountFilter(DiscountFree)
	assert.False(t, ok, "没写 BrushList 的站点")
	_, ok = (&BrushListConfig{DiscountFilters: map[DiscountLevel]string{DiscountFree: ""}}).DiscountFilter(DiscountFree)
	assert.False(t, ok, "空值不算")
}

// 刷流按站点实例拿读列表能力：带站点定义的 BaseSite 给出定义里的 BrushList，别的站点没有。
func TestBrushListOf(t *testing.T) {
	def := brushListDef()
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://brush.example", Cookie: "c=1"})
	d.SetSiteDefinition(def)
	site := NewBaseSite(d, BaseSiteConfig{ID: "brushsite", Name: "BrushSite", Kind: SiteNexusPHP})
	assert.Same(t, def.BrushList, BrushListOf(site))

	plain := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://brush.example", Cookie: "c=1"})
	assert.Nil(t, BrushListOf(NewBaseSite(plain, BaseSiteConfig{ID: "x", Kind: SiteNexusPHP})), "驱动没有站点定义")

	plain.SetSiteDefinition(&SiteDefinition{ID: "x"})
	assert.Nil(t, BrushListOf(NewBaseSite(plain, BaseSiteConfig{ID: "x", Kind: SiteNexusPHP})), "定义里没写")

	assert.Nil(t, BrushListOf(NewBaseSite[string, string](&MockDriver{}, BaseSiteConfig{ID: "m"})), "驱动不带站点定义")
	assert.Nil(t, BrushListOf(&MockSite{}), "不是 BaseSite 的站点")
}

func TestSiteDefinitionValidate_BrushList(t *testing.T) {
	valid := func() *SiteDefinition {
		def := brushListDef()
		def.Selectors = &SiteSelectors{TableRows: "tr", Title: "a", TitleLink: "a"}
		def.UserInfo = &UserInfoConfig{
			Process:   []UserInfoProcess{{RequestConfig: RequestConfig{URL: "/index.php"}, Fields: []string{"name"}}},
			Selectors: map[string]FieldSelector{"name": {Selector: []string{"a"}}},
		}
		return def
	}
	require.NoError(t, valid().Validate())

	paging := valid()
	paging.BrushList = &BrushListConfig{Paging: true}
	require.NoError(t, paging.Validate(), "只写能翻页")

	cases := map[string]struct {
		mod   func(*SiteDefinition)
		field string
		rule  string
	}{
		"别的架构不认优惠筛选": {func(d *SiteDefinition) { d.Schema = SchemaUnit3D; d.Selectors, d.UserInfo = nil, nil }, "BrushList.DiscountFilters", "SchemaUnsupported"},
		"不认识的优惠类型":   {func(d *SiteDefinition) { d.BrushList.DiscountFilters["HALF"] = "3" }, "BrushList.DiscountFilters", "InvalidValue"},
		"无优惠不能当筛选":   {func(d *SiteDefinition) { d.BrushList.DiscountFilters[DiscountNone] = "1" }, "BrushList.DiscountFilters", "InvalidValue"},
		"筛选值为空":      {func(d *SiteDefinition) { d.BrushList.DiscountFilters[DiscountPercent50] = " " }, `BrushList.DiscountFilters["PERCENT_50"]`, "Required"},
		"间隔为负":       {func(d *SiteDefinition) { d.BrushList.MinInterval = -time.Second }, "BrushList.MinInterval", "InvalidValue"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			def := valid()
			c.mod(def)
			err := def.Validate()
			require.Error(t, err)
			var errs ValidationErrors
			require.ErrorAs(t, err, &errs)
			found := false
			for _, e := range errs {
				if e.Field == c.field && e.Rule == c.rule {
					found = true
				}
			}
			assert.True(t, found, "%v", err)
		})
	}
}

// 刷流按优惠类型读的列表：站点定义里有这一类的筛选值时设 spstate（优先于 FreeOnly），没有时不加；普通搜索照旧按 FreeOnly。
func TestNexusPHPPrepareSearch_BrushDiscount(t *testing.T) {
	withDef := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://brush.example", Cookie: "c=1"})
	withDef.SetSiteDefinition(brushListDef())
	noDef := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://brush.example", Cookie: "c=1"})

	cases := []struct {
		name  string
		d     *NexusPHPDriver
		query SearchQuery
		want  url.Values
	}{
		{"免费列表", withDef, SearchQuery{BrushDiscount: DiscountFree}, url.Values{"spstate": {"2"}}},
		{"2x 免费列表", withDef, SearchQuery{BrushDiscount: Discount2xFree}, url.Values{"spstate": {"4"}}},
		{"筛选值优先于 FreeOnly", withDef, SearchQuery{BrushDiscount: Discount2xFree, FreeOnly: true}, url.Values{"spstate": {"4"}}},
		{"定义里没有这一类：不加筛选", withDef, SearchQuery{BrushDiscount: DiscountPercent50}, url.Values{}},
		{"没有这一类时 FreeOnly 照旧", withDef, SearchQuery{BrushDiscount: DiscountPercent50, FreeOnly: true}, url.Values{"spstate": {"2"}}},
		{"没有站点定义：不加筛选", noDef, SearchQuery{BrushDiscount: Discount2xFree}, url.Values{}},
		{"普通搜索的 FreeOnly 不变", withDef, SearchQuery{Keyword: "x", FreeOnly: true}, url.Values{"search": {"x"}, "spstate": {"2"}}},
		{"第 2 页请求 page=1", withDef, SearchQuery{BrushDiscount: DiscountFree, Page: 2}, url.Values{"spstate": {"2"}, "page": {"1"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, err := c.d.PrepareSearch(c.query)
			require.NoError(t, err)
			assert.Equal(t, c.want, req.Params)
		})
	}
}

// 真实请求：刷流按优惠类型读的列表经 BaseSite 发出去，带上站点定义里的 spstate 与从 0 起的 page。
func TestNexusPHPSearch_BrushDiscountRequest(t *testing.T) {
	var got []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.URL.Query())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(listWith2faInScripts))
	}))
	t.Cleanup(srv.Close)
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: srv.URL, Cookie: "c=1"})
	d.SetSiteDefinition(brushListDef())
	site := NewBaseSite(d, BaseSiteConfig{ID: "brushsite", Kind: SiteNexusPHP, RateLimit: 100, RateBurst: 100})

	items, err := site.Search(context.Background(), SearchQuery{BrushDiscount: Discount2xFree, Page: 2})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Len(t, got, 1)
	assert.Equal(t, url.Values{"spstate": {"4"}, "page": {"1"}}, got[0])
}
