package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

var CrabPTDefinition = &v2.SiteDefinition{
	ID:             "crabpt",
	Name:           "CrabPT",
	Aka:            []string{"蟹黄堡", "蟹堡", "蟹PT"},
	Description:    "蟹黄堡 — 综合性 NexusPHP PT 站点（crabpt.vip）",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://crabpt.vip/"},
	FaviconURL:     "https://crabpt.vip/favicon.ico",
	TimezoneOffset: "+0800",
	RateLimit:      0.5,
	RateBurst:      2,
	UserInfo: &v2.UserInfoConfig{
		PickLast:     []string{"id"},
		RequestDelay: 500,
		Process: []v2.UserInfoProcess{
			{
				RequestConfig: v2.RequestConfig{URL: "/index.php", ResponseType: "document"},
				Fields:        []string{"id", "name", "bonus", "seeding", "leeching"},
			},
			{
				RequestConfig: v2.RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
				Assertion:     map[string]string{"id": "params.id"},
				Fields:        []string{"name", "uploaded", "downloaded", "ratio", "levelName", "joinTime", "lastAccessAt"},
			},
			{
				RequestConfig: v2.RequestConfig{URL: "/mybonus.php", ResponseType: "document"},
				Fields:        []string{"bonusPerHour"},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			"id": {
				Selector: []string{"a[href*='userdetails.php'][class*='Name']", "#info_block a[href*='userdetails.php']", "a[href*='userdetails.php']"},
				Attr:     "href",
				Filters:  []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{"a[href*='userdetails.php'][class*='Name']", "#info_block a[href*='userdetails.php']"},
			},
			"uploaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('上传量') + td", "#info_block"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`(?:上传量|上傳量|Uploaded)[^\d]*([\d.,]+\s*[KMGTP]?i?B)`}}, {Name: "parseSize"}},
			},
			"downloaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('下载量') + td", "#info_block"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`(?:下载量|下載量|Downloaded)[^\d]*([\d.,]+\s*[KMGTP]?i?B)`}}, {Name: "parseSize"}},
			},
			"ratio": {
				Selector: []string{"#info_block", "body"},
				Attr:     "html",
				Filters: []v2.Filter{
					// 标签与数值之间只跳过标签、空白和冒号：分享率不到 1 时数值包在 <font color="#880000"> 里，
					// 不能停在颜色值上；也不会越过「无限」抓到相邻的上传量。
					// 无穷：简体「无限」、繁体「無限」、英文「Inf.」（上游 lang_functions.php 的 text_inf）
					{Name: "regex", Args: []any{`(?:分享率|Ratio)(?:<[^>]*>|[\s：:])*([\d.,]+|无限|無限|∞|Inf|---)`}},
					// 无穷分享率折算成远大于 1 的有限值，避免分享率门槛判断把它当成不达标。
					// 这里不接 parseNumber：ratio 最终由 parseRatio 解析，而 parseNumber 会把该值
					// 转成 float64 并以科学计数法输出（9.999999e+06）。
					{Name: "replace", Args: []any{"无限", "9999999"}},
					{Name: "replace", Args: []any{"無限", "9999999"}},
					{Name: "replace", Args: []any{"∞", "9999999"}},
					{Name: "replace", Args: []any{"Inf", "9999999"}},
				},
			},
			"levelName": {
				Selector: []string{"td.rowhead:contains('等级') + td > img", "td.rowhead:contains('等级') + td img", "td.rowhead:contains('Class') + td > img"},
				Attr:     "title",
			},
			"bonus": {
				Selector: []string{"#info_block", "td.rowhead:contains('蟹币值') + td", "td.rowhead:contains('蟹幣值') + td", "td.rowhead:contains('魔力值') + td", "td.rowhead:contains('Bonus') + td"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`(?:蟹币值|蟹幣值|魔力值|Bonus)[^\d]*([\d.,]+)`}}, {Name: "parseNumber"}},
			},
			"joinTime": {
				Selector: []string{"td.rowhead:contains('加入日期') + td", "td.rowhead:contains('Join') + td"},
				Filters:  []v2.Filter{{Name: "split", Args: []any{" (", 0}}, {Name: "parseTime"}},
			},
			"lastAccessAt": {
				Selector: []string{
					"td.rowhead:contains('最近动向') + td",
					"td.rowhead:contains('最近動向') + td",
					"td.rowhead:contains('最后活动') + td",
					"td.rowhead:contains('最後活動') + td",
					"td.rowhead:contains('上次访问') + td",
					"td.rowhead:contains('上次訪問') + td",
					"td.rowhead:contains('Last access') + td",
				},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			"seeding": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`class="arrowup"[^>]*/>\s*(\d+)`}}, {Name: "parseNumber"}},
			},
			"leeching": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`class="arrowdown"[^>]*/>\s*(\d+)`}}, {Name: "parseNumber"}},
			},
			"bonusPerHour": {
				Selector: []string{"#outer", "body"},
				Attr:     "html",
				Filters: []v2.Filter{
					// 蟹币值页「每小时获得的合计蟹币值」表格末列，含勋章/官种/后宫加成的合计值
					// 表头简体「合计」、繁体「合計」、英文「Total」（上游 resources/lang/*/bonus.php）
					{Name: "regex", Args: []any{`(?:合计|合計|Total)</td>.*?<td rowspan="\d+">([\d.,]+)</td>`}},
					{Name: "parseNumber"},
				},
			},
		},
	},
	Selectors: &v2.SiteSelectors{
		TableRows:          "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		Title:              "table.torrentname a[href*='details.php']",
		TitleLink:          "table.torrentname a[href*='details.php']",
		Subtitle:           "table.torrentname td.embedded > span:last-of-type, table.torrentname td.embedded > span:not(.optiontag)",
		Size:               "td.rowfollow:nth-child(5)",
		Seeders:            "td.rowfollow:nth-child(6)",
		Leechers:           "td.rowfollow:nth-child(7)",
		Snatched:           "td.rowfollow:nth-child(8)",
		DiscountIcon:       "img.pro_free, img.pro_free2up, img.pro_50pctdown, img.pro_30pctdown, img.pro_2up, img.pro_50pctdown2up",
		Category:           "td.rowfollow:nth-child(1) img[alt]",
		UploadTime:         "td.rowfollow:nth-child(4) span[title]",
		DetailDownloadLink: "td.rowhead:contains('下载') + td a[href*='download.php']",
		DetailSubtitle:     "td.rowhead:contains('副标题') + td",
	},
	DetailParser: &v2.DetailParserConfig{
		TimeLayout: "2006-01-02 15:04:05",
		DiscountMapping: map[string]v2.DiscountLevel{
			"free":          v2.DiscountFree,
			"twoup":         v2.Discount2xUp,
			"twoupfree":     v2.Discount2xFree,
			"thirtypercent": v2.DiscountPercent30,
			"halfdown":      v2.DiscountPercent50,
			"twouphalfdown": v2.Discount2x50,
		},
		HRKeywords:       []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run", "H&R"},
		TitleSelector:    "h1",
		DiscountSelector: "h1 font.free, h1 font[class]",
		EndTimeSelector:  "h1 span[title]",
		SizeSelector:     "td.rowfollow:contains('大小')",
		SizeRegex:        `大小[：:]\s*([\d.]+)\s*(GB|MB|KB|TB)`,
	},
}

func init() {
	v2.RegisterSiteDefinition(CrabPTDefinition)
}
