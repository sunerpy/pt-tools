package organize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/media/scrape"
	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

// HistoryQuery 是整理记录的查询。
type HistoryQuery struct {
	Status string
	// Kind 是失败与跳过的分类（models.MediaFailKinds 之一）：只列失败与跳过的记录，other 也包括升级前没有分类的
	Kind    string
	Keyword string
	// MediaType、TMDBID 与 Season 选一组识别出条目的记录（见 HistoryGroups）：TMDBID 不为 0 时只列这个条目这一季的
	MediaType string
	TMDBID    int
	Season    int
	// Torrent 选一组没识别出条目的记录：种子名（监控目录的项、指定的路径是最后一段）
	Torrent string
	Limit   int
	Offset  int
}

// HistoryItem 是一条整理记录，带上媒体库的名字、字幕数，以及库里有没有记着的、可以一起删的文件。
type HistoryItem struct {
	models.MediaTransferHistory
	LibraryName string `json:"library_name,omitempty"`
	Subtitles   int    `json:"subtitles"`
	HasFiles    bool   `json:"has_files"`
	// ManualPath 是手动指定路径整理的记录当时填的路径（别的记录没有）：网页上带着它重新打开「手动整理」
	ManualPath string `json:"manual_path,omitempty"`
	// DiscFiles 是原盘记录放进库的文件数（不是原盘时为 0）
	DiscFiles int `json:"disc_files,omitempty"`
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

// historyDB 是按查询条件（状态、分类、关键字、组）筛选整理记录的查询；每次调用都是新的。
func (s *Service) historyDB(ctx context.Context, q HistoryQuery) (*gorm.DB, error) {
	db := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{})
	switch q.Status {
	case "":
	case models.MediaTransferDone, models.MediaTransferFailed, models.MediaTransferSkipped, models.MediaTransferRemoved:
		db = db.Where("status = ?", q.Status)
	default:
		return nil, fmt.Errorf("%w: 状态要选 done、failed、skipped 或 removed", ErrInvalid)
	}
	switch {
	case q.Kind == "":
	case !slices.Contains(models.MediaFailKinds, q.Kind):
		return nil, fmt.Errorf("%w: 分类要选 %s", ErrInvalid, strings.Join(models.MediaFailKinds, "、"))
	case q.Kind == models.MediaFailOther:
		db = db.Where("status IN ? AND fail_kind IN ?", []string{models.MediaTransferFailed, models.MediaTransferSkipped}, []string{models.MediaFailOther, ""})
	default:
		db = db.Where("status IN ? AND fail_kind = ?", []string{models.MediaTransferFailed, models.MediaTransferSkipped}, q.Kind)
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		if len(kw) > 200 {
			return nil, fmt.Errorf("%w: 关键字太长", ErrInvalid)
		}
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(kw) + "%"
		db = db.Where(`torrent_name LIKE ? ESCAPE '\' OR title LIKE ? ESCAPE '\' OR source_path LIKE ? ESCAPE '\' OR target_path LIKE ? ESCAPE '\'`, like, like, like, like)
	}
	switch {
	case q.TMDBID < 0 || q.Season < 0:
		return nil, fmt.Errorf("%w: 条目编号与季不能是负数", ErrInvalid)
	case q.TMDBID > 0:
		if !validKind(q.MediaType) {
			return nil, fmt.Errorf("%w: 按条目看记录时要写明类型（movie 或 tv）", ErrInvalid)
		}
		db = db.Where("media_type = ? AND tmdb_id = ? AND season = ?", q.MediaType, q.TMDBID, q.Season)
	case q.Torrent != "":
		db = db.Where("tmdb_id = 0 AND torrent_name = ?", q.Torrent)
	}
	return db, nil
}

