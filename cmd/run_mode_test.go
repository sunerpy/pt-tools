package cmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestRunSingle_Path(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	store := core.NewConfigStore(db)
	_ = store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, DefaultEnabled: true})
	require.NoError(t, genTorrentsWithRSSOnce(context.Background()))
}

func TestRunPersistent_EarlyReturn(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	// empty download dir triggers early return path
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = genTorrentsWithRSS(ctx)
}
