// MIT License
// Copyright (c) 2025 pt-tools

package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/models"
)

// TB1：RetainHours<=0 时，手动清理（特性 D）不再整类禁用，而是用兜底保留期 24h 继续清理。
//   - 孤立 / 已推送 与 mtime 无关 → 仍应删除
//   - 超兜底保留期（>24h）未推送 → 用 24h 兜底判定后删除
//   - 新鲜（<24h）未推送 → 保留
//   - Note 提示使用默认保留期，而非"已禁用"
func TestCleanStaging_FallbackWhenRetainZero(t *testing.T) {
	home, db := setupCleanerHome(t)
	// RetainHours=0（禁用自动 sweep 的合法值），MaxRetry=3。
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), RetainHours: 24, MaxRetry: 3,
	}))
	// GORM 的 default:24 会在 INSERT 零值时写 24，故显式 UPDATE 成 0 模拟被清零的脏数据。
	require.NoError(t, db.DB.Model(&models.SettingsGlobal{}).Where("1 = 1").Update("retain_hours", 0).Error)
	tag := filepath.Join(home, models.WorkDir, "downloads", "mytag")
	require.NoError(t, os.MkdirAll(tag, 0o755))

	pushedPath, pushedHash := writeTorrent(t, tag, "springsunday-pushed.torrent", "pushed-z")
	seedTorrent(t, db, pushedHash, true, 0) // 已推送 → 删（与 mtime 无关）

	orphanPath, _ := writeTorrent(t, tag, "springsunday-orphan.torrent", "orphan-z") // 无 DB → 删

	agedPath, agedHash := writeTorrent(t, tag, "springsunday-aged.torrent", "aged-z")
	seedTorrent(t, db, agedHash, false, 0)
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(agedPath, old, old)) // 超兜底 24h 未推送 → 删

	freshPath, freshHash := writeTorrent(t, tag, "springsunday-fresh.torrent", "fresh-z")
	seedTorrent(t, db, freshHash, false, 0) // 新鲜未推送 → 保留

	c := NewCleaner(home, db, defaultZap())
	res, err := c.Clean(context.Background(), CleanOptions{Categories: []CleanCategory{CategoryStaging}, DryRun: false})
	require.NoError(t, err)

	assert.NoFileExists(t, pushedPath, "已推送应删（retain=0 也不禁用）")
	assert.NoFileExists(t, orphanPath, "孤立应删（retain=0 也不禁用）")
	assert.NoFileExists(t, agedPath, "超兜底保留期未推送应删")
	assert.FileExists(t, freshPath, "新鲜未推送应保留")

	require.Len(t, res.Categories, 1)
	note := res.Categories[0].Note
	assert.NotContains(t, note, "已禁用", "retain<=0 不应再显示已禁用")
	assert.Contains(t, note, "默认保留期", "应提示使用默认兜底保留期")
	_ = strings.TrimSpace(note)
}
