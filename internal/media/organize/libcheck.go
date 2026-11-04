package organize

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/serverindex"
	"github.com/sunerpy/pt-tools/models"
)

// 库文件丢失巡检（媒体中心 M7）：每天一次看已整理的记录在库里的文件还在不在，不在的记成 library_missing 问题，
// 又在了的改成解决了（规则同 media_issues 的 Raise、Clear：用户忽略的在文件回来以前一直忽略）。只记问题，不删任何东西。

const (
	// libCheckEvery 是多久巡检一次
	libCheckEvery = 24 * time.Hour
	// libCheckFirst 是从没巡检过时，启动以后多久做第一次（不和启动时的别的事挤在一起）
	libCheckFirst = 10 * time.Minute
	// libCheckMeta 是 app_meta 里记巡检进度的键（重启以后不从头查）
	libCheckMeta = "media_library_check"
)

// libCheckBatch 是一轮最多查几条记录：按编号轮着查，下一轮从上一轮停下的地方接着（测试里调小）
var libCheckBatch = 5000

// libCheckState 是巡检的进度：下一轮从编号大于 AfterID 的记录查起，At 是上一轮的时间。
type libCheckState struct {
	AfterID uint      `json:"after_id"`
	At      time.Time `json:"at"`
}

// LibraryCheckResult 是一轮巡检的结果。
type LibraryCheckResult struct {
	Checked int
	Missing int
	Back    int
	// Skipped 是媒体库目录不在（例如没挂载）或媒体库删了、这一轮没查的记录
	Skipped int
}

func (s *Service) loadLibCheck(ctx context.Context) (libCheckState, error) {
	var st libCheckState
	raw, err := models.ReadAppMeta(s.cfg.DB.WithContext(ctx), libCheckMeta)
	if err != nil || raw == "" {
		return st, err
	}
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return libCheckState{}, nil // 写坏了的进度从头来
	}
	return st, nil
}

// libCheckNext 是下一次巡检的时间：上一轮的 24 小时以后；从没巡检过时启动 10 分钟以后。
func libCheckNext(st libCheckState, started time.Time) time.Time {
	if st.At.IsZero() {
		return started.Add(libCheckFirst)
	}
	return st.At.Add(libCheckEvery)
}

// CheckLibraryFiles 巡检一轮：从上一轮停下的地方起，查最多 libCheckBatch 条已整理、有目标路径的记录。媒体库目录不在的不查
// （没挂载时不能当成文件丢了）。整理记录已经删了、不再是已整理的问题一并改成解决了。记下进度与时间。
func (s *Service) CheckLibraryFiles(ctx context.Context) (LibraryCheckResult, error) {
	var res LibraryCheckResult
	st, err := s.loadLibCheck(ctx)
	if err != nil {
		return res, err
	}
	db := s.cfg.DB.WithContext(ctx)
	var rows []models.MediaTransferHistory
	if err := db.Where("status = ? AND target_path <> '' AND id > ?", models.MediaTransferDone, st.AfterID).
		Order("id").Limit(libCheckBatch).Find(&rows).Error; err != nil {
		return res, fmt.Errorf("读取整理记录失败: %w", err)
	}
	// 已经有问题（开着的、忽略的）的记录：文件回来了要改成解决了；别的记录在了不用写
	var open []uint
	if err := db.Model(&models.MediaIssue{}).Where("kind = ? AND status IN ?", models.MediaIssueLibraryMissing,
		[]string{models.MediaIssueOpen, models.MediaIssueDismissed}).Pluck("history_id", &open).Error; err != nil {
		return res, fmt.Errorf("读取问题失败: %w", err)
	}
	hasIssue := make(map[uint]bool, len(open))
	for _, id := range open {
		hasIssue[id] = true
	}
	libOK := map[uint]bool{}
	libChecked := map[uint]bool{}
	libraryUp := func(id uint) bool {
		if libChecked[id] {
			return libOK[id]
		}
		libChecked[id] = true
		var lib models.MediaLibrary
		if err := db.Where("id = ?", id).Limit(1).Find(&lib).Error; err != nil || lib.ID == 0 {
			return false
		}
		info, err := os.Stat(lib.Path)
		libOK[id] = err == nil && info.IsDir()
		return libOK[id]
	}
	now := s.cfg.Now()
	for i := range rows {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		r := &rows[i]
		if !libraryUp(r.LibraryID) {
			res.Skipped++
			continue
		}
		res.Checked++
		if libraryFilePresent(r) {
			if hasIssue[r.ID] {
				if err := serverindex.Clear(db, serverindex.MissingKey(r.ID), "库里的文件又有了", now); err != nil {
					return res, err
				}
				res.Back++
			}
			continue
		}
		title := r.Title
		if title == "" {
			title = r.TorrentName
		}
		info := serverindex.IssueInfo{
			MediaType: r.MediaType, TMDBID: r.TMDBID, Season: r.Season, Episode: r.Episode,
			Title: truncate(title, 255), Detail: truncate("库里的文件不见了："+r.TargetPath, 1024),
		}
		if err := serverindex.Raise(db, serverindex.MissingKey(r.ID), info, now); err != nil {
			return res, err
		}
		res.Missing++
	}
	if err := s.clearStaleMissing(ctx, now); err != nil {
		return res, err
	}
	next := libCheckState{At: now}
	if len(rows) == libCheckBatch {
		next.AfterID = rows[len(rows)-1].ID
	}
	b, _ := json.Marshal(next)
	if err := models.WriteAppMeta(db, libCheckMeta, string(b)); err != nil {
		return res, fmt.Errorf("记下巡检进度失败: %w", err)
	}
	return res, nil
}

