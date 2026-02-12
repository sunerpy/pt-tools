package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// SpringSundayDefinition is the site definition for SpringSunday
var SpringSundayDefinition = &v2.SiteDefinition{
	ID:             "springsunday",
	Name:           "SpringSunday",
	Aka:            []string{"SSD"},
	Description:    "Classic Movie Compression Team",
	Schema:         v2.SchemaNexusPHP,
	RateLimit:      0.5,
	RateBurst:      2,
	URLs:           []string{"https://springsunday.net/"},
	LegacyURLs:     []string{"https://hdcmct.org/"},
	FaviconURL:     "https://springsunday.net/favicon.ico",
	TimezoneOffset: "+0800",
	// Custom selectors for SpringSunday's unique HTML structure
	// Row structure: 类型 | 标题 | 评论 | 存活时间 | 大小 | 种子 | 下载 | 完成 | 发布者
	// Note: goquery doesn't support "> td:nth-of-type(n)" - use "td.rowfollow:nth-child(n)" instead
	Selectors: &v2.SiteSelectors{
		// Search result rows - select tr that has torrentname table (data rows)
		TableRows: "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		// Title is in div.torrent-title > a
		Title:     "div.torrent-title > a[href*='details.php']",
		TitleLink: "div.torrent-title > a[href*='details.php']",
		// Subtitle is in div.torrent-smalldescr > span[title] (the last one with title attr is the actual subtitle)
		Subtitle: "div.torrent-smalldescr > span[title]:last-of-type",
		// Size - td.rowfollow:nth-child(5) selects the 5th td cell
		Size: "td.rowfollow:nth-child(5)",
		// Seeders - 6th td cell
		Seeders: "td.rowfollow:nth-child(6)",
		// Leechers - 7th td cell
		Leechers: "td.rowfollow:nth-child(7)",
		// Snatched - 8th td cell
		Snatched: "td.rowfollow:nth-child(8)",
		// Discount icon - SpringSunday uses span.torrent-pro-* classes
		DiscountIcon: "span.torrent-pro-free, span.torrent-pro-2up, span.torrent-pro-50pctdown, span.torrent-pro-30pctdown, span.torrent-pro-2xfree",
		// Discount end time - look for span inside (限时...) that has title with datetime
		DiscountEndTime: "div.torrent-title span[style*='DimGray'] span[title]",
		// Download link
		DownloadLink: "a[href*='download.php']",
		// Category image - 1st td cell
		Category: "td.rowfollow:nth-child(1) img[alt]",
		// Upload time - td.rowfollow.nowrap (4th column) has span with title containing datetime
		// Format: <span title="2026-01-11 16:44:28">5时<br />5分</span>
		UploadTime: "td.rowfollow.nowrap span[title]",
		// Detail page subtitle selector
		DetailSubtitle: "td.rowhead:contains('副标题') + td.rowfollow",
	},
	UserInfo: &v2.UserInfoConfig{
		PickLast:     []string{"id"},
		RequestDelay: 500,
		Process: []v2.UserInfoProcess{
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/index.php",
					ResponseType: "document",
				},
				Fields: []string{"id", "name", "seeding", "leeching", "bonus"},
			},
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/userdetails.php",
					ResponseType: "document",
				},
				Assertion: map[string]string{"id": "params.id"},
				Fields: []string{
					"name", "uploaded", "downloaded", "ratio", "levelName",
					"seedingBonus", "joinTime", "messageCount",
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
			// User ID from index.php - link is in #info_block
			// HTML: <a href="userdetails.php?id=81583"><b><span class="UserClass_Name Elite_Name">ilwpbb1314</span></b></a>
			"id": {
				Selector: []string{
					"#info_block a[href*='userdetails.php']",
					"a[href*='userdetails.php']",
				},
				Attr:    "href",
				Filters: []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			// Username from the link text
			"name": {
				Selector: []string{
					"#info_block a[href*='userdetails.php'] span",
					"#info_block a[href*='userdetails.php']",
				},
			},
			// Upload from userdetails.php - in "传输" row
			// HTML: <strong>上传量</strong>: 5.392 TB
			"uploaded": {
				Selector: []string{
					"td.rowhead:contains('传输') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>上传量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			// Download from userdetails.php
			// HTML: <strong>下载量</strong>: 462.06 GB
			"downloaded": {
				Selector: []string{
					"td.rowhead:contains('传输') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>下载量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			// Ratio from userdetails.php
			// HTML: <strong>分享率</strong>: <font color="">11.948</font>
			"ratio": {
				Selector: []string{
					"td.rowhead:contains('传输') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>分享率</strong>[：:\s]*(?:<font[^>]*>)?([\d.,]+|∞|Inf)`}},
					{Name: "parseNumber"},
				},
			},
			// Level name from userdetails.php
			// HTML: <img alt="精英" title="精英" src="pic/elite.gif" />
			"levelName": {
				Selector: []string{
					"td.rowhead:contains('等级') + td img",
				},
				Attr: "title",
			},
			// Bonus (茉莉) from index.php info_block
			// HTML: <a href="mybonus.php" title="茉莉: 546,424.7">茉莉: 546.4K</a>
			"bonus": {
				Selector: []string{
					"#info_block a[href*='mybonus.php']",
				},
				Attr: "title",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`茉莉[：:\s]*([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
			// BonusPerHour from mybonus.php
			// The table after "当前每小时能获得的积分/茉莉" heading contains the data
			// HTML: <tr class="nowrap"><td><b>我的数据</b></td>...<td>71.740</td></tr>
			// The "每小时茉莉" is the last column (11th td) in the "我的数据" row
			"bonusPerHour": {
				Selector: []string{
					"h3:contains('当前每小时') + table tbody",
					"table:has(th:contains('每小时茉莉')) tbody",
				},
				Attr: "html",
				Filters: []v2.Filter{
					// Match: 我的数据</b></td> then skip 9 <td>...</td> cells, then capture last <td>value</td>
					{Name: "regex", Args: []any{`我的数据</b></td>(?:<td[^>]*>[\d.,]*</td>){9}<td[^>]*>([\d.,]+)</td>`}},
					{Name: "parseNumber"},
				},
			},
			// Seeding count from index.php info_block
			// HTML: <img class="arrowup" ... />94
			"seeding": {
				Selector: []string{
					"#info_block",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowup"[^>]*/>\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			// Leeching count from index.php info_block
			// HTML: <img class="arrowdown" ... />0
			"leeching": {
				Selector: []string{
					"#info_block",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowdown"[^>]*/>\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			// Seeding bonus from userdetails.php
			// HTML: <b>做种积分:</b> 913,905.2
			"seedingBonus": {
				Selector: []string{
					"td.rowhead:contains('积分') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`做种积分[：:</b>\s]*([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
			// Join date from userdetails.php
			// HTML: 2015-07-01 00:03:08 (<span title="2015-07-01 00:03:08">10年6月前</span>)
			"joinTime": {
				Selector: []string{
					"td.rowhead:contains('加入日期') + td",
				},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			// Message count - default 0 if no unread messages
			// SpringSunday has two types of messages:
			// 1. System messages (系统短讯): "你有X条新系统短讯！" with orange background
			// 2. Private messages (私人短讯): "你有X条新私人短讯！" with red background
			// We need to sum all unread message counts
			"messageCount": {
				Text: "0",
				Selector: []string{
					// Get the entire page HTML to find all message notifications
					"body",
				},
				Attr: "html",
				Filters: []v2.Filter{
					// Sum all numbers from "你有X条新" patterns (both system and private messages)
					{Name: "sumRegexMatches", Args: []any{`你有(\d+)条新`}},
				},
			},
		},
	},
	LevelRequirements: []v2.SiteLevelRequirement{
		{
			ID:        1,
			Name:      "新人(User)",
			Privilege: "新用户的默认级别。可以发种，可以请求续种；可以在做种/下载/发布的时候选择匿名模式；可以上传字幕或删除自己上传的字幕；可以更新过期的外部信息。",
		},
		{
			ID:         2,
			Name:       "精英(Elite)",
			NameAka:    []string{"Elite", "精英"},
			Downloaded: "500GB",
			Ratio:      1.2,
			Alternative: []v2.AlternativeRequirement{
				{SeedingBonus: 100000, Uploads: 1},
				{SeedingBonus: 150000},
			},
			Privilege: "可以在做种/下载/发布的时候选择匿名模式；可以查看用户列表；可以查看排行榜；可以浏览论坛邀请区；自助申请保种员；等级加成 0.05。",
		},
		{
			ID:         3,
			Name:       "大师(Master)",
			NameAka:    []string{"Master", "大师"},
			Downloaded: "1TB",
			Ratio:      1.2,
			Alternative: []v2.AlternativeRequirement{
				{SeedingBonus: 500000, Uploads: 100},
				{SeedingBonus: 1000000},
			},
			Privilege: "可以访问高级用户论坛，等级加成 0.15。",
		},
		{
			ID:         4,
			Name:       "神仙(God)",
			NameAka:    []string{"God", "神仙"},
			Downloaded: "3TB",
			Ratio:      2,
			Alternative: []v2.AlternativeRequirement{
				{SeedingBonus: 1200000, Uploads: 300},
				{SeedingBonus: 2400000},
			},
			Privilege: "彩色 ID 特权；可以查看普通日志；等级加成 0.25。",
		},
		{
			ID:        5,
			Name:      "神王(Immortal)",
			NameAka:   []string{"Immortal", "神王"},
			Privilege: "成为当月神王时奖励当时邀请茉莉价格的一半茉莉，最酷炫的动态彩色 ID 特权；常规时期可以购买及发送邀请；等级加成0.35。",
		},
		{
			ID:        100,
			Name:      "贵宾(VIP)",
			NameAka:   []string{"VIP", "贵宾"},
			GroupType: v2.LevelGroupVIP,
			Privilege: "贵宾(VIP)的权限和神王(Immortal)相同。贵宾(VIP)及其以上等级免除自动降级。",
		},
	},
}

func init() {
	v2.RegisterSiteDefinition(SpringSundayDefinition)
}
