package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// HDWinGDefinition 是 HDWinG（hdwing.org）的站点定义。站点由 HDChina 重开（首页公告署名 HDChina 管理组，
// 邀请人与等级说明里都是 HDChina），代码是 HDChina 自己改过的 NexusPHP（页脚 Version de93eed）。
// 按 issue #910 报告人 2026-11-18 用扩展采集的页面（简体界面）核对：
//   - 种子列表是 table.torrent_list，标题格里套了一层 table.tbname：h3 是标题，h4 是副标题（推荐的种子前面有「[推荐]」）；
//     大小写成「23.47&nbsp;GB」，发布时间在 span 的 title 里
//   - 优惠状态不在页面里：列表与详情页的优惠位置只有 <span class="sp_state_placeholder" id="种子ID">，页面脚本带着
//     <meta name="x-csrf"> 的令牌 POST /ajax_promotion.php 取回图标（pro_free、pro_2up……）与结束时间再填进去，
//     pt-tools 照这样补（PromotionAJAX）。样本里见到的有免费、2X、2X 免费、50%
//   - 详情页标题在 h2#top（优惠图标也在这里），大小在「基本信息」一项（li，不是表格）；下载链接是签名的
//     download.php?hash=…（hash 里带着用户 ID），页面上没有 download.php?id=，按编号下载时先取详情页再用签名链接
//   - 首页顶栏的「已签到」只是个标记（href="#"），没采到没签到时的入口与签到回执，不做自动签到
//   - 报告人说没有 H&R：列表、详情页没有 H&R 标记，首页顶栏的 H&R 计数被注释掉了（魔力值商店还留着「消除H&R」一项），
//     不按 H&R 处理
//   - 时魔在 mybonus.php：「你当前每小时能获取X个魔力值」（捐赠者前面还有一句不算捐赠加成的，不取它）
//
// 没核实的：RSS 的 guid 与链接；Cookie 失效时站点回什么页面（PT-depiler 记的是「阻断页面」，样本里没有）；做种列表：
// 个人页的「当前做种」由页面脚本另取（PT-depiler 记的是带 csrf 的 POST ajax_getusertorrentlist.php），没采到返回，
// 做种体积照通用写法请求 getusertorrentlistajax.php，这一页在不在没核实，读不到时做种体积记不上。
var HDWinGDefinition = &v2.SiteDefinition{
	ID:          "hdwing",
	Name:        "HDWinG",
	Aka:         []string{"HDW", "HDChina"},
	Description: "HDChina 重开的高清影视站（hdwing.org）",
	Schema:      v2.SchemaNexusPHP,
	URLs:        []string{"https://hdwing.org/"},
	FaviconURL:  "https://hdwing.org/favicon.ico",
	AuthMethod:  v2.AuthMethodCookie,
	Attendance: &v2.AttendanceConfig{
		Unsupported: "HDWinG 的签到入口和签到回执没有采集到（页面上只看到「已签到」标记），暂不支持自动签到，请在网页上签到",
	},
	TimezoneOffset: "+0800",
	RateLimit:      0.5,
	RateBurst:      2,
	PromotionAJAX:  &v2.PromotionAJAXConfig{Path: "/ajax_promotion.php"},
	UserInfo: &v2.UserInfoConfig{
		PickLast:     []string{"id"},
		RequestDelay: 500,
		Process: []v2.UserInfoProcess{
			{
				RequestConfig: v2.RequestConfig{URL: "/index.php", ResponseType: "document"},
				Fields:        []string{"id", "name", "bonus", "seeding", "leeching"},
			},
			{
				RequestConfig: v2.RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
				Assertion:     map[string]string{"id": "params.id"},
				Fields: []string{
					"uploaded", "downloaded", "ratio", "levelName", "seedingBonus", "joinTime", "lastAccessAt",
				},
			},
			{
				RequestConfig: v2.RequestConfig{URL: "/mybonus.php", ResponseType: "document"},
				Fields:        []string{"bonusPerHour"},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			// 顶栏「欢迎回来，<a href="userdetails.php?id=…" class="User_Name"><b>用户名</b></a>」，类名随等级变（PowerUser_Name……）
			"id": {
				Selector: []string{
					"div.userinfo a[href*='userdetails.php'][class*='_Name']",
					"div.userinfo a[href*='userdetails.php']",
				},
				Attr:    "href",
				Filters: []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{
					"div.userinfo a[href*='userdetails.php'][class*='_Name'] b",
					"div.userinfo a[href*='userdetails.php'][class*='_Name']",
				},
			},
			// 「魔力值 [<a href="mybonus.php">使用</a>]：12,345.6」
			"bonus": {
				Selector: []string{"div.userinfo"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`魔力值[^：:]*[：:][\s\p{Zs}]*([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
			// 「(<i class="fas fa-arrow-up" title="当前做种"></i> 50　<i class="fas fa-arrow-down" title="当前下载"></i> 0)」
			"seeding": {
				Selector: []string{"div.userinfo"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`title="当前做种"[^>]*>(?:\s*</i>)?[\s\p{Zs}]*([\d,]+)`}},
					{Name: "parseNumber"},
				},
			},
			"leeching": {
				Selector: []string{"div.userinfo"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`title="当前下载"[^>]*>(?:\s*</i>)?[\s\p{Zs}]*([\d,]+)`}},
					{Name: "parseNumber"},
				},
			},
			// 个人页「传输」一格：<strong>上传量</strong>:  1.25 TB(本月100.00 GB)，括号里是本月的，不取
			"uploaded": {
				Selector: []string{"td.rowhead:contains('传输') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>\s*上传量\s*</strong>[\s\p{Zs}:：]*([\d.,]+[\s\p{Zs}]*[KMGTPE]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"downloaded": {
				Selector: []string{"td.rowhead:contains('传输') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>\s*下载量\s*</strong>[\s\p{Zs}:：]*([\d.,]+[\s\p{Zs}]*[KMGTPE]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			// <strong>分享率</strong>:  <font color="">2.500</font>；分享率不到 1 时 font 带颜色，标签与数值之间只跳过标签、空白和冒号
			"ratio": {
				Selector: []string{"td.rowhead:contains('传输') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:分享率|Ratio)(?:<[^>]*>|[\s\p{Zs}：:])*([\d.,]+|无限|無限|∞|Inf|---)`}},
					{Name: "replace", Args: []any{"无限", "9999999"}},
					{Name: "replace", Args: []any{"無限", "9999999"}},
					{Name: "replace", Args: []any{"∞", "9999999"}},
					{Name: "replace", Args: []any{"Inf", "9999999"}},
					{Name: "parseNumber"},
				},
			},
			// <img alt="群众演员(User)" title="群众演员(User)" src="pic/user.gif">
			"levelName": {
				Selector: []string{"td.rowhead:contains('等级') + td > img[title]", "td.rowhead:contains('等级') + td img[title]"},
				Attr:     "title",
			},
			// 「做种积分」一行（站点公告说做种积分暂时移除，样本里是 0.0）
			"seedingBonus": {
				Selector: []string{"td.rowhead:contains('做种积分') + td"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			"joinTime": {
				Selector: []string{"td.rowhead:contains('加入日期') + td"},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			// 「最近动向」是访问网页的时间；下一行「最近活动(Tracker)」是客户端汇报的时间，不当成登录
			"lastAccessAt": {
				Selector: []string{"td.rowhead:contains('最近动向') + td"},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			// 「如果没有捐赠标志，你每小时仅能获取8.250个魔力值 (A = 12.3); 你当前每小时能获取16.500个魔力值 (A = 24.6)」
			"bonusPerHour": {
				Selector: []string{"div.bonus_bar_box", "body"},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:你当前每小时能获取|你當前每小時能獲取|您当前每小时能获取|您當前每小時能獲取)[^\d-]{0,20}([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
		},
	},
	Selectors: &v2.SiteSelectors{
		TableRows: "table.torrent_list > tbody > tr:has(td.t_name), table.torrent_list > tr:has(td.t_name)",
		Title:     "td.t_name h3 a[href*='details.php']",
		TitleLink: "td.t_name h3 a[href*='details.php']",
		SubtitleSelector: &v2.FieldSelector{
			Selector: []string{"td.t_name h4"},
			Filters: []v2.Filter{
				// 推荐的种子副标题前面有「[推荐]」（<b>[<font class="recommended">推荐</font>]</b>），详情页的副标题没有
				{Name: "regexReplace", Args: []any{`^\[(?:推荐|推薦)\]`, ""}},
				{Name: "trim"},
			},
		},
		Size:     "td.t_size",
		Seeders:  "td.t_torrents",
		Leechers: "td.t_leech",
		Snatched: "td.t_completed",
		// 补上优惠状态以后：<td class="discount"><img class="pro_free" …><span title="2026-12-07 11:45:28">18天20时</span></td>
		DiscountIcon:       "td.discount img[class*='pro_']",
		DiscountMapping:    hdwingDiscountMapping,
		DiscountEndTime:    "td.discount span[title]",
		Category:           "td.t_cat img[alt]",
		UploadTime:         "td.t_time span[title]",
		DetailDownloadLink: "a.torrentdown_button[href*='download.php?hash='], a[href*='download.php?hash=']",
		DetailSubtitle:     "div.m_name h3",
	},
	DetailParser: &v2.DetailParserConfig{
		TimeLayout:       "2006-01-02 15:04:05",
		DiscountMapping:  hdwingDiscountMapping,
		TitleSelector:    "input[name='torrent_name']",
		IDSelector:       "input[name='detail_torrent_id']",
		DiscountSelector: "h2#top img[class*='pro_']",
		EndTimeSelector:  "h2#top span[title]",
		// <li class="left">基本信息</li><li class="right bspace">…<i><b>大小：</b>110.10 GB</i>…</li>
		SizeSelector: "li.left:contains('基本信息')",
		SizeRegex:    `大小[：:][\s\p{Zs}]*([\d.]+)[\s\p{Zs}]*([KMGT]i?B)`,
	},
	LevelRequirements: hdwingLevelRequirements,
}

// hdwingDiscountMapping 是优惠图标的类名。样本里见到的是 pro_free、pro_2up、pro_free2up、pro_50pctdown；
// pro_30pctdown、pro_50pctdown2up 是上游 NexusPHP 另外两种（列表的优惠筛选里也有 30%、2X 50%）。
var hdwingDiscountMapping = map[string]v2.DiscountLevel{
	"pro_free":         v2.DiscountFree,
	"pro_2up":          v2.Discount2xUp,
	"pro_free2up":      v2.Discount2xFree,
	"pro_50pctdown":    v2.DiscountPercent50,
	"pro_30pctdown":    v2.DiscountPercent30,
	"pro_50pctdown2up": v2.Discount2x50,
}

// hdwingLevelRequirements 按报告人抄下的站点规则（rules.php「网站等级规则」，2026-11-18）：注册周数、下载量、
// 分享率三项都要满足（站规写的是「分享率大于」，这里按不低于算）。个人页的等级写成「群众演员(User)」，
// 英文名是上游 NexusPHP 同一级的名字。
var hdwingLevelRequirements = []v2.SiteLevelRequirement{
	{
		ID: 1, Name: "群众演员", NameAka: []string{"群众演员(User)", "User"},
		Privilege: "默认的新用户等级",
	},
	{
		ID: 2, Name: "客串龙套", NameAka: []string{"客串龙套(Power User)", "Power User"},
		Interval: "P5W", Downloaded: "200GB", Ratio: 1.5,
	},
	{
		ID: 3, Name: "领衔主演", NameAka: []string{"领衔主演(Elite User)", "Elite User"},
		Interval: "P10W", Downloaded: "500GB", Ratio: 2.0,
	},
	{
		ID: 4, Name: "成名大腕", NameAka: []string{"成名大腕(Crazy User)", "Crazy User"},
		Interval: "P15W", Downloaded: "1TB", Ratio: 2.5,
	},
	{
		ID: 5, Name: "金牌经纪", NameAka: []string{"金牌经纪(Insane User)", "Insane User"},
		Interval: "P20W", Downloaded: "1.5TB", Ratio: 3.0,
	},
	{
		ID: 6, Name: "最佳导演", NameAka: []string{"最佳导演(Veteran User)", "Veteran User"},
		Interval: "P25W", Downloaded: "2TB", Ratio: 4.0,
	},
	{
		ID: 7, Name: "大制片人", NameAka: []string{"大制片人(Extreme User)", "Extreme User"},
		Interval: "P30W", Downloaded: "3TB", Ratio: 5.0,
	},
	{
		ID: 8, Name: "国际巨星", NameAka: []string{"国际巨星(Ultimate User)", "Ultimate User"},
		Interval: "P40W", Downloaded: "4TB", Ratio: 6.0,
	},
	{
		ID: 9, Name: "影业帝国", NameAka: []string{"影业帝国(Nexus Master)", "Nexus Master"},
		Interval: "P50W", Downloaded: "5TB", Ratio: 8.0,
	},
	{
		ID: v2.MinVipLevelID, Name: "VIP",
		GroupType: v2.LevelGroupVIP,
		Privilege: "捐赠人和广告支持，合作商家等",
	},
	{
		ID: v2.MinVipLevelID + 1, Name: "荣誉会员",
		GroupType: v2.LevelGroupVIP,
		Privilege: "为 HDChina 做出特殊贡献的用户",
	},
	{
		ID: v2.MinManagerLevelID, Name: "Uploader",
		GroupType: v2.LevelGroupManager,
		Privilege: "可以上传种子的会员，属于高级别会员",
	},
	{
		ID: v2.MinManagerLevelID + 1, Name: "编辑",
		GroupType: v2.LevelGroupManager,
		Privilege: "HDC 编辑部工作人员",
	},
	{
		ID: v2.MinManagerLevelID + 2, Name: "Helper",
		GroupType: v2.LevelGroupManager,
		Privilege: "网站协助管理员，处理一些日常小事务",
	},
	// 按名字认等级时看的是「名字里包含个人页上的等级」：Moderator 写在 Forum Moderator 前面，总版主才不会被认成论坛版主
	{
		ID: v2.MinManagerLevelID + 3, Name: "Moderator",
		GroupType: v2.LevelGroupManager,
		Privilege: "总版主",
	},
	{
		ID: v2.MinManagerLevelID + 4, Name: "Forum Moderator",
		GroupType: v2.LevelGroupManager,
		Privilege: "论坛版主",
	},
	{
		ID: v2.MinManagerLevelID + 5, Name: "Administrator",
		GroupType: v2.LevelGroupManager,
		Privilege: "管理员",
	},
	{
		ID: v2.MinManagerLevelID + 6, Name: "SysOP",
		GroupType: v2.LevelGroupManager,
		Privilege: "网站法官，有调整任何内容的权利",
	},
}

func init() {
	v2.RegisterSiteDefinition(HDWinGDefinition)
}
