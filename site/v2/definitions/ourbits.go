package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

var OurBitsDefinition = &v2.SiteDefinition{
	ID:             "ourbits",
	Name:           "OurBits",
	Aka:            []string{"OB", "我们的部落"},
	Description:    "OurBits — NexusPHP 综合性 PT 站点",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://ourbits.club/"},
	FaviconURL:     "https://ourbits.club/favicon.ico",
	AuthMethod:     v2.AuthMethodCookie,
	TimezoneOffset: "+0800",
	RateLimit:      0.5,
	RateBurst:      2,
	HREnabled:      true,
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
					"name", "uploaded", "downloaded", "ratio", "levelName", "joinTime", "lastAccessAt",
					"trueUploaded", "trueDownloaded",
				},
			},
			{
				RequestConfig: v2.RequestConfig{URL: "/mybonus.php", ResponseType: "document"},
				Fields:        []string{"bonusPerHour"},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			"id": {
				Selector: []string{"#info_block a.User_Name", "#info_block a[href*='userdetails.php']"},
				Attr:     "href",
				Filters:  []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{"#info_block a.User_Name", "#info_block a[href*='userdetails.php']"},
			},
			"uploaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('上传量') + td", "#info_block"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`(?:上传量|Uploaded)[^\d]*([\d.,]+\s*[KMGTP]?i?B)`}}, {Name: "parseSize"}},
			},
			"downloaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('下载量') + td", "#info_block"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`(?:下载量|Downloaded)[^\d]*([\d.,]+\s*[KMGTP]?i?B)`}}, {Name: "parseSize"}},
			},
			"ratio": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('分享率') + td", "#info_block"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`(?:分享率|Ratio)(?:<[^>]*>|[\s\p{Zs}：:])*([\d.,]+|∞|Inf|---)`}}, {Name: "parseNumber"}},
			},
			"levelName": {
				Selector: []string{"td.rowhead:contains('等级') + td > img", "td.rowhead:contains('等级') + td img", "td.rowhead:contains('Class') + td img"},
				Attr:     "title",
			},
			"bonus": {
				Selector: []string{"#info_block", "td.rowhead:contains('魔力值') + td", "td.rowhead:contains('Bonus') + td"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`(?:魔力值|Bonus)[^\d]*?\[<a[^>]*>使用</a>\][:：]?\s*([\d.,]+)`}}, {Name: "parseNumber"}},
			},
			"joinTime": {
				Selector: []string{"td.rowhead:contains('加入日期') + td", "td.rowhead:contains('Join') + td"},
				Filters:  []v2.Filter{{Name: "split", Args: []any{" (", 0}}, {Name: "parseTime"}},
			},
			"lastAccessAt": {
				Selector: []string{
					"td.rowhead:contains('最近动向') + td",
					"td.rowhead:contains('最近動向') + td",
					"td.rowhead:contains('最后活动') + td",
					"td.rowhead:contains('最後活動') + td",
					"td.rowhead:contains('上次访问') + td",
					"td.rowhead:contains('上次訪問') + td",
					"td.rowhead:contains('Last access') + td",
				},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			"seeding": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`class="arrowup"[^>]*/>\s*(\d+)`}}, {Name: "parseNumber"}},
			},
			"leeching": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`class="arrowdown"[^>]*/>\s*(\d+)`}}, {Name: "parseNumber"}},
			},
			"trueUploaded": {
				Selector: []string{"td.rowhead:contains('传输') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:真实上传量|实际上传量)[^\d]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"trueDownloaded": {
				Selector: []string{"td.rowhead:contains('传输') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:真实下载量|实际下载量)[^\d]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"bonusPerHour": {
				Selector: []string{"#outer", "body"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:你当前每小时能获取|您当前每小时能获取)[^\d-]{0,20}([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
		},
	},
	Selectors: &v2.SiteSelectors{
		TableRows: "table#torrenttable > tbody > tr:has(table.torrentname), table#torrenttable > tr:has(table.torrentname), table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		Title:     "table.torrentname a[href*='details.php']",
		TitleLink: "table.torrentname a[href*='details.php']",
		// 副标题是 <br> 之后的裸文本，前面可能隔着一块标签 <div>（官方、中字……），取名称单元格的 html 用正则截取
		SubtitleSelector: &v2.FieldSelector{
			Selector: []string{"table.torrentname td.embedded:has(a[href*='details.php'])"},
			Attr:     "html",
			Filters: []v2.Filter{
				{Name: "regex", Args: []any{`(?s)<br\s*/?>\s*(?:<div[^>]*>.*?</div>)?\s*([^<]+)`}},
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
		Size:     "td.rowfollow:nth-child(5)",
		Seeders:  "td.rowfollow:nth-child(6)",
		Leechers: "td.rowfollow:nth-child(7)",
		Snatched: "td.rowfollow:nth-child(8)",
		// 促销的写法跟着用户的「控制面板 → 网站设定 → 促销种子显示方式」：图标（默认）是 img.pro_*，
		// 文字是 [<font class='…'>…</font>]（与详情页标题里的一样）；选「高亮」或「不显示」时列表里没有促销。
		DiscountIcon: "img.pro_free, img.pro_free2up, img.pro_50pctdown, img.pro_30pctdown, img.pro_2up, img.pro_50pctdown2up, " +
			"table.torrentname font.free, table.torrentname font.twoupfree, table.torrentname font.twoup, " +
			"table.torrentname font.halfdown, table.torrentname font.twouphalfdown, table.torrentname font.thirtypercent",
		// 图标与文字两种类名都列上：驱动默认的对照里 pro_50pctdown2up 会先命中「50pct」当成 50%，文字的类名都认不出
		DiscountMapping: map[string]v2.DiscountLevel{
			"pro_free":         v2.DiscountFree,
			"pro_free2up":      v2.Discount2xFree,
			"pro_2up":          v2.Discount2xUp,
			"pro_50pctdown":    v2.DiscountPercent50,
			"pro_50pctdown2up": v2.Discount2x50,
			"pro_30pctdown":    v2.DiscountPercent30,
			"free":             v2.DiscountFree,
			"twoupfree":        v2.Discount2xFree,
			"twoup":            v2.Discount2xUp,
			"halfdown":         v2.DiscountPercent50,
			"twouphalfdown":    v2.Discount2x50,
			"thirtypercent":    v2.DiscountPercent30,
		},
		// 促销后面跟着「剩余时间：<b><span title="结束时刻">」；没有这一段时驱动回退到促销图标或文字的 onmouseover 提示
		DiscountEndTime:    "table.torrentname td.embedded > b > span[title]",
		HRIcon:             "img.hitandrun",
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
		HRKeywords:       []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run", "H&R"},
		DiscountSelector: "h1 font.free, h1 font[class]",
		EndTimeSelector:  "h1 span[title]",
	},
}

func init() {
	v2.RegisterSiteDefinition(OurBitsDefinition)
}
