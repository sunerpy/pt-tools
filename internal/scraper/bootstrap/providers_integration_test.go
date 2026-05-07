//go:build integration

// Package bootstrap end-to-end integration tests: exercise the full wiring
// from bootstrap.ProviderManager → ScrapeService → real Douban scraper.
// Run with: go test -tags=integration ./internal/scraper/bootstrap/
//
// These tests prove that a scrape task actually SUCCEEDS against real external
// services (Douban only by default — TMDB requires a key that must be injected
// via TMDB_API_KEY env var; when absent, TMDB-related assertions are skipped).
//
// Why integration-level and not just unit: the default unit tests mock out
// the source registry and only verify error taxonomy. These tests catch
// regressions like "Douban Frodo key got banned" or "HTML parser broke on
// layout change" that mocks cannot surface.
package bootstrap

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

func newIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, store.Migrate(db))
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// TestIntegration_Bootstrap_DoubanZeroConfig 验证豆瓣零配置生效：
// 空 DB（无任何 ProviderCredential 行）→ Reload → 豆瓣已注册 → 能真实搜索。
func TestIntegration_Bootstrap_DoubanZeroConfig(t *testing.T) {
	db := newIntegrationDB(t)
	reg := core.NewRegistry[core.MediaScraper]()
	pm := NewProviderManager(db, reg, nil)

	active, err := pm.Reload()
	require.NoError(t, err)
	assert.Contains(t, active, "douban", "豆瓣必须零配置可用")
	assert.NotContains(t, active, "tmdb", "空 DB 时 tmdb 不应被注册")

	scraper, err := reg.Get("douban")
	require.NoError(t, err, "sourceReg.Get(douban) 失败 —— bootstrap 未正确注册")
	require.True(t, scraper.IsActive())

	movieScraper, ok := scraper.(core.MovieMetadataScraper)
	require.True(t, ok, "豆瓣 scraper 应实现 MovieMetadataScraper")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	candidates, err := movieScraper.SearchMovie(ctx, core.MovieSearchOptions{
		Query: "让子弹飞",
		Year:  2010,
	})
	require.NoError(t, err, "豆瓣真实搜索失败 —— Frodo key 可能被封禁或网络不通")
	require.NotEmpty(t, candidates)
	t.Logf("豆瓣返回 %d 个候选，第一个: %+v", len(candidates), candidates[0])
}

// TestIntegration_Bootstrap_TMDBFromCredential 验证 TMDB 凭证从 DB 加载：
// 仅在 TMDB_API_KEY 环境变量存在时运行（构建期通过 GitHub Secret 注入）。
// 验证：(a) 保存凭证 → Reload → tmdb 注册成功；(b) Ping 通过。
func TestIntegration_Bootstrap_TMDBFromCredential(t *testing.T) {
	apiKey := os.Getenv("TMDB_API_KEY")
	bearerToken := os.Getenv("TMDB_BEARER_TOKEN")
	if apiKey == "" && bearerToken == "" {
		t.Skip("跳过：TMDB_API_KEY / TMDB_BEARER_TOKEN 未设置")
	}

	db := newIntegrationDB(t)
	require.NoError(t, db.Create(&store.ProviderCredential{
		Provider:    "tmdb",
		APIKey:      apiKey,
		BearerToken: bearerToken,
		Enabled:     true,
	}).Error)

	reg := core.NewRegistry[core.MediaScraper]()
	pm := NewProviderManager(db, reg, nil)
	active, err := pm.Reload()
	require.NoError(t, err)
	assert.Contains(t, active, "tmdb", "TMDB 凭证有效时应注册成功")

	scraper, err := reg.Get("tmdb")
	require.NoError(t, err)

	type pinger interface {
		Ping(ctx context.Context) error
	}
	p, ok := scraper.(pinger)
	require.True(t, ok, "TMDBScraper 应实现 Ping")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, p.Ping(ctx), "TMDB 凭证 Ping 失败 —— key 可能已失效")
}

// TestIntegration_Bootstrap_HotReload 验证凭证保存后热加载无需重启。
func TestIntegration_Bootstrap_HotReload(t *testing.T) {
	db := newIntegrationDB(t)
	reg := core.NewRegistry[core.MediaScraper]()
	pm := NewProviderManager(db, reg, nil)

	// 初次 reload：只有豆瓣（零配置）。
	active, err := pm.Reload()
	require.NoError(t, err)
	assert.Equal(t, []string{"douban"}, active)

	// 写入无效的 TMDB 凭证 —— Register 应失败，active 仍只含豆瓣。
	require.NoError(t, db.Create(&store.ProviderCredential{
		Provider: "tmdb",
		APIKey:   "", // 空 key → Register 返回 ErrUnauthorized，bootstrap 记录 warning
		Enabled:  true,
	}).Error)
	active2, err := pm.Reload()
	require.NoError(t, err)
	assert.NotContains(t, active2, "tmdb", "空 key 不应注册 tmdb")
	assert.Contains(t, active2, "douban", "豆瓣应在 reload 后保持注册")

	// 禁用豆瓣行不影响零配置默认（只是没 custom BaseURL）。
	assert.True(t, reg.Has("douban"))
}