// History 按时间倒序列出整理记录，可以按状态、分类、关键字（种子名、标题、路径）与组筛选。
func (s *Service) History(ctx context.Context, q HistoryQuery) (HistoryPage, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	q.Offset = max(q.Offset, 0)
	db, err := s.historyDB(ctx, q)
	if err != nil {
		return HistoryPage{}, err
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
		subs, disc := 0, 0
		for _, e := range decodeExtras(r.Extras) {
			switch e.Kind {
			case "", extraSiteSub:
				subs++
			case extraDisc:
				disc++
			}
		}
		item := HistoryItem{MediaTransferHistory: r, LibraryName: names[r.LibraryID], Subtitles: subs, HasFiles: hasLibraryFiles(&r), DiscFiles: disc}
		if pathRow(r) {
			item.ManualPath = requestOf(r).SourcePath
		}
		page.Items = append(page.Items, item)
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

// requestOf 是重新整理这条记录所在的种子（或目录监控的那一项、手动指定的那个路径）的请求。
// 手动整理时选了整理方式的，还按选的方式（跟媒体库的按库现在的方式，不按上次实际用的）。
func requestOf(r models.MediaTransferHistory) Request {
	if pathRow(r) {
		// 指定的路径记成上级目录（SaveRoot）与最后一段（TorrentName）：规整过的路径 Join 回来还是它
		return Request{SourcePath: filepath.Join(r.SaveRoot, r.TorrentName), Mode: r.ChosenMode}
	}
	if r.WatchDirID > 0 {
		return Request{WatchDirID: r.WatchDirID, Entry: r.TorrentName, Mode: r.ChosenMode}
	}
	return Request{DownloaderID: r.DownloaderID, Hash: firstNonEmpty(r.TaskID, r.InfoHash), Mode: r.ChosenMode}
}

// Retry 重新整理这条记录所在的种子（按手动整理处理）。之前手动指定过条目的，沿用那个条目。
func (s *Service) Retry(ctx context.Context, id uint) (*Result, error) {
	row, err := s.historyRow(ctx, id)
	if err != nil {
		return nil, err
	}
	if importRow(row) {
		return nil, fmt.Errorf("%w: %s", ErrInvalid, importRetryNote)
	}
	return s.Organize(ctx, retryRequest(row))
}

// retryRequest 是重试这条记录的请求：之前手动指定过条目的，沿用那个条目与媒体库。
func retryRequest(row models.MediaTransferHistory) Request {
	req := requestOf(row)
	if row.Trigger == models.MediaTriggerManual && row.TMDBID > 0 {
		req.MediaType, req.TMDBID, req.LibraryID = row.MediaType, row.TMDBID, row.LibraryID
	}
	return req
}

// hasLibraryFiles 报告记录里有没有记着 pt-tools 放进库的文件：已整理的，或者之后变成跳过、失败（例如库里的视频被换掉了），
// 但之前整理出过文件的。
func hasLibraryFiles(r *models.MediaTransferHistory) bool {
	if r == nil || r.Status == models.MediaTransferRemoved {
		return false
	}
	return (r.Status == models.MediaTransferDone && r.TargetPath != "") || r.TargetFileID != "" || len(decodeExtras(r.Extras)) > 0
}

// DeleteOptions 是删除整理记录的选项。
type DeleteOptions struct {
	// Files 为真时连库里整理出的文件一起删
	Files bool
	// AllowExisting 为真时导入的已有文件（MediaModeExisting）也删：那是导入时就在库里的文件、不是 pt-tools 放进去的，要另外确认
	AllowExisting bool
}

// DeleteHistory 删除一条整理记录，files 为真时连库里的文件一起删（导入的已有文件不删，见 DeleteHistoryWith）。
func (s *Service) DeleteHistory(ctx context.Context, id uint, files bool) ([]string, error) {
	return s.DeleteHistoryWith(ctx, id, DeleteOptions{Files: files})
}

// DeleteHistoryWith 删除一条整理记录。opt.Files 为真时连库里整理出的文件一起删：视频、字幕、同名的 NFO 与图片，
// 目录里不再有视频时再删目录级的海报与 NFO 和空目录。只删确认还是当初整理出的那个文件，换掉或改过的留着，
// 返回留着的文件的说明；移动整理的文件是唯一的一份，不在这里删；导入的已有文件只在 opt.AllowExisting 时删。
func (s *Service) DeleteHistoryWith(ctx context.Context, id uint, opt DeleteOptions) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.historyRow(ctx, id)
	if err != nil {
		return nil, err
	}
	var kept []string
	if opt.Files && hasLibraryFiles(&row) {
		if row.Mode == models.MediaModeMove {
			return nil, fmt.Errorf("%w: 移动整理的文件是唯一的一份，不在这里删除；只删记录时去掉「同时删除库里的文件」", ErrInvalid)
		}
		if row.Mode == models.MediaModeExisting && !opt.AllowExisting {
			return nil, fmt.Errorf("%w: %s", ErrInvalid, existingDeleteNote)
		}
		if kept, err = s.removeLibraryFiles(ctx, row); err != nil {
			return nil, err
		}
	}
	if err := s.cfg.DB.WithContext(ctx).Delete(&models.MediaTransferHistory{}, row.ID).Error; err != nil {
		return nil, fmt.Errorf("删除整理记录失败: %w", err)
	}
	s.recordGone(ctx, row, "pt-tools 的整理记录删了")
	return kept, nil
}

// Retire 用在洗版：换成了更好的版本以后，删掉一条已整理记录在库里整理出的文件（规则同连文件删除记录，只删确认是自己放的），
// 记录改成已删除并写明原因。移动整理的文件是唯一的一份，不删，返回 ErrInvalid；导入的已有文件不删，写明以后返回 ErrExistingKept。
// 不是已整理的记录不动。
func (s *Service) Retire(ctx context.Context, id uint, reason string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.historyRow(ctx, id)
	if err != nil {
		return nil, err
	}
	if row.Status != models.MediaTransferDone {
		return nil, nil
	}
	if row.Mode == models.MediaModeMove {
		return nil, fmt.Errorf("%w: 移动整理的文件是唯一的一份，洗版时不删", ErrInvalid)
	}
	if row.Mode == models.MediaModeExisting {
		// 导入时就在库里的文件不是 pt-tools 放进去的：不删，记录照旧是已整理（库里有两个版本，要删到整理历史里删），
		// 说明只写一次（每次洗版都会再看到它）
		if msg := truncate(reason+"；"+existingRetireNote, 1024); row.Message != msg {
			if uerr := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).Where("id = ?", row.ID).
				UpdateColumn("message", msg).Error; uerr != nil {
				return nil, fmt.Errorf("更新整理记录失败: %w", uerr)
			}
		}
		return nil, ErrExistingKept
	}
	kept, err := s.removeLibraryFiles(ctx, row)
	if err != nil {
		return nil, err
	}
	msg := reason
	if len(kept) > 0 {
		msg += "；" + strings.Join(kept, "；")
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).Where("id = ?", row.ID).Updates(map[string]any{
		"status": models.MediaTransferRemoved, "message": truncate(msg, 1024), "updated_at": s.cfg.Now(),
	}).Error; err != nil {
		return nil, fmt.Errorf("更新整理记录失败: %w", err)
	}
	s.recordGone(ctx, row, "洗版换掉了这个版本")
	return kept, nil
}

