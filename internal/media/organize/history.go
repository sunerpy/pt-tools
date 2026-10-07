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

// HistoryItem 是一条整理记录，带上媒体库的名字、字幕数，以及库里有没有记着的、可以一起删的文件。
type HistoryItem struct {
	models.MediaTransferHistory
	LibraryName string `json:"library_name,omitempty"`
	Subtitles   int    `json:"subtitles"`
	HasFiles    bool   `json:"has_files"`
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
		subs := 0
		for _, e := range decodeExtras(r.Extras) {
			if e.Kind == "" {
				subs++
			}
		}
		page.Items = append(page.Items, HistoryItem{MediaTransferHistory: r, LibraryName: names[r.LibraryID], Subtitles: subs, HasFiles: hasLibraryFiles(&r)})
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

// hasLibraryFiles 报告记录里有没有记着 pt-tools 放进库的文件：已整理的，或者之后变成跳过、失败（例如库里的视频被换掉了），
// 但之前整理出过文件的。
func hasLibraryFiles(r *models.MediaTransferHistory) bool {
	if r == nil || r.Status == models.MediaTransferRemoved {
		return false
	}
	return (r.Status == models.MediaTransferDone && r.TargetPath != "") || r.TargetFileID != "" || len(decodeExtras(r.Extras)) > 0
}

// DeleteHistory 删除一条整理记录。files 为真时连库里整理出的文件一起删：视频、字幕、同名的 NFO 与图片，
// 目录里不再有视频时再删目录级的海报与 NFO 和空目录。只删确认还是当初整理出的那个文件，换掉或改过的留着，
// 返回留着的文件的说明；移动整理的文件是唯一的一份，不在这里删。
func (s *Service) DeleteHistory(ctx context.Context, id uint, files bool) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.historyRow(ctx, id)
	if err != nil {
		return nil, err
	}
	var kept []string
	if files && hasLibraryFiles(&row) {
		if row.Mode == models.MediaModeMove {
			return nil, fmt.Errorf("%w: 移动整理的文件是唯一的一份，不在这里删除；只删记录时去掉「同时删除库里的文件」", ErrInvalid)
		}
		if kept, err = s.removeLibraryFiles(ctx, row); err != nil {
			return nil, err
		}
	}
	if err := s.cfg.DB.WithContext(ctx).Delete(&models.MediaTransferHistory{}, row.ID).Error; err != nil {
		return nil, fmt.Errorf("删除整理记录失败: %w", err)
	}
	return kept, nil
}

var seasonPosterRe = regexp.MustCompile(`^season(\d+|-specials)-poster\.jpg$`)

// isDirArtifact 报告文件名是不是刮削写在目录里的文件（海报、背景、tvshow.nfo、season.nfo、季海报）。
func isDirArtifact(name string) bool {
	switch name {
	case "poster.jpg", "fanart.jpg", "tvshow.nfo", "season.nfo":
		return true
	}
	return seasonPosterRe.MatchString(name)
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

// removeLibraryFiles 删掉一条记录在库里整理出的文件（调用方持有 s.mu）：视频与字幕按文件编号或软链接指向确认；
// 刮削写的文件只删记录里有、文件编号还对得上的，目录里的那几个要等目录里不再有视频时才删，还有视频时转给目录里另一条记录。
// 最后删空目录。换掉或改过的文件留着，返回它们的说明；转交失败时返回错误，调用方不要删记录。
func (s *Service) removeLibraryFiles(ctx context.Context, row models.MediaTransferHistory) ([]string, error) {
	var kept []string
	var errs []error
	remove := func(target, mode, fileID, src string) error {
		err := transfer.RemoveIfOurs(target, mode, fileID, src)
		if errors.Is(err, transfer.ErrNotOurs) {
			kept = append(kept, err.Error())
			return nil
		}
		return err
	}
	if row.TargetPath != "" {
		if err := remove(row.TargetPath, row.Mode, row.TargetFileID, row.SourcePath); err != nil {
			return nil, err
		}
	}
	var shared []extra
	for _, e := range decodeExtras(row.Extras) {
		switch {
		case e.Kind == extraMeta && isDirArtifact(filepath.Base(e.Target)):
			shared = append(shared, e)
		case e.Kind == extraMeta:
			// 刮削写的文件是普通文件：按文件编号确认（软链接方式整理的视频也一样）
			if err := remove(e.Target, models.MediaModeCopy, e.FileID, ""); err != nil {
				errs = append(errs, err)
			}
		default:
			if err := remove(e.Target, row.Mode, e.FileID, e.Source); err != nil {
				errs = append(errs, err)
			}
		}
	}
	var lib models.MediaLibrary
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", row.LibraryID).Limit(1).Find(&lib).Error; err != nil {
		return nil, fmt.Errorf("读取媒体库失败: %w", err)
	}
	if lib.ID == 0 {
		return kept, errors.Join(errs...)
	}
	var stay []extra
	for _, e := range shared {
		d := filepath.Dir(e.Target)
		if !transfer.Within(lib.Path, d) {
			continue
		}
		if hasVideo(d) {
			stay = append(stay, e)
			continue
		}
		if err := remove(e.Target, models.MediaModeCopy, e.FileID, ""); err != nil {
			errs = append(errs, err)
		}
	}
	if err := s.handOver(ctx, row, stay); err != nil {
		return nil, err
	}
	dirs := []string{filepath.Dir(row.TargetPath)}
	if sd := showDir(&lib, row.TargetPath); sd != "" && sd != dirs[0] {
		dirs = append(dirs, sd)
	}
	for _, d := range dirs {
		if transfer.Within(lib.Path, d) {
			transfer.RemoveEmptyDirs(d, lib.Path)
		}
	}
	return kept, errors.Join(errs...)
}