// clearStaleMissing 把整理记录已经删了、或者不再是已整理的 library_missing 问题改成解决了。
func (s *Service) clearStaleMissing(ctx context.Context, now time.Time) error {
	db := s.cfg.DB.WithContext(ctx)
	var ids []uint
	if err := db.Model(&models.MediaIssue{}).
		Where("kind = ? AND status IN ?", models.MediaIssueLibraryMissing, []string{models.MediaIssueOpen, models.MediaIssueDismissed}).
		Where("NOT EXISTS (SELECT 1 FROM media_transfer_histories h WHERE h.id = media_issues.history_id AND h.status = ?)", models.MediaTransferDone).
		Pluck("history_id", &ids).Error; err != nil {
		return fmt.Errorf("读取问题失败: %w", err)
	}
	for _, id := range ids {
		if err := serverindex.Clear(db, serverindex.MissingKey(id), "整理记录删了，或者不再是已整理的", now); err != nil {
			return err
		}
	}
	return nil
}

// libCheck 是后台节拍里的巡检排期：next 为零时先读上一轮的时间；到了时间巡检一轮，排下一次。出错只记日志：
// 读不到进度时下一拍再读，巡检失败时一小时以后再试。
type libCheck struct {
	started, next time.Time
}

func (s *Service) libCheckTick(ctx context.Context, lc *libCheck) {
	now := s.cfg.Now()
	if lc.next.IsZero() {
		st, err := s.loadLibCheck(ctx)
		if err != nil {
			s.cfg.Logger.Warnf("[整理入库] 读取库文件巡检的进度失败: %v", err)
			return
		}
		lc.next = libCheckNext(st, lc.started)
	}
	if now.Before(lc.next) {
		return
	}
	res, err := s.CheckLibraryFiles(ctx)
	if err != nil {
		s.cfg.Logger.Warnf("[整理入库] 库文件巡检失败: %v", err)
		lc.next = now.Add(time.Hour)
		return
	}
	lc.next = now.Add(libCheckEvery)
	if res.Missing > 0 || res.Back > 0 {
		s.cfg.Logger.Infof("[整理入库] 库文件巡检：查了 %d 条，%d 条的文件不见了，%d 条又有了", res.Checked, res.Missing, res.Back)
	}
}
