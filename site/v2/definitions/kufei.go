package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// KufeiDefinition 描述库非（kufei.org）站点（issue #870）。
//
// 架构为新版 NexusPHP（CHD/scenetorrents 分类图标皮肤，页头「[签到已得N, 补签卡: M]」），
// 详情页保留 input[name='torrent_name'] / input[name='detail_torrent_id'] 隐藏域，
// 结构与 RailgunPT 基本一致：
//  1. 种子列表的名称子表在名称单元格前多一个 46x46 海报缩略图 td.embedded，
//     副标题用 :has(a[href*='details.php']) 锁定名称单元格；副标题是 <br /> 之后的裸文本，
//     前面可能有 <span title="">国语</span> 这类标签，走 SubtitleSelector 的 innerHTML + 正则。
//  2. 列表的促销剩余时间在促销图标的 onmouseover 提示里，不设 DiscountEndTime，交给驱动解析提示。
//  3. 站点装了 H&R 插件（页头「H&R: [0/0/10]」、myhr.php），但抓取的列表与详情页里没有一个种子带 hitandrun 标记，
//     考核规则也不在页面上，因此不设置 HREnabled；HRKeywords 不写裸的「H&R」，页头的计数器不会被当成 H&R 种子。
//     详情页带 hitandrun 标记的种子照样按 H&R 处理，做种时长按站点设置里的「H&R 做种要求」算；
//     没有标记的种子 pt-tools 不当作 H&R，站点自己的考核要用户留意。
//  4. userdetails.php 有「魔力值」「做种积分」两行，直接从这两行取；
//     「最近动向」后面还有一行「最近动向(地点)」，选择器取第一个匹配，就是「最近动向」。
var KufeiDefinition = &v2.SiteDefinition{
	ID:             "kufei",
	Name:           "库非",
	Aka:            []string{"Kufei", "kufei.org"},
	Description:    "综合性 PT 站点，影视、剧集、动漫资源较多",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://kufei.org/"},
	FaviconURL:     "https://kufei.org/favicon.ico",
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
					"levelName", "bonus", "seedingBonus", "joinTime", "lastAccessAt",
				},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			// info_block 里的用户链接是绝对地址（https://kufei.org/userdetails.php?id=...）
			"id": {
				Selector: []string{
					"#info_block a[href*='userdetails.php']",
					"a[href*='userdetails.php'][class*='Name']",
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
			// 「传输」行是内嵌小表格：
			// <strong>分享率</strong>: <font color="">3.250</font>（<strong>实际分享率</strong>：1.020）
			// <strong>上传量</strong>: 1.200 TB   <strong>下载量</strong>: 380.50 GB
			// <strong>实际上传量</strong>: 860.20 GB  <strong>实际下载量</strong>: 840.10 GB
			// 正则要求 <strong> 紧邻标签名，因此不会被「实际上传量 / 实际下载量 / 实际分享率」误命中
			"uploaded": {
				Selector: []string{"td.rowhead:contains('传输') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>上传量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"downloaded": {
				Selector: []string{"td.rowhead:contains('传输') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>下载量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"trueUploaded": {
				Selector: []string{"td.rowhead:contains('传输') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>实际上传量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"trueDownloaded": {
				Selector: []string{"td.rowhead:contains('传输') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>实际下载量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			// 分享率不到 1 时数值包在 <font color="#880000"> 里：标签与数值之间只跳过标签、空白和冒号，不停在颜色值上。
			// 不接 parseNumber：分享率由 parseRatio 解析，它认得「无限」「Inf.」这些无穷写法
			"ratio": {
				Selector: []string{"td.rowhead:contains('传输') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>分享率</strong>(?:<[^>]*>|[\s\p{Zs}：:])*([\d.,]+|无限|無限|∞|Inf)`}},
				},
			},
			"levelName": {
				Selector: []string{"td.rowhead:contains('等级') + td img"},
				Attr:     "title",
			},
			// 「魔力值」行形如 12,345.6 [明细]
			"bonus": {
				Selector: []string{"td.rowhead:contains('魔力值') + td"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			// 「做种积分」行形如 23,456.7 (更新时间: 2026-07-01 08:00:00)，parseNumber 取第一个数
			"seedingBonus": {
				Selector: []string{"td.rowhead:contains('做种积分') + td"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			// 「加入日期」形如 2025-01-02 03:04:05 (1年前, 52周)，正则只取绝对时间
			"joinTime": {
				Selector: []string{"td.rowhead:contains('加入日期') + td"},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			// 保号探测只读取 UserInfo.LastAccess；「最近动向(地点)」排在「最近动向」后面，取第一个匹配
			"lastAccessAt": {
				Selector: []string{"td.rowhead:contains('最近动向') + td"},
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
		// 名称子表的 td.embedded 依次为：海报缩略图 / 名称 / 下载按钮，用 :has(a[href*='details.php']) 锁定名称单元格。
		// 副标题是 <br /> 之后的裸文本，前面可能有几个 <span title="">标签</span>，正则先吞掉标签再取到下一个标签之前。
		SubtitleSelector: &v2.FieldSelector{
			Selector: []string{"table.torrentname td.embedded:has(a[href*='details.php'])"},
			Attr:     "html",
			Filters: []v2.Filter{
				{Name: "regex", Args: []any{`(?s)<br\s*/?>(?:\s*<span[^>]*>[^<]*</span>)*\s*([^<]+)`}},
				// 走 html 取值会保留实体编码，需手动还原；&amp; 必须最后替换以避免二次解码
				{Name: "replace", Args: []any{"&#39;", "'"}},
				{Name: "replace", Args: []any{"&#34;", `"`}},
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
		// 实测抓取到 pro_free / pro_free2up / pro_50pctdown，其余为 NexusPHP 通用促销图标类名
		DiscountIcon: "img.pro_free, img.pro_free2up, img.pro_2up, img.pro_50pctdown, img.pro_50pctdown2up, img.pro_30pctdown",
		// 驱动默认的对照里 pro_50pctdown2up 会先命中「50pct」当成 50%，这里按图标类名逐个对应（长的类名先比）
		DiscountMapping: map[string]v2.DiscountLevel{
			"pro_free":         v2.DiscountFree,
			"pro_free2up":      v2.Discount2xFree,
			"pro_2up":          v2.Discount2xUp,
			"pro_50pctdown":    v2.DiscountPercent50,
			"pro_50pctdown2up": v2.Discount2x50,
			"pro_30pctdown":    v2.DiscountPercent30,
		},
		DownloadLink:       "a[href*='download.php']",
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
		HRKeywords:    []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"},
		TitleSelector: "input[name='torrent_name']",
		IDSelector:    "input[name='detail_torrent_id']",
		// h1 里促销标记形如 <b>[<font class="free">免费</font>]</b>，后面紧跟无 class 的
		// <font color="#0000FF">剩余时间：<span title="...">...</span></font>
		DiscountSelector: "h1 font.free, h1 font[class]",
		EndTimeSelector:  "h1 font span[title]",
		// ParseSizeMB 读取 SizeSelector 元素的 .Next() 兄弟节点，因此必须指向标签单元格本身
		SizeSelector: "td.rowhead:contains('基本信息')",
		SizeRegex:    `大小[：:][^\d]*([\d.]+)[\s\x{00a0}]*(TiB|TB|GiB|GB|MiB|MB|KiB|KB)`,
	},
}

func init() {
	v2.RegisterSiteDefinition(KufeiDefinition)
}
