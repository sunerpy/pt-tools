package organize

import (
	"context"
	"crypto/rand" //nolint:gosec // 只用来给目录监控的一项取个短标识，不涉及安全
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

// replace 按覆盖策略用这个文件替换库里的旧版本（调用方持有 s.mu）。每一步都记在替换日志（MediaReplaceJournal）里，
// pt-tools 中途退出时下次启动按它恢复（recoverReplacements）：
//  1. 新文件按媒体库的方式放到目标旁边的临时名；
//  2. 旧文件挪到旁边的备份名，确认挪走的就是旧记录放的那个（整理期间被换过就放回去，不替换）；
//  3. 新文件不覆盖地放到目标位置；
//  4. 同一个事务里写新记录、把旧记录标成已删除、把日志标成已提交；
//  5. 收尾：删掉备份（旧文件）与旧记录的字幕、同名刮削文件（目录里的刮削文件归新记录），再放新字幕（旧字幕删掉以后
//     才不会撞上）并记进新记录，都做完了删掉日志。收尾没做完的，日志留着，后台每隔一会儿与下次启动时接着做。
//
// 1–4 任一步失败都撤回：新文件删掉，旧文件放回原处，这次记成失败（之后自动重试）；在这之前旧版本的字幕与刮削文件都不动。
// 返回是否换好了。
func (s *Service) replace(ctx context.Context, p *Plan, it *PlanItem, trigger string, res *Result) bool {
	old := it.replaces
	fail := func(msg string) bool {
		it.Status, it.Message = ItemFailed, msg
		res.Failed++
		s.record(ctx, p, it, trigger, models.MediaTransferFailed, msg, true, "", nil)
		return false
	}
	undo := func(j models.MediaReplaceJournal, why string) bool {
		if err := s.undoReplace(ctx, j); err != nil {
			why += "；撤回时出错（" + err.Error() + "），下次启动时再撤回"
		}
		return fail("替换旧版本失败，旧版本留着：" + why)
	}
	db := s.cfg.DB.WithContext(ctx)
	subs, _ := json.Marshal(it.Subtitles)
	// 临时名与备份名先确认没有被占着：之后在这两个名字上看到的文件就是这次放的
	temp, backup := freeSideName(it.Target, "new"), freeSideName(it.Target, "old")
	if temp == "" || backup == "" {
		return fail("替换旧版本失败，旧版本留着：找不到没被占用的临时名")
	}
	j := models.MediaReplaceJournal{
		OldID: old.ID, Source: it.Source, Mode: firstNonEmpty(it.mode, p.Mode), Target: it.Target,
		Temp: temp, Backup: backup, Subs: string(subs), CreatedAt: s.cfg.Now(),
	}
	if err := db.Create(&j).Error; err != nil {
		return fail("替换旧版本失败，旧版本留着：替换日志写不进去（" + err.Error() + "）")
	}
	if _, err := transferTemp(it.Source, j.Temp, p.Mode); err != nil {
		// 没放成：临时名上即使有文件也不是这次放的，撤回时不碰它
		untouched := j
		untouched.Temp = ""
		return undo(untouched, err.Error())
	}
	tid, terr := transfer.FileID(j.Temp)
	if terr != nil {
		// 没有编号就没法在撤回、收尾时认出它：撤回，不往下做（临时文件认不出来，留着并写进日志）
		return undo(j, "读不出放好的新文件的编号（"+terr.Error()+"）")
	}
	j.TempID = tid
	if err := db.Model(&j).Update("temp_id", j.TempID).Error; err != nil {
		return undo(j, "替换日志写不进去（"+err.Error()+"）")
	}
	if err := transfer.Place(j.Target, j.Backup); err != nil {
		return undo(j, "旧文件挪不开（"+err.Error()+"）")
	}
	if _, err := os.Lstat(j.Target); err == nil {
		// 硬链接放好了、原来的名字却删不掉（例如正被占用）：目标位置还是旧文件
		return undo(j, "旧文件挪不开（可能正被占用）")
	}
	// 只读一次文件编号：拿它确认挪开的是旧记录放的那个，再把同一个编号记进日志（收尾时按它删）
	bid, berr := transfer.FileID(j.Backup)
	if berr != nil {
		return undo(j, "读不出挪开的旧文件的编号（"+berr.Error()+"）")
	}
	if bid != old.TargetFileID {
		return undo(j, "目标位置的文件在整理期间换过了")
	}
	j.BackupID = bid
	if err := db.Model(&j).Update("backup_id", j.BackupID).Error; err != nil {
		return undo(j, "替换日志写不进去（"+err.Error()+"）")
	}
	if err := transfer.Place(j.Temp, j.Target); err != nil {
		return undo(j, err.Error())
	}
	oldName := firstNonEmpty(old.TorrentName, filepath.Base(old.SourcePath))
	note := "替换了旧版本（" + oldName + "）"
	err := s.cfg.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.writeRecordIn(ctx, tx, p, it, trigger, models.MediaTransferDone, note, false, j.TempID,
			mergeExtras(rowExtras(it.row), dirArtifacts(old))); err != nil {
			return err
		}
		if err := tx.Model(&models.MediaTransferHistory{}).Where("id = ?", old.ID).Updates(map[string]any{
			"status": models.MediaTransferRemoved, "message": truncate("换成了 "+firstNonEmpty(p.Name, filepath.Base(it.Source))+"（覆盖策略）", 1024),
			"updated_at": s.cfg.Now(),
		}).Error; err != nil {
			return err
		}
		return tx.Model(&models.MediaReplaceJournal{}).Where("id = ?", j.ID).Update("committed", true).Error
	})
	if err != nil {
		return undo(j, "整理记录写不进去（"+err.Error()+"）")
	}
	msgs, ferr := s.finishReplace(ctx, j, *old)
	if ferr != nil {
		msgs = append(msgs, "收尾没做完（"+ferr.Error()+"），之后接着做（包括放新字幕）")
	}
	if len(msgs) > 0 {
		_ = s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).Where("source_path = ?", it.Source).
			Update("message", truncate(note+"；"+strings.Join(msgs, "；"), 1024)).Error
	}
	res.Messages = append(res.Messages, msgs...)
	it.Status = ItemDone
	return true
}

