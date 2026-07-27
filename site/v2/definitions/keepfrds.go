package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// KeepFRDSDefinition 描述 PT@KEEPFRDS 站点。
//
// 该站点为标准 NexusPHP 架构，但有三处与常规站点不同，选择器需要专门适配：
//  1. 详情页没有 input[name='torrent_name'] / input[name='detail_torrent_id'] 隐藏域，
//     标题只存在于 h1#top 文本中，并在尾部附带 [免费] 促销标记（由共享解析器剥离）。
//  2. 体积单位使用二进制形式（GiB / TiB / MiB），默认 SizeRegex 不包含 i 形式，
//     因此必须自定义 SizeRegex。
//  3. 搜索页副标题是 <br /> 之后的裸文本节点，没有任何包裹元素，
//     无法用普通 CSS 选择器命中，需要走 SubtitleSelector（html + regex）。
var KeepFRDSDefinition = &v2.SiteDefinition{
	ID:             "keepfrds",
	Name:           "PT@KEEPFRDS",
	Aka:            []string{"FRDS", "朋友"},
	Description:    "综合性 PT 站点，以影视压制资源为主",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://pt.keepfrds.com/"},
	FaviconURL:     "https://pt.keepfrds.com/static/favicon-64x64.png",
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
				Fields:        []string{"id", "name", "bonus", "bonusPerHour", "seeding", "leeching"},
			},
			{
				RequestConfig: v2.RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
				Assertion:     map[string]string{"id": "params.id"},
				Fields: []string{
					"uploaded", "downloaded", "ratio",
					"trueUploaded", "trueDownloaded",
					"levelName", "joinTime", "lastAccessAt",
				},
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
			// 魔力值：#info_block 中 span#totalBonus 直接给出数值（含千分位）
			"bonus": {
				Selector: []string{"#info_block #totalBonus", "#totalBonus"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			"bonusPerHour": {
				Selector: []string{"#info_block #perBonus", "#perBonus"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			// 做种/下载数：本站用 Font Awesome <i> 图标而非 arrowup/arrowdown 图片，
			// 数字是 <a> 内 <i> 之后的文本节点，取整个 <a> 的文本即可
			"seeding": {
				Selector: []string{"#info_block a[href*='option-torrents=3']"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			"leeching": {
				Selector: []string{"#info_block a[href*='option-torrents=5']"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			// 传输行是合并单元格（内嵌 table），需以 html + regex 拆分
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
			"trueUploaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('傳輸') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>实际上传量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"trueDownloaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('傳輸') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>实际下载量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"levelName": {
				Selector: []string{"td.rowhead:contains('等级') + td img", "td.rowhead:contains('等級') + td img"},
				Attr:     "title",
			},
			"joinTime": {
				Selector: []string{
					"td.rowfollow.join_date",
					"td.rowhead:contains('加入日期') + td",
					"td.rowhead:contains('Join') + td",
				},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			// 保号探测只读取 UserInfo.LastAccess，缺失会导致线上探测解析失败
			"lastAccessAt": {
				Selector: []string{
					"td.rowfollow.last_seen",
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
		},
	},
	Selectors: &v2.SiteSelectors{
		TableRows: "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		Title:     "table.torrentname a[href*='details.php']",
		TitleLink: "table.torrentname a[href*='details.php']",
		// 副标题是 <br /> 之后的裸文本节点（部分行被 <span style="color: Red;"> 包裹），
		// 结尾紧跟 <b>[...标签...]</b>，因此以 html + regex 截取
		SubtitleSelector: &v2.FieldSelector{
			Selector: []string{"td.browse_td_name_cell", "table.torrentname td.embedded"},
			Attr:     "html",
			Filters: []v2.Filter{
				{Name: "regex", Args: []any{`(?s)<br\s*/?>(?:\s*<span[^>]*>)?\s*([^<]+)`}},
				// 走 html 取值会保留实体编码，需手动还原；&amp; 必须最后替换以避免二次解码
				{Name: "replace", Args: []any{"&#39;", "'"}},
				{Name: "replace", Args: []any{"&quot;", `"`}},
				{Name: "replace", Args: []any{"&lt;", "<"}},
				{Name: "replace", Args: []any{"&gt;", ">"}},
				{Name: "replace", Args: []any{"&nbsp;", " "}},
				{Name: "replace", Args: []any{"&amp;", "&"}},
				{Name: "trim"},
			},
		},
		Size:     "td.rowfollow:nth-child(5)",
		Seeders:  "td.rowfollow:nth-child(6)",
		Leechers: "td.rowfollow:nth-child(7)",
		Snatched: "td.rowfollow:nth-child(8)",
		// 实际抓取到的仅有 pro_free / pro_50pctdown，其余为 NexusPHP 通用促销图标类名
		DiscountIcon: "img.pro_free, img.pro_free2up, img.pro_2up, img.pro_50pctdown, img.pro_50pctdown2up, img.pro_30pctdown",
		// 促销结束时间没有独立元素，仅存在于促销图标的 onmouseover 提示中，
		// 交由驱动的 parseDiscountEndTimeFromOnmouseover 兜底解析
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
		// 详情页无隐藏域：标题取 h1#top 文本（共享解析器会剥离尾部 [免费] 促销标记），
		// 种子 ID 取快速评论表单的 input[name='pid']
		TitleSelector:    "h1#top",
		IDSelector:       "input[name='pid']",
		DiscountSelector: "h1#top font[class]",
		// 促销剩余时间限定在 h1#top 内查找，避免误取发布时间；本站详情页通常不渲染该 span，
		// 此时结束时间为空（未知），促销结束时间以搜索页 onmouseover 为准
		EndTimeSelector: "h1#top span[title]",
		SizeSelector:    "td.rowhead:contains('基本信息')",
		// 体积为二进制单位（8.47 GiB），默认正则不含 i 形式，必须显式覆盖
		SizeRegex: `大小[：:]\s*([\d.]+)\s*([KMGTP]i?B)`,
	},
}

func init() {
	v2.RegisterSiteDefinition(KeepFRDSDefinition)
}
