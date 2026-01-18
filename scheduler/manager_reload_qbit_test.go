package scheduler

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestReload_QbitNotConfiguredWarnPaths(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	store := core.NewConfigStore(db)
	_ = store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, DefaultEnabled: true, AutoStart: true})
	cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true}, Sites: map[models.SiteGroup]models.SiteConfig{}}
	// add three sites with enabled=true and invalid rss to hit qbit not configured warnings
	e := true
	cfg.Sites[models.SpringSunday] = models.SiteConfig{Enabled: &e, RSS: []models.RSSConfig{{Name: "r1", URL: "http://"}}}
	cfg.Sites[models.HDSKY] = models.SiteConfig{Enabled: &e, RSS: []models.RSSConfig{{Name: "r2", URL: "http://"}}}
	cfg.Sites[models.MTEAM] = models.SiteConfig{Enabled: &e, RSS: []models.RSSConfig{{Name: "r3", URL: "http://"}}}
	m := newTestManager(t)
	require.NotPanics(t, func() { m.Reload(cfg) })
}
