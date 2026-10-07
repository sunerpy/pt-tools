package cmd

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

func TestWireReseedWorker(t *testing.T) {
	global.InitLogger(zap.NewNop())
	prevDB := global.GlobalDB
	t.Cleanup(func() { global.GlobalDB = prevDB })

	global.GlobalDB = nil
	noDB := scheduler.NewManager()
	t.Cleanup(noDB.StopAll)
	assert.Nil(t, wireReseedWorker(noDB, nil, nil), "没有数据库时不接线")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.TorrentTransferJob{}, &models.TransferRule{}, &models.DownloaderSetting{}, &models.ReseedSetting{}, &models.ReseedRecord{}))
	global.GlobalDB = &models.TorrentDB{DB: db}
	store := core.NewConfigStore(global.GlobalDB)
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	assert.Nil(t, wireReseedWorker(mgr, nil, store), "转移做种后台没起来时不接线")

	require.NotNil(t, wireTransferWorker(mgr, nil))
	w := wireReseedWorker(mgr, nil, store)
	require.NotNil(t, w)
	assert.Same(t, w, mgr.GetReseedWorker())
	assert.NotNil(t, w.Service())

	t.Setenv("PT_TOOLS_SECRET_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	c := storeCipher{store: store}
	enc, err := c.Encrypt("tok")
	require.NoError(t, err)
	assert.NotEqual(t, "tok", enc)
	plain, err := c.Decrypt(enc)
	require.NoError(t, err)
	assert.Equal(t, "tok", plain)
}