// dirArtifacts 是旧记录里目录级的刮削文件（海报、tvshow.nfo 一类）中还是当初写的那个的：替换以后归新记录。
func dirArtifacts(old *models.MediaTransferHistory) []extra {
	var out []extra
	for _, e := range decodeExtras(old.Extras) {
		if e.Kind != extraMeta || !isDirArtifact(filepath.Base(e.Target)) {
			continue
		}
		if id, err := fileIDOf(e.Target); err == nil && id == e.FileID {
			out = append(out, e)
		}
	}
	return out
}

// undoReplace 撤回一次没提交的替换：目标位置上是这次放的新文件就删掉，旧文件（备份）放回目标位置，删掉临时文件，
// 都做完了才删日志。哪一步做不了就返回错误、留着日志，之后（后台每隔一会儿、下次启动）再撤回。
func (s *Service) undoReplace(ctx context.Context, j models.MediaReplaceJournal) error {
	if j.TempID != "" {
		if id, err := transfer.FileID(j.Target); err == nil && id == j.TempID {
			if err := os.Remove(j.Target); err != nil {
				return fmt.Errorf("删不掉放进去的新文件: %w", err)
			}
		}
	}
	bi, err := os.Lstat(j.Backup)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("看不了 %s: %w", filepath.Base(j.Backup), err)
	}
	if err == nil {
		if ti, terr := os.Lstat(j.Target); terr == nil && os.SameFile(bi, ti) {
			// 备份只是旧文件的另一个名字（挪的时候原来的名字没删掉）
			if err := os.Remove(j.Backup); err != nil {
				return fmt.Errorf("删不掉多出来的 %s: %w", filepath.Base(j.Backup), err)
			}
		} else if err := transfer.Place(j.Backup, j.Target); err != nil {
			return fmt.Errorf("旧文件放不回 %s（还在 %s）: %w", j.Target, j.Backup, err)
		}
	}
	switch _, err := os.Lstat(j.Temp); {
	case err == nil:
		if j.TempID == "" {
			// 没记下编号（没放成、放好以后记编号以前退出，或当时没读出来）：认不出上面是不是这次放的文件，
			// 不删，写进日志让用户看（宁可留下一个临时文件，也不删可能是别人的文件）
			s.cfg.Logger.Warnf("[整理入库] 替换用的临时文件 %s 认不出是不是 pt-tools 放的，没有删除", j.Temp)
			break
		}
		if rerr := transfer.RemoveIfOurs(j.Temp, j.Mode, j.TempID, j.Source); rerr != nil && !errors.Is(rerr, transfer.ErrNotOurs) {
			return fmt.Errorf("删不掉临时文件 %s: %w", filepath.Base(j.Temp), rerr)
		}
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("看不了临时文件 %s: %w", filepath.Base(j.Temp), err)
	}
	return s.cfg.DB.WithContext(ctx).Delete(&models.MediaReplaceJournal{}, j.ID).Error
}

