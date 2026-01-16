package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// HDDolbyDefinition is the site definition for HD Dolby
// Note: HDDolby may require 2FA verification, which can limit access to userdetails.php
// Most data is fetched from index.php info_block instead
var HDDolbyDefinition = &v2.SiteDefinition{
	ID:             "hddolby",
	Name:           "HD Dolby",
	Aka:            []string{"高清杜比"},
	Description:    "高清杜比",
	Schema:         "NexusPHP",
	URLs:           []string{"https://www.hddolby.com/"},
	LegacyURLs:     []string{"https://hddolby.com/"},
	FaviconURL:     "https://www.hddolby.com/favicon.ico",
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
				Fields: []string{"id", "name", "seeding", "leeching", "bonus", "seedingBonus", "uploaded", "downloaded", "ratio", "levelName"},
			},
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/userdetails.php",
					ResponseType: "document",
				},
				Assertion: map[string]string{"id": "params.id"},
				Fields: []string{
					"joinTime", "messageCount",
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
			// User ID from index.php info_block
			// HTML: <a href="userdetails.php?id=34059" class='EliteUser_Name'><b>sunerpy</b></a>
			"id": {
				Selector: []string{
					"#info_block a[href*='userdetails.php'][class*='_Name']",
					"#info_block a[href*='userdetails.php']",
					"a[href*='userdetails.php'][class*='_Name']",
				},
				Attr:    "href",
				Filters: []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			// Username from index.php
			"name": {
				Selector: []string{
					"#info_block a[href*='userdetails.php'][class*='_Name'] b",
					"#info_block a[href*='userdetails.php'][class*='_Name']",
					"#info_block a[href*='userdetails.php']",
				},
			},
			// Upload from index.php info_block
			// HTML: 上传量：</font>3.642 TB
			"uploaded": {
				Selector: []string{
					"#info_block",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`上传量[：:</font>\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			// Download from index.php info_block
			// HTML: 下载量：</font>383.84 GB
			"downloaded": {
				Selector: []string{
					"#info_block",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`下载量[：:</font>\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			// Ratio from index.php info_block
			// HTML: 分享率：</font> 9.715
			"ratio": {
				Selector: []string{
					"#info_block",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`分享率[：:</font>\s]*([\d.,]+|∞|Inf)`}},
					{Name: "parseNumber"},
				},
			},
			// Level name from index.php - extract from username link class
			// HTML: <a href="userdetails.php?id=34059" class='EliteUser_Name'><b>sunerpy</b></a>
			// Class format: {LevelName}_Name, e.g., EliteUser_Name, PowerUser_Name
			"levelName": {
				Selector: []string{
					"#info_block a[href*='userdetails.php'][class*='_Name']",
				},
				Attr: "class",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`^(\w+)_Name$`}},
				},
			},
			// Bonus (鲸币) from index.php info_block
			// HTML: 鲸币[使用]: 537,263.4
			"bonus": {
				Selector: []string{
					"#info_block",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`鲸币[^:：]*[：:]\s*([\d,]+\.?\d*)`}},
					{Name: "parseNumber"},
				},
			},
			// Seeding bonus from index.php
			// HTML: 做种积分: </font>883,738.0
			"seedingBonus": {
				Selector: []string{
					"#info_block",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`做种积分[：:\s</font>]*([\d,]+\.?\d*)`}},
					{Name: "parseNumber"},
				},
			},
			// BonusPerHour from mybonus.php
			"bonusPerHour": {
				Selector: []string{
					"table tr:contains('合计') td:last-child",
					"tr:contains('合计') td:last-child",
				},
				Filters: []v2.Filter{
					{Name: "split", Args: []any{"/", 0}},
					{Name: "parseNumber"},
				},
			},
			// Seeding count from index.php
			// HTML: <img class="arrowup" ... title="当前做种" .../>66
			"seeding": {
				Selector: []string{
					"#info_block",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`title="当前做种"[^>]*/>(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			// Leeching count from index.php
			// HTML: <img class="arrowdown" ... title="当前下载" .../>0
			"leeching": {
				Selector: []string{
					"#info_block",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`title="当前下载"[^>]*/>(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			// Join date from userdetails.php
			// HTML: <td class="rowhead nowrap">加入日期</td><td class="rowfollow">2022-12-26 21:21:48 (...)
			"joinTime": {
				Selector: []string{
					"td.rowhead:contains('加入日期') + td",
					"td.rowhead.nowrap:contains('加入日期') + td",
				},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			// Message count from index.php
			"messageCount": {
				Text: "0",
				Selector: []string{
					"#info_block a[href*='messages.php'] img[title*='新短讯']",
				},
				Attr: "title",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`\((\d+)\s*新\)`}},
					{Name: "parseNumber"},
				},
			},
		},
	},
	LevelRequirements: []v2.SiteLevelRequirement{
		{
			ID:        1,
			Name:      "User",
			Privilege: "新用户的默认级别。",
		},
		{
			ID:           2,
			Name:         "Power User",
			Interval:     "P2W",
			Downloaded:   "120GB",
			Ratio:        2.0,
			SeedingBonus: 60000,
			Privilege:    "得到0个邀请名额；可以直接发布种子；可以查看NFO文档；可以查看用户列表；可以请求续种；可以查看排行榜；可以查看其它用户的种子历史；可以删除自己上传的字幕。",
		},
		{
			ID:           3,
			Name:         "(连长)Elite User",
			NameAka:      []string{"Elite User"},
			Interval:     "P4W",
			Downloaded:   "256GB",
			Ratio:        2.5,
			SeedingBonus: 120000,
			Privilege:    "Elite User及以上用户封存账号后不会被删除。",
		},
		{
			ID:           4,
			Name:         "Crazy User",
			Interval:     "P8W",
			Downloaded:   "512GB",
			Ratio:        3.0,
			SeedingBonus: 240000,
			Privilege:    "得到0个邀请名额；可以在做种/下载/发布的时候选择匿名模式。",
		},
		{
			ID:           5,
			Name:         "Insane User",
			Interval:     "P12W",
			Downloaded:   "768GB",
			Ratio:        3.5,
			SeedingBonus: 360000,
			Privilege:    "无",
		},
		{
			ID:           6,
			Name:         "Veteran User",
			Interval:     "P20W",
			Downloaded:   "1TB",
			Ratio:        4.0,
			SeedingBonus: 600000,
			Privilege:    "可以查看其它用户的评论、帖子历史。",
		},
		{
			ID:           7,
			Name:         "Extreme User",
			Interval:     "P28W",
			Downloaded:   "2TB",
			Ratio:        4.5,
			SeedingBonus: 720000,
			Privilege:    "Extreme User及以上用户会永远保留账号。",
		},
		{
			ID:           8,
			Name:         "Ultimate User",
			Interval:     "P40W",
			Downloaded:   "4TB",
			Ratio:        5.0,
			SeedingBonus: 1200000,
			Privilege:    "无",
		},
		{
			ID:           9,
			Name:         "Nexus Master",
			Interval:     "P56W",
			Downloaded:   "8TB",
			Ratio:        5.5,
			SeedingBonus: 1680000,
			Privilege:    "无",
		},
	},
}

func init() {
	v2.RegisterSiteDefinition(HDDolbyDefinition)
}
