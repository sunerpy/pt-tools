package organize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sunerpy/pt-tools/internal/media/scrape"
	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

// HistoryQuery 是整理记录的查询。
type HistoryQuery struct {
	Status  string
	Keyword string
	Limit   int
	Offset  int
}

// HistoryItem 是一条整理记录，带上媒体库的名字与字幕数。
type HistoryItem struct {
	models.MediaTransferHistory
	LibraryName string `json:"library_name,omitempty"`
	Subtitles   int    `json:"subtitles"`
}

// HistoryPage 是一页整理记录。
type HistoryPage struct {
	Items []HistoryItem `json:"items"`
	Total int64         `json:"total"`
}

func decodeExtras(raw string) []extra {
	var out []extra
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out
}

// History 按时间倒序列出整理记录，可以按状态与关键字（种子名、标题、路径）筛选。
func (s *Service) History(ctx context.Context, q HistoryQuery) (HistoryPage, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	q.Offset = max(q.Offset, 0)
	db := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{})
	switch q.Status {
	case "":
	case models.MediaTransferDone, models.MediaTransferFailed, models.MediaTransferSkipped, models.MediaTransferRemoved:
		db = db.Where("status = ?", q.Status)
	default:
		return HistoryPage{}, fmt.Errorf("%w: 状态要选 done、failed、skipped 或 removed", ErrInvalid)
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		if len(kw) > 200 {
			return HistoryPage{}, fmt.Errorf("%w: 关键字太长", ErrInvalid)
		}
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(kw) + "%"
		db = db.Where(`torrent_name LIKE ? ESCAPE '\' OR title LIKE ? ESCAPE '\' OR source_path LIKE ? ESCAPE '\' OR target_path LIKE ? ESCAPE '\'`, like, like, like, like)
	}
	var page HistoryPage
	if err := db.Count(&page.Total).Error; err != nil {
		return HistoryPage{}, fmt.Errorf("读取整理记录失败: %w", err)
	}
	var rows []models.MediaTransferHistory
	if err := db.Order("updated_at DESC, id DESC").Limit(q.Limit).Offset(q.Offset).Find(&rows).Error; err != nil {
		return HistoryPage{}, fmt.Errorf("读取整理记录失败: %w", err)
	}
	var libs []models.MediaLibrary
	if err := s.cfg.DB.WithContext(ctx).Find(&libs).Error; err != nil {
		return HistoryPage{}, fmt.Errorf("读取媒体库失败: %w", err)
	}
	names := map[uint]string{}
	for _, l := range libs {
		names[l.ID] = l.Name
	}
	page.Items = make([]HistoryItem, 0, len(rows))
	for _, r := range rows {
		page.Items = append(page.Items, HistoryItem{MediaTransferHistory: r, LibraryName: names[r.LibraryID], Subtitles: len(decodeExtras(r.Extras))})
	}
	return page, nil
}

func (s *Service) historyRow(ctx context.Context, id uint) (models.MediaTransferHistory, error) {
	var row models.MediaTransferHistory
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&row).Error; err != nil {
		return row, fmt.Errorf("读取整理记录失败: %w", err)
	}
	if row.ID == 0 {
		return row, ErrNotFound
	}
	return row, nil
}

// Retry 重新整理这条记录所在的种子（按手动整理处理）。之前手动指定过条目的，沿用那个条目。
func (s *Service) Retry(ctx context.Context, id uint) (*Result, error) {
	row, err := s.historyRow(ctx, id)
	if err != nil {
		return nil, err
	}
	req := Request{DownloaderID: row.DownloaderID, Hash: firstNonEmpty(row.TaskID, row.InfoHash)}
	if row.Trigger == models.MediaTriggerManual && row.TMDBID > 0 {
		req.MediaType, req.TMDBID, req.LibraryID = row.MediaType, row.TMDBID, row.LibraryID
	}
	return s.Organize(ctx, req)
}

// DeleteHistory 删除一条整理记录。files 为真时连库里整理出的文件一起删：视频、字幕、同名的 NFO 与图片，
// 目录里不再有视频时再删目录级的海报与 NFO 和空目录。只删确认还是当初整理出的那个文件；
// 移动整理的文件是唯一的一份，不在这里删。
func (s *Service) DeleteHistory(ctx context.Context, id uint, files bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.historyRow(ctx, id)
	if err != nil {
		return err
	}
	if files && row.Status == models.MediaTransferDone && row.TargetPath != "" {
		if row.Mode == models.MediaModeMove {
			return fmt.Errorf("%w: 移动整理的文件是唯一的一份，不在这里删除；只删记录时去掉「同时删除库里的文件」", ErrInvalid)
		}
		if err := s.removeLibraryFiles(ctx, row); err != nil {
			return err
		}
	}
	if err := s.cfg.DB.WithContext(ctx).Delete(&models.MediaTransferHistory{}, row.ID).Error; err != nil {
		return fmt.Errorf("删除整理记录失败: %w", err)
	}
	return nil
}

// companions 是和视频同名、刮削时写的文件。
func companions(target string) []string {
	stem := strings.TrimSuffix(target, filepath.Ext(target))
	return []string{stem + ".nfo", stem + "-thumb.jpg", stem + "-poster.jpg", stem + "-fanart.jpg"}
}

