package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/nas"
	"github.com/sunerpy/pt-tools/models"
)

// prefixCipher 是测试用的「加密」：加上 enc: 前缀；解不带前缀的报错（当作密钥换了）。
type prefixCipher struct{ failEncrypt bool }

func (c prefixCipher) Encrypt(plain string) (string, error) {
	if c.failEncrypt {
		return "", errors.New("没有加密密钥")
	}
	return "enc:" + plain, nil
}

func (prefixCipher) Decrypt(text string) (string, error) {
	if !strings.HasPrefix(text, "enc:") {
		return "", errors.New("解不开")
	}
	return strings.TrimPrefix(text, "enc:"), nil
}

func newNASTestDB(t *testing.T, migrate bool) *gorm.DB {
	t.Helper()
	global.InitLogger(zap.NewNop())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1) // 内存库每个连接各是一份
	t.Cleanup(func() { _ = sqlDB.Close() })
	if migrate {
		require.NoError(t, db.AutoMigrate(&models.NasSource{}, &models.NasSetting{}))
	}
	return db
}

func TestLoadNASConfig(t *testing.T) {
	ctx := context.Background()
	db := newNASTestDB(t, true)
	cfg, err := loadNASConfig(ctx, db, prefixCipher{})
	require.NoError(t, err)
	assert.Equal(t, nas.Config{LocalEnabled: true, Sources: []nas.SourceConfig{}}, cfg, "没有设置时本机来源开着")

	require.NoError(t, models.SaveNasSetting(db, models.NasSetting{LocalEnabled: false, HomeSource: "local"}))
	rows := []models.NasSource{
		{
			Name: "好的", Kind: "truenas", Enabled: true, URL: "https://nas", Username: "u", SecretEncrypted: "enc:key",
			ExtraEncrypted: `enc:{"did":"d1"}`, TLSPinSHA256: "ab", InsecureAck: true, Revision: 3,
		},
		{Name: "密钥解不开", Kind: "truenas", Enabled: true, URL: "https://nas2", SecretEncrypted: "garbage", Revision: 1},
		{Name: "凭据解不开", Kind: "fnos", Enabled: false, URL: "https://nas3", SecretEncrypted: "enc:k3", ExtraEncrypted: "garbage", Revision: 2},
		{Name: "凭据格式不对", Kind: "fnos", Enabled: true, URL: "https://nas4", ExtraEncrypted: "enc:not json", Revision: 1},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}
	cfg, err = loadNASConfig(ctx, db, prefixCipher{})
	require.NoError(t, err)
	assert.False(t, cfg.LocalEnabled)
	require.Len(t, cfg.Sources, 4, "停用的来源也列出")
	assert.Equal(t, nas.SourceConfig{
		ID: rows[0].ID, Name: "好的", Kind: "truenas", Enabled: true, URL: "https://nas", Username: "u", Secret: "key",
		Extra: map[string]string{"did": "d1"}, PinSHA256: "ab", InsecureAck: true, Revision: 3,
	}, cfg.Sources[0])
	assert.Equal(t, nasBrokenSecret, cfg.Sources[1].Broken, "密钥解不开的照样列出，标成用不了")
	assert.Empty(t, cfg.Sources[1].Secret)
	assert.Equal(t, "k3", cfg.Sources[2].Secret)
	assert.Nil(t, cfg.Sources[2].Extra, "刷新出的凭据解不开时丢掉，下次重新登录")
	assert.Empty(t, cfg.Sources[2].Broken)
	assert.Nil(t, cfg.Sources[3].Extra)

	_, err = loadNASConfig(ctx, nil, prefixCipher{})
	assert.EqualError(t, err, "数据库没有初始化")
	_, err = loadNASConfig(ctx, newNASTestDB(t, false), prefixCipher{})
	assert.ErrorContains(t, err, "读取 NAS 监控设置失败")
	noSources := newNASTestDB(t, false)
	require.NoError(t, noSources.AutoMigrate(&models.NasSetting{}))
	_, err = loadNASConfig(ctx, noSources, prefixCipher{})
	assert.ErrorContains(t, err, "读取 NAS 监控来源失败")
}

func TestSaveNASExtra(t *testing.T) {
	ctx := context.Background()
	db := newNASTestDB(t, true)
	row := models.NasSource{Name: "飞牛", Kind: "fnos", Enabled: true, URL: "https://fn", Revision: 5}
	require.NoError(t, db.Create(&row).Error)
	reload := func() models.NasSource {
		var got models.NasSource
		require.NoError(t, db.First(&got, row.ID).Error)
		return got
	}

	require.NoError(t, saveNASExtra(ctx, db, prefixCipher{}, row.ID, 4, map[string]string{"token": "old"}))
	assert.Empty(t, reload().ExtraEncrypted, "用户刚改了设置（revision 变了），不盖掉")
	require.NoError(t, saveNASExtra(ctx, db, prefixCipher{}, row.ID, 5, map[string]string{"token": "t"}))
	got := reload()
	assert.Equal(t, `enc:{"token":"t"}`, got.ExtraEncrypted)
	assert.Equal(t, int64(5), got.Revision, "写回不改 revision")
	require.NoError(t, saveNASExtra(ctx, db, prefixCipher{}, row.ID, 5, map[string]string{}))
	assert.Empty(t, reload().ExtraEncrypted, "空的存成空串")
	err := saveNASExtra(ctx, db, prefixCipher{failEncrypt: true}, row.ID, 5, map[string]string{"token": "t"})
	assert.ErrorContains(t, err, "加密登录凭据失败")

	enc, err := encodeNASExtra(prefixCipher{}, nil)
	require.NoError(t, err)
	assert.Empty(t, enc)
	_, err = decodeNASExtra(prefixCipher{}, "garbage")
	assert.Error(t, err)
}

func TestNewNASCollector(t *testing.T) {
	ctx := context.Background()
	db := newNASTestDB(t, true)
	c := newNASCollectorWith(db, prefixCipher{})
	cfg, err := c.Load(ctx)
	require.NoError(t, err)
	assert.True(t, cfg.LocalEnabled)
	row := models.NasSource{Name: "群晖", Kind: "synology", Enabled: true, URL: "https://ds", Revision: 1}
	require.NoError(t, db.Create(&row).Error)
	require.NoError(t, c.SaveExtra(ctx, row.ID, 1, map[string]string{"did": "x"}))
	var got models.NasSource
	require.NoError(t, db.First(&got, row.ID).Error)
	assert.Equal(t, `enc:{"did":"x"}`, got.ExtraEncrypted)
	c.Logf("测试 %d", 1)

	store := core.NewConfigStore(&models.TorrentDB{DB: db})
	assert.NotNil(t, newNASCollector(db, store).Load)
}
