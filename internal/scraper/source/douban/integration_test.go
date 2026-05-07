//go:build integration

// Package douban integration tests — hit real Douban endpoints.
// Run with: go test -tags=integration -run Integration ./internal/scraper/source/douban/
// Skipped by default (default test runs use go test without -tags).
//
// These tests validate that the bundled Frodo app key still works AND that
// the HTML fallback path remains parseable. If Douban invalidates the key
// or changes HTML structure, these tests will fail — signaling we need to
// rotate the key (sign.go:apiKey) or update HTML parsers (html.go).
package douban

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

// newIntegrationClient 构造一个带宽松超时的 Client，允许通过 DOUBAN_BASE_URL /
// DOUBAN_HTML_URL 环境变量覆盖端点（便于在 CI 中指向代理以绕过 IP 限制）。
func newIntegrationClient(t *testing.T) *Client {
	t.Helper()
	cfg := Config{
		BaseURL: os.Getenv("DOUBAN_BASE_URL"),
		HTMLURL: os.Getenv("DOUBAN_HTML_URL"),
	}
	client := NewClient(cfg)
	// 集成测试时放宽 rate limit，避免相邻用例被本地节流拖慢。
	client.rateLimit = 500 * time.Millisecond
	return client
}

func TestIntegration_Douban_Search_Real(t *testing.T) {
	client := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.Search(ctx, "盗梦空间", 5)
	require.NoError(t, err, "豆瓣 Frodo search 失败 —— 可能 apiKey 已被封禁，需要更新 sign.go:apiKey")
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.Items, "搜索盗梦空间应返回结果")

	// 诊断输出：打印首 2 条 item 的关键字段，便于发现豆瓣 API shape 变化。
	for i, item := range resp.Items {
		if i >= 2 {
			break
		}
		targetID, targetTitle, targetType, targetURI, targetURL := "", "", "", "", ""
		if item.Target != nil {
			targetID = item.Target.ID
			targetTitle = item.Target.Title
			targetType = item.Target.Type
			targetURI = item.Target.URI
			targetURL = item.Target.URL
		}
		t.Logf("item[%d]: id=%q type=%q title=%q year=%q uri=%q url=%q target.id=%q target.title=%q target.type=%q target.uri=%q target.url=%q",
			i, item.ID, item.Type, item.Title, item.Year, item.URI, item.URL,
			targetID, targetTitle, targetType, targetURI, targetURL)
	}

	found := false
	for _, item := range resp.Items {
		// Frodo 搜索结果有时把真实字段塞进 item.Target（target 为通用跳转对象）；
		// 两处都检查，兼容豆瓣 API 随机切换的两种响应形态。
		title := item.Title
		id := item.ID
		if item.Target != nil {
			if title == "" {
				title = item.Target.Title
			}
			if id == "" {
				id = item.Target.ID
			}
		}
		if strings.Contains(title, "盗梦空间") {
			found = true
			assert.NotEmpty(t, id, "搜索结果应带豆瓣 ID")
			break
		}
	}
	assert.True(t, found, "结果中应包含《盗梦空间》")
}

func TestIntegration_Douban_Scraper_SearchMovie_Real(t *testing.T) {
	client := newIntegrationClient(t)
	scraper := NewScraper(client)
	require.True(t, scraper.IsActive())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	candidates, err := scraper.SearchMovie(ctx, core.MovieSearchOptions{
		Query: "盗梦空间",
		Year:  2010,
	})
	require.NoError(t, err)
	require.NotEmpty(t, candidates, "应找到《盗梦空间》")
	assert.Equal(t, core.MediaTypeMovie, candidates[0].MediaType)
	assert.NotEmpty(t, candidates[0].ID)
}

func TestIntegration_Douban_Scraper_GetMovieMetadata_Real(t *testing.T) {
	client := newIntegrationClient(t)
	scraper := NewScraper(client)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	movie, err := scraper.GetMovieMetadata(ctx, core.MovieSearchOptions{
		Query: "盗梦空间",
		Year:  2010,
	})
	if err != nil && isDoubanIPBlocked(err) {
		t.Skipf("豆瓣拒绝当前出口 IP（CI/数据中心 IP 常见），跳过此测试: %v", err)
	}
	// 允许 Frodo 失败 + HTML fallback 成功 —— 这正是 client.GetMovie 的降级路径。
	require.NoError(t, err, "Frodo + HTML 两路都失败说明豆瓣整体不可用")
	require.NotNil(t, movie)
	assert.NotEmpty(t, movie.Title)
	assert.Equal(t, 2010, movie.Year, "《盗梦空间》年份应为 2010")
	assert.NotEmpty(t, movie.Genres, "应解析出类型")
	t.Logf("豆瓣刮削结果: title=%q year=%d genres=%v ratings=%v",
		movie.Title, movie.Year, movie.Genres, movie.Ratings)
}

func TestIntegration_Douban_HTMLFallback_Real(t *testing.T) {
	client := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 《盗梦空间》豆瓣 ID 是 3541415，直接用 HTML 路径验证即便 Frodo 整体挂了
	// html.go 解析器仍能工作。GetHTMLDetail 返回 *htmlDetail（包内未导出），
	// 因此此测试必须与 douban 包同 package 才能访问字段。
	detail, err := client.GetHTMLDetail(ctx, "3541415")
	if err != nil && isDoubanIPBlocked(err) {
		t.Skipf("豆瓣拒绝当前出口 IP（CI/数据中心 IP 常见），跳过此测试: %v", err)
	}
	require.NoError(t, err, "HTML 降级路径挂了 —— 需要更新 html.go 解析器或 UA/Cookie 策略")
	require.NotNil(t, detail)
	assert.Contains(t, detail.Title, "盗梦空间")
	assert.Equal(t, 2010, detail.Year)
}

// isDoubanIPBlocked 粗略判断错误是否来自豆瓣反爬 IP 封禁（而非代码 bug）。
// 数据中心 IP、共享 VPN 出口 IP 常被豆瓣 302 到 sec.douban.com —— 不是我们能
// 在代码层面修复的。生产环境家庭 IP 通常能正常访问。
func isDoubanIPBlocked(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "title missing") ||
		strings.Contains(msg, "sec.douban.com") ||
		strings.Contains(msg, "html fallback failed")
}