var seasonPosterRe = regexp.MustCompile(`^season\d+-poster\.jpg$`)

// dirArtifacts 是刮削时写在目录里的文件：目录里不再有视频时一起删。季海报按目录里的文件名找
// （不用 Glob：目录名里常有 [tmdbid-…]，方括号会被当成通配符）。
func dirArtifacts(dir string) []string {
	out := []string{"poster.jpg", "fanart.jpg", "season.nfo", "tvshow.nfo", scrape.SeasonPosterName(0)}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && seasonPosterRe.MatchString(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out
}

// hasVideo 报告目录里（含子目录）还有没有视频文件。读不了目录时当作有。
func hasVideo(dir string) bool {
	found := false
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && transfer.IsVideo(d.Name()) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found || (err != nil && !errors.Is(err, os.ErrNotExist))
}

// removeLibraryFiles 删掉一条记录在库里整理出的文件（调用方持有 s.mu）。
func (s *Service) removeLibraryFiles(ctx context.Context, row models.MediaTransferHistory) error {
	if err := transfer.RemoveIfOurs(row.TargetPath, row.Mode, row.TargetFileID, row.SourcePath); err != nil {
		return err
	}
	var errs []error
	for _, e := range decodeExtras(row.Extras) {
		if err := transfer.RemoveIfOurs(e.Target, row.Mode, e.FileID, e.Source); err != nil {
			errs = append(errs, err)
		}
	}
	for _, c := range companions(row.TargetPath) {
		if err := os.Remove(c); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	var lib models.MediaLibrary
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", row.LibraryID).Limit(1).Find(&lib).Error; err != nil || lib.ID == 0 {
		return errors.Join(errs...)
	}
	dirs := []string{filepath.Dir(row.TargetPath)}
	if sd := showDir(&lib, row.TargetPath); sd != "" && sd != dirs[0] {
		dirs = append(dirs, sd)
	}
	for _, d := range dirs {
		if !transfer.Within(lib.Path, d) || hasVideo(d) {
			continue
		}
		for _, name := range dirArtifacts(d) {
			_ = os.Remove(filepath.Join(d, name))
		}
		transfer.RemoveEmptyDirs(d, lib.Path)
	}
	return errors.Join(errs...)
}

// Reconcile 用在打开了「删种时一并删除入库链接」时：种子已经不在下载器里、源文件也没了（连数据删掉了）的，
// 删掉库里整理出的硬链接与软链接，记录改成 removed。复制与移动的文件不动。返回清理的文件数。
func (s *Service) Reconcile(ctx context.Context) int {
	var rows []models.MediaTransferHistory
	if err := s.cfg.DB.WithContext(ctx).
		Where("status = ? AND mode IN ? AND target_path != ''", models.MediaTransferDone, []string{models.MediaModeHardlink, models.MediaModeSymlink}).
		Order("downloader_id, id").Find(&rows).Error; err != nil {
		s.cfg.Logger.Warnf("[整理入库] 读取整理记录失败: %v", err)
		return 0
	}
	byDL := map[uint][]models.MediaTransferHistory{}
	for _, r := range rows {
		byDL[r.DownloaderID] = append(byDL[r.DownloaderID], r)
	}
	libRoots := map[uint]string{}
	var libs []models.MediaLibrary
	if err := s.cfg.DB.WithContext(ctx).Find(&libs).Error; err == nil {
		for _, l := range libs {
			libRoots[l.ID] = l.Path
		}
	}
	removed := 0
	for dlID, list := range byDL {
		dl, _, err := s.cfg.Downloaders.Get(ctx, dlID)
		if err != nil {
			continue
		}
		all, err := dl.GetAllTorrents()
		if err != nil {
			continue
		}
		present := make(map[string]bool, len(all))
		for _, t := range all {
			present[strings.ToLower(t.InfoHash)] = true
		}
		for _, r := range list {
			if present[r.InfoHash] {
				continue
			}
			if _, err := os.Lstat(r.SourcePath); !errors.Is(err, os.ErrNotExist) {
				continue // 数据还在（只删了种子、没删数据，或者移走了）
			}
			root, ok := libRoots[r.LibraryID]
			if !ok {
				continue
			}
			if info, err := os.Stat(root); err != nil || !info.IsDir() {
				continue // 库目录不在（例如没挂载），不动记录
			}
			s.mu.Lock()
			err := s.removeLibraryFiles(ctx, r)
			s.mu.Unlock()
			if err != nil {
				s.cfg.Logger.Warnf("[整理入库] 清理入库链接失败 (%s): %v", r.TargetPath, err)
				continue
			}
			s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).Where("id = ?", r.ID).Updates(map[string]any{
				"status": models.MediaTransferRemoved, "message": "种子与数据已删除，库里的链接一并删除", "updated_at": s.cfg.Now(),
			})
			removed++
		}
	}
	if removed > 0 {
		s.cfg.Logger.Infof("[整理入库] 种子连数据删掉后，清理了 %d 个入库链接", removed)
	}
	return removed
}
