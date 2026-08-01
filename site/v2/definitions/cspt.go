package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// CsptDefinition 描述财神（cspt.top）站点。
//
// 站点内核为标准 NexusPHP，但套用了 Tailwind 皮肤，因此呈现出「列表页 div、详情页 table」
// 的混合结构，是目前唯一需要 div 版行选择器的站点：
//
//  1. torrents.php 的种子列表完全由 div 组成：外层 div.torrents 内每行是
//     div.torrent-table-sub-info，其下并列 div.torrent-cat（分类）、div.torrent-title
//     （标题/副标题/评分/标签）、包裹 div.torrent-info 的统计列以及 div.torrent-manage
//     （收藏/下载）。表头行是另一个 div（bg-[main_tittle]），同样带 torrent-cat /
//     torrent-title / torrent-info，但**不带** torrent-table-sub-info，因此行选择器
//     必须锚定 torrent-table-sub-info，并额外用 :has(a[href*='details.php']) 兜底。
//  2. 统计列有稳定的语义类名（torrent-info-text-size / -seeders / -leechers /
//     -finished / -added），优先用类名而非 nth-child，避免站点调整列序后失配。
//  3. details.php 仍是常规 table 布局，且保留 input[name='torrent_name'] /
//     input[name='detail_torrent_id'] 隐藏域，因此详情解析与 btschool/eastgame 一致。
//  4. 站点没有 #info_block（源码里被注释掉了），顶部个人信息改为 div.menu-base-info，
//     做种/下载数藏在 svg <title> 之后的 <a> 里，只能用 html + 正则取值。
//  5. 魔力值在本站叫「金元宝」，userdetails.php 的行标签同名。
var CsptDefinition = &v2.SiteDefinition{
	ID:             "cspt",
	Name:           "财神",
	Description:    "综合性 PT 站点（NexusPHP + Tailwind 皮肤）",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://cspt.top/"},
	FaviconURL:     "https://cspt.top/favicon.ico",
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
			// 顶栏用户名链接位于 span.user-id 内，class 形如 PowerUser_Name；
			// 先按 span.user-id 限定，避免在 userdetails.php 上误取「邀请人」那一行的
			// EliteUser_Name 链接。
			"id": {
				Selector: []string{
					"span.user-id a[href*='userdetails.php'][class*='Name']",
					"#user-info-pannl a[href*='userdetails.php']",
					"a[href*='userdetails.php']",
				},
				Attr:    "href",
				Filters: []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{
					"span.user-id a[href*='userdetails.php'][class*='Name']",
					"a[href*='userdetails.php'][class*='Name']",
				},
			},
			// 顶栏没有 img.arrowup/arrowdown，做种/下载数是
			// <svg ...><title id="downing">当前做种</title>...</svg>&nbsp;<a href="userdetails.php?id=...">329</a>
			// 的结构，只能取 div.menu-base-info 的 html 后按 svg 标题文案定位。
			"seeding": {
				Selector: []string{"div.menu-base-info"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?s)当前做种</title>.*?<a[^>]*>\s*([\d,]+)\s*</a>`}},
					{Name: "parseNumber"},
				},
			},
			"leeching": {
				Selector: []string{"div.menu-base-info"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?s)正在下载</title>.*?<a[^>]*>\s*([\d,]+)\s*</a>`}},
					{Name: "parseNumber"},
				},
			},
			// 本站魔力值命名为「金元宝」：顶栏是 mybonus.php 链接，
			// userdetails.php 上是同名行标签。
			"bonus": {
				Selector: []string{
					"div.menu-base-info a[href*='mybonus.php']",
					"td.rowhead:contains('金元宝') + td",
					"td.rowhead:contains('魔力值') + td",
				},
				Filters: []v2.Filter{{Name: "parseNumber"}},
			},
			// 「传输」行是合并单元格（内嵌 table），需以 html + regex 拆分。
			// 单元格内同时存在 上传量/实际上传量、下载量/实际下载量、分享率/实际分享率，
			// 正则以 <strong> 紧跟标签名的方式区分，不会被「实际」前缀串味。
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
		// div 版行选择器：torrent-table-sub-info 只出现在真实种子行上（表头行只有
		// torrent-cat / torrent-title / torrent-info），:has 再兜一层保证行内确有种子链接。
		TableRows: "div.torrent-table-sub-info:has(a[href*='details.php'])",
		// 标题链接自带 torrent-info-text-name 类，<b> 内文本即完整标题（title 属性同值）。
		Title:     "a.torrent-info-text-name",
		TitleLink: "a.torrent-info-text-name",
		// 副标题有独立类名，无需 SubtitleSelector 的 html + 正则方案。
		Subtitle: "div.torrent-info-text-small_name",
		// 统计列按语义类名取值，均为纯文本（种子数外层包 <b><a><font>，Text() 后仍是数字）。
		Size:         "div.torrent-info-text-size",
		Seeders:      "div.torrent-info-text-seeders",
		Leechers:     "div.torrent-info-text-leechers",
		Snatched:     "div.torrent-info-text-finished",
		DiscountIcon: "img.pro_free, img.pro_free2up, img.pro_2up, img.pro_50pctdown, img.pro_50pctdown2up, img.pro_30pctdown",
		// 促销结束时间以 <font color='#0000FF'>剩余时间：<span title="..."> 真实渲染，
		// 限定 font[color] 可避免误取 div.torrent-info 内的发布时间 span；
		// 若某些页面不渲染该 span，驱动会回退解析促销图标的 onmouseover 提示。
		DiscountEndTime:    "div.torrent-title font[color] span[title]",
		DownloadLink:       "div.torrent-manage a[href*='download.php']",
		Category:           "div.torrent-cat img[alt]",
		UploadTime:         "div.torrent-info-text-added span[title]",
		DetailDownloadLink: "td.rowhead:contains('下载') + td a[href*='download.php'], td.rowhead:contains('行为') + td a[href*='download.php']",
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
		// 顶栏有 [H&R]: [0/0/10] 计数器，若把 "H&R" 列为关键字会让每个种子都被判定为 H&R，
		// 因此只保留 NexusPHP 的图标/文案关键字。
		HRKeywords: []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"},
		// 详情页保留隐藏域，标题/ID 直接取 value
		TitleSelector: "input[name='torrent_name']",
		IDSelector:    "input[name='detail_torrent_id']",
		// h1 内促销标记为 <font class='free'>（单引号 class，goquery 已归一化）
		DiscountSelector: "h1 font.free, h1 font[class]",
		// h1 里还有一个 <span title="通过"> 状态图标，限定 font[color] 只命中剩余时间 span
		EndTimeSelector: "h1 font[color] span[title]",
		// ParseSizeMB 读取 SizeSelector 元素的 .Next() 兄弟节点，因此必须指向标签单元格；
		// 标签文本包在 <span class="td-text"> 内，:contains 匹配后代文本仍然命中。
		// 本站正文体积为十进制单位 + 全角冒号（大小：12.98 GB），但页内他处混用 GiB，
		// 故单位组同时接受十进制与 IEC 写法。
		SizeSelector: "td.rowhead:contains('基本信息')",
		SizeRegex:    `大小[：:]\s*([\d.]+)\s*(TiB|GiB|MiB|KiB|TB|GB|MB|KB)`,
	},
}

func init() {
	v2.RegisterSiteDefinition(CsptDefinition)
}