// finishReplace 收尾一次已提交的替换：删掉备份（目标位置上确实是这次放的新文件、备份还是旧记录放的那个才删）与旧记录的
// 字幕、同名刮削文件（按文件编号确认，目录里的已经归了新记录）。都做完了才删日志；没做完的返回错误、留着日志，之后再试。
// 第一个返回值是不需要再试的说明（例如备份已经换过、留着不动）。
func (s *Service) finishReplace(ctx context.Context, j models.MediaReplaceJournal, old models.MediaTransferHistory) ([]string, error) {
	var msgs []string
	var errs []error
	id, ierr := transfer.FileID(j.Target)
	if ierr != nil && !errors.Is(ierr, os.ErrNotExist) {
		// 读不出目标的编号（权限、网络存储一时出错）：不能当成目标没了，留着日志之后再看
		return msgs, fmt.Errorf("读不出 %s 的文件编号: %w", filepath.Base(j.Target), ierr)
	}
	targetOK := ierr == nil && j.TempID != "" && id == j.TempID
	switch _, err := os.Lstat(j.Backup); {
	case err == nil:
		bid, berr := transfer.FileID(j.Backup)
		switch {
		case !targetOK:
			// 新文件不在目标位置了（中途退出期间被删或被换）：旧文件就是库里仅剩的版本，不删
			errs = append(errs, fmt.Errorf("目标位置上不是这次放的新文件，旧文件留在 %s", j.Backup))
		case berr != nil:
			// 读不出编号（权限、网络存储一时出错）：不能当成换过了，留着日志之后再看
			errs = append(errs, fmt.Errorf("读不出 %s 的文件编号: %w", filepath.Base(j.Backup), berr))
		case j.BackupID == "" || bid != j.BackupID:
			// 挪开以后又被换过（或者编号读不出来）：不是确认过的那个旧文件，不删
			msgs = append(msgs, "旧版本挪开的文件 "+filepath.Base(j.Backup)+" 换过了，留着")
		default:
			if rerr := os.Remove(j.Backup); rerr != nil {
				errs = append(errs, fmt.Errorf("旧版本的文件 %s 没删掉: %w", filepath.Base(j.Backup), rerr))
			}
		}
	case !errors.Is(err, os.ErrNotExist):
		errs = append(errs, fmt.Errorf("看不了 %s: %w", filepath.Base(j.Backup), err))
	}
	for _, e := range decodeExtras(old.Extras) {
		if e.Kind == extraMeta && isDirArtifact(filepath.Base(e.Target)) {
			continue
		}
		mode := old.Mode
		if e.Kind == extraMeta {
			mode = models.MediaModeCopy
		}
		if err := removeIfOurs(e.Target, mode, e.FileID, e.Source); err != nil && !errors.Is(err, transfer.ErrNotOurs) {
			errs = append(errs, fmt.Errorf("旧版本的 %s 没删掉: %w", filepath.Base(e.Target), err))
		}
	}
	if len(errs) > 0 {
		return msgs, errors.Join(errs...)
	}
	if !targetOK {
		// 上一次收尾已经删了旧文件，之后目标位置上的视频被删或被换：新字幕放过去对不上，不放了
		msgs = append(msgs, "目标位置上的视频已经不是这次放的新文件，新字幕没放")
	} else {
		subMsgs, err := s.placeReplaceSubs(ctx, j)
		msgs = append(msgs, subMsgs...)
		if err != nil {
			return msgs, err
		}
	}
	if err := s.cfg.DB.WithContext(ctx).Delete(&models.MediaReplaceJournal{}, j.ID).Error; err != nil {
		return msgs, fmt.Errorf("替换日志没删掉: %w", err)
	}
	return msgs, nil
}

