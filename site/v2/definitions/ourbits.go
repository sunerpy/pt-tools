package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// OurBitsDefinition is the site definition for OurBits
var OurBitsDefinition = &v2.SiteDefinition{
	ID:             "ourbits",
	Name:           "OurBits",
	Aka:            []string{"OB"},
	Description:    "OurBits PT Site",
	Schema:         "NexusPHP",
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
				Fields: []string{"id", "name", "seeding", "leeching", "bonus", "uploaded", "downloaded", "ratio", "levelName", "messageCount"},
			},
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/userdetails.php",
					ResponseType: "document",
				},
				Assertion: map[string]string{"id": "params.id"},
				Fields: []string{
					"joinTime",
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
			// HTML: <a href="userdetails.php?id=46514" class="CrazyUser_Name"><b>392198523</b></a>
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
			// HTML: <font class="color_uploaded">上传量：</font> 2.523 TB
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
			// HTML: <font class="color_downloaded"> 下载量：</font> 773.46 GB
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
			// HTML: <span class="color_ratio">分享率：</span> 3.340
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
			// Level name from userdetails.php
			// HTML: <img alt="(营长)Crazy User" title="(营长)Crazy User" src=".../crazy.gif">
			"levelName": {
				Selector: []string{
					"td.rowhead:contains('等级') + td img",
					"td.rowhead:contains('等級') + td img",
					"td.rowhead:contains('Class') + td img",
				},
				Attr: "title",
			},
			// Bonus (魔力值) from index.php info_block
			// HTML: <span class="color_bonus">魔力值 </span>[<a href="mybonus.php">使用</a>]: 757,603.3
			// Note: OurBits has NO seedingBonus field, only bonus
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
			// BonusPerHour from mybonus.php
			// HTML: 你当前每小时能获取44.149个魔力值
			"bonusPerHour": {
				Selector: []string{
					"body",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`你当前每小时能获取([\d.,]+)个魔力值`}},
					{Name: "parseNumber"},
				},
			},
			// Seeding count from index.php
			// HTML: <img class="arrowup" alt="Torrents seeding" title="当前做种" .../>6
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
			// Leeching count from index.php
			// HTML: <img class="arrowdown" alt="Torrents leeching" title="当前下载" .../>0
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
			// Join date from userdetails.php
			// HTML: <td class="rowhead nowrap">加入日期</td><td class="rowfollow">2019-09-12 10:05:10 (...)
			"joinTime": {
				Selector: []string{
					"td.rowhead:contains('加入日期') + td",
					"td.rowhead:contains('Join') + td",
				},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			// Message count from index.php
			// HTML: <a href="messages.php"><img class="inbox" ... title="收件箱 (无新短讯)"></a> 11 (0 新)
			"messageCount": {
				Text: "0",
				Selector: []string{
					"#info_block a[href*='messages.php'] img[title*='新短讯']",
				},
				Attr: "title",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(\d+)\s*新`}},
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
			Privilege: "被降级的用户，他们有7天时间来提升分享率，否则他们会被踢。不能发表趣味盒内容；不能申请友情链接；不能上传字幕。最多可以同时下载10个种子。",
		},
		{
			ID:        1,
			Name:      "User",
			Privilege: "新用户的默认级别。最多可以同时下载10个种子。",
		},
		{
			ID:         2,
			Name:       "Power User",
			Interval:   "P5W",
			Downloaded: "100GB",
			Ratio:      2.0,
			Privilege:  "可以查看NFO文档；可以查看用户列表；可以请求续种；可以查看排行榜；可以查看其它用户的种子历史(如果用户隐私等级未设置为\"强\")；可以删除自己上传的字幕。最多可以同时下载20个种子。",
		},
		{
			ID:         3,
			Name:       "Elite User",
			Interval:   "P10W",
			Downloaded: "350GB",
			Ratio:      2.5,
			Privilege:  "Elite User及以上用户封存账号后不会被删除。此等级及以上没有下载数限制。可以查看论坛Elite User(邀请交流版)。",
		},
		{
			ID:         4,
			Name:       "Crazy User",
			Interval:   "P15W",
			Downloaded: "500GB",
			Ratio:      3.0,
			Privilege:  "可以在做种/下载/发布的时候选择匿名模式。",
		},
		{
			ID:         5,
			Name:       "Insane User",
			Interval:   "P20W",
			Downloaded: "1TB",
			Ratio:      3.5,
			Privilege:  "可以查看普通日志。",
		},
		{
			ID:         6,
			Name:       "Veteran User",
			Interval:   "P25W",
			Downloaded: "2TB",
			Ratio:      4.0,
			Privilege:  "可以查看其它用户的评论、帖子历史。Veteran User及以上用户会永远保留账号。",
		},
		{
			ID:         7,
			Name:       "Extreme User",
			Interval:   "P30W",
			Downloaded: "4TB",
			Ratio:      4.5,
			Privilege:  "得到一个永久邀请；可以更新过期的外部信息。",
		},
		{
			ID:         8,
			Name:       "Ultimate User",
			Interval:   "P40W",
			Downloaded: "6TB",
			Ratio:      5.0,
			Privilege:  "得到两个永久邀请；",
		},
		{
			ID:         9,
			Name:       "Nexus Master",
			Interval:   "P52W",
			Downloaded: "8TB",
			Ratio:      5.5,
			Privilege:  "得到三个永久邀请",
		},
		{
			ID:        100,
			Name:      "VIP",
			GroupType: v2.LevelGroupVIP,
			Privilege: "免除自动降级，只计算上传量，不计算下载量。",
		},
	},
}

func init() {
	v2.RegisterSiteDefinition(OurBitsDefinition)
}
