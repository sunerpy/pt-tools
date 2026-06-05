package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

var NicePTDefinition = &v2.SiteDefinition{
	ID:             "nicept",
	Name:           "NicePT",
	Aka:            []string{"NicePT", "好趣", "www.nicept.net"},
	Description:    "NicePT — NexusPHP PT 站点（繁体界面，www.nicept.net）",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://www.nicept.net/"},
	FaviconURL:     "https://www.nicept.net/favicon.ico",
	TimezoneOffset: "+0800",
	RateLimit:      0.5,
	RateBurst:      2,
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
				Selector: []string{"a[href*='userdetails.php'][class*='Name']", "#info_block a[href*='userdetails.php']", "a[href*='userdetails.php']"},
				Attr:     "href",
				Filters:  []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{"a[href*='userdetails.php'][class*='Name']", "#info_block a[href*='userdetails.php']"},
			},
			"uploaded": {
				Selector: []string{"td.rowhead:contains('傳送') + td", "td.rowhead:contains('传输') + td", "td.rowhead:contains('上傳量') + td", "td.rowhead:contains('上传量') + td", "#info_block"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`(?:上傳量|上传量|Uploaded)[^\d]*([\d.,]+\s*[KMGTP]?i?B)`}}, {Name: "parseSize"}},
			},
			"downloaded": {
				Selector: []string{"td.rowhead:contains('傳送') + td", "td.rowhead:contains('传输') + td", "td.rowhead:contains('下載量') + td", "td.rowhead:contains('下载量') + td", "#info_block"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`(?:下載量|下载量|Downloaded)[^\d]*([\d.,]+\s*[KMGTP]?i?B)`}}, {Name: "parseSize"}},
			},
			"ratio": {
				Selector: []string{"td.rowhead:contains('傳送') + td", "td.rowhead:contains('传输') + td", "td.rowhead:contains('分享率') + td", "#info_block"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`(?:分享率|Ratio)[^\d∞]*([\d.,]+|∞|Inf|---)`}}, {Name: "parseNumber"}},
			},
			"levelName": {
				Selector: []string{"td.rowhead:contains('等級') + td > img", "td.rowhead:contains('等级') + td > img", "td.rowhead:contains('Class') + td > img"},
				Attr:     "title",
			},
			"bonus": {
				Selector: []string{"#info_block", "td.rowhead:contains('魔力值') + td", "td.rowhead:contains('Bonus') + td"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`(?:魔力值|Bonus)[^\d]*([\d.,]+)`}}, {Name: "parseNumber"}},
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
				Selector: []string{
					"td.rowhead:contains('傳送') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('传输') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:實際上傳量|实际上传量|實際上傳|实际上传)</strong>[\s:：]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"trueDownloaded": {
				Selector: []string{
					"td.rowhead:contains('傳送') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('传输') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:實際下載量|实际下载量|實際下載|实际下载)</strong>[\s:：]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"bonusPerHour": {
				Selector: []string{
					"#outer",
					"body",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:你當前每小時能獲取|你当前每小时能获取|您當前每小時能獲取|您当前每小时能获取)[^\d-]{0,20}([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
		},
	},
	Selectors: &v2.SiteSelectors{
		TableRows:          "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		Title:              "table.torrentname a[href*='details.php']",
		TitleLink:          "table.torrentname a[href*='details.php']",
		Subtitle:           "table.torrentname td.embedded > span:last-of-type, table.torrentname td.embedded > span:not(.optiontag)",
		Size:               "td.rowfollow:nth-child(5)",
		Seeders:            "td.rowfollow:nth-child(6)",
		Leechers:           "td.rowfollow:nth-child(7)",
		Snatched:           "td.rowfollow:nth-child(8)",
		DiscountIcon:       "img.pro_free, img.pro_free2up, img.pro_50pctdown, img.pro_30pctdown, img.pro_2up, img.pro_50pctdown2up",
		Category:           "td.rowfollow:nth-child(1) img[alt]",
		UploadTime:         "td.rowfollow:nth-child(4) span[title]",
		DetailDownloadLink: "td.rowhead:contains('下載連結') + td a[href*='download.php'], td.rowhead:contains('下载链接') + td a[href*='download.php'], td.rowhead:contains('下載') + td a[href*='download.php']",
		DetailSubtitle:     "td.rowhead:contains('副標題') + td, td.rowhead:contains('副标题') + td",
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
		SizeSelector:     "td.rowhead:contains('基本資訊'), td.rowhead:contains('基本信息')",
		SizeRegex:        `大小[：:]\s*([\d.]+)\s*(GB|MB|KB|TB)`,
	},
}

func init() {
	v2.RegisterSiteDefinition(NicePTDefinition)
}
