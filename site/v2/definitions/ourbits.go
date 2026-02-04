package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// OurBitsDefinition is the site definition for OurBits
var OurBitsDefinition = &v2.SiteDefinition{
	ID:             "ourbits",
	Name:           "OurBits",
	Aka:            []string{"OB", "Ours"},
	Description:    "综合性PT站点",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://ourbits.club/"},
	FaviconURL:     "https://ourbits.club/favicon.ico",
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
				Fields: []string{
					"id", "name", "uploaded", "downloaded", "ratio",
					"seeding", "leeching", "bonus",
				},
			},
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/userdetails.php",
					ResponseType: "document",
				},
				Assertion: map[string]string{"id": "params.id"},
				Fields: []string{
					"levelName", "joinTime",
				},
			},
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/mybonus.php",
					ResponseType: "document",
				},
				Fields: []string{
					"bonusPerHour",
				},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			"id": {
				Selector: []string{
					"#info_block a[href*='userdetails.php'][class*='_Name']",
				},
				Attr:    "href",
				Filters: []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{
					"#info_block a[href*='userdetails.php'][class*='_Name']",
				},
			},
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
			"bonus": {
				Selector: []string{
					"#info_block",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`魔力值[^:：]*[：:]\s*([\d,]+\.?\d*)`}},
					{Name: "parseNumber"},
				},
			},
			"levelName": {
				Selector: []string{
					"td.rowhead:contains('等級') + td > img",
					"td.rowhead:contains('等级') + td > img",
					"td.rowhead:contains('Class') + td > img",
				},
				Attr: "title",
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
			"bonusPerHour": {
				Selector: []string{
					"body",
				},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`你当前每小时能获取([\d.,]+)个魔力值`}},
					{Name: "parseNumber"},
				},
			},
		},
	},
	LevelRequirements: []v2.SiteLevelRequirement{
		{
			ID:        1,
			Name:      "Peasant",
			Privilege: "新用户的默认级别。",
		},
		{
			ID:         2,
			Name:       "User",
			Interval:   "P4W",
			Downloaded: "50GB",
			Ratio:      0.6,
			Privilege:  "可以查看排行榜。",
		},
		{
			ID:         3,
			Name:       "Power User",
			Interval:   "P8W",
			Downloaded: "120GB",
			Ratio:      1.05,
			Bonus:      50000,
			Privilege:  "可以查看其它用户的种子历史。",
		},
		{
			ID:         4,
			Name:       "Elite User",
			Interval:   "P13W",
			Downloaded: "300GB",
			Ratio:      1.55,
			Bonus:      120000,
			Privilege:  "可以直接发布种子；可以请求续种。",
		},
		{
			ID:         5,
			Name:       "Crazy User",
			Interval:   "P21W",
			Downloaded: "500GB",
			Ratio:      2.05,
			Bonus:      250000,
			Privilege:  "可以在做种/下载/发布的时候选择匿名模式；可以删除自己上传的字幕。",
		},
		{
			ID:         6,
			Name:       "Insane User",
			Interval:   "P34W",
			Downloaded: "750GB",
			Ratio:      2.55,
			Bonus:      500000,
			Privilege:  "查看普通日志。",
		},
		{
			ID:         7,
			Name:       "Veteran User",
			Interval:   "P52W",
			Downloaded: "1TB",
			Ratio:      3.05,
			Bonus:      750000,
			Privilege:  "封存账号后不会被删除；查看其它用户的评论、帖子历史。",
		},
		{
			ID:         8,
			Name:       "Extreme User",
			Interval:   "P78W",
			Downloaded: "1.5TB",
			Ratio:      3.55,
			Bonus:      1000000,
			Privilege:  "更新过期的外部信息；查看Extreme User论坛。",
		},
		{
			ID:         9,
			Name:       "Ultimate User",
			Interval:   "P104W",
			Downloaded: "2TB",
			Ratio:      4.05,
			Bonus:      1500000,
			Privilege:  "永远保留账号。",
		},
		{
			ID:         10,
			Name:       "Nexus Master",
			Interval:   "P156W",
			Downloaded: "3TB",
			Ratio:      4.55,
			Bonus:      2500000,
			Privilege:  "直接发布种子；可以查看排行榜；在网站开放邀请期间发送邀请。",
		},
		{
			ID:        100,
			Name:      "VIP",
			GroupType: v2.LevelGroupVIP,
		},
	},
}

func init() {
	v2.RegisterSiteDefinition(OurBitsDefinition)
}
