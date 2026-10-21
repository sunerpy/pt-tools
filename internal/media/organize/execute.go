package organize

import (
	"context" //nolint:gosec // 只用来给目录监控的一项取个短标识，不涉及安全
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

// retryDelays 是失败后第 1 到 5 次自动重试前的等待时间；之后不再自动重试。
var retryDelays = []time.Duration{10 * time.Minute, 30 * time.Minute, time.Hour, 3 * time.Hour, 12 * time.Hour}

// extra 是跟着一个视频整理或写出的文件：Kind 为空是字幕，meta 是刮削写的 NFO 与图片（SHA256 是写的内容的摘要，
// 重新刮削时按它认文件是不是还是 pt-tools 写的那份；升级前记下的没有）。
type extra struct {
	Source string `json:"source,omitempty"`
	Target string `json:"target"`
	FileID string `json:"file_id,omitempty"`
	Kind   string `json:"kind,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

const extraMeta = "meta"

// mergeExtras 把新的项并进旧的：同一个目标用新的。
func mergeExtras(old, add []extra) []extra {
	out := append([]extra(nil), old...)
	for _, a := range add {
		replaced := false
		for i := range out {
			if out[i].Target == a.Target {
				out[i], replaced = a, true
			}
		}
		if !replaced {
			out = append(out, a)
		}
	}
	return out
}

func rowExtras(r *models.MediaTransferHistory) []extra {
	if r == nil {
		return nil
	}
	return decodeExtras(r.Extras)
}

// organize 按计划整理一个种子并写整理记录（调用方持有 s.mu）。
func (s *Service) organize(ctx context.Context, req Request, trigger string) (*Result, error) {
	p, err := s.plan(ctx, req)
	if err != nil {
		return nil, err
	}
	res := &Result{Plan: p}
	if p.noRecord {
		return res, nil
	}
	if p.Problem != "" {
		s.recordProblem(ctx, p, trigger, res)
		return res, nil
	}
	var created []*PlanItem
	for i := range p.Items {
		it := &p.Items[i]
		if it.otherFile {
			// 记录管着库里的文件，不改成这个新文件的（见 markOtherFiles）
			res.Skipped++
			s.noteOtherFile(ctx, it)
			continue
		}
		switch it.Status {
		case ItemFailed:
			res.Failed++
			s.record(ctx, p, it, trigger, models.MediaTransferFailed, it.Message, false, "", nil)
			continue
		case ItemExists:
			res.Skipped++
			s.record(ctx, p, it, trigger, models.MediaTransferSkipped, it.Message, false, "", nil)
			continue
		case ItemReplace:
			if s.replace(ctx, p, it, trigger, res) {
				res.Created++
				created = append(created, it)
			}
			continue
		case ItemDone:
			// 库里已经是这个文件：补上之前没整理成的字幕；没有记录时（例如记录被删了）补一条
			if it.row != nil && it.row.Status == models.MediaTransferDone && it.row.Mode != "" {
				it.mode = it.row.Mode // 库里这份是当初按这个方式放的（媒体库后来换了方式、移动时留下了源文件也一样）
			}
			// 字幕跟视频用同一个方式，和记录里的方式一致（删除时按它确认）
			mode := firstNonEmpty(it.mode, p.Mode)
			subs, newSubs, msgs := s.transferSubtitles(it, mode, rowExtras(it.row))
			if it.row == nil || it.row.Status != models.MediaTransferDone || len(newSubs) > 0 || len(msgs) > 0 {
				fileID := ""
				if it.row != nil && it.row.Status == models.MediaTransferDone {
					fileID = it.row.TargetFileID
				}
				if fileID == "" {
					fileID, _ = transfer.FileID(it.Target)
				}
				if err := s.writeRecord(ctx, p, it, trigger, models.MediaTransferDone, strings.Join(msgs, "；"), false, fileID, mergeExtras(rowExtras(it.row), subs)); err != nil {
					unplace(it, mode, "", false, newSubs)
					res.Failed++
					res.Messages = append(res.Messages, fmt.Sprintf("%s：整理记录写不进去，这次补上的字幕已撤回（%v）", filepath.Base(it.Source), err))
					continue
				}
			}
			res.Messages = append(res.Messages, msgs...)
			res.Done++
			continue
		}
		// 自动整理不重做记录为已整理、但库里的文件不在了的（多半是用户自己删的）
		if it.row != nil && it.row.Status == models.MediaTransferDone && trigger != models.MediaTriggerManual {
			res.Done++
			continue
		}
		out, terr := transfer.Transfer(it.Source, it.Target, p.Mode)
		var keptNote string
		if kept := (*transfer.SourceKeptError)(nil); errors.As(terr, &kept) {
			// 移动没做成，但库里已经放好了一份：按实际的方式（硬链接或复制）记成已整理，写明源文件还在
			it.mode, keptNote, out, terr = kept.Mode, kept.Error(), transfer.Created, nil
		}
		if terr != nil {
			status, retry := models.MediaTransferFailed, true
			switch {
			case errors.Is(terr, transfer.ErrTargetExists):
				status, retry = models.MediaTransferSkipped, false
				res.Skipped++
			case errors.Is(terr, transfer.ErrCrossDevice), errors.Is(terr, transfer.ErrSourceMissing):
				retry = false
				res.Failed++
			default:
				res.Failed++
			}
			it.Status, it.Message = ItemFailed, terr.Error()
			s.record(ctx, p, it, trigger, status, terr.Error(), retry, "", nil)
			continue
		}
		mode := firstNonEmpty(it.mode, p.Mode)
		fileID, _ := transfer.FileID(it.Target)
		extras, newSubs, msgs := s.transferSubtitles(it, mode, rowExtras(it.row))
		if keptNote != "" {
			msgs = append([]string{keptNote}, msgs...)
		}
		if err := s.writeRecord(ctx, p, it, trigger, models.MediaTransferDone, strings.Join(msgs, "；"), false, fileID, mergeExtras(rowExtras(it.row), extras)); err != nil {
			// 记录写不进去时库里的文件没人认领：源文件还在的撤回这次放进去的（移动过去的是唯一的一份，留着）
			unplace(it, mode, fileID, out == transfer.Created, newSubs)
			it.Status, it.Message = ItemFailed, fmt.Sprintf("整理记录写不进去：%v", err)
			res.Failed++
			res.Messages = append(res.Messages, fmt.Sprintf("%s：%s", filepath.Base(it.Source), it.Message))
			continue
		}
		res.Messages = append(res.Messages, msgs...)
		it.Status = ItemDone
		if out == transfer.AlreadyDone {
			res.Done++
			continue
		}
		res.Created++
		created = append(created, it)
	}
	if len(created) > 0 {
		res.Messages = append(res.Messages, s.scrape(ctx, p, created)...)
		res.Messages = append(res.Messages, s.refreshServers(ctx, p, created)...)
		if msg := s.notify(ctx, p, created); msg != "" {
			res.Messages = append(res.Messages, msg)
		}
	}
	return res, nil
}

// transferSubtitles 用同样的方式整理字幕；失败的记在消息里（写进整理记录），不影响视频，下次整理时再试。
// 返回整理好的字幕（含之前就在的）、这次新放进库的字幕与失败的消息。
func (s *Service) transferSubtitles(it *PlanItem, mode string, prior []extra) ([]extra, []extra, []string) {
	var out, created []extra
	var msgs []string
	for _, sub := range it.Subtitles {
		// 之前整理好的（复制与移动认不出来历，按记录里的文件编号认）
		if i := slices.IndexFunc(prior, func(e extra) bool { return e.Kind == "" && e.Target == sub.Target }); i >= 0 && extraOurs(prior[i], mode) {
			out = append(out, prior[i])
			continue
		}
		res, err := transfer.Transfer(sub.Source, sub.Target, mode)
		if kept := (*transfer.SourceKeptError)(nil); errors.As(err, &kept) {
			// 移动时删不掉源文件：库里这份字幕留着、照样记下，写明源文件还在
			msgs = append(msgs, fmt.Sprintf("字幕 %s：%v", filepath.Base(sub.Source), err))
			res, err = transfer.Created, nil
		}
		if err != nil {
			msgs = append(msgs, fmt.Sprintf("字幕 %s：%v", filepath.Base(sub.Source), err))
			continue
		}
		id, _ := transfer.FileID(sub.Target)
		e := extra{Source: sub.Source, Target: sub.Target, FileID: id}
		out = append(out, e)
		if res == transfer.Created {
			created = append(created, e)
		}
	}
	return out, created, msgs
}

// unplace 撤回这次放进库里、但整理记录没写进去的文件（视频要 placed 为真）：源文件还在，按文件编号或指向确认是自己放的才删；
// 移动过去的是唯一的一份，不撤。
func unplace(it *PlanItem, mode, fileID string, placed bool, subs []extra) {
	if mode == models.MediaModeMove {
		return
	}
	if placed {
		_ = transfer.RemoveIfOurs(it.Target, mode, fileID, it.Source)
	}
	for _, e := range subs {
		_ = transfer.RemoveIfOurs(e.Target, mode, e.FileID, e.Source)
	}
}

// recordProblem 为整个种子整理不了的情况写记录：知道视频文件时每个文件一行，否则用种子的内容路径记一行。
func (s *Service) recordProblem(ctx context.Context, p *Plan, trigger string, res *Result) {
	status := models.MediaTransferFailed
	if len(p.Items) == 0 {
		status = models.MediaTransferSkipped
		it := &PlanItem{Rel: p.Name, Source: p.contentSrc}
		var row models.MediaTransferHistory
		if err := s.cfg.DB.WithContext(ctx).Where("source_path = ?", it.Source).Limit(1).Find(&row).Error; err == nil && row.ID != 0 {
			it.row = &row
		}
		s.record(ctx, p, it, trigger, status, p.Problem, false, "", nil)
		res.Skipped++
		return
	}
	for i := range p.Items {
		it := &p.Items[i]
		// 已经整理好、库里还是那个文件的（plan 标成了 ItemDone）不因为这次识别失败改成失败
		if it.Status == ItemDone {
			res.Done++
			continue
		}
		if it.otherFile {
			res.Skipped++
			s.noteOtherFile(ctx, it)
			continue
		}
		it.Status, it.Message = ItemFailed, p.Problem
		s.record(ctx, p, it, trigger, status, p.Problem, p.retryable, "", nil)
		res.Failed++
	}
}

// record 写一个文件的整理记录（按源文件更新或新建）；写不进去时只记日志（不认领库里文件的情况用）。
func (s *Service) record(ctx context.Context, p *Plan, it *PlanItem, trigger, status, msg string, retry bool, fileID string, extras []extra) {
	_ = s.writeRecord(ctx, p, it, trigger, status, msg, retry, fileID, extras)
}

// writeRecord 写一个文件的整理记录并返回错误：整理出文件以后用，写不进去时调用方要撤回。
func (s *Service) writeRecord(ctx context.Context, p *Plan, it *PlanItem, trigger, status, msg string, retry bool, fileID string, extras []extra) error {
	return s.writeRecordIn(ctx, s.cfg.DB, p, it, trigger, status, msg, retry, fileID, extras)
}

// writeRecordIn 同 writeRecord，写进 db（替换旧版本时是事务）。
func (s *Service) writeRecordIn(ctx context.Context, db *gorm.DB, p *Plan, it *PlanItem, trigger, status, msg string, retry bool, fileID string, extras []extra) error {
	now := s.cfg.Now()
	row := models.MediaTransferHistory{
		DownloaderID: p.DownloaderID, DownloaderName: p.DownloaderName, WatchDirID: p.WatchDirID, InfoHash: p.Hash, TaskID: p.TaskID,
		TorrentName: truncate(p.Name, 512), SourcePath: it.Source, SaveRoot: p.LocalPath, TargetPath: it.Target, Mode: firstNonEmpty(it.mode, p.Mode),
		Season: it.Season, Episode: it.Episode, EpisodeEnd: it.EpisodeEnd, Size: it.Size, Status: status,
		Message: truncate(msg, 1024), Trigger: trigger, TargetFileID: fileID, SourceFileID: it.sourceID, CreatedAt: now, UpdatedAt: now,
	}
	if p.Library != nil {
		row.LibraryID = p.Library.ID
		if it.Target != "" {
			row.RootPath = p.rootFor(it.Target)
		}
	}
	if p.Match != nil {
		row.MediaType, row.TMDBID, row.Title, row.Year = p.Match.MediaType, p.Match.ID, truncate(p.Match.Title, 255), p.Match.Year
	}
	if len(extras) > 0 {
		b, _ := json.Marshal(extras)
		row.Extras = string(b)
	}
	// 没整理成时保留记录里之前整理出的文件（视频的位置、方式与文件编号，字幕与刮削写的文件）：删除记录时还要按它们清理
	if status != models.MediaTransferDone && hasLibraryFiles(it.row) {
		prior := it.row
		row.LibraryID, row.TargetPath, row.Mode, row.TargetFileID, row.Extras = prior.LibraryID, prior.TargetPath, prior.Mode, prior.TargetFileID, prior.Extras
		row.RootPath = prior.RootPath
	}
	attempts := 0
	if it.row != nil {
		attempts = it.row.Attempts
	}
	if status == models.MediaTransferFailed {
		attempts++
		if retry && attempts <= len(retryDelays) {
			next := now.Add(retryDelays[attempts-1])
			row.NextRetryAt = &next
		}
	} else {
		attempts = 0
	}
	row.Attempts = attempts
	err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "source_path"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"downloader_id", "downloader_name", "watch_dir_id", "info_hash", "task_id", "torrent_name", "library_id", "save_root", "target_path", "root_path",
			"extras", "mode", "media_type", "tmdb_id", "title", "year", "season", "episode", "episode_end", "size",
			"status", "message", "attempts", "next_retry_at", "target_file_id", "source_file_id", "trigger", "updated_at",
		}),
	}).Create(&row).Error
	if err != nil {
		s.cfg.Logger.Errorf("[整理入库] 写整理记录失败 (%s): %v", it.Source, err)
		return fmt.Errorf("写整理记录失败: %w", err)
	}
	if it.HistoryID == 0 {
		var saved models.MediaTransferHistory
		if db.WithContext(ctx).Select("id").Where("source_path = ?", it.Source).Limit(1).Find(&saved).Error == nil {
			it.HistoryID = saved.ID
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !isRuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }
