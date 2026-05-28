// MIT License
// Copyright (c) 2025 pt-tools

// Issue #373 regression tests: 多下载器场景下，绑定到站点的下载器必须生效。
//
// 修复后的优先级（GetDownloaderForRSSAndSiteWithInfo）：
//  1. rssCfg.DownloaderID（RSS 行级覆盖）
//  2. SiteSetting.DownloaderID（站点绑定，本 issue 修复点）
//  3. is_default=true（兜底）
//
// 旧 API GetDownloaderForRSSWithInfo（无站点上下文）保留向后兼容，仅 1+3 路径。

package internal

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	sm "github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// installFakeDMWithMocks 安装一个 manager，把"创建下载器"路径短路掉。
// 工厂总是返回一个全部 AnyTimes 的 MockDownloader —— 我们不关心 dm.GetDownloader
// 之后能不能跑，只关心 resolver 的"我选谁"决策。
func installFakeDMWithMocks(t *testing.T) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	dm := downloader.NewDownloaderManager()
	factory := func(_ downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		md := sm.NewMockDownloader(ctrl)
		md.EXPECT().GetName().Return(name).AnyTimes()
		md.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
		md.EXPECT().IsHealthy().Return(true).AnyTimes()
		md.EXPECT().Ping().Return(true, nil).AnyTimes()
		md.EXPECT().Close().Return(nil).AnyTimes()
		return md, nil
	}
	dm.RegisterFactory(downloader.DownloaderQBittorrent, factory)
	SetGlobalDownloaderManager(dm)
	t.Cleanup(func() { SetGlobalDownloaderManager(nil) })
}

// seedTwoDownloadersAndSiteBinding 在 DB 内写入一份「典型多下载器」状态：
//   - qbit-default:  is_default=true,  enabled=true
//   - qbit-mteam:    is_default=false, enabled=true
//   - SiteSetting{ Name: "mteam", DownloaderID: qbit-mteam.id }
//
// 返回两个下载器行的主键，便于断言。
func seedTwoDownloadersAndSiteBinding(t *testing.T) (defaultID, mteamID uint) {
	t.Helper()
	db := global.GlobalDB
	require.NotNil(t, db, "调用前先 setupDB(t)")

	dlDefault := models.DownloaderSetting{
		Name:      "qbit-default",
		Type:      "qbittorrent",
		URL:       "http://127.0.0.1:8080",
		Username:  "admin",
		Password:  "x",
		IsDefault: true,
		Enabled:   true,
	}
	require.NoError(t, db.DB.Create(&dlDefault).Error)

	dlMteam := models.DownloaderSetting{
		Name:      "qbit-mteam",
		Type:      "qbittorrent",
		URL:       "http://127.0.0.1:8081",
		Username:  "admin",
		Password:  "y",
		IsDefault: false,
		Enabled:   true,
	}
	require.NoError(t, db.DB.Create(&dlMteam).Error)

	site := models.SiteSetting{
		Name:         "mteam",
		BaseURL:      "https://example.invalid",
		Enabled:      true,
		AuthMethod:   "cookie",
		DownloaderID: &dlMteam.ID,
	}
	require.NoError(t, db.DB.Create(&site).Error)

	return dlDefault.ID, dlMteam.ID
}

// TestIssue373_NewResolver_HonorsSiteBinding 修复后的核心契约：
// 站点绑定必须被 resolver 读取。RSS 行 DownloaderID=nil 时，
// 走 SiteSetting.DownloaderID 路径，选中 qbit-mteam（不是 qbit-default）。
func TestIssue373_NewResolver_HonorsSiteBinding(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	installFakeDMWithMocks(t)

	defaultID, mteamID := seedTwoDownloadersAndSiteBinding(t)
	require.NotEqual(t, defaultID, mteamID)

	rssCfg := models.RSSConfig{
		ID:           42,
		Name:         "mteam-free",
		URL:          "https://example.invalid/torrentrss.php?xxx",
		DownloaderID: nil,
	}

	_, info, err := GetDownloaderForRSSAndSiteWithInfo(rssCfg, "mteam")
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.Equal(t, "qbit-mteam", info.Name,
		"修复 #373：RSS resolver 必须读取 SiteSetting.DownloaderID，选 qbit-mteam")
	assert.Equal(t, mteamID, info.ID)
}

// TestIssue373_NewResolver_RSSOverrideWinsOverSiteBinding 优先级 1 > 2：
// RSS 行自己指定了 DownloaderID 时，必须忽略站点绑定，走 RSS 自己的下载器。
func TestIssue373_NewResolver_RSSOverrideWinsOverSiteBinding(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	installFakeDMWithMocks(t)

	defaultID, mteamID := seedTwoDownloadersAndSiteBinding(t)
	_ = mteamID

	rssCfg := models.RSSConfig{
		ID:           1,
		Name:         "rss-override",
		URL:          "https://x.invalid/",
		DownloaderID: &defaultID, // RSS 行明确指定 qbit-default
	}

	_, info, err := GetDownloaderForRSSAndSiteWithInfo(rssCfg, "mteam")
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.Equal(t, "qbit-default", info.Name,
		"优先级 1 (RSS.DownloaderID) 必须高于优先级 2 (SiteSetting.DownloaderID)")
	assert.Equal(t, defaultID, info.ID)
}

