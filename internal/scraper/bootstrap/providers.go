// Package bootstrap wires provider factories into the scraper source registry
// from persisted ProviderCredential rows. Kept in a separate package so:
//   - The web API layer can trigger reload without importing concrete providers.
//   - Unit tests can verify registration behavior without spinning up HTTP.
//   - `cmd/pt-scraper` (standalone) and `web/server_scraper.go` (embedded)
//     share the same DB-driven wiring logic.
package bootstrap

import (
	"os"
	"strings"
	"sync"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/scraper/bootstrap/buildkeys"
	"github.com/sunerpy/pt-tools/internal/scraper/core"
	"github.com/sunerpy/pt-tools/internal/scraper/source/douban"
	"github.com/sunerpy/pt-tools/internal/scraper/source/tmdb"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

// Logger is a minimal logger interface matching zap.SugaredLogger so the
// bootstrap package can log warnings without pulling in a concrete logger.
type Logger interface {
	Warnf(format string, args ...any)
	Infof(format string, args ...any)
}

// ProviderManager owns the lifecycle of source provider registration.
// Use Reload to re-register all providers from the DB (idempotent — safely
// deregisters any stale factory before re-registering).
type ProviderManager struct {
	db        *gorm.DB
	sourceReg *core.ScraperRegistry
	logger    Logger

	mu         sync.Mutex
	registered map[string]bool
}

// NewProviderManager constructs a manager that owns provider registrations.
// db and sourceReg must not be nil.
func NewProviderManager(db *gorm.DB, sourceReg *core.ScraperRegistry, logger Logger) *ProviderManager {
	return &ProviderManager{
		db:         db,
		sourceReg:  sourceReg,
		logger:     logger,
		registered: make(map[string]bool),
	}
}

// Reload reads all enabled ProviderCredential rows and re-registers each
// supported provider factory into sourceReg. Previously-registered providers
// are deregistered first so stale credentials do not linger.
//
// Unsupported provider names are silently skipped (LLM providers are handled
// by the separate LLM layer, not by this source registry).
//
// Zero-config providers: douban is ALWAYS registered (pt-tools ships a Frodo
// app key + HTML scraping fallback — see internal/scraper/source/douban/
// sign.go and client.go#GetHTMLDetail). Users may still add a custom BaseURL
// via ProviderCredential to override the defaults.
//
// Returns the list of (successfully) active provider names for diagnostic
// logging. An error is only returned if the DB query itself fails — per-
// provider registration failures (e.g. missing API key for TMDB) are logged
// as warnings and that provider is simply not registered.
func (pm *ProviderManager) Reload() ([]string, error) {
	var creds []store.ProviderCredential
	if err := pm.db.Where("enabled = ?", true).Find(&creds).Error; err != nil {
		return nil, err
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	for name := range pm.registered {
		pm.sourceReg.Deregister(name)
	}
	pm.registered = make(map[string]bool)

	credByProvider := make(map[string]store.ProviderCredential, len(creds))
	for _, cred := range creds {
		key := strings.ToLower(strings.TrimSpace(cred.Provider))
		if key != "" {
			credByProvider[key] = cred
		}
	}

	active := make([]string, 0)

	tmdbCfg := resolveTMDBConfig(credByProvider["tmdb"])
	if tmdbCfg.BearerToken != "" || tmdbCfg.APIKey != "" {
		if err := tmdb.Register(pm.sourceReg, tmdbCfg); err != nil {
			if pm.logger != nil {
				pm.logger.Warnf("scraper bootstrap: tmdb 注册失败: %v", err)
			}
		} else {
			pm.registered["tmdb"] = true
			active = append(active, "tmdb")
		}
	}

	// 豆瓣：无凭证也注册（pt-tools 内置 Frodo app key + HTML 爬取降级）。
	// 用户配置的 BaseURL 会覆盖 douban.defaultBaseURL；无则使用默认值。
	doubanCfg := douban.Config{}
	if cred, ok := credByProvider["douban"]; ok {
		doubanCfg.BaseURL = cred.BaseURL
	}
	doubanClient := douban.NewClient(doubanCfg)
	doubanFactory := func() core.MediaScraper { return douban.NewScraper(doubanClient) }
	if err := pm.sourceReg.Register("douban", doubanFactory); err != nil {
		if pm.logger != nil {
			pm.logger.Warnf("scraper bootstrap: douban 注册失败: %v", err)
		}
	} else {
		pm.registered["douban"] = true
		active = append(active, "douban")
	}

	if pm.logger != nil {
		pm.logger.Infof("scraper bootstrap: 已注册 provider %v", active)
	}
	return active, nil
}

// resolveTMDBConfig 按照 buildkeys 包文档的优先级链构造 tmdb.Config：
//
//	DB 凭证 > PT_SCRAPER_TMDB_{APIKEY,BEARER} 环境变量 > buildkeys ldflags 默认值
//
// 只要任一层级提供了 BearerToken 或 APIKey，就会注册 TMDB；否则跳过（豆瓣仍能用）。
// 分层写这个函数是为了让优先级逻辑可单测（否则混在 Reload 里很难覆盖所有组合）。
func resolveTMDBConfig(cred store.ProviderCredential) tmdb.Config {
	cfg := tmdb.Config{
		BearerToken: cred.BearerToken,
		APIKey:      cred.APIKey,
		BaseURL:     cred.BaseURL,
	}
	if cfg.BearerToken == "" {
		cfg.BearerToken = os.Getenv("PT_SCRAPER_TMDB_BEARER")
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("PT_SCRAPER_TMDB_APIKEY")
	}
	if cfg.BearerToken == "" {
		cfg.BearerToken = buildkeys.TmdbBearerToken
	}
	if cfg.APIKey == "" {
		cfg.APIKey = buildkeys.TmdbApiKey
	}
	return cfg
}
