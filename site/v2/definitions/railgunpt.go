package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// RailgunptDefinition 描述 RailgunPT（bilibili.download）站点。
//
// 域名与站点自称不一致：域名是 bilibili.download，但页面 <title>、页脚版权链接
// （<a href="https://bilibili.download">RailgunPT</a>）与官方 TG 群（t.me/RailgunPT）
// 均自称 RailgunPT，因此站点 ID / Name 取 RailgunPT，URL 取真实域名。
//
// 架构为标准 NexusPHP（CHD/scenetorrents 分类图标皮肤），详情页保留
// input[name='torrent_name'] / input[name='detail_torrent_id'] 隐藏域，
// 因此大部分选择器与 btschool / eastgame 一致。需要单独适配的地方：
//  1. 种子列表的名称子表在标题单元格之前多出一个 46x46 海报缩略图 td.embedded，
//     所以副标题不能直接取 table.torrentname 下第一个 td.embedded，
//     必须用 :has(a[href*='details.php']) 锁定真正的名称单元格。
//  2. 副标题是 <br /> 之后的裸文本节点，且前面可能有 1~3 个 <span title="">标签</span>
//     徽章（官方 / 国语 / 中字 / HDR），纯 CSS 选不中，故走 SubtitleSelector 的
//     innerHTML + 正则，先吞掉徽章 span 再截取裸文本。
//  3. 促销剩余时间同时存在于促销图标的 onmouseover 提示与名称单元格的可见
//     「剩余时间：<span title>」中；但徽章 span 带的是 title=""（空值），
//     用 span[title] 选择器有误命中风险，因此不设置 DiscountEndTime，
//     交由驱动的 parseDiscountEndTimeFromOnmouseover() 兜底解析。
//     实测抓取样本 30 个促销种子全部带 onmouseover 提示，无长期促销。
//  4. 详情页体积单位混用十进制（基本信息行 2.17 GB）与 IEC（MediaInfo 里的 2.17 GiB），
//     SizeRegex 的单位组同时接受两种写法（ParseSizeMB 已支持 KiB/MiB/GiB/TiB）。
//  5. 站点无 H&R（三张抓取页面 hitandrun 出现 0 次），故不设置 HREnabled。
var RailgunptDefinition = &v2.SiteDefinition{
	ID:             "railgunpt",
	Name:           "RailgunPT",
	Aka:            []string{"bilibili.download"},
	Description:    "综合性 PT 站点，影视 / 剧集 / 音乐 / 动漫资源较多",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://bilibili.download/"},
	FaviconURL:     "https://bilibili.download/favicon.ico",
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
			// info_block 里的用户链接是绝对地址（https://bilibili.download/userdetails.php?id=...）
			"id": {
				Selector: []string{
					"#info_block a[href*='userdetails.php']",
					"a[href*='userdetails.php'][class*='Name']",
					"a[href*='userdetails.php']",
				},
				Attr:    "href",
				Filters: []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			// 勋章 <img> 在 </a> 之外，取 <a> 文本即用户名
			"name": {
				Selector: []string{
					"#info_block a[href*='userdetails.php']",
					"a[href*='userdetails.php'][class*='Name']",
				},
			},
			// 「传输」行是合并单元格（内嵌 table），须以 html + 正则拆分：
			// <strong>分享率</strong>: <font>31.453</font>（<strong>实际分享率</strong>：2.541）
			// <strong>上传量</strong>: 130.393 TB   <strong>下载量</strong>: 4.146 TB
			// <strong>实际上传量</strong>: 78.269 TB  <strong>实际下载量</strong>: 30.801 TB
			// 正则要求 <strong> 紧邻标签名，因此不会被「实际上传量 / 实际下载量 / 实际分享率」误命中
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
			// 魔力值只在 #info_block 出现：<font class='color_bonus'>魔力值 </font>[使用]: 64,077.7
			"bonus": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`魔力值\s*</font>\s*\[[^\]]*\]\s*[:：]\s*([\d.,]+)`}},
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
			// 「加入日期」形如 2025-04-29 16:04:51 (1年3月前, 64周)，正则只取绝对时间
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
			// 本站 userdetails.php 只有一个「最近动向」行（无「最近动向(地点)」），无 :contains 冲突。
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
		// 名称子表的 td.embedded 依次为：海报缩略图 / 名称 / 豆瓣+IMDb 评分 / 下载按钮，
		// 因此必须用 :has(a[href*='details.php']) 锁定名称单元格（海报单元格排在最前）。
		// 副标题是 <br /> 之后的裸文本，前面可能有 1~3 个 <span title="">徽章</span>，
		// 正则先贪婪吞掉整段徽章 span，再截取到下一个标签（inactivity 进度条 <div>）之前。
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
		// 实测抓取到 pro_free / pro_free2up / pro_50pctdown / pro_2up，
		// 其余为 NexusPHP 通用促销图标类名，一并保留
		DiscountIcon: "img.pro_free, img.pro_free2up, img.pro_2up, img.pro_50pctdown, img.pro_50pctdown2up, img.pro_30pctdown",
		DownloadLink: "a[href*='download.php']",
		Category:     "td.rowfollow:nth-child(1) img[alt]",
		UploadTime:   "td.rowfollow:nth-child(4) span[title]",
		// 详情页「下载」行给出干净的 download.php?id=；「种子链接」行是带 JWT 的当日临时链接，放在后面兜底
		DetailDownloadLink: "td.rowhead:contains('下载') + td a[href*='download.php'], td.rowhead:contains('种子链接') + td a[href*='download.php']",
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
		// 详情页保留隐藏域，标题 / ID 直接取 value
		TitleSelector: "input[name='torrent_name']",
		IDSelector:    "input[name='detail_torrent_id']",
		// h1 里促销标记形如 <b>[<font class='twoupfree' >2X免费</font>]</b>，
		// 后面紧跟无 class 的 <font color='#00CC66'>剩余时间：...</font>，故按 font[class] 命中
		DiscountSelector: "h1 font.free, h1 font[class]",
		EndTimeSelector:  "h1 span[title]",
		// ParseSizeMB 读取 SizeSelector 元素的 .Next() 兄弟节点，因此必须指向标签单元格本身；
		// 「基本信息」行用全角冒号，单位组同时接受十进制（GB）与 IEC（GiB）写法
		SizeSelector: "td.rowhead:contains('基本信息')",
		SizeRegex:    `大小[：:][^\d]*([\d.]+)[\s\x{00a0}]*(TiB|TB|GiB|GB|MiB|MB|KiB|KB)`,
	},
}

func init() {
	v2.RegisterSiteDefinition(RailgunptDefinition)
}
