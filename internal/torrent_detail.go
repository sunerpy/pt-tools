package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
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
	Title     string // 种子标题（优先使用中文名）
	Tag       string // 种子标签/副标题
	IsFree    bool   // 是否免费
	SizeBytes int64  // 种子大小（详情里的，没有时用 RSS 附件的长度；都没有是 0）
	// DetailMissing 是详情没取到：标题与标签按 RSS 条目里的，免费状态不知道（按非免费算）
	DetailMissing bool
}

var mtorrentDetailLimiter = struct {
	sync.Mutex
	next map[models.SiteGroup]time.Time
}{next: make(map[models.SiteGroup]time.Time)}

// GetTorrentDetailForTest 获取种子详情用于过滤规则测试
// 根据站点类型调用对应的 API 获取完整的种子信息
func GetTorrentDetailForTest(ctx context.Context, siteName models.SiteGroup, item *gofeed.Item) (*TorrentDetailForTest, error) {
	if item == nil {
		return nil, fmt.Errorf("RSS item 为空")
	}

	// 默认使用 RSS 条目的标题、分类与附件长度
	result := &TorrentDetailForTest{
		Title:     item.Title,
		SizeBytes: rssEnclosureSize(item),
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
				result.DetailMissing = true
				return result, nil
			}
			if name := detail.GetName(); name != "" {
				result.Title = name
			}
			if subTitle := detail.GetSubTitle(); subTitle != "" {
				result.Tag = subTitle
			}
			result.IsFree = detail.IsFree()
			if n, err := strconv.ParseInt(strings.TrimSpace(detail.Size), 10, 64); err == nil && n > 0 {
				result.SizeBytes = n
			}
			sLogger().Debugf("[TorrentDetail] %s: Name=%s, SubTitle=%s, IsFree=%v", siteName, result.Title, result.Tag, result.IsFree)
			return result, nil
		case v2.SchemaNexusPHP:
			detail, err := fetchNexusPHPDetail(ctx, siteName, item)
			if err != nil {
				sLogger().Debugf("[TorrentDetail] 获取 %s 种子详情失败: %v", siteName, err)
				result.DetailMissing = true
				return result, nil
			}
			sLogger().Debugf("[TorrentDetail] %s: Name=%s, SubTitle=%s, IsFree=%v", siteName, detail.Title, detail.Tag, detail.IsFree)
			if detail.SizeBytes <= 0 {
				detail.SizeBytes = result.SizeBytes
			}
			return detail, nil
		}
	}

	sLogger().Debugf("[TorrentDetail] 未知站点类型: %s，使用 RSS 条目信息", siteName)
	// 这类站点不取详情：免费状态不知道
	result.DetailMissing = true
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

	// 站点代理开着时按站点判定，关着时按环境变量；无论走不走代理都显式 WithProxy（清掉 Transport 池里残留的代理）
	session := httpclient.NewSession().WithTimeout(30 * time.Second).WithProxy(httpclient.ResolveSiteProxyOrEnv(string(siteName), apiPath))
	defer func() { _ = session.Close() }()

	req, err := requests.NewPost(apiPath).WithBody(strings.NewReader(data)).Build()
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}
	req.AddHeader("Content-Type", "application/x-www-form-urlencoded")
	req.AddHeader("x-api-key", sc.APIKey)
	if rateErr := waitMTorrentDetailRateLimit(ctx, siteName); rateErr != nil {
		return nil, rateErr
	}

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
	if code := strings.TrimSpace(fmt.Sprint(responseData.Code)); code != "" && code != "0" {
		return nil, fmt.Errorf("API error: %s - %s", code, responseData.Message)
	}

	return &responseData.Data, nil
}

func waitMTorrentDetailRateLimit(ctx context.Context, siteName models.SiteGroup) error {
	mtorrentDetailLimiter.Lock()
	now := time.Now()
	waitUntil := mtorrentDetailLimiter.next[siteName]
	if now.After(waitUntil) {
		waitUntil = now
	}
	mtorrentDetailLimiter.next[siteName] = waitUntil.Add(1200 * time.Millisecond)
	mtorrentDetailLimiter.Unlock()

	if delay := time.Until(waitUntil); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
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
		Title:     torrentItem.Title,
		Tag:       torrentItem.GetSubTitle(),
		IsFree:    torrentItem.IsFree(),
		SizeBytes: torrentItem.SizeBytes,
	}, nil
}

// rssEnclosureSize 是 RSS 条目附件（种子）的长度；站点不写或写得不对时是 0。
func rssEnclosureSize(item *gofeed.Item) int64 {
	for _, e := range item.Enclosures {
		if e == nil {
			continue
		}
		if n, err := strconv.ParseInt(strings.TrimSpace(e.Length), 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return 0
}
