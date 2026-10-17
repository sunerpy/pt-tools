package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

var GameGamePTDefinition = &v2.SiteDefinition{
	ID:             "gamegamept",
	Name:           "GameGamePT",
	Aka:            []string{"GGPT", "游戏PT"},
	Description:    "GGPT — 以游戏资源为主的 NexusPHP PT 站点（gamegamept.com）",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://www.gamegamept.com/"},
	FaviconURL:     "https://www.gamegamept.com/favicon.ico",
	TimezoneOffset: "+0800",
	RateLimit:      0.5,
	RateBurst:      2,
	// 整站 H&R（#727）。站点规则里的做种时长没写进来：用户在站点设置里填「H&R 做种要求」
	HREnabled: true,
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
				Fields: []string{
					"name", "uploaded", "downloaded", "ratio", "levelName", "joinTime",
					"lastAccessAt", "trueUploaded", "trueDownloaded",
				},
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
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('分享率') + td", "#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					// 标签与数值之间只跳过标签、空白和冒号：分享率不到 1 时数值包在 <font color="#880000"> 里，
					// 原来的 [^\d∞]* 会停在颜色值上，把 880000 当成分享率
					{Name: "regex", Args: []any{`(?:分享率|Ratio)(?:<[^>]*>|[\s\p{Zs}：:])*([\d.,]+|无限|無限|∞|Inf|---)`}},
					// 无穷分享率（无限 / 無限 / Inf. / ∞）和 CrabPT 一样折算成 9999999；原来 parseNumber 把它变成 0，
					// 分享率门槛判断当成不达标。parseNumber 输出 9.999999e+06，parseRatio 照样解析成 9999999
					{Name: "replace", Args: []any{"无限", "9999999"}},
					{Name: "replace", Args: []any{"無限", "9999999"}},
					{Name: "replace", Args: []any{"∞", "9999999"}},
					{Name: "replace", Args: []any{"Inf", "9999999"}},
					{Name: "parseNumber"},
				},
			},
			"levelName": {
				Selector: []string{"td.rowhead:contains('等级') + td > img", "td.rowhead:contains('等级') + td img", "td.rowhead:contains('Class') + td > img"},
				Attr:     "title",
			},
			"bonus": {
				// index.php 的币种是「G值」（userdetails.php 同位置写的是「魔力值」）
				Selector: []string{"#info_block", "td.rowhead:contains('G值') + td", "td.rowhead:contains('魔力值') + td", "td.rowhead:contains('Bonus') + td"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`(?:G值|魔力值|Bonus)[^\d]*([\d.,]+)`}}, {Name: "parseNumber"}},
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
			"trueUploaded": {
				Selector: []string{
					"td.rowhead:contains('传输') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('傳送') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:实际上传量|實際上傳量|實際上傳|实际上传)</strong>[\s:：]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"trueDownloaded": {
				Selector: []string{
					"td.rowhead:contains('传输') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('傳送') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:实际下载量|實際下載量|實際下載|实际下载)</strong>[\s:：]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"bonusPerHour": {
				Selector: []string{
					"#outer",
					"body",
				},
				Attr: "html",
				Filters: []v2.Filter{
					// 时魔取「每小时获得的合计魔力值」表格末列 = 基本奖励 + 勋章加成 + 官种加成 + 后宫加成
					// （例：299.172 + 274.172 + 7.994 + 0 = 581.337）。
					// 页面上方的「你当前每小时能获取X个魔力值」只给基本奖励，且位于该表格之前，
					// 所以不能写成「合计…|你当前…」的交替（正则会取最左匹配，命中上方那句）。
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
	v2.RegisterSiteDefinition(GameGamePTDefinition)
}
