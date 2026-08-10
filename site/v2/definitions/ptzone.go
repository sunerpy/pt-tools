package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// PtzoneDefinition 描述 PTzone（ptzone.xyz）站点。
//
// 站点为标准 NexusPHP 架构，但整站使用**繁体中文**界面，所有标签与常见简体站点不同，
// 选择器需要专门适配（与 discfan 高度一致）：
//  1. 用户详情页的传输行标签是「傳送」，既不是简体「传输」也不是繁体「傳輸」；
//     行内为嵌套 table 的合并单元格（分享率/上傳量/下載量/實際上傳量/實際下載量），需以 html + regex 拆分。
//  2. 详情页体积行标签是「基本資訊」（非「基本信息」），且 ParseSizeMB 取的是标签单元格的
//     下一个兄弟节点，因此 SizeSelector 必须指向标签列本身。
//  3. 搜索页副标题是 <br /> 之后的裸文本节点，前面可能还有若干语言/来源标签 <span>
//     （官方/国语/中字 等），无法用普通 CSS 选择器命中，需要走 SubtitleSelector。
//  4. 种子名称单元格里存在一个审核状态图标 <span title="通過">（另有 title="未審"），
//     它同样是 span[title]，因此促销结束时间的选择器必须收紧到 <font color> 之内，
//     否则会把审核状态当成时间去解析。
var PtzoneDefinition = &v2.SiteDefinition{
	ID:             "ptzone",
	Name:           "PTzone",
	Description:    "综合性 PT 站点（繁体中文界面）",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://ptzone.xyz/"},
	FaviconURL:     "https://ptzone.xyz/favicon.ico",
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
				Fields: []string{
					"uploaded", "downloaded", "ratio",
					"trueUploaded", "trueDownloaded",
					"levelName", "joinTime", "lastAccessAt",
				},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			// #info_block 必须优先：用户详情页正文里还有「邀請」等其他 userdetails.php 链接
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
			// 做种/下载数：数字是 arrowup/arrowdown 图标之后的裸文本节点。
			// 正则用 [^>]*> 而非 [^>]*/>，兼容渲染器是否输出自闭合斜杠
			"seeding": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowup"[^>]*>\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			"leeching": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowdown"[^>]*>\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			// 魔力值：<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 850,026.1
			"bonus": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`魔力值\s*</font>\s*\[[^\]]*\]\s*[:：]\s*([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
			// 传输行标签为繁体「傳送」；同时保留「傳輸」/「传输」兜底以适配站点改版。
			// 注意：「邀請」行的链接带 title="傳送邀請"，但 :contains 只匹配文本节点，
			// 该行 td.rowhead 的文本是「邀請」，不会误命中
			"uploaded": {
				Selector: []string{
					"td.rowhead:contains('傳送') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('传输') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>上傳量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"downloaded": {
				Selector: []string{
					"td.rowhead:contains('傳送') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('传输') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>下載量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			// <strong>分享率</strong> 不会误命中 <strong>實際分享率</strong>（前缀不同）；
			// 数值被 <font color="">包裹，需可选跳过该标签
			"ratio": {
				Selector: []string{
					"td.rowhead:contains('傳送') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('传输') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>分享率</strong>[：:\s]*(?:<font[^>]*>)?([\d.,]+|∞|Inf)`}},
					{Name: "parseNumber"},
				},
			},
			"trueUploaded": {
				Selector: []string{
					"td.rowhead:contains('傳送') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('传输') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>實際上傳量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"trueDownloaded": {
				Selector: []string{
					"td.rowhead:contains('傳送') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('传输') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>實際下載量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"levelName": {
				Selector: []string{
					"td.rowhead:contains('等級') + td img",
					"td.rowhead:contains('等级') + td img",
				},
				Attr: "title",
			},
			"joinTime": {
				Selector: []string{
					"td.rowhead:contains('加入日期') + td",
					"td.rowhead:contains('Join') + td",
				},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			// 保号探测只读取 UserInfo.LastAccess，缺失会导致线上探测解析失败。
			// 站点同时存在「最近動向」与「最近動向(地點)」两行（后者本例为空），
			// 取值走 First()，文档顺序上时间行在前，因此不会误取地点行
			"lastAccessAt": {
				Selector: []string{
					"td.rowhead:contains('最近動向') + td",
					"td.rowhead:contains('最近动向') + td",
					"td.rowhead:contains('最後活動') + td",
					"td.rowhead:contains('上次訪問') + td",
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
		// 副标题是 <br /> 之后的裸文本节点，其前可能存在若干标签 <span>（官方/国语/中字 等），
		// 因此正则先跳过连续的完整 <span>...</span> 块，再截取剩余纯文本。
		// 选择器限定在含 details.php 链接的 td.embedded（名称单元格），
		// 避免命中海报单元格或右侧的下载单元格（后者也含 <br />）
		SubtitleSelector: &v2.FieldSelector{
			Selector: []string{
				"table.torrentname td.embedded:has(a[href*='details.php'])",
				"table.torrentname td.embedded",
			},
			Attr: "html",
			Filters: []v2.Filter{
				{Name: "regex", Args: []any{`(?s)<br\s*/?>(?:\s*<span[^>]*>[^<]*</span>)*\s*([^<]+)`}},
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
		// 列序取自 colhead：類型 / 標題 / 評論數 / 存活時間 / 大小 / 種子數 / 下載數 / 完成數 / 發佈者
		Size:     "td.rowfollow:nth-child(5)",
		Seeders:  "td.rowfollow:nth-child(6)",
		Leechers: "td.rowfollow:nth-child(7)",
		Snatched: "td.rowfollow:nth-child(8)",
		// 实际抓取到 pro_free / pro_free2up，其余为 NexusPHP 通用促销图标类名。
		// 不设置 DiscountMapping，交由驱动内置规则判定，
		// 避免自定义 map 迭代顺序导致 pro_free2up 被 "free" 抢先命中
		DiscountIcon: "img.pro_free, img.pro_free2up, img.pro_2up, img.pro_50pctdown, img.pro_50pctdown2up, img.pro_30pctdown",
		// 限时促销在名称单元格里真实渲染为 <font color='#0000FF'>剩餘時間：<span title="...">，
		// 必须限定在 font[color] 之内：同一单元格还有审核状态 <span title="通過">，
		// 第 4 列也有发布时间 span[title]，不收紧会取错值。
		// 永久促销既无该 span 也无 onmouseover，结束时间保持为零值；
		// 若站点某些页面只渲染 tooltip，驱动会回退解析促销图标的 onmouseover 提示
		DiscountEndTime:    "table.torrentname font[color] span[title]",
		Category:           "td.rowfollow:nth-child(1) img[alt]",
		UploadTime:         "td.rowfollow:nth-child(4) span[title]",
		DetailDownloadLink: "td.rowhead:contains('下載') + td a[href*='download.php'], td.rowhead:contains('下载') + td a[href*='download.php']",
		DetailSubtitle:     "td.rowhead:contains('副標題') + td, td.rowhead:contains('副标题') + td",
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
		// 详情页存在隐藏域（字幕上传表单内），直接取 value，无需回退 h1 文本
		TitleSelector: "input[name='torrent_name']",
		IDSelector:    "input[name='detail_torrent_id']",
		// h1#top 内的促销标记形如 <b>[<font class='twoupfree' >2X免費</font>]</b>，
		// 也可能同时出现非促销的 <font class='hot'>，ParseDiscount 会跳过未映射的类名
		DiscountSelector: "h1#top font[class]",
		// 同样必须限定在 font[color] 内：h1#top 末尾还有审核状态 <span title="通過">
		EndTimeSelector: "h1#top font[color] span[title]",
		// ParseSizeMB 取该元素的下一个兄弟节点文本，所以必须指向标签列而非内容列；
		// 站点使用繁体「基本資訊」+ 全角冒号「大小：」，单位组放宽到二进制单位以便站点改版后仍可解析
		SizeSelector: "td.rowhead:contains('基本資訊'), td.rowhead:contains('基本信息')",
		SizeRegex:    `大小[：:]\s*([\d.]+)\s*([KMGTP]i?B)`,
	},
}

func init() {
	v2.RegisterSiteDefinition(PtzoneDefinition)
}
