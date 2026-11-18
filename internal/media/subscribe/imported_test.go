package subscribe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// imported 记一条导入的已有文件的整理记录（同 organize 的导入）。
func (e *env) imported(kind string, tmdbID int, name, target string) models.MediaTransferHistory {
	e.t.Helper()
	row := models.MediaTransferHistory{
		DownloaderName: "导入", InfoHash: "import:" + name, TorrentName: name, SourcePath: target, TargetPath: target, MediaType: kind, TMDBID: tmdbID,
		Status: models.MediaTransferDone, Mode: models.MediaModeExisting, Trigger: models.MediaTriggerImport,
	}
	require.NoError(e.t, e.db.Create(&row).Error)
	return row
}

// 导入的已有文件算作库里已有：电影的订阅完成
func TestImportedFilesCountAsInLibrary(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	e.imported(models.MediaKindMovie, 693134, "Dune.Part.Two.2024.1080p.mkv", "/lib/dune2/dune2.mkv")
	e.svc.refresh(e.ctx)
	row := e.subRow(m.ID)
	assert.Equal(t, models.MediaSubDone, row.Status)
	assert.Equal(t, "已经入库", row.Message)
}

// 洗版：导入的已有文件不删（Retire 回 ErrExistingKept），新版本照常入库；新版本和它同名时不先删它，也不重新整理
func TestUpgradeKeepsImportedFiles(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	p, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "4K", Resolutions: []string{"2160p", "1080p"}})
	require.NoError(t, err)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: p.ID, Upgrade: true})
	old := e.imported(models.MediaKindMovie, 693134, "Dune.Part.Two.2024.1080p.WEB-DL.mkv", "/lib/dune2/dune2.mkv")
	e.search.set(e.item("hdsky", "81", "Dune.Part.Two.2024.2160p.WEB-DL.H265-X", "", 20, 50))
	msg, err := e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "下载了 Dune.Part.Two.2024.2160p", "库里的是 1080p：洗版接着找")

	// 新版本和导入的文件同名：整理跳过了，不删导入的文件、不重新整理
	skipped := models.MediaTransferHistory{
		InfoHash: e.hashOf("81"), SourcePath: "/dl/81.mkv", TargetPath: old.TargetPath, MediaType: models.MediaKindMovie, TMDBID: 693134, Status: models.MediaTransferSkipped,
	}
	require.NoError(t, e.db.Create(&skipped).Error)
	e.svc.refresh(e.ctx)
	assert.Equal(t, []uint{old.ID}, e.org.retired)
	assert.Empty(t, e.org.retried, "导入的文件没删：不重新整理")

	// 新版本整理到了别的文件名：导入的文件留着，订阅达到目标
	require.NoError(t, e.db.Model(&skipped).Updates(map[string]any{"status": models.MediaTransferDone, "target_path": "/lib/dune2/2160p.mkv"}).Error)
	e.org.retired = nil
	e.svc.refresh(e.ctx)
	assert.Equal(t, []uint{old.ID}, e.org.retired, "洗版时换下导入的文件")
	var kept models.MediaTransferHistory
	require.NoError(t, e.db.First(&kept, old.ID).Error)
	assert.Equal(t, models.MediaTransferDone, kept.Status, "导入的文件留着")
	assert.Equal(t, models.MediaSubDone, e.subRow(m.ID).Status)
}
