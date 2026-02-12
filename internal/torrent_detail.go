package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/sunerpy/requests"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/utils/httpclient"
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
	if def, ok := v2.GetDefinitionRegistry().Get(string(siteName)); ok {
		switch def.Schema {
		case v2.SchemaMTorrent:
			detail, err := fetchMTorrentDetail(ctx, siteName, item)
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
		case v2.SchemaNexusPHP:
			detail, err := fetchNexusPHPDetail(ctx, siteName, item)
			if err != nil {
				sLogger().Debugf("[TorrentDetail] 获取 %s 种子详情失败: %v", siteName, err)
				return result, nil
			}
			sLogger().Debugf("[TorrentDetail] %s: Name=%s, SubTitle=%s, IsFree=%v", siteName, detail.Title, detail.Tag, detail.IsFree)
			return detail, nil
		}
	}

	sLogger().Debugf("[TorrentDetail] 未知站点类型: %s，使用 RSS 条目信息", siteName)
	return result, nil
}

// fetchMTorrentDetail 获取 mTorrent 架构站点的种子详情
func fetchMTorrentDetail(ctx context.Context, siteName models.SiteGroup, item *gofeed.Item) (*models.MTTorrentDetail, error) {
	cfg, err := core.NewConfigStore(global.GlobalDB).Load()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %v", err)
	}

	sc := cfg.Sites[siteName]
	if sc.APIUrl == "" || sc.APIKey == "" {
		return nil, fmt.Errorf("%s API 未配置", siteName)
	}

	data := fmt.Sprintf("id=%s", item.GUID)
	// API 路径需要加上 /api 前缀
	apiPath := fmt.Sprintf("%s/api/torrent/detail", sc.APIUrl)

	session := requests.NewSession().WithTimeout(30 * time.Second)
	if proxyURL := httpclient.ResolveProxyFromEnvironment(apiPath); proxyURL != "" {
		session = session.WithProxy(proxyURL)
	}
	defer func() { _ = session.Close() }()

	req, err := requests.NewPost(apiPath).WithBody(strings.NewReader(data)).Build()
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}
	req.AddHeader("Content-Type", "application/x-www-form-urlencoded")
	req.AddHeader("x-api-key", sc.APIKey)

	resp, err := session.DoWithContext(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}

	var responseData models.APIResponse[models.MTTorrentDetail]
	if err := json.NewDecoder(bytes.NewReader(resp.Bytes())).Decode(&responseData); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %v", err)
	}

	return &responseData.Data, nil
}

// fetchNexusPHPDetail 获取 NexusPHP 架构站点的种子详情
func fetchNexusPHPDetail(ctx context.Context, siteName models.SiteGroup, item *gofeed.Item) (*TorrentDetailForTest, error) {
	cfg, err := core.NewConfigStore(global.GlobalDB).Load()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %v", err)
	}

	sc := cfg.Sites[siteName]
	if sc.Cookie == "" {
		return nil, fmt.Errorf("%s Cookie 未配置", siteName)
	}

	// 使用统一站点实现获取详情
	impl, err := NewUnifiedSiteImpl(ctx, siteName)
	if err != nil {
		return nil, fmt.Errorf("创建 %s 实例失败: %v", siteName, err)
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
