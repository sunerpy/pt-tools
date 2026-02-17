package definitions

import (
	"time"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// HDSky new level requirements (for users joined after 2025-03-01)
var hdSkyNewLevelRequirements = []v2.SiteLevelRequirement{
	{
		ID:        1,
		Name:      "User",
		NameAka:   []string{"新人"},
		Privilege: "新用户的默认级别。",
	},
	{
		ID:         2,
		Name:       "Power User",
		NameAka:    []string{"配角"},
		Interval:   "P5W",
		Downloaded: "200GB",
		Ratio:      2.0,
		Bonus:      600000,
		Privilege:  "NFO文档；请求续种；查看其它用户的种子历史；删除自己上传的字幕",
	},
	{
		ID:         3,
		Name:       "Elite User",
		NameAka:    []string{"主角"},
		Interval:   "P10W",
		Downloaded: "500GB",
		Ratio:      2.5,
		Bonus:      800000,
		Privilege:  "查看邀请区",
	},
	{
		ID:         4,
		Name:       "Crazy User",
		NameAka:    []string{"主演"},
		Interval:   "P15W",
		Downloaded: "1TB",
		Ratio:      3.0,
		Bonus:      1000000,
		Privilege:  "在做种/下载/发布的时候选择匿名模式",
	},
	{
		ID:         5,
		Name:       "Insane User",
		NameAka:    []string{"明星"},
		Interval:   "P20W",
		Downloaded: "2TB",
		Ratio:      3.5,
		Bonus:      1500000,
		Privilege:  "查看普通日志",
	},
	{
		ID:         6,
		Name:       "Veteran User",
		NameAka:    []string{"巨星"},
		Interval:   "P25W",
		Downloaded: "4TB",
		Ratio:      4.0,
		Bonus:      2000000,
		Privilege:  "封存账号后不会被删除；查看其它用户的评论、帖子历史",
	},
	{
		ID:         7,
		Name:       "Extreme User",
		NameAka:    []string{"天王"},
		Interval:   "P30W",
		Downloaded: "6TB",
		Ratio:      4.5,
		Bonus:      2500000,
		Privilege:  "更新过期的外部信息；查看Extreme User论坛",
	},
	{
		ID:         8,
		Name:       "Ultimate User",
		NameAka:    []string{"宗师"},
		Interval:   "P45W",
		Downloaded: "8TB",
		Ratio:      5.0,
		Bonus:      3500000,
		Privilege:  "永远保留账号",
	},
	{
		ID:         9,
		Name:       "Nexus Master",
		NameAka:    []string{"大师"},
		Interval:   "P65W",
		Downloaded: "10TB",
		Ratio:      5.5,
		Bonus:      5000000,
		Privilege:  "直接发布种子；可以查看排行榜；在网站开放邀请期间发送邀请",
	},
}

// HDSky old level requirements (for users joined before 2025-03-01, without bonus requirement)
var hdSkyOldLevelRequirements = []v2.SiteLevelRequirement{
	{
		ID:        1,
		Name:      "User",
		NameAka:   []string{"新人"},
		Privilege: "新用户的默认级别。",
	},
	{
		ID:         2,
		Name:       "Power User",
		NameAka:    []string{"配角"},
		Interval:   "P5W",
		Downloaded: "200GB",
		Ratio:      2.0,
		Privilege:  "NFO文档；请求续种；查看其它用户的种子历史；删除自己上传的字幕",
	},
	{
		ID:         3,
		Name:       "Elite User",
		NameAka:    []string{"主角"},
		Interval:   "P10W",
		Downloaded: "500GB",
		Ratio:      2.5,
		Privilege:  "查看邀请区",
	},
	{
		ID:         4,
		Name:       "Crazy User",
		NameAka:    []string{"主演"},
		Interval:   "P15W",
		Downloaded: "1TB",
		Ratio:      3.0,
		Privilege:  "在做种/下载/发布的时候选择匹配模式",
	},
	{
		ID:         5,
		Name:       "Insane User",
		NameAka:    []string{"明星"},
		Interval:   "P20W",
		Downloaded: "2TB",
		Ratio:      3.5,
		Privilege:  "查看普通日志",
	},
	{
		ID:         6,
		Name:       "Veteran User",
		NameAka:    []string{"巨星"},
		Interval:   "P25W",
		Downloaded: "4TB",
		Ratio:      4.0,
		Privilege:  "封存账号后不会被删除；查看其它用户的评论、帖子历史",
	},
	{
		ID:         7,
		Name:       "Extreme User",
		NameAka:    []string{"天王"},
		Interval:   "P30W",
		Downloaded: "6TB",
		Ratio:      4.5,
		Privilege:  "更新过期的外部信息；查看Extreme User论坛",
	},
	{
		ID:         8,
		Name:       "Ultimate User",
		NameAka:    []string{"宗师"},
		Interval:   "P45W",
		Downloaded: "8TB",
		Ratio:      5.0,
		Privilege:  "永远保留账号",
	},
	{
		ID:         9,
		Name:       "Nexus Master",
		NameAka:    []string{"大师"},
		Interval:   "P65W",
		Downloaded: "10TB",
		Ratio:      5.5,
		Privilege:  "直接发布种子；可以查看排行榜；在网站开放邀请期间发送邀请",
	},
}

// HDSkyDefinition is the site definition for HDSky
var HDSkyDefinition = &v2.SiteDefinition{
	ID:             "hdsky",
	Name:           "HDSky",
	Aka:            []string{"HDS", "天空"},
	Description:    "高清发烧友后花园PT",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://hdsky.me/"},
	FaviconURL:     "https://hdsky.me/favicon.ico",
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
					"bonus", "seedingBonus", "joinTime",
					"hnrUnsatisfied", "hnrPreWarning", "messageCount",
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
					// HDSky uses "传输" row with combined upload/download info
					"td.rowhead:contains('传输') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('Transfer') + td",
					// Standard NexusPHP format (fallback)
					"td.rowhead:contains('上传量') + td",
					"td.rowhead:contains('上傳量') + td",
					"td.rowhead:contains('Uploaded') + td",
				},
				Attr: "html", // Get HTML to preserve structure
				Filters: []v2.Filter{
					// HDSky format: <strong>上传量</strong>:  17.020 TB
					{Name: "regex", Args: []any{`上[传傳][量]?</strong>[：:\s]*\s*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"downloaded": {
				Selector: []string{
					// HDSky uses "传输" row with combined upload/download info
					"td.rowhead:contains('传输') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('Transfer') + td",
					// Standard NexusPHP format (fallback)
					"td.rowhead:contains('下载量') + td",
					"td.rowhead:contains('下載量') + td",
					"td.rowhead:contains('Downloaded') + td",
				},
				Attr: "html", // Get HTML to preserve structure
				Filters: []v2.Filter{
					// HDSky format: <strong>下载量</strong>:  1.499 TB
					{Name: "regex", Args: []any{`下[载載][量]?</strong>[：:\s]*\s*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"ratio": {
				Selector: []string{
					// HDSky uses "传输" row with combined info
					"td.rowhead:contains('传输') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('Transfer') + td",
					// Standard NexusPHP format (fallback)
					"td.rowhead:contains('分享率') + td font",
					"td.rowhead:contains('分享率') + td > font",
					"td.rowhead:contains('分享率') + td",
					"td.rowhead:contains('Ratio') + td font",
					"td.rowhead:contains('Ratio') + td",
				},
				Attr: "html", // Get HTML to preserve structure
				Filters: []v2.Filter{
					// HDSky format: <strong>分享率</strong>:  <font color="">11.356</font>
					{Name: "regex", Args: []any{`分享率</strong>[：:\s]*\s*(?:<font[^>]*>)?([\d.,]+|∞|Inf)`}},
					{Name: "parseNumber"},
				},
			},
			"levelName": {
				Selector: []string{
					"td.rowhead:contains('等级') + td > img",
					"td.rowhead:contains('等級') + td > img",
					"td.rowhead:contains('Class') + td > img",
					// 备用选择器：直接在 td 中查找 img
					"td.rowhead:contains('等级') + td img",
					"td.rowhead:contains('等級') + td img",
					"td.rowhead:contains('Class') + td img",
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
			"seedingBonus": {
				Selector: []string{
					"td.rowhead:contains('做种积分') + td",
					"td.rowhead:contains('做種積分') + td",
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
					// HDSky index.php: info_block format - number follows img.arrowup
					"#info_block",
				},
				Attr: "html", // Get HTML to match against img tags
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowup"[^>]*/>(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			"leeching": {
				Selector: []string{
					// HDSky index.php: info_block format - number follows img.arrowdown
					"#info_block",
				},
				Attr: "html", // Get HTML to match against img tags
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowdown"[^>]*/>(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			"hnrUnsatisfied": {
				Text:     "0",
				Selector: []string{"#info_block a[href*='myhr.php']"},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`\d+\s*/\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			"hnrPreWarning": {
				Text:     "0",
				Selector: []string{"#info_block a[href*='myhr.php']"},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`^(\d+)`}},
					{Name: "parseNumber"},
				},
			},
		},
	},
	// Custom selectors for HDSky search and detail pages
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
		DetailDownloadLink: "td.rowhead:contains('下载链接') + td a[href*='download.php']",
		DetailSubtitle:     "td.rowhead:contains('副标题') + td",
	},
	LevelRequirements: hdSkyNewLevelRequirements,
}

// HDSkyNewRequirementsDate is the date after which new level requirements apply
var HDSkyNewRequirementsDate = time.Date(2025, 3, 1, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))

// GetHDSkyLevelRequirements returns appropriate level requirements based on join date
func GetHDSkyLevelRequirements(joinTime int64) []v2.SiteLevelRequirement {
	if joinTime == 0 {
		return hdSkyNewLevelRequirements
	}
	joinDate := time.Unix(joinTime, 0)
	if joinDate.Before(HDSkyNewRequirementsDate) {
		return hdSkyOldLevelRequirements
	}
	return hdSkyNewLevelRequirements
}

func init() {
	v2.RegisterSiteDefinition(HDSkyDefinition)
}
