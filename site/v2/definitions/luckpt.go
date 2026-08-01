package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// LuckptDefinition 描述 LuckPT（pt.luckpt.de）站点。
//
// 站点为标准 NexusPHP 架构（页面标题 "LuckPT :: ... - Powered by NexusPHP"），
// 详情页保留了 input[name='torrent_name'] / input[name='detail_torrent_id'] 隐藏域，
// 因此标题/ID 直接走隐藏域取值。需要单独适配的点：
//  1. 搜索页名称单元格前有封面缩略图单元格（data-has-cover=1，20/20 行都有），
//     td.embedded 的第一个是封面而不是名称，必须用 :has(a[href*='details.php']) 定位名称单元格。
//  2. 副标题是 <br /> 之后的裸文本节点，且前面还有 1~3 个彩色标签 <span>（官方/中字/完结…），
//     eastgame 只跳过一个 span 的正则无法命中，需要跳过任意个完整 span。
//  3. 名称单元格内除促销的「剩余时间」<span title> 外，还有一个审核状态 <span title="通过">，
//     促销结束时间选择器必须限定在 font:contains('剩余时间') 内，否则非促销行会误取 title="通过"。
//  4. 「基本信息」行体积同时出现十进制（46.11 GB）与二进制（GiB）单位，SizeRegex 的单位组放宽为
//     [KMGT]i?B；共享的 ParseSizeMB 已能换算 KiB/MiB/GiB/TiB。
//  5. userdetails.php 的「传输」行为合并单元格，且魔力值在本站叫「幸运星」，
//     标签后面跟了两个 [链接]（详情/商城）才是冒号，需要专用正则。
var LuckptDefinition = &v2.SiteDefinition{
	ID:             "luckpt",
	Name:           "LuckPT",
	Description:    "综合性 PT 站点（NexusPHP）",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://pt.luckpt.de/"},
	FaviconURL:     "https://pt.luckpt.de/favicon.ico",
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
					"uploaded", "downloaded", "ratio", "trueUploaded",
					"levelName", "joinTime", "lastAccessAt",
				},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			// #info_block 里的用户链接是绝对地址（https://pt.luckpt.de/userdetails.php?id=…），
			// querystring 过滤器基于 url.Parse，绝对/相对地址都能取到 id。
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
			// 「传输」行是合并单元格（内嵌 table），需以 html + regex 拆分。
			// 行内同时存在 实际上传量 / 保种上传量，字面量 <strong>上传量</strong> 只会命中纯上传量那一项。
			"uploaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('傳輸') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>上传量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			// 实际上传量（不参与分享率计算），仅作记录展示
			"trueUploaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('傳輸') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>实际上传量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
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
			// 分享率后面括号里还有「实际分享率」，字面量 <strong>分享率</strong> 只命中前者
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
			// 本站魔力值叫「幸运星」，格式：
			// <font class = 'color_bonus'>幸运星 </font>[<a>详情</a>][<a>商城</a>]: 86,666.1
			// 标签后可能跟多个 [链接]，需用 (?:\[...\]\s*)+ 吞掉后再取冒号后的数值。
			"bonus": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:幸运星|魔力值)\s*</font>\s*(?:\[[^\]]*\]\s*)+[:：]\s*([\d.,]+)`}},
					{Name: "parseNumber"},
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
			// 保号探测只读取 UserInfo.LastAccess，缺失会导致线上探测解析失败。
			// 「最近动向(地点)」行同样命中 :contains('最近动向')，但取值走 First()，
			// 文档顺序上「最近动向」在前，因此不会误取地点行。
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
		},
	},
	Selectors: &v2.SiteSelectors{
		TableRows: "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		Title:     "table.torrentname a[href*='details.php']",
		TitleLink: "table.torrentname a[href*='details.php']",
		// 名称单元格是 table.torrentname 内「包含 details.php 链接」的那个 td.embedded：
		// 第一个 td.embedded 是封面缩略图（20/20 行都有），不能用 First() 直接取。
		// 副标题是 <br /> 之后的裸文本，前面还有若干彩色标签 <span>（官方/中字/完结…），
		// 需先吞掉任意个完整 <span>…</span> 再截取裸文本。
		SubtitleSelector: &v2.FieldSelector{
			Selector: []string{"table.torrentname td.embedded:has(a[href*='details.php'])"},
			Attr:     "html",
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
		// 列序（colhead）：1 类型 2 标题 3 评论数 4 存活时间 5 大小 6 种子数 7 下载数 8 完成数 9 发布者
		Size:     "td.rowfollow:nth-child(5)",
		Seeders:  "td.rowfollow:nth-child(6)",
		Leechers: "td.rowfollow:nth-child(7)",
		Snatched: "td.rowfollow:nth-child(8)",
		// 采集样本中实际出现的是 pro_free 与 pro_free2up，其余为 NexusPHP 通用促销图标类名。
		// 这里不设置 DiscountMapping：驱动内建的判定顺序会先识别 free2up 再识别 free，
		// 而自定义 mapping 是无序遍历 + 子串匹配，会让 pro_free2up 有概率被误判为 free。
		DiscountIcon: "img.pro_free, img.pro_free2up, img.pro_2up, img.pro_50pctdown, img.pro_50pctdown2up, img.pro_30pctdown",
		// 促销结束时间真实渲染为 <font color='#0000FF'>剩余时间：<span title="…">6天19时</span></font>，
		// 必须限定在 font:contains('剩余时间') 内：名称单元格里还有审核状态 <span title="通过">，
		// 而第 4 列的存活时间也是 span[title]。若某些页面不渲染该 span，
		// 驱动会回退解析促销图标的 onmouseover 提示。
		DiscountEndTime:    "table.torrentname font:contains('剩余时间') span[title]",
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
		// 详情页保留隐藏域，标题/ID 直接取 value
		TitleSelector: "input[name='torrent_name']",
		IDSelector:    "input[name='detail_torrent_id']",
		// h1 里促销标记为 <font class='free' >免费</font>（单引号 + 尾随空格），
		// 同时可能出现 <font class='classic'>经典</font> 之类的非促销标记，
		// ParseDiscount 会逐个比对 class 直到命中 DiscountMapping，因此取全部带 class 的 font 即可。
		DiscountSelector: "h1 font[class]",
		// h1 内除「剩余时间」span 外还有审核状态 <span title="通过">，
		// 限定 font:contains('剩余时间') 保证取到促销结束时间；保留通用兜底。
		EndTimeSelector: "h1 font:contains('剩余时间') span[title], h1 span[title]",
		// ParseSizeMB 读取 SizeSelector 元素的 .Next() 兄弟节点，因此必须指向标签单元格；
		// 体积使用全角冒号（大小：46.11 GB），且本站十进制/二进制单位混用，单位组放宽为 [KMGT]i?B
		SizeSelector: "td.rowhead:contains('基本信息')",
		SizeRegex:    `大小[：:][^\d]*([\d.]+)[\s\x{00a0}]*([KMGT]i?B)`,
	},
}

func init() {
	v2.RegisterSiteDefinition(LuckptDefinition)
}