// fileIDOf 读文件编号（测试里换成会出错的）。
var fileIDOf = transfer.FileID

// removeIfOurs 删替换下来的旧版本的字幕与刮削文件（测试里换成会出错的）。
var removeIfOurs = transfer.RemoveIfOurs

// transferTemp 把替换用的新文件放到临时名（测试里换成会撞上的）。
var transferTemp = transfer.Transfer

// isDirArtifact 报告文件名是不是刮削写在目录里、几个视频共用的文件（见 scrape.IsDirArtifact）。
func isDirArtifact(name string) bool { return scrape.IsDirArtifact(name) }

// hasVideo 报告目录里（含子目录）还有没有视频文件。读不了目录时当作有。
func hasVideo(dir string) bool {
	found := false
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && transfer.IsLibraryVideo(d.Name()) {
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
			// 说明里只留「库里的 X 已经换成了别的文件或改过，没有删除」这一句
			kept = append(kept, strings.TrimPrefix(err.Error(), transfer.ErrNotOurs.Error()+"："))
			return nil
		}
		return err
	}
	// 原盘记录的目标是电影目录：不删它本身，原盘的文件在 extras 里逐个确认
	_, disc := discMain(&row)
	if row.TargetPath != "" && !disc {
		if err := remove(row.TargetPath, row.Mode, row.TargetFileID, row.SourcePath); err != nil {
			return nil, err
		}
	}
	var shared, discFiles []extra
	for _, e := range decodeExtras(row.Extras) {
		switch {
		// 原盘的电影目录里只有这一张原盘：它的 movie.nfo 是这条记录自己的（别的电影目录里的 movie.nfo 是几个版本共用的）
		case e.Kind == extraMeta && isDirArtifact(filepath.Base(e.Target)) && (!disc || filepath.Base(e.Target) != scrape.MovieNFOName):
			shared = append(shared, e)
		case e.Kind == extraMeta || e.Kind == extraSiteSub:
			// 刮削写的文件与站点字幕是 pt-tools 写的普通文件：按文件编号确认（软链接方式整理的视频也一样）
			if err := remove(e.Target, models.MediaModeCopy, e.FileID, ""); err != nil {
				errs = append(errs, err)
			}
		default:
			if e.Kind == extraDisc {
				discFiles = append(discFiles, e)
			}
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
	if disc {
		// 从每个原盘文件的目录往上删空目录，最后是电影目录
		removeDiscDirs(discFiles, row.TargetPath, lib.Path)
		return kept, errors.Join(errs...)
	}
	dirs := []string{filepath.Dir(row.TargetPath)}
	if sd := showDir(lib.Path, row.TargetPath); sd != "" && sd != dirs[0] {
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
		// 文件已经没了，或者换过（开着覆盖时后一批整理重写了它，或者用户换了）：这条归属过期了，不转。
		// 后一批重写的文件记在写它的那批记录里，转过去会把那边对得上的编号盖掉。
		// 别的读取错误（权限、网络存储）不能当成过期：返回错误，记录留着下次再删
		id, err := fileIDOf(e.Target)
		if errors.Is(err, os.ErrNotExist) || (err == nil && id != e.FileID) {
			continue
		}
		if err != nil {
			return fmt.Errorf("读取刮削文件 %s 失败: %w", filepath.Base(e.Target), err)
		}
		d := filepath.Dir(e.Target)
		for _, r := range rows {
			if transfer.Within(d, rowVideo(r)) {
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
		// 目录监控整理的（没有下载器）不跟着删：那里的文件不是种子，删不删由用户决定
		// 在条目详情里删除时选了留着库里的文件的（KeepLinks）不删
		Where("status = ? AND mode IN ? AND target_path != '' AND downloader_id > 0 AND keep_links = ?", models.MediaTransferDone, []string{models.MediaModeHardlink, models.MediaModeSymlink}, false).
		Order("downloader_id, id").Find(&rows).Error; err != nil {
		s.cfg.Logger.Warnf("[整理入库] 读取整理记录失败: %v", err)
		return 0
	}
	byDL := map[uint][]models.MediaTransferHistory{}
	for _, r := range rows {
		byDL[r.DownloaderID] = append(byDL[r.DownloaderID], r)
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
			s.mu.Lock()
			ok, err := s.reconcileRow(ctx, r)
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

// reconcileRow 清理一条记录在库里的链接并把记录改成 removed（调用方持有 s.mu）。snap 是查种子在不在时用的那份记录；
// 这里按编号重新读一遍再判断：同一轮里前面的记录可能刚把目录级的刮削文件转给了这一条（只有 Extras 变了，照常清理）；
// 记录被删掉，或者期间被重新整理过（目标、方式、来源变了，例如改成了移动）的不动，留给下一轮重新判断。
func (s *Service) reconcileRow(ctx context.Context, snap models.MediaTransferHistory) (bool, error) {
	row, err := s.historyRow(ctx, snap.ID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if row.Status != models.MediaTransferDone || (row.Mode != models.MediaModeHardlink && row.Mode != models.MediaModeSymlink) ||
		row.Mode != snap.Mode || row.TargetPath != snap.TargetPath || row.TargetFileID != snap.TargetFileID ||
		row.SourcePath != snap.SourcePath || row.LibraryID != snap.LibraryID ||
		row.DownloaderID != snap.DownloaderID || row.InfoHash != snap.InfoHash || row.KeepLinks {
		return false, nil
	}
	// 保存目录不在（例如下载目录没挂载）时认不出数据是不是真的删了，不动
	if info, serr := os.Stat(row.SaveRoot); row.SaveRoot == "" || serr != nil || !info.IsDir() {
		return false, nil
	}
	if _, serr := os.Lstat(row.SourcePath); !errors.Is(serr, os.ErrNotExist) {
		return false, nil // 数据还在（只删了种子、没删数据，或者移走了）
	}
	var lib models.MediaLibrary
	if err = s.cfg.DB.WithContext(ctx).Where("id = ?", row.LibraryID).Limit(1).Find(&lib).Error; err != nil {
		return false, fmt.Errorf("读取媒体库失败: %w", err)
	}
	if info, serr := os.Stat(lib.Path); lib.ID == 0 || serr != nil || !info.IsDir() {
		return false, nil // 库目录不在（例如没挂载），不动记录
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
	s.recordGone(ctx, row, "种子与数据删了，库里的链接一并删了")
	return true, nil
}
