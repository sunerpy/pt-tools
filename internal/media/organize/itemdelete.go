package organize

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/sunerpy/pt-tools/internal/media/serverindex"
	"github.com/sunerpy/pt-tools/models"
)

// 条目详情里的「删除一部片」（媒体中心 M7）要用的整理记录：预览列出库里的文件，删的时候逐条走 DeleteHistoryWith。

// FileQuery 选一个条目的整理记录：电影按 TMDB 编号；剧集还要季，Episodes 不为空时只要和这几集有交集的。
type FileQuery struct {
	MediaType string
	TMDBID    int
	Season    int
	Episodes  []int
}

// 库里的文件现在的样子（LibraryFile.State）。
const (
	// FileOurs 是还是当初整理出的那个文件
	FileOurs = "ours"
	// FileMissing 是库里没有这个文件了（删记录照样可以）
	FileMissing = "missing"
	// FileChanged 是换成了别的文件或改过：删的时候留着
	FileChanged = "changed"
)

// LibraryFile 是删除预览里的一条整理记录在库里的文件。Deletable 为假时不能在这里连文件删（Note 写明原因）；
// Existing 是导入的已有文件（导入时就在库里），要另外确认才删。
type LibraryFile struct {
	ID         uint   `json:"id"`
	Path       string `json:"path"`
	Library    string `json:"library,omitempty"`
	Mode       string `json:"mode"`
	Existing   bool   `json:"existing"`
	Size       int64  `json:"size"`
	Season     int    `json:"season"`
	Episode    int    `json:"episode"`
	EpisodeEnd int    `json:"episode_end"`
	State      string `json:"state"`
	Deletable  bool   `json:"deletable"`
	Note       string `json:"note,omitempty"`
	// DownloaderID、InfoHash 是整理的种子（目录监控与导入的没有下载器）
	DownloaderID uint   `json:"downloader_id,omitempty"`
	InfoHash     string `json:"info_hash,omitempty"`
}

// Matches 报告整理记录是不是这个条目（剧集是这一季、选了集时和这几集有交集）的。
func (q FileQuery) Matches(r models.MediaTransferHistory) bool {
	if r.MediaType != q.MediaType || r.TMDBID != q.TMDBID || q.TMDBID <= 0 {
		return false
	}
	if q.MediaType != models.MediaKindTV {
		return true
	}
	if r.Season != q.Season {
		return false
	}
	if len(q.Episodes) == 0 {
		return true
	}
	if r.Episode <= 0 {
		return false
	}
	return slices.ContainsFunc(q.Episodes, func(n int) bool { return n >= r.Episode && n <= max(r.EpisodeEnd, r.Episode) })
}

// LibraryFiles 是这个条目在库里有文件的整理记录（见 hasLibraryFiles），按季、集、编号排。
func (s *Service) LibraryFiles(ctx context.Context, q FileQuery) ([]LibraryFile, error) {
	out := []LibraryFile{}
	if q.TMDBID <= 0 {
		return out, nil
	}
	db := s.cfg.DB.WithContext(ctx).Where("media_type = ? AND tmdb_id = ? AND status <> ?", q.MediaType, q.TMDBID, models.MediaTransferRemoved)
	if q.MediaType == models.MediaKindTV {
		db = db.Where("season = ?", q.Season)
	}
	var rows []models.MediaTransferHistory
	if err := db.Order("season, episode, id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取整理记录失败: %w", err)
	}
	var libs []models.MediaLibrary
	if err := s.cfg.DB.WithContext(ctx).Find(&libs).Error; err != nil {
		return nil, fmt.Errorf("读取媒体库失败: %w", err)
	}
	names := map[uint]string{}
	for _, l := range libs {
		names[l.ID] = l.Name
	}
	for i := range rows {
		r := &rows[i]
		if !hasLibraryFiles(r) || !q.Matches(*r) {
			continue
		}
		f := LibraryFile{
			ID: r.ID, Path: r.TargetPath, Library: names[r.LibraryID], Mode: r.Mode, Existing: r.Mode == models.MediaModeExisting,
			Size: r.Size, Season: r.Season, Episode: r.Episode, EpisodeEnd: r.EpisodeEnd, State: fileState(r), Deletable: true,
			DownloaderID: r.DownloaderID, InfoHash: r.InfoHash,
		}
		switch {
		case r.Mode == models.MediaModeMove:
			f.Deletable, f.Note = false, "移动整理的文件是唯一的一份，不在这里删"
		case f.State == FileChanged:
			f.Note = "库里的文件换成了别的或改过，删的时候留着它，只删记录"
		case f.State == FileMissing:
			f.Note = "库里已经没有这个文件了，删的是记录"
		}
		out = append(out, f)
	}
	return out, nil
}

// fileState 是整理记录在库里的文件现在的样子（见 FileOurs 等）。
func fileState(r *models.MediaTransferHistory) string {
	if !libraryFilePresent(r) {
		return FileMissing
	}
	if ours(r) {
		return FileOurs
	}
	return FileChanged
}

// libraryFilePresent 报告记录在库里的文件还在不在：原盘看主文件，软链接要指向的文件也在，别的看文件本身。
func libraryFilePresent(r *models.MediaTransferHistory) bool {
	if main, ok := discMain(r); ok {
		_, err := os.Stat(main.Target)
		return err == nil
	}
	if r.Mode == models.MediaModeSymlink {
		_, err := os.Stat(r.TargetPath)
		return err == nil
	}
	_, err := os.Lstat(r.TargetPath)
	return err == nil
}

// KeepLinks 标上这几条记录留着库里的文件（在条目详情里连数据删种子、没选删库文件时）：打开了「删种时一并删除入库链接」
// 也不删它们的硬链接。只标已整理的硬链接记录：复制、移动、导入的文件本来就不跟着删，软链接在数据删了以后打不开。
func (s *Service) KeepLinks(ctx context.Context, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).
		Where("id IN ? AND status = ? AND mode = ?", ids, models.MediaTransferDone, models.MediaModeHardlink).
		UpdateColumn("keep_links", true).Error; err != nil {
		return fmt.Errorf("更新整理记录失败: %w", err)
	}
	return nil
}

// recordGone 在整理记录删了或改成已删除以后收拾它的问题：这条记录的 library_missing 改成解决了；这一部还开着的
// server_removed 对不上任何已整理记录的也改成解决了。出错只记日志：记录与文件已经处理完了。
func (s *Service) recordGone(ctx context.Context, row models.MediaTransferHistory, detail string) {
	now := s.cfg.Now()
	db := s.cfg.DB.WithContext(ctx)
	err := serverindex.Clear(db, serverindex.MissingKey(row.ID), detail, now)
	if err == nil && row.TMDBID > 0 {
		err = serverindex.ClearUnmatched(db, row.MediaType, row.TMDBID, detail, now)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		s.cfg.Logger.Warnf("[整理入库] 收拾整理记录 %d 的问题失败: %v", row.ID, err)
	}
}
