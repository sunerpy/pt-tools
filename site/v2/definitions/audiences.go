package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

var AudiencesDefinition = &v2.SiteDefinition{
	ID:             "audiences",
	Name:           "Audiences",
	Aka:            []string{"audiences.me", "AD"},
	Description:    "Audiences 私人影视站",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://audiences.me/"},
	FaviconURL:     "https://audiences.me/favicon.ico",
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
				Fields:        []string{"id", "name", "bonus", "seeding", "leeching"},
			},
			{
				RequestConfig: v2.RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
				Assertion:     map[string]string{"id": "params.id"},
				Fields:        []string{"name", "uploaded", "downloaded", "ratio", "levelName", "bonus", "joinTime"},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			"id": {
				Selector: []string{"a[href*='userdetails.php'][class*='Name']", "#info_block a[href*='userdetails.php']", "a[href*='userdetails.php']"},
				Attr:     "href",
				Filters:  []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{"a[href*='userdetails.php'][class*='Name']", "#info_block a[href*='userdetails.php']", "h1 .nowrap b"},
			},
			"uploaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('Transfer') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>上传量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"downloaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('Transfer') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>下载量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"ratio": {
				Selector: []string{"td.rowhead:contains('传输') + td", "#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:分享率|Ratio)(?:</strong>)?[：:\s]*(?:<font[^>]*>)?([\d.,]+|∞|Inf)`}},
					{Name: "parseNumber"},
				},
			},
			"levelName": {
				Selector: []string{"td.rowhead:contains('等级') + td img", "td.rowhead:contains('Class') + td img"},
				Attr:     "title",
			},
			"bonus": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:爆米花|爆米花系统)[^\d]*([\d.,]+)`}},
					{Name: "parseNumber"},
				},
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
		},
	},
	Selectors: &v2.SiteSelectors{
		TableRows:          "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		Title:              "table.torrentname a[href*='details.php']",
		TitleLink:          "table.torrentname a[href*='details.php']",
		Subtitle:           "table.torrentname td.embedded > span[style*='line-height']",
		Size:               "td.rowfollow:nth-child(5)",
		Seeders:            "td.rowfollow:nth-child(6)",
		Leechers:           "td.rowfollow:nth-child(7)",
		Snatched:           "td.rowfollow:nth-child(8)",
		DiscountIcon:       "img.pro_free, img.pro_free2up, img.pro_50pctdown, img.pro_30pctdown, img.pro_2up, img.pro_50pctdown2up",
		Category:           "td.rowfollow:nth-child(1) img[alt]",
		UploadTime:         "td.rowfollow:nth-child(4) span[title]",
		HRIcon:             "img.hitandrun, img[alt*='H&R'], img[title*='H&R']",
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
		TitleSelector:    "input[name='torrent_name']",
		IDSelector:       "input[name='detail_torrent_id']",
		DiscountSelector: "h1 font.free, h1 font.twoupfree, h1 font[class]",
		EndTimeSelector:  "h1 span[title]",
		SizeSelector:     "td.rowhead:contains('基本信息')",
		SizeRegex:        `大小[：:]\s*([\d.]+)\s*(GB|MB|KB|TB)`,
	},
}

func init() {
	v2.RegisterSiteDefinition(AudiencesDefinition)
}
