package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupNasTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(db) })
	require.NoError(t, db.AutoMigrate(&NasSource{}, &NasSetting{}))
	return db
}

func TestNasTableNames(t *testing.T) {
	assert.Equal(t, "nas_sources", NasSource{}.TableName())
	assert.Equal(t, "nas_settings", NasSetting{}.TableName())
}

func TestGetNasSettingDefaultsWithoutRow(t *testing.T) {
	db := setupNasTestDB(t)

	set, err := GetNasSetting(db)
	require.NoError(t, err)
	assert.Equal(t, uint(1), set.ID)
	assert.True(t, set.LocalEnabled, "没有这一行时本机来源开着")
	assert.Empty(t, set.HomeSource, "没有这一行时首页自动选来源")
}

func TestSaveNasSettingKeepsSingleRow(t *testing.T) {
	db := setupNasTestDB(t)

	// 关掉本机来源：布尔列不带 default，false 要原样存下来
	require.NoError(t, SaveNasSetting(db, NasSetting{ID: 7, LocalEnabled: false, HomeSource: "local"}))
	set, err := GetNasSetting(db)
	require.NoError(t, err)
	assert.Equal(t, uint(1), set.ID, "不管传进来的编号，只写 id=1 那一行")
	assert.False(t, set.LocalEnabled)
	assert.Equal(t, "local", set.HomeSource)

	require.NoError(t, SaveNasSetting(db, NasSetting{LocalEnabled: true, HomeSource: "3"}))
	set, err = GetNasSetting(db)
	require.NoError(t, err)
	assert.True(t, set.LocalEnabled)
	assert.Equal(t, "3", set.HomeSource)

	var n int64
	require.NoError(t, db.Model(&NasSetting{}).Count(&n).Error)
	assert.Equal(t, int64(1), n)
}

func TestGetNasSettingReturnsQueryError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(db) })

	// 没建表：查询报错要原样返回，不能当成「没有这一行」回默认
	_, err = GetNasSetting(db)
	require.Error(t, err)
}
