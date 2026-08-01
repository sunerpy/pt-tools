package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// HhanclubDefinition 描述 HHCLUB（hhanclub.net）站点。
//
// 站点内核仍是 NexusPHP（<title>HHCLUB :: 种子 - Powered by NexusPHP</title>），
// 但皮肤（styles/HHan）用 Tailwind 全量重写了 DOM：**列表页与详情页都没有任何
// table/tr/td**，且促销标记不是 NexusPHP 通用的 img.pro_* 图标。三处关键适配：
//
//  1. 列表行是纯 div。页面内 `class="torrents"` 出现 0 次，`<table>`/`<tr>` 出现 0 次。
//     真实行的公共父元素是 `div.torrent-table-sub-info`（100 个，与 download.php?id=
//     的 100 次出现一一对应）；表头行是同级的另一个 div（携带 torrent-cat /
//     torrent-title 但**没有** torrent-table-sub-info），因此按该 class 选行即天然
//     排除表头，无需 :has() 判别。列内统计值都带语义 class
//     （torrent-info-text-size / -added / -seeders / -leechers / -finished），
//     一律用 class 选择器而非 nth-child。
//
//  2. 促销标记是站点自定义 span：<span class="promotion-tag promotion-tag-free">免费</span>。
//     页面内嵌的 tailwindcss 声明只定义了 4 种：-free / -50 / -30 / -2xfree。
//     搜索侧 parseDiscountFromElement 用 class+src+alt 的**子串**匹配，映射 key 采用
//     完整 class 名后互不为子串（"promotion-tag-free" 不是
//     "promotion-tag promotion-tag-2xfree" 的子串，反之亦然），因此 map 遍历顺序
//     随机也不会出现 free 覆盖 2xfree；详情侧 ParseDiscount 按空白拆分 class 后做
//     **精确**查表，同一份映射同时满足两条路径。
//
//  3. 详情页体积在嵌套 div 里：
//     <span class="font-bold"><b>大小：</b></span><span class="">17.33 GB</span>
//     ParseSizeMB 取 SizeSelector 元素的 .Next() 兄弟节点文本，所以 SizeSelector 必须
//     指向**外层 span**（其下一个兄弟正是数值 span），且正则里不能再带「大小：」前缀，
//     否则匹配不到（兄弟节点文本只有 "17.33 GB"）。
//
// 已知限制（无法由选择器解决，非本站定义可控）：列表页标题锚点内嵌新种角标
// <a class='... torrent-info-text-name'>标题<span class='new'>[新]</span></a>（实测 100 行中 91 行携带），
// 而 ParseSearch 取标题走 strings.TrimSpace(titleElem.Text())，没有 Attr/Filters 钩子
// （SiteSelectors 只为副标题提供了 SubtitleSelector *FieldSelector），CSS 也无法排除子元素，
// 因此搜索结果标题会带 "[新]" 后缀。行内不存在任何携带干净标题的备选元素（锚点无 title 属性）。
// 影响面有限：详情页走 input[name='torrent_name'] 的 value，标题干净，
// 因此 RSS 下载、免费到期监控与 .torrent 文件名均不受影响，仅搜索列表展示带角标。
//
// 已核对：三个页面均无 hitandrun / hit_run / Hit and Run 痕迹，故不声明 H&R；
// 采集数据未包含等级门槛页面，故不声明 LevelRequirements。
var HhanclubDefinition = &v2.SiteDefinition{
	ID:             "hhanclub",
	Name:           "HHCLUB",
	Description:    "综合性 PT 站点（NexusPHP 内核 + Tailwind 皮肤）",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://hhanclub.net/"},
	FaviconURL:     "https://hhanclub.net/favicon.ico",
	AuthMethod:     v2.AuthMethodCookie,
	TimezoneOffset: "+0800",
	RateLimit:      0.5,
	RateBurst:      2,
	UserInfo: &v2.UserInfoConfig{
		PickLast:     []string{"id"},
		RequestDelay: 500,
		Process: []v2.UserInfoProcess{
			{
				// 全站左侧用户面板（userdetails.php 页面上位于文档最前的
				// a[href*='userdetails.php'][class*='Name']）在 index.php 同样渲染，
				// id / name / 做种数 / 下载数 均取自这里。
				RequestConfig: v2.RequestConfig{URL: "/index.php", ResponseType: "document"},
				Fields:        []string{"id", "name", "seeding", "leeching"},
			},
			{
				RequestConfig: v2.RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
				Assertion:     map[string]string{"id": "params.id"},
				Fields: []string{
					"name", "uploaded", "downloaded", "ratio", "levelName",
					"bonus", "seedingBonus", "joinTime", "lastAccessAt",
				},
			},
		},
		// 正文字段统一是 <span class="font-bold">标签：</span><值元素> 结构。
		// 「上传量：」是「实际上传量：」的子串，:contains 会同时命中两者，因此首选
		// :matchesOwn 做整串精确匹配；若某些页面标签带额外空白导致 :matchesOwn 落空，
		// 再回退 :contains —— 取值走 First()，文档顺序上「上传量」先于「实际上传量」，
		// 回退路径同样取到正确值。
		Selectors: map[string]v2.FieldSelector{
			"id": {
				Selector: []string{
					"a[href*='userdetails.php'][class*='Name']",
					"a[href*='userdetails.php']",
				},
				Attr:    "href",
				Filters: []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{
					"a[href*='userdetails.php'][class*='Name']",
					"a[href*='userdetails.php']",
				},
			},
			"uploaded": {
				Selector: []string{
					`span.font-bold:matchesOwn(^\s*上传量：\s*$) + span`,
					"span.font-bold:contains('上传量：') + span",
				},
				Filters: []v2.Filter{{Name: "parseSize"}},
			},
			"downloaded": {
				Selector: []string{
					`span.font-bold:matchesOwn(^\s*下载量：\s*$) + span`,
					"span.font-bold:contains('下载量：') + span",
				},
				Filters: []v2.Filter{{Name: "parseSize"}},
			},
			// 分享率单元格形如 <font>6.300</font>（<strong>实际分享率</strong>：0.785），
			// 取 font 内的官方分享率，避免把实际分享率一起吃进来。
			"ratio": {
				Selector: []string{
					`span.font-bold:matchesOwn(^\s*分享率：\s*$) + span font`,
					"span.font-bold:contains('分享率：') + span font",
					`span.font-bold:matchesOwn(^\s*分享率：\s*$) + span`,
				},
				Filters: []v2.Filter{{Name: "parseNumber"}},
			},
			// 等级：<span>...<img title="Crazy User(...)"><b class='XXX_Name'>Crazy User(...)</b></span>
			"levelName": {
				Selector: []string{
					`span.font-bold:matchesOwn(^\s*等级：\s*$) + span b`,
					`span.font-bold:matchesOwn(^\s*等级：\s*$) + span img`,
					"span.font-bold:contains('等级：') + span b",
				},
			},
			// 憨豆（魔力值）。正文里精确到小数位；侧边栏只有千分位整数，作为兜底。
			"bonus": {
				Selector: []string{
					`span.font-bold:matchesOwn(^\s*憨豆：\s*$) + div`,
					"span.font-bold:contains('憨豆：') + div",
					"a[href*='mybonus.php'] div",
				},
				Filters: []v2.Filter{{Name: "parseNumber"}},
			},
			"seedingBonus": {
				Selector: []string{
					`span.font-bold:matchesOwn(^\s*做种积分：\s*$) + span`,
					"span.font-bold:contains('做种积分：') + span",
				},
				Filters: []v2.Filter{{Name: "parseNumber"}},
			},
			// 侧边栏做种/下载数：<div><img alt="做种数">&nbsp;140</div>。
			// 必须用 :haschild（cascadia 的直接子元素判定）；:has(> img) 语法 cascadia v1.3.3
			// 不支持（实测匹配 0 个），而裸 :has(img) 会连外层祖先 div 一起命中。
			// 备选按 action 参数锚定侧边栏链接（个人中心导航里同名链接在文档顺序之后）。
			"seeding": {
				Selector: []string{
					"div:haschild(img[alt='做种数'])",
					"a[href*='userdetails.php'][href*='action=2'] div",
				},
				Filters: []v2.Filter{{Name: "parseNumber"}},
			},
			"leeching": {
				Selector: []string{
					"div:haschild(img[alt='下载数'])",
					"a[href*='userdetails.php'][href*='action=3'] div",
				},
				Filters: []v2.Filter{{Name: "parseNumber"}},
			},
			// 加入日期/最近动向文本形如 "2025-08-19 22:40:26 (11月11天前, 48周)"，
			// 直接用正则截首个完整时间戳再交给 parseTime（按 TimezoneOffset 解析）。
			"joinTime": {
				Selector: []string{
					`span.font-bold:matchesOwn(^\s*加入日期：\s*$) + span`,
					"span.font-bold:contains('加入日期：') + span",
				},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			// 保号探测只读取 UserInfo.LastAccess，缺失会导致线上探测解析失败。
			"lastAccessAt": {
				Selector: []string{
					`span.font-bold:matchesOwn(^\s*最近动向：\s*$) + span`,
					"span.font-bold:contains('最近动向：') + span",
				},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
		},
	},
	Selectors: &v2.SiteSelectors{
		// 真实种子行；表头行没有该 class，天然被排除。
		TableRows: "div.torrent-table-sub-info",
		Title:     "a.torrent-info-text-name",
		TitleLink: "a.torrent-info-text-name",
		Subtitle:  "div.torrent-info-text-small_name",
		// 统计列都有语义 class，不依赖 grid 中的列序。
		Size:     "div.torrent-info-text-size",
		Seeders:  "div.torrent-info-text-seeders",
		Leechers: "div.torrent-info-text-leechers",
		Snatched: "div.torrent-info-text-finished",
		// 站点自定义促销 span，取代 NexusPHP 通用 img.pro_*。
		DiscountIcon: "span.promotion-tag",
		DiscountMapping: map[string]v2.DiscountLevel{
			"promotion-tag-free":   v2.DiscountFree,
			"promotion-tag-2xfree": v2.Discount2xFree,
			"promotion-tag-50":     v2.DiscountPercent50,
			"promotion-tag-30":     v2.DiscountPercent30,
		},
		// 促销剩余时间是可见 span：<span>[ 剩余时间：<span title="...">7时21分钟</span> ]</span>。
		// 本站促销标记没有 onmouseover，驱动的 onmouseover 兜底不可用，必须命中这个 span；
		// 同时必须限定在「剩余时间」外层 span 之内，否则会误取
		// div.torrent-info-text-added 里同样是 span[title] 的发布时间。
		DiscountEndTime: "span:contains('剩余时间') > span[title]",
		DownloadLink:    "a[href*='download.php']",
		// 分类图标的 alt 恒为列头字样「类型」，无法反映真实分类，故不配置 Category，
		// 以免所有行都被写入同一个无意义取值。
		UploadTime: "div.torrent-info-text-added span[title]",
		// 详情页同为 div 布局：标签 div 与取值 div 是相邻兄弟。
		DetailDownloadLink: "a.index[href*='download.php'], a[href*='download.php?id=']",
		DetailSubtitle:     `div.font-bold:matchesOwn(^\s*副标题\s*$) + div`,
	},
	DetailParser: &v2.DetailParserConfig{
		TimeLayout: "2006-01-02 15:04:05",
		// 与搜索侧共用同一份映射：ParseDiscount 按 class token 精确查表。
		DiscountMapping: map[string]v2.DiscountLevel{
			"promotion-tag-free":   v2.DiscountFree,
			"promotion-tag-2xfree": v2.Discount2xFree,
			"promotion-tag-50":     v2.DiscountPercent50,
			"promotion-tag-30":     v2.DiscountPercent30,
		},
		HRKeywords: []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"},
		// 详情页保留了隐藏域，标题/ID 直接取 value（不含列表页的 [新] 角标）。
		TitleSelector:    "input[name='torrent_name']",
		IDSelector:       "input[name='detail_torrent_id']",
		DiscountSelector: "span.promotion-tag",
		EndTimeSelector:  "span:contains('剩余时间') > span[title]",
		// ParseSizeMB 取 .Next() 兄弟节点，所以指向「大小：」所在的外层 span，
		// 其兄弟文本仅为 "17.33 GB"，正则不能再要求「大小：」前缀。
		// 单位类只列 ParseSizeMB 能正确换算的 K/M/G/T（含 KiB/MiB/GiB/TiB）。
		SizeSelector: `span.font-bold:contains('大小：')`,
		SizeRegex:    `([\d.]+)\s*([KMGT]i?B)`,
	},
}

func init() {
	v2.RegisterSiteDefinition(HhanclubDefinition)
}
