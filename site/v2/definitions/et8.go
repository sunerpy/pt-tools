package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// Et8Definition 站点 et8.org（页面 <title> 自称 TCCF，种子标题常见 [TorrentCCF首发] 前缀）。
//
// 与标准 NexusPHP 的差异：
//   - 种子列表多出一列「进度」，位于「完成数」之后、「发布者」之前（共 10 列）；
//     由于该列排在 size/seeders/leechers/snatched 之后，nth-child(5~8) 不受影响。
//   - 副标题是 <br /> 之后的裸文本节点，无标签包裹，纯 CSS 选不中，
//     故改用 SubtitleSelector 取 td.embedded 的 innerHTML 后正则截取。
//   - 优惠剩余时间同时存在于优惠图标的 onmouseover tooltip 与单元格可见文本中，
//     这里不设置 DiscountEndTime，交由驱动的 onmouseover 兜底解析，
//     避免 CSS 选择器误命中同行的发布时间 span。
var Et8Definition = &v2.SiteDefinition{
	ID:             "et8",
	Name:           "TCCF",
	Aka:            []string{"TorrentCCF"},
	Description:    "综合性 PT 站点，影视 / 纪录片 / 电子学习资源较多",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://et8.org/"},
	FaviconURL:     "https://et8.org/favicon.ico",
	AuthMethod:     v2.AuthMethodCookie,
	TimezoneOffset: "+0800",
	RateLimit:      0.5,
	RateBurst:      2,
	UserInfo: &v2.UserInfoConfig{
		PickLast:     []string{"id"},
		RequestDelay: 500,
		Process: []v2.UserInfoProcess{
			{
				RequestConfig: v2.RequestConfig{URL: "/index.php", ResponseType: "document"},
				Fields:        []string{"id", "name", "seeding", "leeching", "bonus"},
			},
			{
				RequestConfig: v2.RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
				Assertion:     map[string]string{"id": "params.id"},
				Fields:        []string{"uploaded", "downloaded", "ratio", "levelName", "joinTime", "lastAccessAt"},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			"id": {
				Selector: []string{
					"#info_block a[href*='userdetails.php']",
					"a[href*='userdetails.php'][class*='Name']",
					"a[href*='userdetails.php']",
				},
				Attr:    "href",
				Filters: []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{
					"#info_block a[href*='userdetails.php']",
					"a[href*='userdetails.php'][class*='Name']",
				},
			},
			// 「传输」行为嵌套小表格：<strong>分享率</strong>/<strong>上传量</strong>/<strong>下载量</strong>
			"uploaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('傳輸') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>上传量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"downloaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('傳輸') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>下载量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"ratio": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('傳輸') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>分享率</strong>[：:\s]*(?:<font[^>]*>)?([\d.,]+|∞|Inf)`}},
					{Name: "parseNumber"},
				},
			},
			"levelName": {
				Selector: []string{"td.rowhead:contains('等级') + td img", "td.rowhead:contains('等級') + td img"},
				Attr:     "title",
			},
			// info_block 中形如：<font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 1,219,747.6
			"bonus": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`魔力值\s*</font>\s*\[[^\]]*\]\s*[:：]\s*([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
			"joinTime": {
				Selector: []string{"td.rowhead:contains('加入日期') + td", "td.rowhead:contains('Join') + td"},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			// 保号探测依赖该字段；et8 的「最近动向」在「最近动向(地点)」之前，First() 取到的是真实时间行
			"lastAccessAt": {
				Selector: []string{
					"td.rowhead:contains('最近动向') + td",
					"td.rowhead:contains('最近動向') + td",
					"td.rowhead:contains('上次访问') + td",
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
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowup"[^>]*/>\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			"leeching": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowdown"[^>]*/>\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
		},
	},
	Selectors: &v2.SiteSelectors{
		TableRows: "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		Title:     "table.torrentname a[href*='details.php']",
		TitleLink: "table.torrentname a[href*='details.php']",
		// 副标题为 <br /> 之后的裸文本节点，需要走 innerHTML + 正则
		SubtitleSelector: &v2.FieldSelector{
			Selector: []string{"table.torrentname td.embedded"},
			Attr:     "html",
			Filters: []v2.Filter{
				{Name: "regex", Args: []any{`<br\s*/?>\s*([^<]+)`}},
				{Name: "trim"},
			},
		},
		// 「进度」列排在第 9 位，不影响以下 5~8 列
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
		HRKeywords: []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"},
		// 详情页存在隐藏 input，标题/ID 直接读 value
		TitleSelector:    "input[name='torrent_name']",
		IDSelector:       "input[name='detail_torrent_id']",
		DiscountSelector: "h1 font[class]",
		EndTimeSelector:  "h1 span[title]",
		// ParseSizeMB 取该元素的 Next() 兄弟节点文本，因此必须指向标签单元格本身
		SizeSelector: "td.rowhead:contains('基本信息')",
		SizeRegex:    `大小[：:][^\d]*([\d.]+)\s*(GB|MB|KB|TB)`,
	},
}

func init() {
	v2.RegisterSiteDefinition(Et8Definition)
}
