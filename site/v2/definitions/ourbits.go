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
					"name", "uploaded", "downloaded", "ratio", "levelName", "joinTime",
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
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`(?:分享率|Ratio)[^\d∞]*([\d.,]+|∞|Inf|---)`}}, {Name: "parseNumber"}},
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
		TableRows:          "table#torrenttable > tbody > tr:has(table.torrentname), table#torrenttable > tr:has(table.torrentname), table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		Title:              "table.torrentname a[href*='details.php']",
		TitleLink:          "table.torrentname a[href*='details.php']",
		Subtitle:           "table.torrentname td.embedded > span:not(.optiontag)",
		Size:               "td.rowfollow:nth-child(5)",
		Seeders:            "td.rowfollow:nth-child(6)",
		Leechers:           "td.rowfollow:nth-child(7)",
		Snatched:           "td.rowfollow:nth-child(8)",
		DiscountIcon:       "img.pro_free, img.pro_free2up, img.pro_50pctdown, img.pro_30pctdown, img.pro_2up, img.pro_50pctdown2up",
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