// TestIssue373_NewResolver_FallsBackToDefaultWhenSiteHasNoBinding 优先级 3：
// 站点存在但未绑定下载器（DownloaderID=NULL）时，回落 is_default。
func TestIssue373_NewResolver_FallsBackToDefaultWhenSiteHasNoBinding(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	installFakeDMWithMocks(t)

	defaultID, _ := seedTwoDownloadersAndSiteBinding(t)
	// 把 mteam 的绑定清掉，模拟"站点存在但没绑下载器"
	require.NoError(t, global.GlobalDB.DB.
		Model(&models.SiteSetting{}).
		Where("name = ?", "mteam").
		Update("downloader_id", nil).Error)

	rssCfg := models.RSSConfig{ID: 1, Name: "x", URL: "https://x.invalid/", DownloaderID: nil}
	_, info, err := GetDownloaderForRSSAndSiteWithInfo(rssCfg, "mteam")
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.Equal(t, "qbit-default", info.Name, "站点未绑定 → fallback is_default")
	assert.Equal(t, defaultID, info.ID)
}

// TestIssue373_NewResolver_FallsBackWhenSiteRowMissing 优先级 3 边界：
// 站点行根本不存在（site_settings 里没这一行）时也安全 fallback。
func TestIssue373_NewResolver_FallsBackWhenSiteRowMissing(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	installFakeDMWithMocks(t)

	defaultID, _ := seedTwoDownloadersAndSiteBinding(t)

	rssCfg := models.RSSConfig{ID: 1, Name: "x", URL: "https://x.invalid/", DownloaderID: nil}
	_, info, err := GetDownloaderForRSSAndSiteWithInfo(rssCfg, "this-site-does-not-exist")
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.Equal(t, "qbit-default", info.Name, "未知站点 → fallback is_default")
	assert.Equal(t, defaultID, info.ID)
}

// TestIssue373_NewResolver_FallsBackWhenBoundDownloaderDeleted 优先级 3 边界：
// 站点绑定的下载器被删除（外键悬挂）时也安全 fallback，不报错。
func TestIssue373_NewResolver_FallsBackWhenBoundDownloaderDeleted(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	installFakeDMWithMocks(t)

	defaultID, mteamID := seedTwoDownloadersAndSiteBinding(t)
	// 删掉 qbit-mteam（站点绑定指向悬挂 ID）
	require.NoError(t, global.GlobalDB.DB.Delete(&models.DownloaderSetting{}, mteamID).Error)

	rssCfg := models.RSSConfig{ID: 1, Name: "x", URL: "https://x.invalid/", DownloaderID: nil}
	_, info, err := GetDownloaderForRSSAndSiteWithInfo(rssCfg, "mteam")
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.Equal(t, "qbit-default", info.Name,
		"站点绑定指向已删除的下载器 → 不应报错，应安全 fallback is_default")
	assert.Equal(t, defaultID, info.ID)
}

// TestIssue373_NewResolver_RejectsDisabledBoundDownloader 反向：
// 站点绑定的下载器被禁用（Enabled=false）时必须明确报错，不能静默 fallback —
// 否则用户配置意图被悄悄改写。
func TestIssue373_NewResolver_RejectsDisabledBoundDownloader(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	installFakeDMWithMocks(t)

	_, mteamID := seedTwoDownloadersAndSiteBinding(t)
	require.NoError(t, global.GlobalDB.DB.
		Model(&models.DownloaderSetting{}).
		Where("id = ?", mteamID).
		Update("enabled", false).Error)

	rssCfg := models.RSSConfig{ID: 1, Name: "x", URL: "https://x.invalid/", DownloaderID: nil}
	_, _, err := GetDownloaderForRSSAndSiteWithInfo(rssCfg, "mteam")
	require.Error(t, err, "绑定的下载器被禁用必须报错而非 fallback")
	assert.Contains(t, err.Error(), "qbit-mteam")
	assert.Contains(t, err.Error(), "未启用")
}

// TestIssue373_LegacyResolver_NoSiteContextStillWorks 向后兼容：
// 旧 API GetDownloaderForRSSWithInfo（无站点上下文）保持原有行为，
// 仍然走 RSS.DownloaderID > is_default 两路径，不查 SiteSetting。
//
// 这条测试存在的意义：保证现有 cmd/rss、CLI 等无站点上下文的调用点不被修复破坏。
func TestIssue373_LegacyResolver_NoSiteContextStillWorks(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	installFakeDMWithMocks(t)

	defaultID, mteamID := seedTwoDownloadersAndSiteBinding(t)
	_ = mteamID

	rssCfg := models.RSSConfig{ID: 99, Name: "no-site-ctx", URL: "https://x.invalid/", DownloaderID: nil}
	_, info, err := GetDownloaderForRSSWithInfo(rssCfg)
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.Equal(t, "qbit-default", info.Name,
		"无站点上下文 → 不查 SiteSetting，直接 fallback is_default（向后兼容）")
	assert.Equal(t, defaultID, info.ID)
}