// placeReplaceSubs 放替换进来的新文件的字幕（日志里记着），记进新记录。字幕放不了（例如源文件没了、目标被占着）只写进说明，
// 不再重试；记不进新记录时撤回这次放的并返回错误（日志留着，之后再放）。
func (s *Service) placeReplaceSubs(ctx context.Context, j models.MediaReplaceJournal) ([]string, error) {
	var subs []SubtitlePlan
	if j.Subs != "" {
		_ = json.Unmarshal([]byte(j.Subs), &subs)
	}
	var msgs []string
	// placed 是要记进新记录的（这次放的，以及上次没做完时已经放好的）；created 是这次放的，记不进去时撤回
	var placed, created []extra
	for _, sub := range subs {
		out, err := transfer.Transfer(sub.Source, sub.Target, j.Mode)
		if err != nil {
			msgs = append(msgs, fmt.Sprintf("字幕 %s：%v", filepath.Base(sub.Source), err))
			continue
		}
		id, _ := transfer.FileID(sub.Target)
		e := extra{Source: sub.Source, Target: sub.Target, FileID: id}
		placed = append(placed, e)
		if out == transfer.Created {
			created = append(created, e)
		}
	}
	if len(placed) == 0 {
		return msgs, nil
	}
	var row models.MediaTransferHistory
	err := s.cfg.DB.WithContext(ctx).Where("source_path = ?", j.Source).Limit(1).Find(&row).Error
	if err == nil && row.ID == 0 {
		err = errors.New("新文件的整理记录不在了")
	}
	if err == nil {
		b, _ := json.Marshal(mergeExtras(decodeExtras(row.Extras), placed))
		err = s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).Where("id = ?", row.ID).Update("extras", string(b)).Error
	}
	if err != nil {
		for _, e := range created {
			_ = transfer.RemoveIfOurs(e.Target, j.Mode, e.FileID, e.Source)
		}
		return msgs, fmt.Errorf("字幕记不进整理记录: %w", err)
	}
	return msgs, nil
}

// recoverReplacements 处理没做完的替换（pt-tools 中途退出，或者撤回、收尾时文件被占用）：没提交的撤回，已提交的收尾。
// 整理的后台开始时与之后每隔一会儿调用；做不完的日志留着，下次再试。
func (s *Service) recoverReplacements(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var js []models.MediaReplaceJournal
	if err := s.cfg.DB.WithContext(ctx).Order("id").Find(&js).Error; err != nil {
		s.cfg.Logger.Warnf("[整理入库] 读取替换日志失败: %v", err)
		return
	}
	for _, j := range js {
		if !j.Committed {
			if err := s.undoReplace(ctx, j); err != nil {
				s.cfg.Logger.Warnf("[整理入库] 撤回没做完的替换失败 (%s): %v", j.Target, err)
			}
			continue
		}
		var old models.MediaTransferHistory
		if err := s.cfg.DB.WithContext(ctx).Where("id = ?", j.OldID).Limit(1).Find(&old).Error; err != nil {
			s.cfg.Logger.Warnf("[整理入库] 替换收尾时读取旧记录失败 (%s): %v", j.Target, err)
			continue
		}
		msgs, err := s.finishReplace(ctx, j, old)
		for _, m := range msgs {
			s.cfg.Logger.Warnf("[整理入库] 替换收尾 (%s): %s", j.Target, m)
		}
		if err != nil {
			s.cfg.Logger.Warnf("[整理入库] 替换收尾没做完，之后再试 (%s): %v", j.Target, err)
		}
	}
}

// freeSideName 是一个现在没被占用的临时名（几次都撞上时为空）。
func freeSideName(target, what string) string {
	for range 5 {
		name := sideName(target, what)
		if _, err := os.Lstat(name); errors.Is(err, os.ErrNotExist) {
			return name
		}
	}
	return ""
}

// sideName 是目标旁边的临时名：同一个目录（改名与硬链接才不跨文件系统），扩展名不是视频（媒体服务器不会扫进去）。
func sideName(target, what string) string {
	var rnd [6]byte
	_, _ = rand.Read(rnd[:])
	return target + ".pt-tools-" + what + "-" + hex.EncodeToString(rnd[:]) + ".tmp"
}
