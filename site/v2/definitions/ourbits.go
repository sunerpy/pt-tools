package definitions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// ourbitsDriver 是 OurBits 的自定义驱动
// 继承 NexusPHPDriver 的所有功能，但重写 GetTorrentDetail 以使用 Link 字段
type ourbitsDriver struct {
	*v2.NexusPHPDriver
}

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
			Name:      "User",
			Privilege: "新用户的默认级别。",
		},
		{
			ID:         2,
			Name:       "Power User",
			Interval:   "P5W",
			Downloaded: "100GB",
			Ratio:      2.0,
			Privilege:  "可以查看其它用户的种子历史。",
		},
		{
			ID:         3,
			Name:       "Elite User",
			Interval:   "P10W",
			Downloaded: "350GB",
			Ratio:      2.5,
			Privilege:  "可以直接发布种子；可以请求续种。",
		},
		{
			ID:         4,
			Name:       "Crazy User",
			Interval:   "P15W",
			Downloaded: "500GB",
			Ratio:      3.0,
			Privilege:  "可以在做种/下载/发布的时候选择匿名模式；可以删除自己上传的字幕。",
		},
		{
			ID:         5,
			Name:       "Insane User",
			Interval:   "P20W",
			Downloaded: "1TB",
			Ratio:      3.5,
			Privilege:  "查看普通日志。",
		},
		{
			ID:         6,
			Name:       "Veteran User",
			Interval:   "P25W",
			Downloaded: "2TB",
			Ratio:      4.0,
			Privilege:  "封存账号后不会被删除；查看其它用户的评论、帖子历史。",
		},
		{
			ID:         7,
			Name:       "Extreme User",
			Interval:   "P30W",
			Downloaded: "4TB",
			Ratio:      4.5,
			Privilege:  "更新过期的外部信息；查看Extreme User论坛。",
		},
		{
			ID:         8,
			Name:       "Ultimate User",
			Interval:   "P40W",
			Downloaded: "6TB",
			Ratio:      5.0,
			Privilege:  "永远保留账号。",
		},
		{
			ID:         9,
			Name:       "Nexus Master",
			Interval:   "P52W",
			Downloaded: "8TB",
			Ratio:      5.5,
			Privilege:  "直接发布种子；可以查看排行榜；在网站开放邀请期间发送邀请。",
		},
		{
			ID:        100,
			Name:      "VIP",
			GroupType: v2.LevelGroupVIP,
		},
	},
	CreateDriver: createOurbitsDriver,
}

func init() {
	v2.RegisterSiteDefinition(OurBitsDefinition)
}

// createOurbitsDriver 创建 OurBits 自定义驱动
func createOurbitsDriver(config v2.SiteConfig, logger *zap.Logger) (v2.Site, error) {
	var opts v2.NexusPHPOptions
	if len(config.Options) > 0 {
		if err := json.Unmarshal(config.Options, &opts); err != nil {
			return nil, fmt.Errorf("parse OurBits options: %w", err)
		}
	}

	if opts.Cookie == "" {
		return nil, fmt.Errorf("OurBits 站点需要配置 Cookie")
	}

	siteDef := v2.GetDefinitionRegistry().GetOrDefault(config.ID)

	baseURL := config.BaseURL
	if baseURL == "" && siteDef != nil && len(siteDef.URLs) > 0 {
		baseURL = siteDef.URLs[0]
	}

	// 创建标准 NexusPHP 驱动
	nexusDriver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{
		BaseURL: baseURL,
		Cookie:  opts.Cookie,
	})

	if siteDef != nil {
		nexusDriver.SetSiteDefinition(siteDef)
	}

	// 包装为 ourbitsDriver
	driver := &ourbitsDriver{
		NexusPHPDriver: nexusDriver,
	}

	return v2.NewBaseSite(driver, v2.BaseSiteConfig{
		ID:        config.ID,
		Name:      config.Name,
		Kind:      v2.SiteNexusPHP,
		RateLimit: config.RateLimit,
		RateBurst: config.RateBurst,
		Logger:    logger.With(zap.String("site", config.ID)),
	}), nil
}

// GetTorrentDetail 重写以仅使用 Link 字段提取种子 ID
func (d *ourbitsDriver) GetTorrentDetail(ctx context.Context, guid, link string) (*v2.TorrentItem, error) {
	// 仅使用 Link 字段提取种子 ID，不使用 GUID
	torrentID := extractTorrentIDFromLink(link)
	if torrentID == "" {
		return nil, fmt.Errorf("无法从 link 提取种子 ID: %s", link)
	}

	req, err := d.PrepareDetail(torrentID)
	if err != nil {
		return nil, fmt.Errorf("prepare detail request for torrent %s: %w", torrentID, err)
	}

	res, err := d.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("execute detail request for torrent %s: %w", torrentID, err)
	}

	if res.Document == nil {
		return nil, v2.ErrParseError
	}

	parser := v2.NewNexusPHPParserFromDefinition(d.GetSiteDefinition())
	detailInfo := parser.ParseAll(res.Document.Selection)

	siteID := "ourbits"
	if def := d.GetSiteDefinition(); def != nil {
		siteID = def.ID
	}

	item := &v2.TorrentItem{
		ID:              detailInfo.TorrentID,
		Title:           detailInfo.Title,
		SizeBytes:       int64(detailInfo.SizeMB * 1024 * 1024),
		DiscountLevel:   detailInfo.DiscountLevel,
		DiscountEndTime: detailInfo.DiscountEnd,
		HasHR:           detailInfo.HasHR,
		SourceSite:      siteID,
	}

	return item, nil
}

// extractTorrentIDFromLink 从 Link URL 中提取种子 ID
func extractTorrentIDFromLink(link string) string {
	if link == "" {
		return ""
	}
	// 从 URL 如 https://ourbits.club/details.php?id=12345 中提取 ID
	parts := strings.Split(link, "id=")
	if len(parts) < 2 {
		return ""
	}
	idPart := parts[1]
	// 处理可能的额外参数，如 &hit=1
	if idx := strings.Index(idPart, "&"); idx != -1 {
		idPart = idPart[:idx]
	}
	return idPart
}
