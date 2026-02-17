package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// NovaHDDefinition is the site definition for NovaHD
var NovaHDDefinition = &v2.SiteDefinition{
	ID:             "novahd",
	Name:           "NovaHD",
	Aka:            []string{"Nova"},
	Description:    "NovaHD PT站点",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://pt.novahd.top/"},
	FaviconURL:     "https://pt.novahd.top/favicon.ico",
	TimezoneOffset: "+0800",
	RateLimit:      0.5,
	RateBurst:      2,
	UserInfo: &v2.UserInfoConfig{
		PickLast:     []string{"id"},
		RequestDelay: 500,
		Process: []v2.UserInfoProcess{
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/index.php",
					ResponseType: "document",
				},
				Fields: []string{"id", "name", "seeding", "leeching"},
			},
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/userdetails.php",
					ResponseType: "document",
				},
				Assertion: map[string]string{"id": "params.id"},
				Fields: []string{
					"name", "uploaded", "downloaded", "ratio", "levelName",
					"bonus", "joinTime", "messageCount",
				},
			},
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/mybonus.php",
					ResponseType: "document",
				},
				Fields: []string{"bonusPerHour"},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			"id": {
				Selector: []string{
					"a[href*='userdetails.php'][class*='Name']",
					"a.User_Name[href*='userdetails.php']",
					"#info_block a[href*='userdetails.php']",
					"a[href*='userdetails.php']",
				},
				Attr:    "href",
				Filters: []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{
					"a[href*='userdetails.php'][class*='Name']",
					"a.User_Name[href*='userdetails.php']",
					"#info_block a[href*='userdetails.php']",
				},
			},
			"uploaded": {
				Selector: []string{
					"td.rowhead:contains('上传量') + td",
					"td.rowhead:contains('上傳量') + td",
					"td.rowhead:contains('Uploaded') + td",
				},
				Filters: []v2.Filter{
					{Name: "parseSize"},
				},
			},
			"downloaded": {
				Selector: []string{
					"td.rowhead:contains('下载量') + td",
					"td.rowhead:contains('下載量') + td",
					"td.rowhead:contains('Downloaded') + td",
				},
				Filters: []v2.Filter{
					{Name: "parseSize"},
				},
			},
			"ratio": {
				Selector: []string{
					"td.rowhead:contains('分享率') + td font",
					"td.rowhead:contains('分享率') + td > font",
					"td.rowhead:contains('分享率') + td",
					"td.rowhead:contains('Ratio') + td font",
					"td.rowhead:contains('Ratio') + td",
				},
				Filters: []v2.Filter{
					{Name: "parseNumber"},
				},
			},
			"levelName": {
				Selector: []string{
					"td.rowhead:contains('等级') + td > img",
					"td.rowhead:contains('等級') + td > img",
					"td.rowhead:contains('Class') + td > img",
					"td.rowhead:contains('等级') + td img",
				},
				Attr: "title",
			},
			"bonus": {
				Selector: []string{
					"td.rowhead:contains('魔力值') + td",
					"td.rowhead:contains('魔力') + td",
					"td.rowhead:contains('Bonus') + td",
				},
				Filters: []v2.Filter{{Name: "parseNumber"}},
			},
			"bonusPerHour": {
				Selector: []string{
					"#outer td[rowspan]",
					"div:contains('你当前每小时能获取')",
				},
				Filters: []v2.Filter{{Name: "parseNumber"}},
			},
			"messageCount": {
				Text:     "0",
				Selector: []string{"td[style*='background: red'] a[href*='messages.php']"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			"joinTime": {
				Selector: []string{
					"td.rowhead:contains('加入日期') + td",
					"td.rowhead:contains('Join') + td",
				},
				Filters: []v2.Filter{
					{Name: "split", Args: []any{" (", 0}},
					{Name: "parseTime"},
				},
			},
			"seeding": {
				Selector: []string{
					"#info_block",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowup"[^>]*/>(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			"leeching": {
				Selector: []string{
					"#info_block",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowdown"[^>]*/>(\d+)`}},
					{Name: "parseNumber"},
				},
			},
		},
	},
	// Custom selectors for NovaHD search and detail pages
	Selectors: &v2.SiteSelectors{
		TableRows:          "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
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
	// Detail page parser config for NovaHD
	// NovaHD uses: <font class='free'>免费</font> and <span title="2026-02-03 10:25:55">2天23时</span>
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
		HRKeywords:       []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"},
		TitleSelector:    "h1",
		DiscountSelector: "h1 font.free, h1 font[class]",
		EndTimeSelector:  "h1 span[title]",
		SizeSelector:     "td.rowfollow:contains('大小')",
		SizeRegex:        `大小[：:]\s*([\d.]+)\s*(GB|MB|KB|TB)`,
	},
	LevelRequirements: novaHDLevelRequirements,
}

// NovaHD level requirements (standard NexusPHP levels)
var novaHDLevelRequirements = []v2.SiteLevelRequirement{
	{
		ID:        1,
		Name:      "User",
		NameAka:   []string{"用户"},
		Privilege: "新用户的默认级别",
	},
	{
		ID:         2,
		Name:       "Power User",
		NameAka:    []string{"高级用户"},
		Interval:   "P4W",
		Downloaded: "50GB",
		Ratio:      1.05,
		Privilege:  "得到一个邀请名额；可以直接发布种子；可以查看NFO文档；可以查看用户列表；可以请求续种；可以发送邀请；可以查看排行榜；可以查看其它用户的种子历史；可以删除自己上传的字幕",
	},
	{
		ID:         3,
		Name:       "Elite User",
		NameAka:    []string{"精英用户"},
		Interval:   "P8W",
		Downloaded: "120GB",
		Ratio:      1.55,
		Privilege:  "Elite User及以上用户封存账号后不会被删除",
	},
	{
		ID:         4,
		Name:       "Crazy User",
		NameAka:    []string{"疯狂用户"},
		Interval:   "P15W",
		Downloaded: "300GB",
		Ratio:      2.05,
		Privilege:  "得到两个邀请名额；可以在做种/下载/发布的时候选择匿名模式",
	},
	{
		ID:         5,
		Name:       "Insane User",
		NameAka:    []string{"变态用户"},
		Interval:   "P25W",
		Downloaded: "500GB",
		Ratio:      2.55,
		Privilege:  "可以查看普通日志",
	},
	{
		ID:         6,
		Name:       "Veteran User",
		NameAka:    []string{"资深用户"},
		Interval:   "P40W",
		Downloaded: "750GB",
		Ratio:      3.05,
		Privilege:  "得到三个邀请名额；可以查看其它用户的评论、帖子历史；Veteran User及以上用户会永远保留账号",
	},
	{
		ID:         7,
		Name:       "Extreme User",
		NameAka:    []string{"极限用户"},
		Interval:   "P60W",
		Downloaded: "1TB",
		Ratio:      3.55,
		Privilege:  "可以更新过期的外部信息；可以查看Extreme User论坛",
	},
	{
		ID:         8,
		Name:       "Ultimate User",
		NameAka:    []string{"终极用户"},
		Interval:   "P80W",
		Downloaded: "1.5TB",
		Ratio:      4.05,
		Privilege:  "得到五个邀请名额",
	},
	{
		ID:         9,
		Name:       "Nexus Master",
		NameAka:    []string{"大师"},
		Interval:   "P100W",
		Downloaded: "3TB",
		Ratio:      4.55,
		Privilege:  "得到十个邀请名额",
	},
}

func init() {
	v2.RegisterSiteDefinition(NovaHDDefinition)
}
