// MIT License
// Copyright (c) 2025 pt-tools

package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// TestEvalSymlinksExisting_NonexistentTail：不存在的候选路径按已存在父目录解析。
func TestEvalSymlinksExisting_NonexistentTail(t *testing.T) {
	dir := t.TempDir()
	resolved, err := evalSymlinksExisting(filepath.Join(dir, "sub", "deep", "file.torrent"))
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(resolved))
	assert.Contains(t, resolved, "file.torrent")
}

// TestStagingRetention_NilDB：db 为 nil 时返回禁用值。
func TestStagingRetention_NilDB(t *testing.T) {
	c := NewCleaner(t.TempDir(), nil, defaultZap())
	rh, mr := c.stagingRetention()
	assert.Equal(t, 0, rh)
	assert.Equal(t, 0, mr)
}

// TestCleanStaging_NilDBDisabled：db 为 nil 时 staging 清理禁用（无删除）。
func TestCleanStaging_NilDBDisabled(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, models.WorkDir)
	require.NoError(t, os.MkdirAll(filepath.Join(work, "downloads"), 0o755))
	c := NewCleaner(home, nil, defaultZap())
	cr := &CategoryResult{Category: CategoryStaging}
	c.cleanStaging(filepath.Join(work, "downloads"), false, cr)
	assert.NotEmpty(t, cr.Note)
	assert.Empty(t, cr.Deleted)
}

// TestCollectTorrentFiles_UnreadableRoot：不存在的根返回空。
func TestCollectTorrentFiles_UnreadableRoot(t *testing.T) {
	assert.Empty(t, collectTorrentFiles(filepath.Join(t.TempDir(), "nope")))
}

// TestCleanLogsAndBackups_UnreadableRoot：读目录失败时安全返回（无 panic/删除）。
func TestCleanLogsAndBackups_UnreadableRoot(t *testing.T) {
	c := NewCleaner(t.TempDir(), nil, defaultZap())
	missing := filepath.Join(t.TempDir(), "gone")
	crLogs := &CategoryResult{Category: CategoryLogs}
	c.cleanLogs(missing, false, crLogs)
	assert.Empty(t, crLogs.Deleted)
	crBk := &CategoryResult{Category: CategoryBackups}
	c.cleanBackups(missing, 5, false, crBk)
	assert.Empty(t, crBk.Deleted)
}

// TestSLoggerWithRealLogger：设置全局 logger 后 sLogger 返回真实实例。
func TestSLoggerWithRealLogger(t *testing.T) {
	orig := global.GlobalLogger
	global.GlobalLogger = zap.NewNop()
	t.Cleanup(func() { global.GlobalLogger = orig })
	assert.NotNil(t, sLogger())
}

// TestCandidateInRoot_SymlinkToExternalDirRejected：软链指向外部已存在目录下的候选被拒绝。
func TestCandidateInRoot_SymlinkToExternalDirRejected(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	extTarget := filepath.Join(external, "victim.torrent")
	require.NoError(t, os.WriteFile(extTarget, []byte("x"), 0o644))
	link := filepath.Join(root, "link.torrent")
	require.NoError(t, os.Symlink(extTarget, link))
	ok, why := candidateInRoot(root, link)
	assert.False(t, ok, "指向外部已存在文件的软链必须被拒绝")
	assert.NotEmpty(t, why)
}

// TestClean_ContextIgnoredButAccepts：传入 context 不影响执行（占位以覆盖签名）。
func TestClean_ContextIgnoredButAccepts(t *testing.T) {
	home, db := setupCleanerHome(t)
	c := NewCleaner(home, db, defaultZap())
	res, err := c.Clean(context.Background(), CleanOptions{Categories: []CleanCategory{CategoryBackups}})
	require.NoError(t, err)
	assert.False(t, res.DryRun)
}
