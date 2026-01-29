package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// TTGDefinition is the site definition for TTG (To The Glory)
var TTGDefinition = &v2.SiteDefinition{
	ID:             "ttg",
	Name:           "TTG",
	Aka:            []string{"TTG", "To The Glory"},
	Description:    "TTG (To The Glory) PT Site",
	Schema:         "NexusPHP",
	URLs:           []string{"https://totheglory.im/"},
	FaviconURL:     "https://totheglory.im/favicon.ico",
	TimezoneOffset: "+0800",
	UserInfo: &v2.UserInfoConfig{
		PickLast:     []string{"id"},
		RequestDelay: 500,
		Process: []v2.UserInfoProcess{
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/index.php",
					ResponseType: "document",
				},
				Fields: []string{"id", "name", "uploaded", "downloaded", "ratio", "seeding", "leeching", "bonus", "messageCount"},
			},
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/userdetails.php",
					ResponseType: "document",
				},
				Assertion: map[string]string{"id": "params.id"},
				Fields: []string{
					"joinTime",
					"levelName",
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
			// User ID from index.php top navigation
			// HTML: 欢迎回来，<b><a href="https://totheglory.im/userdetails.php?id=151907">392198523</a></b>
			"id": {
				Selector: []string{
					"a[href*='userdetails.php']",
				},
				Attr:    "href",
				Filters: []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			// Username from index.php top navigation
			// HTML: 欢迎回来，<b><a href="https://totheglory.im/userdetails.php?id=151907">392198523</a></b>
			"name": {
				Selector: []string{
					"a[href*='userdetails.php']",
				},
			},
			// Upload from index.php top navigation
			// HTML: <font color="green">上传量 : </font> <font color="black"><a href="..." title="4,400,128.37 MB">4.196 TB</a></font>
			"uploaded": {
				Selector: []string{
					"td.bottom",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`上传量\s*:\s*</font>\s*<font[^>]*><a[^>]*>([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			// Download from index.php top navigation
			// HTML: <font color="darkred">下载量 :</font> <font color="black"><a href="..." title="1,558,484.04 MB">1.486 TB</a></font>
			"downloaded": {
				Selector: []string{
					"td.bottom",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`下载量\s*:\s*</font>\s*<font[^>]*><a[^>]*>([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			// Ratio from index.php top navigation
			// HTML: <font color="1900D1">分享率 :</font> <font color="#000000">2.823</font>
			"ratio": {
				Selector: []string{
					"td.bottom",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`分享率\s*:\s*</font>\s*<font[^>]*>([\d.,]+|∞|Inf)`}},
					{Name: "parseNumber"},
				},
			},
			// Seeding count from index.php top navigation
			// HTML: <img alt="做种中" .../><span class="smallfont">10</span>
			"seeding": {
				Selector: []string{
					"td.bottom",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`做种中.*?smallfont[^>]*>(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			// Leeching count from index.php top navigation
			// HTML: <img alt="下载中" .../><span class="smallfont">0</span>
			"leeching": {
				Selector: []string{
					"td.bottom",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`下载中.*?smallfont[^>]*>(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			// Bonus (积分) from index.php top navigation
			// HTML: 积分 : <a href="https://totheglory.im/mybonus.php">908728.22</a>
			"bonus": {
				Selector: []string{
					"td.bottom",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`积分\s*:\s*<[^>]*>([\d,]+\.?\d*)`}},
					{Name: "parseNumber"},
				},
			},
			// BonusPerHour from mybonus.php
			// HTML: <tr><td class="rowhead">总计</td><td>27.64 分</td></tr>
			"bonusPerHour": {
				Selector: []string{
					"body",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`总计</td>.*?([\d.,]+)\s*分`}},
					{Name: "parseNumber"},
				},
			},
			// Level name from userdetails.php
			// HTML: <tr><td class="rowhead">等级</td><td align="left">...PetaByte</td></tr>
			"levelName": {
				Selector: []string{
					"td.rowhead:contains('等级') + td",
					"td.rowhead:contains('等級') + td",
					"td.rowhead:contains('Class') + td",
				},
			},
			// Join date from userdetails.php
			// HTML: <tr><td class="rowhead">注册日期</td><td align="left">2019-09-03 23:02:35</td></tr>
			"joinTime": {
				Selector: []string{
					"td.rowhead:contains('注册日期') + td",
					"td.rowhead:contains('Join') + td",
				},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			// Message count from index.php top navigation
			// HTML: <a href="messages.php?action=viewmailbox&box=1">...</a> 96 (0 <b>新</b>)
			"messageCount": {
				Text: "0",
				Selector: []string{
					"a[href*='messages.php'][href*='viewmailbox']",
				},
				Filters: []v2.Filter{
					{Name: "parentText"},
					{Name: "regex", Args: []any{`(\d+)\s*\(\s*(\d+)\s*新\)`}},
					{Name: "index", Args: []any{1}},
					{Name: "parseNumber"},
				},
			},
		},
	},
	Selectors: &v2.SiteSelectors{
		TableRows:       "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		Title:           "table.torrentname a[href*='details.php']",
		TitleLink:       "table.torrentname a[href*='details.php']",
		Subtitle:        "table.torrentname td.embedded > span:not(.tags)",
		Size:            "td.rowfollow:nth-child(5)",
		Seeders:         "td.rowfollow:nth-child(6)",
		Leechers:        "td.rowfollow:nth-child(7)",
		Snatched:        "td.rowfollow:nth-child(8)",
		DiscountIcon:    "img.pro_free, img.pro_free2up, img.pro_50pctdown, img.pro_30pctdown, img.pro_2up, img.pro_50pctdown2up",
		DiscountEndTime: "span.free_end_time[title], font.free span[title], font.twoupfree span[title]",
		Category:        "td.rowfollow:nth-child(1) img[alt]",
		UploadTime:      "td.rowfollow:nth-child(4) span[title]",
	},
	LevelRequirements: []v2.SiteLevelRequirement{
		{
			ID:        0,
			Name:      "Peasant",
			Privilege: "新人考核：从注册之日起，30天内上传下载各40G以上，分享率不低于1，做种积分5000。",
		},
		{
			ID:        1,
			Name:      "User",
			Interval:  "P5W",
			Downloaded: "60GB",
			Ratio:     1.1,
			Privilege: "此等级为正式会员，可申请种子候选。升级条件：注册满5周，下载量60G以上，分享率大于1.1。小于1.0会自动降级。",
		},
		{
			ID:         2,
			Name:       "Power User",
			Interval:   "P8W",
			Downloaded: "150GB",
			Ratio:      2.0,
			Privilege:  "升级条件：注册满8周，下载量150G以上，分享率大于2.0会升级，小于1.9会自动降级。",
		},
		{
			ID:         3,
			Name:       "Elite User",
			Interval:   "P8W",
			Downloaded: "250GB",
			Ratio:      2.0,
			Privilege:  "此等级可挂起，可进入积分商城。升级条件：注册满8周，下载量250G以上，分享率大于2.0会升级，小于1.9会自动降级。",
		},
		{
			ID:         4,
			Name:       "Crazy User",
			Interval:   "P8W",
			Downloaded: "500GB",
			Ratio:      2.5,
			Privilege:  "此等级可用积分购买邀请，并可浏览全站。升级条件：注册满8周，下载量500G以上，分享率大于2.5会升级，小于2.4会自动降级。",
		},
		{
			ID:         5,
			Name:       "Insane User",
			Interval:   "P16W",
			Downloaded: "750GB",
			Ratio:      2.5,
			Privilege:  "此等级可直接发布种子。升级条件：注册满16周，下载量750G以上，分享率大于2.5会升级，低于2.4会自动降级。",
		},
		{
			ID:         6,
			Name:       "Veteran User",
			Interval:   "P24W",
			Downloaded: "1TB",
			Ratio:      3.0,
			Privilege:  "此等级自行挂起账号后不会被清除。升级条件：注册满24周，下载量1TB以上，分享率大于3.0会升级，低于2.9自动降级。",
		},
		{
			ID:         7,
			Name:       "Extreme User",
			Interval:   "P24W",
			Downloaded: "1.5TB",
			Ratio:      3.5,
			Privilege:  "此等级免除流量考核。升级条件：注册满24周，下载量1.5TB以上，分享率大于3.5会升级，低于3.4自动降级。",
		},
		{
			ID:         8,
			Name:       "Ultimate User",
			Interval:   "P24W",
			Downloaded: "2.5TB",
			Ratio:      4.0,
			Privilege:  "此等级可查看排行榜。升级条件：注册满24周，下载量2.5TB以上，分享率大于4.0会升级，低于3.9会自动降级。",
		},
		{
			ID:         9,
			Name:       "Nexus Master",
			Interval:   "P32W",
			Downloaded: "3.5TB",
			Ratio:      5.0,
			Privilege:  "此等级及以上用户会永远保留账号。升级条件：注册满32周，下载量3.5TB以上，分享率大于5.0会升级，低于4.9会自动降级。",
		},
		{
			ID:         10,
			Name:       "NonaByte",
			Interval:   "P48W",
			Downloaded: "5TB",
			Uploaded:   "50TB",
			Ratio:      6.0,
			Privilege:  "升级条件：注册满48周，上传量50TB以上，下载量5TB以上，分享率大于6.0会升级，低于5.9会自动降级。",
		},
		{
			ID:         11,
			Name:       "DoggaByte",
			Interval:   "P48W",
			Downloaded: "10TB",
			Uploaded:   "100TB",
			Ratio:      6.0,
			Privilege:  "升级条件：注册满48周，上传量100TB以上，下载量10TB以上，分享率大于6.0会升级，低于5.9会自动降级。",
		},
		{
			ID:        100,
			Name:      "VIP",
			GroupType: v2.LevelGroupVIP,
			Privilege: "为TTG做出特殊重大贡献的用户或合作者等。只计算上传量，不计算下载量。",
		},
	},
}

func init() {
	v2.RegisterSiteDefinition(TTGDefinition)
}
