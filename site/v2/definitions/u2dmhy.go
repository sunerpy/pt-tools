package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// U2DMHYDefinition 描述 U2分享園@動漫花園（u2.dmhy.org）站点。
//
// 站点名称取自页面 <title>「U2分享園@動漫花園 :: 种子」，保留繁体字「園」。
// 搜索页是标准 NexusPHP <table class="torrents"> 布局，但详情页与用户页有四处需要专门适配：
//  1. 详情页没有 input[name='torrent_name'] 隐藏域（只有 detail_torrent_id），
//     标题只存在于 h1#top 文本中，须走共享解析器的文本兜底路径。
//  2. 「基本信息」行的第一个字段是「发布时间:」而非「大小:」，且冒号为半角、
//     体积单位为二进制形式（GiB / TiB），因此 SizeRegex 必须同时放宽冒号与单位。
//  3. 促销状态在详情页由 img.pro_* 图标表示（而非 <font class="free">），
//     搜索页的促销剩余时间使用 <time title="..."> 而非 <span title="...">。
//  4. 用户页标签为简体（加入日期 / 最近动向 / 传输 / 等级），
//     站点货币是 UCoin 而非魔力值，「原积分」行只有历史链接、没有数值。
var U2DMHYDefinition = &v2.SiteDefinition{
	ID:             "u2dmhy",
	Name:           "U2分享園@動漫花園",
	Aka:            []string{"U2", "動漫花園"},
	Description:    "以动漫资源为主的 PT 站点，使用 UCoin 经济体系",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://u2.dmhy.org/"},
	FaviconURL:     "https://u2.dmhy.org/favicon.ico",
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
				Fields:        []string{"id", "name", "seeding", "leeching"},
			},
			{
				RequestConfig: v2.RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
				Assertion:     map[string]string{"id": "params.id"},
				Fields: []string{
					"uploaded", "downloaded", "ratio",
					"trueUploaded", "trueDownloaded",
					"bonus", "levelName", "joinTime", "lastAccessAt",
				},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			// #info_block 中用户名链接的 class 形如 NexusMaster_Name（等级名 + _Name），
			// 优先用它命中，避免误取同一区块内的 clientlist / ullist 链接
			"id": {
				Selector: []string{
					"#info_block a[href*='userdetails.php'][class*='_Name']",
					"#info_block a[href*='userdetails.php']",
					"a[href*='userdetails.php']",
				},
				Attr:    "href",
				Filters: []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{
					"#info_block a[href*='userdetails.php'][class*='_Name']",
					"#info_block a[href*='userdetails.php']",
				},
			},
			// 做种数是 img.arrowup 之后的 <a href="...&ullist=1#seedlist">数字</a>
			"seeding": {
				Selector: []string{"#info_block a[href*='ullist=1']"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			// 下载数是 img.arrowdown 之后的裸文本节点，无法用 CSS 直接命中，
			// 只能对 #info_block 取 html 后用正则截取
			"leeching": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowdown"[^>]*>\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			// 「传输」行是内嵌 table 的合并单元格，标签为 <strong>xxx</strong> + 半角冒号 + 两个空格；
			// 实际上传/下载的标签是「实际上传」「实际下载」（没有「量」字）
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
					{Name: "regex", Args: []any{`<strong>实际上传</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"trueDownloaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('傳輸') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>实际下载</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			// 本站货币是 UCoin：「原积分」行只有历史链接没有数值，
			// UCoin 行在图形化符号之后用括号给出精确值 (9,999,999.999)
			"bonus": {
				Selector: []string{"td.rowhead:contains('UCoin') + td"},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`\(([\d.,]+)\)`}},
					{Name: "parseNumber"},
				},
			},
			"levelName": {
				Selector: []string{"td.rowhead:contains('等级') + td img", "td.rowhead:contains('等級') + td img"},
				Attr:     "title",
			},
			// 时间行形如 <time>2018-01-24 20:00:03</time> (<time title="...">8年6月前</time>/443 周 4 天 前)
			"joinTime": {
				Selector: []string{
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
					"td.rowhead:contains('最近动向') + td",
					"td.rowhead:contains('最近動向') + td",
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
		// 列顺序（由 colhead 行确认）：1 分类 / 2 名称 / 3 评论数 / 4 生存时间 / 5 大小 / 6 种子数 / 7 下载数 / 8 完成数
		Title:     "table.torrentname a.tooltip[href*='details.php']",
		TitleLink: "table.torrentname a.tooltip[href*='details.php']",
		// 副标题在名称子表第二行的 <span class="tooltip">（标题链接同样带 tooltip，故限定 span）
		Subtitle: "table.torrentname span.tooltip",
		// 大小列写作 "9.328<br />GiB"，parseSize 去空白后按二进制单位换算
		Size:     "td.rowfollow:nth-child(5)",
		Seeders:  "td.rowfollow:nth-child(6)",
		Leechers: "td.rowfollow:nth-child(7)",
		Snatched: "td.rowfollow:nth-child(8)",
		// 实际抓取到的促销图标只有 pro_free2up / pro_2up / pro_50pctdown，
		// 其余为 NexusPHP 通用类名，一并保留以便站点后续启用
		DiscountIcon: "img.pro_free, img.pro_free2up, img.pro_2up, img.pro_50pctdown, img.pro_30pctdown",
		// 故意不配置 DiscountMapping：搜索页解析走 parseDiscountFromElement，它用
		// strings.Contains 遍历 map，而 "pro_free" 是 "pro_free2up" 的前缀，
		// 显式映射会因 map 遍历顺序随机而把 2X Free 误判为 Free。
		// 驱动内置的兜底判定按 free2up → free → 50pct → 2up 顺序短路，结果确定。
		// 促销剩余时间在名称子表内的 [剩余 <time title="...">]，用 <time> 而非 <span>；
		// 限定在 table.torrentname 内，避免误取第 4 列的生存时间
		DiscountEndTime: "table.torrentname time[title]",
		// 分类单元格是纯文本链接 <a href="?cat=40">Other</a>，没有 img[alt]；
		// 驱动只读取 alt 属性，因此本站分类字段会保持为空
		Category:           "td.rowfollow:nth-child(1) a[href*='cat=']",
		UploadTime:         "td.rowfollow:nth-child(4) time[title]",
		DetailDownloadLink: "td.rowhead:contains('下载') + td a[href*='download.php']",
		DetailSubtitle:     "td.rowhead:contains('副标题') + td",
	},
	DetailParser: &v2.DetailParserConfig{
		TimeLayout: "2006-01-02 15:04:05",
		// 详情页「流量优惠」行用 img.pro_* 表示促销，映射键即图标类名
		DiscountMapping: map[string]v2.DiscountLevel{
			"pro_free2up":   v2.Discount2xFree,
			"pro_free":      v2.DiscountFree,
			"pro_2up":       v2.Discount2xUp,
			"pro_50pctdown": v2.DiscountPercent50,
			"pro_30pctdown": v2.DiscountPercent30,
		},
		HRKeywords: []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"},
		// 详情页无 torrent_name 隐藏域：标题取 h1#top 文本（共享解析器的文本兜底路径），
		// 种子 ID 仍可从字幕上传表单的 input[name='detail_torrent_id'] 取得
		TitleSelector:    "h1#top",
		IDSelector:       "input[name='detail_torrent_id']",
		DiscountSelector: "td.rowhead:contains('流量优惠') + td img[class*='pro_'], td.rowhead:contains('流量優惠') + td img[class*='pro_']",
		// 促销剩余时间同样限定在「流量优惠」行内，避免误取「基本信息」行的发布时间 <time>。
		// 采集样本是永久 2X，该行未渲染剩余时间，故此选择器尚未在真实数据上验证过；
		// 形态参照搜索页已确认的 [剩余 <time title="...">] 标记
		EndTimeSelector: "td.rowhead:contains('流量优惠') + td time[title], td.rowhead:contains('流量優惠') + td time[title]",
		SizeSelector:    "td.rowhead:contains('基本信息')",
		// 「基本信息」行首字段是「发布时间:」，须跳到后面的「大小:」；
		// 冒号为半角、单位为二进制形式（9.328 GiB），默认正则两处都不匹配
		SizeRegex: `大小[：:][^\d]*([\d.]+)\s*([KMGTP]i?B)`,
	},
}

func init() {
	v2.RegisterSiteDefinition(U2DMHYDefinition)
}
