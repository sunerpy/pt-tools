package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// TorrentDetailForTest 用于测试的种子详情
type TorrentDetailForTest struct {
	Title  string // 种子标题（优先使用中文名）
	Tag    string // 种子标签/副标题
	IsFree bool   // 是否免费
}

// GetTorrentDetailForTest 获取种子详情用于过滤规则测试
// 根据站点类型调用对应的 API 获取完整的种子信息
func GetTorrentDetailForTest(ctx context.Context, siteName models.SiteGroup, item *gofeed.Item) (*TorrentDetailForTest, error) {
	if item == nil {
		return nil, fmt.Errorf("RSS item 为空")
	}

	// 默认使用 RSS 条目的标题和分类
	result := &TorrentDetailForTest{
		Title: item.Title,
	}
	if len(item.Categories) > 0 {
		result.Tag = strings.Join(item.Categories, ",")
	}

	// 根据站点类型获取详情
	switch siteName {
	case models.MTEAM:
		detail, err := fetchMteamDetail(ctx, item)
		if err != nil {
			sLogger().Debugf("[TorrentDetail] 获取 %s 种子详情失败: %v", siteName, err)
			return result, nil
		}
		if name := detail.GetName(); name != "" {
			result.Title = name
		}
		if subTitle := detail.GetSubTitle(); subTitle != "" {
			result.Tag = subTitle
		}
		result.IsFree = detail.IsFree()
		sLogger().Debugf("[TorrentDetail] %s: Name=%s, SubTitle=%s, IsFree=%v", siteName, result.Title, result.Tag, result.IsFree)
		return result, nil

	case models.HDSKY:
		detail, err := fetchHdskyDetail(ctx, item)
		if err != nil {
			sLogger().Debugf("[TorrentDetail] 获取 %s 种子详情失败: %v", siteName, err)
			return result, nil
		}
		sLogger().Debugf("[TorrentDetail] %s: Name=%s, SubTitle=%s, IsFree=%v", siteName, detail.Title, detail.Tag, detail.IsFree)
		return detail, nil

	case models.SpringSunday:
		detail, err := fetchSpringSundayDetail(ctx, item)
		if err != nil {
			sLogger().Debugf("[TorrentDetail] 获取 %s 种子详情失败: %v", siteName, err)
			return result, nil
		}
		sLogger().Debugf("[TorrentDetail] %s: Name=%s, SubTitle=%s, IsFree=%v", siteName, detail.Title, detail.Tag, detail.IsFree)
		return detail, nil

	default:
		sLogger().Debugf("[TorrentDetail] 未知站点类型: %s，使用 RSS 条目信息", siteName)
		return result, nil
	}
}

// fetchMteamDetail 获取 MTeam 种子详情
func fetchMteamDetail(ctx context.Context, item *gofeed.Item) (*models.MTTorrentDetail, error) {
	cfg, err := core.NewConfigStore(global.GlobalDB).Load()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %v", err)
	}

	sc := cfg.Sites[models.MTEAM]
	if sc.APIUrl == "" || sc.APIKey == "" {
		return nil, fmt.Errorf("MTeam API 未配置")
	}

	data := fmt.Sprintf("id=%s", item.GUID)
	// API 路径需要加上 /api 前缀
	apiPath := fmt.Sprintf("%s/api/torrent/detail", sc.APIUrl)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiPath, strings.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("x-api-key", sc.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}

	var responseData models.APIResponse[models.MTTorrentDetail]
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %v", err)
	}

	return &responseData.Data, nil
}

// fetchHdskyDetail 获取 HDSKY 种子详情
func fetchHdskyDetail(ctx context.Context, item *gofeed.Item) (*TorrentDetailForTest, error) {
	cfg, err := core.NewConfigStore(global.GlobalDB).Load()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %v", err)
	}

	sc := cfg.Sites[models.HDSKY]
	if sc.Cookie == "" {
		return nil, fmt.Errorf("HDSKY Cookie 未配置")
	}

	// 使用统一站点实现获取详情
	impl, err := NewUnifiedSiteImpl(ctx, models.HDSKY)
	if err != nil {
		return nil, fmt.Errorf("创建 HDSKY 实例失败: %v", err)
	}

	torrentItem, err := impl.GetTorrentDetails(item)
	if err != nil {
		return nil, err
	}

	// 直接使用 v2.TorrentItem 的方法
	return &TorrentDetailForTest{
		Title:  torrentItem.Title,
		Tag:    torrentItem.GetSubTitle(),
		IsFree: torrentItem.IsFree(),
	}, nil
}

// fetchSpringSundayDetail 获取 SpringSunday 种子详情
func fetchSpringSundayDetail(ctx context.Context, item *gofeed.Item) (*TorrentDetailForTest, error) {
	cfg, err := core.NewConfigStore(global.GlobalDB).Load()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %v", err)
	}

	sc := cfg.Sites[models.SpringSunday]
	if sc.Cookie == "" {
		return nil, fmt.Errorf("SpringSunday Cookie 未配置")
	}

	// 使用统一站点实现获取详情
	impl, err := NewUnifiedSiteImpl(ctx, models.SpringSunday)
	if err != nil {
		return nil, fmt.Errorf("创建 SpringSunday 实例失败: %v", err)
	}

	torrentItem, err := impl.GetTorrentDetails(item)
	if err != nil {
		return nil, err
	}

	// 直接使用 v2.TorrentItem 的方法
	return &TorrentDetailForTest{
		Title:  torrentItem.Title,
		Tag:    torrentItem.GetSubTitle(),
		IsFree: torrentItem.IsFree(),
	}, nil
}
