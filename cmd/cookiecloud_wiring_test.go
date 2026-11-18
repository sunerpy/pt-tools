package cmd

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/cookiecloud"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func newCookieCloudWiringEnv(t *testing.T) (*core.ConfigStore, *v2.SiteRegistry, *scheduler.Manager) {
	t.Helper()
	global.InitLogger(zap.NewNop())
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	prevDB := global.GlobalDB
	t.Cleanup(func() { global.GlobalDB = prevDB })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.RSSSubscription{}, &models.CookieCloudSetting{}))
	global.GlobalDB = &models.TorrentDB{DB: db}
	store := core.NewConfigStore(global.GlobalDB)
	registry := v2.NewSiteRegistry(global.GetLogger())
	require.NoError(t, store.SyncSites(getRegisteredSitesFromRegistry(registry)))
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	return store, registry, mgr
}

// 只列用 Cookie 登录的站点；地址用站点设置里存的，没有时用定义的默认地址。
func TestCookieCloudSites(t *testing.T) {
	store, registry, _ := newCookieCloudWiringEnv(t)
	// 内置站点的地址只能由站点定义改；这里直接改库，模拟站点设置里存的地址与定义的默认地址不同
	require.NoError(t, global.GlobalDB.DB.Model(&models.SiteSetting{}).Where("name = ?", "hdsky").Update("api_url", "https://hdsky.example/").Error)

	sites, err := cookieCloudSites(store, registry)(context.Background())
	require.NoError(t, err)
	byName := map[string]cookiecloud.SiteState{}
	for _, s := range sites {
		byName[s.Name] = s
	}
	require.Contains(t, byName, "hdsky")
	assert.Equal(t, "https://hdsky.example/", byName["hdsky"].BaseURL)
	assert.False(t, byName["hdsky"].Enabled)
	for _, id := range registry.List() {
		meta, _ := registry.Get(id)
		if meta.AuthMethod == v2.AuthMethodAPIKey {
			assert.NotContains(t, byName, id, "只用 API Key 的站点不导入 Cookie")
		}
	}
}

// 写入 Cookie 并启用站点；站点不存在时单独报错，其余照常写。
func TestApplySiteCookies(t *testing.T) {
	store, _, mgr := newCookieCloudWiringEnv(t)
	failed := applySiteCookies(store, mgr)(context.Background(), map[string]string{"hdsky": "uid=1; pass=2", "no-such-site": "x=1"})
	require.Len(t, failed, 1)
	assert.Contains(t, failed, "no-such-site")
	sc, err := store.GetSiteConf("hdsky")
	require.NoError(t, err)
	assert.Equal(t, "uid=1; pass=2", sc.Cookie)
	require.NotNil(t, sc.Enabled)
	assert.True(t, *sc.Enabled)

	assert.Empty(t, applySiteCookies(store, mgr)(context.Background(), map[string]string{}))
}

func TestWireCookieCloudWorker(t *testing.T) {
	store, registry, mgr := newCookieCloudWiringEnv(t)
	assert.Nil(t, wireCookieCloudWorker(nil, store, registry))
	w := wireCookieCloudWorker(mgr, store, registry)
	require.NotNil(t, w)
	assert.Same(t, w, mgr.GetCookieCloudWorker())
	require.NotNil(t, w.Service())
	got, err := w.Service().Settings(context.Background())
	require.NoError(t, err)
	assert.False(t, got.AutoSync, "默认不定时同步")
}
