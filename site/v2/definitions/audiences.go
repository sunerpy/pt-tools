package definitions

import (
	"fmt"
	"regexp"
	"strings"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// audiencesStatFilter 兼容 audiences.me 2026-11 改版后的自定义主题：
// 旧版为 <strong>上传量</strong>: 1.00 TB，新版改为
// <div class="ud-stat__value">2.212 TB</div><div class="ud-stat__label">上传量</div>，
// 同时保留了 data-uploader-stats JSON 属性。按 label 依次尝试多种布局。
func audiencesStatFilter(value any, args ...any) any {
	s := fmt.Sprint(value)
	if len(args) == 0 {
		return ""
	}
	label := fmt.Sprint(args[0])
	esc := regexp.QuoteMeta(label)
	patterns := []string{}
	if label == "加入日期" {
		patterns = []string{`加入日期[：:]\s*(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2})`}
	} else {
		patterns = []string{
			`<div class="ud-stat__value"[^>]*>\s*([^<]*?)\s*</div>\s*<div class="ud-stat__label">\s*` + esc + `\s*</div>`,
			`<span>\s*` + esc + `\s*</span>\s*</div>\s*<div class="ud-row__value">\s*([^<]*?)\s*</div>`,
			`&quot;label&quot;:&quot;` + esc + `[：:]?&quot;,&quot;value&quot;:&quot;([^&]*)&quot;`,
			`<strong>\s*` + esc + `\s*</strong>\s*[：:]\s*([\d.,]+\s*[KMGTP]?i?B)`,
			esc + `[：:]\s*([\d.,]+\s*[KMGTP]?i?B)`,
		}
	}
	for _, pat := range patterns {
		re := regexp.MustCompile(pat)
		if m := re.FindStringSubmatch(s); len(m) > 1 {
			out := strings.TrimSpace(m[1])
			out = strings.ReplaceAll(out, "&infin;", "∞")
			if label == "分享率" && (out == "∞" || out == "Inf" || out == "inf") {
				return "999999"
			}
			return out
		}
	}
	return ""
}

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
				Fields:        []string{"name", "uploaded", "downloaded", "ratio", "levelName", "bonus", "joinTime", "lastAccessAt"},
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
				Selector: []string{"body"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "audiencesStat", Args: []any{"上传量"}},
					{Name: "parseSize"},
				},
			},
			"downloaded": {
				Selector: []string{"body"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "audiencesStat", Args: []any{"下载量"}},
					{Name: "parseSize"},
				},
			},
			"ratio": {
				Selector: []string{"body"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "audiencesStat", Args: []any{"分享率"}},
					{Name: "parseNumber"},
				},
			},
			"levelName": {
				Selector: []string{"b.VIP_Name", "td.rowhead:contains('等级') + td img"},
			},
			"bonus": {
				Selector: []string{"body"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "audiencesStat", Args: []any{"爆米花"}},
					{Name: "parseNumber"},
				},
			},
			"lastAccessAt": {
				Selector: []string{"body"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?s)最近动向.{0,300}?(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			"joinTime": {
				Selector: []string{"body"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "audiencesStat", Args: []any{"加入日期"}}, {Name: "parseTime"}},
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
	v2.RegisterFilter("audiencesStat", audiencesStatFilter)
	v2.RegisterSiteDefinition(AudiencesDefinition)
}
