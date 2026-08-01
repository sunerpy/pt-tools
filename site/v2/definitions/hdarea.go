package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// HdareaDefinition 描述 HDArea（hdarea.club）站点。
//
// 站点为 NexusPHP 1.5 魔改（页面自称 "Powered by HDAREA(基于NexusPHP1.5修改)"），详情页保留
// input[name='torrent_name'] / input[name='detail_torrent_id'] 隐藏域，userdetails.php 的
// 「传输」行是标准的 <strong>上传量</strong> 合并单元格，因此多数选择器与 btschool 一致。
// 需要单独适配的三处：
//
//  1. torrents.php 上存在 **两个** table.torrents：
//     第一个是「预告区（前 5 最新预告）」面板，表头为 td.colhead_notice（colspan=10），
//     内容藏在 tbody#kokk 里；第二个才是真正的种子列表，表头为 10 个 td.colhead。
//     若按常规写 `table.torrents > tbody > tr:has(table.torrentname)`，预告区一旦有内容
//     就会被一起解析进来，因此这里用 :has(td.colhead) 把范围锁死在真实列表表格。
//  2. 副标题是名称单元格内的第二个 <div>，带 title 属性（<div style="color: #666" title="...">），
//     全页仅此一种 div[title]，可直接用普通字符串选择器命中。
//  3. 促销结束时间以 (<font color='...'>剩余时间：<span title="..."></font>) 真实渲染，
//     但同一子表里的豆瓣评分 <span title="豆瓣评分"> 也是 span[title]，必须用 font[color]
//     限定范围；不同促销等级的 font color 取值不同（免费 #0000FF、2X 免费 #00CC66），
//     故只判断属性存在而不判断取值。
var HdareaDefinition = &v2.SiteDefinition{
	ID:             "hdarea",
	Name:           "HDArea",
	Aka:            []string{"高清视界", "High Definition Area"},
	Description:    "HDArea 高清设备交流分享平台",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://hdarea.club/"},
	FaviconURL:     "https://hdarea.club/favicon.ico",
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
			// <strong>分享率</strong>: <font color="">5.603</font>
			// <strong>上传量</strong>: 5.607 TB   <strong>下载量</strong>: 1.001 TB
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
			// 魔力值只在 #info_block 渲染，且 class 属性带空格：
			// <font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 2,747,702.4
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
			// 本站 userdetails.php 只有一行「最近动向」，不存在同名的「(地点)」行。
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
		// :has(td.colhead) 用于排除「预告区」表格（其表头是 td.colhead_notice）；
		// tr:has(table.torrentname) 再排除真实列表自身的表头行。
		TableRows: "table.torrents:has(td.colhead) > tbody > tr:has(table.torrentname), " +
			"table.torrents:has(td.colhead) > tr:has(table.torrentname)",
		Title:     "table.torrentname a[href*='details.php']",
		TitleLink: "table.torrentname a[href*='details.php']",
		// 副标题：名称单元格内带 title 的 div，全页唯一（100 行 100 个，无其它 div[title]）
		Subtitle: "table.torrentname td.embedded > div[title]",
		// 列序（colhead）：1 类型 2 海报 3 标题 4 评论数 5 存活时间 6 大小 7 种子数 8 下载数 9 完成数 10 发布者
		Size:     "td.rowfollow:nth-child(6)",
		Seeders:  "td.rowfollow:nth-child(7)",
		Leechers: "td.rowfollow:nth-child(8)",
		Snatched: "td.rowfollow:nth-child(9)",
		// 实抓到 pro_free / pro_free2up，其余为 NexusPHP 通用促销图标类名。
		// 不设置 DiscountMapping：驱动内置分支会先判 free2up 再判 free，
		// 自定义 mapping 走的是无序 map 包含匹配，可能把 pro_free2up 误判为 free。
		DiscountIcon: "img.pro_free, img.pro_free2up, img.pro_2up, img.pro_50pctdown, img.pro_50pctdown2up, img.pro_30pctdown",
		// 限定在 font[color] 内，避免误取同一子表里的豆瓣评分 span[title="豆瓣评分"]；
		// 若某些页面不渲染该 span，驱动会回退解析促销图标的 onmouseover 提示。
		DiscountEndTime:    "table.torrentname font[color] span[title]",
		Category:           "td.rowfollow:nth-child(1) img[alt]",
		UploadTime:         "td.rowfollow:nth-child(5) span[title]",
		DetailDownloadLink: "td.rowhead:contains('下载链接') + td a[href*='download.php'], td.rowhead:contains('下载') + td a[href*='download.php']",
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
		// h1#top 内促销标记为 <font class='free' >免费</font>（单引号 + 尾随空格），
		// 解析器按 class token 匹配 DiscountMapping，能正确命中
		DiscountSelector: "h1 font.free, h1 font[class]",
		EndTimeSelector:  "h1 span[title]",
		// ParseSizeMB 读取 SizeSelector 元素的 .Next() 兄弟节点，因此必须指向标签单元格；
		// 单元格文本形如「大小：482.76 MB   类型: TV Series ...」，全角冒号，
		// 单位放宽为 [KMGTP]i?B 以兼容站点混用二进制单位的情况
		SizeSelector: "td.rowhead:contains('基本信息')",
		SizeRegex:    `大小[：:][^\d]*([\d.]+)\s*([KMGTP]i?B)`,
	},
}

func init() {
	v2.RegisterSiteDefinition(HdareaDefinition)
}
