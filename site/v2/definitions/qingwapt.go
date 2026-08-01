package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// QingwaptDefinition 描述青蛙（www.qingwapt.com）站点。
//
// 站点为标准 NexusPHP 架构（qw 皮肤），详情页保留了 input[name='torrent_name'] /
// input[name='detail_torrent_id'] 隐藏域，userdetails.php 的「传输」行是经典的
// <strong>上传量</strong> 合并单元格，因此绝大多数选择器与 btschool 一致。
// 需要单独适配的三处：
//  1. 搜索页促销标记不是 img.pro_free，而是自绘的文字徽章
//     <span style="..." alt="Free" onmouseover="...">Free</span>，
//     必须用 span[alt] 命中（parseDiscountFromElement 会读 class+src+alt）。
//  2. 促销剩余时间渲染为 <font color='#0000FF'>剩余时间：<span title="...">…</span></font>，
//     名称单元格里还存在 title="置顶促销" / title="自动审核通过" 等非时间 span，
//     因此必须限定到 font[color] 之内，否则会取到非时间值。
//  3. 搜索页副标题是名称单元格末尾的裸文本节点，且前面穿插若干自绘标签 span
//     （官方 / 中字 / 分集），只能取单元格 html 后用正则截取尾部文本。
//
// 魔力值在本站叫「蝌蚪」，#info_block 中的标签文本据此适配。
var QingwaptDefinition = &v2.SiteDefinition{
	ID:             "qingwapt",
	Name:           "青蛙",
	Description:    "综合性 PT 站点（NexusPHP）",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://www.qingwapt.com/"},
	FaviconURL:     "https://www.qingwapt.com/favicon.ico",
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
					"uploaded", "downloaded", "ratio", "trueUploaded", "trueDownloaded",
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
			// 「传输」行是合并单元格（内嵌 table），需以 html + regex 拆分：
			//   <strong>上传量</strong>: 3.131 TB   <strong>下载量</strong>: 1.538 TB
			//   <strong>实际上传量</strong>: 2.070 TB  <strong>实际下载量</strong>: 3.758 TB
			// 正则要求 <strong> 紧跟标签，因此「实际上传量」不会误命中「上传量」。
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
			// 分享率行形如 <strong>分享率</strong>: <font color="">2.035</font>（<strong>实际分享率</strong>：0.550），
			// 取 <font> 内的第一个数值即名义分享率。
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
			// 本站把魔力值叫「蝌蚪」：
			// <font class="color_bonus">蝌蚪 </font>[详情][使用]: 260,200.3
			// #info_block 内还有第二个 color_bonus（认领），正则取首个匹配即蝌蚪。
			"bonus": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:蝌蚪|魔力值)\s*</font>(?:\s*\[[^\]]*\])*\s*[:：]\s*([\d.,]+)`}},
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
		// 副标题是名称单元格末尾的裸文本节点，前面可能穿插 img.sticky、<b>(新)</b>、
		// <b>[热门]</b>、促销徽章、剩余时间以及若干自绘标签 span（官方/中字/分集），
		// 无法用普通 CSS 选择器命中，因此取名称单元格 html 后正则截取末尾文本。
		// 名称子表内还有海报单元格与评分单元格（同为 td.embedded），
		// 用 :has(a[href*='details.php']) 精确锁定名称单元格。
		SubtitleSelector: &v2.FieldSelector{
			Selector: []string{"table.torrentname td.embedded:has(a[href*='details.php'])"},
			Attr:     "html",
			Filters: []v2.Filter{
				{Name: "regex", Args: []any{`(?s)>\s*([^<>]+?)\s*$`}},
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
		// 本站促销标记为自绘文字徽章 <span ... alt="Free" onmouseover="...">Free</span>，
		// 名称单元格内只有该徽章带 alt 属性；img.pro_* 为 NexusPHP 通用类名，留作兜底。
		DiscountIcon: "table.torrentname span[alt], img.pro_free, img.pro_free2up, img.pro_2up, img.pro_50pctdown, img.pro_50pctdown2up, img.pro_30pctdown",
		// 剩余时间固定包在 <font color='#0000FF'> 内，限定 font[color] 可避开
		// title="置顶促销" / title="自动审核通过" 等非时间 span；若某些页面不渲染该
		// 元素，驱动会回退解析促销徽章的 onmouseover 提示。
		DiscountEndTime:    "table.torrentname font[color] span[title]",
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
		// h1 内为 <b>[<font class='free' >免费</font>]</b>（单引号 + 尾部空格），
		// ParseDiscount 按 strings.Fields 拆 class，能正确命中 free
		DiscountSelector: "h1 font.free, h1 font[class]",
		// h1 内第一个 span[title] 就是剩余时间，晚于它的 title="自动审核通过" 不会被取到
		EndTimeSelector: "h1 span[title]",
		// ParseSizeMB 读取 SizeSelector 元素的 .Next() 兄弟节点，因此必须指向标签单元格；
		// 采集样本为「大小：1.44 GB」（全角冒号 + 十进制单位），单位组一并覆盖二进制写法
		SizeSelector: "td.rowhead:contains('基本信息')",
		SizeRegex:    `大小[：:]\s*([\d.]+)\s*(TiB|GiB|MiB|KiB|TB|GB|MB|KB)`,
	},
}

func init() {
	v2.RegisterSiteDefinition(QingwaptDefinition)
}