// handOver 把目录里还有视频、这次没删的刮削文件转给这个目录里另一条已整理的记录，删到最后一个视频时还能删掉。
func (s *Service) handOver(ctx context.Context, row models.MediaTransferHistory, stay []extra) error {
	if len(stay) == 0 {
		return nil
	}
	var rows []models.MediaTransferHistory
	if err := s.cfg.DB.WithContext(ctx).
		Where("library_id = ? AND status = ? AND id <> ?", row.LibraryID, models.MediaTransferDone, row.ID).
		Order("id").Find(&rows).Error; err != nil {
		return fmt.Errorf("读取整理记录失败: %w", err)
	}
	add := map[uint][]extra{}
	for _, e := range stay {
		d := filepath.Dir(e.Target)
		for _, r := range rows {
			if transfer.Within(d, r.TargetPath) {
				add[r.ID] = append(add[r.ID], e)
				break
			}
		}
	}
	for _, r := range rows {
		if len(add[r.ID]) == 0 {
			continue
		}
		b, _ := json.Marshal(mergeExtras(decodeExtras(r.Extras), add[r.ID]))
		if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).Where("id = ?", r.ID).Update("extras", string(b)).Error; err != nil {
			return fmt.Errorf("把目录里的刮削文件转给其他整理记录失败: %w", err)
		}
	}
	return nil
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
			// 保存目录不在（例如下载目录没挂载）时认不出数据是不是真的删了，不动
			if info, err := os.Stat(r.SaveRoot); r.SaveRoot == "" || err != nil || !info.IsDir() {
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
			ok, err := s.reconcileRow(ctx, r.ID)
			s.mu.Unlock()
			if err != nil {
				s.cfg.Logger.Warnf("[整理入库] 清理入库链接失败 (%s): %v", r.TargetPath, err)
				continue
			}
			if ok {
				removed++
			}
		}
	}
	if removed > 0 {
		s.cfg.Logger.Infof("[整理入库] 种子连数据删掉后，清理了 %d 个入库链接", removed)
	}
	return removed
}

// reconcileRow 清理一条记录在库里的链接并把记录改成 removed（调用方持有 s.mu）。按编号重新读一遍：
// 同一轮里前面的记录可能刚把目录级的刮削文件转给了这一条，这一条也可能刚被删掉或重新整理过。
func (s *Service) reconcileRow(ctx context.Context, id uint) (bool, error) {
	row, err := s.historyRow(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if row.Status != models.MediaTransferDone {
		return false, nil
	}
	kept, err := s.removeLibraryFiles(ctx, row)
	if err != nil {
		return false, err
	}
	msg := "种子与数据已删除，库里的链接一并删除"
	if len(kept) > 0 {
		msg += "；" + strings.Join(kept, "；")
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).Where("id = ?", row.ID).Updates(map[string]any{
		"status": models.MediaTransferRemoved, "message": truncate(msg, 1024), "updated_at": s.cfg.Now(),
	}).Error; err != nil {
		return false, fmt.Errorf("更新整理记录失败: %w", err)
	}
	return true, nil
}
