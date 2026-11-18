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
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/models"
)

// 媒体识别服务用 ConfigStore 的密钥加密 API Key：库里存的是密文，读设置只说有没有 Key。
func TestNewMediaService(t *testing.T) {
	global.InitLogger(zap.NewNop())
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	prevDB := global.GlobalDB
	t.Cleanup(func() { global.GlobalDB = prevDB })
	assert.Nil(t, newMediaService(nil))

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SettingsGlobal{}, &models.MediaSetting{}, &models.MediaWordRule{}, &models.MediaOverride{}, &models.MediaCache{}))
	global.GlobalDB = &models.TorrentDB{DB: db}
	svc := newMediaService(core.NewConfigStore(global.GlobalDB))
	require.NotNil(t, svc)
	key := "0123456789abcdef0123456789abcdef"
	got, err := svc.SaveSettings(context.Background(), recognize.SettingsInput{TMDBKey: &key})
	require.NoError(t, err)
	assert.True(t, got.HasTMDBKey)
	var row models.MediaSetting
	require.NoError(t, db.First(&row, 1).Error)
	assert.NotEmpty(t, row.TMDBKeyEncrypted)
	assert.NotContains(t, row.TMDBKeyEncrypted, key)
	assert.Empty(t, qaTMDBBaseURL(), "正式构建里 TMDB 地址不能改")
}
