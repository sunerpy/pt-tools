package organize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

// changeSet 是一次整理里换了条目的文件：moved 是换了位置的（新文件放好、记录写好，旧位置的文件等都处理完以后删），
// inPlace 是位置不变、只改了记录的（重新刮削）。
type changeSet struct {
	moved, inPlace []*PlanItem
}

// changeItem 把已整理的一个文件换到这次的条目（调用方持有 s.mu，见 ItemChange）。先放新的、记录写好，旧位置的文件留给
// finishChanges 删：中途出错或进程退出时最多多留一份旧文件，不会丢文件。放不进去、记录写不进去时旧文件与旧记录都不动。
// 返回换成了没有。
func (s *Service) changeItem(ctx context.Context, p *Plan, it *PlanItem, trigger string, res *Result, set *changeSet) bool {
	base := filepath.Base(it.Source)
	failed := func(why string) bool {
		res.Failed++
		it.Status, it.Message = ItemFailed, "换条目没成："+why+"，库里还是原来的"
		res.Messages = append(res.Messages, base+"："+it.Message)
		return false
	}
	if it.inPlace {
		// 位置不变：文件、文件编号、字幕照旧，只改记录（条目、媒体库、来源），之后按新条目重新刮削
		it.mode = it.prior.Mode
		if err := s.writeRecord(ctx, p, it, trigger, models.MediaTransferDone, "", false, it.prior.TargetFileID, rowExtras(it.prior)); err != nil {
			return failed(err.Error())
		}
		s.restartRecord(ctx, it)
		set.inPlace = append(set.inPlace, it)
		res.Changed++
		it.Status, it.Message = ItemDone, "换好了：整理记录改成这个条目，重新刮削"
		return true
	}
	mode := p.Mode
	out := transfer.AlreadyDone
	var msgs []string
	if !it.placed {
		o, detail, terr := transferDetail(it.Source, it.Target, mode)
		if kept := (*transfer.SourceKeptError)(nil); errors.As(terr, &kept) {
			// 移动没做成，但新位置已经放好了一份：按实际的方式记，写明源文件还在
			it.mode, o, terr = kept.Mode, transfer.Created, nil
			msgs = append(msgs, kept.Error())
			mode = kept.Mode
		}
		if terr != nil {
			return failed(terr.Error())
		}
		if detail.Cloned {
			msgs = append(msgs, cloneNote)
		}
		out = o
	}
	fileID, _ := transfer.FileID(it.Target)
	// 字幕按新位置放；旧记录的字幕与刮削写的文件跟着旧位置一起删，不合并进新记录
	extras, newSubs, subMsgs := s.transferSubtitles(it, mode, nil)
	msgs = append(msgs, subMsgs...)
	if err := s.writeRecord(ctx, p, it, trigger, models.MediaTransferDone, strings.Join(msgs, "；"), false, fileID, extras); err != nil {
		unplace(it, mode, fileID, out == transfer.Created, newSubs)
		return failed("整理记录写不进去（" + err.Error() + "），这次放到新位置的文件已撤回")
	}
	res.Messages = append(res.Messages, msgs...)
	s.restartRecord(ctx, it)
	set.moved = append(set.moved, it)
	res.Changed++
	it.Status, it.Message = ItemDone, "换好了：放到了新位置，旧位置里 pt-tools 放的文件删掉"
	return true
}

// restartRecord 把换了条目的记录的整理时间改成现在、刮削状态清掉：整理后 1、3、7、14 天的自动补刮按这次算，
// 不刮削的库也不再显示旧条目的刮削结果。刮削的库在这次整理的刮削里重新记。
func (s *Service) restartRecord(ctx context.Context, it *PlanItem) {
	id := it.HistoryID
	if id == 0 && it.prior != nil {
		id = it.prior.ID
	}
	if id == 0 {
		return
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).Where("id = ?", id).
		Updates(map[string]any{"created_at": s.cfg.Now(), "scrape_status": "", "scrape_note": "", "scraped_at": nil}).Error; err != nil {
		s.cfg.Logger.Warnf("[整理入库] 换了条目的记录 %d 改整理时间失败: %v", id, err)
	}
}

// finishChanges 在一个种子的文件都处理完以后收尾（调用方持有 s.mu）：换了位置的，按换之前的记录删掉旧位置里 pt-tools 放的文件
// （和「连文件删除」一样：每条删自己的视频、字幕、单集 NFO 与截图，目录里共用的等到不再有视频时才删，最后删空目录）；
// 旧位置的文件还被另一条已整理的记录用着（同一份数据的辅种）时不删，转给那一条。位置不变的、媒体库开着刮削时按新条目重新刮削。
// 两种都收拾旧条目的问题。返回给人看的说明。
func (s *Service) finishChanges(ctx context.Context, p *Plan, set changeSet) []string {
	var msgs []string
	priors := gatherShared(set.moved)
	for i, it := range set.moved {
		base := filepath.Base(it.Source)
		prior := priors[i]
		other, err := s.otherUser(ctx, prior)
		if err != nil {
			msgs = append(msgs, fmt.Sprintf("%s：换条目以后旧位置的文件没删（%v），可以在整理历史里按旧条目找到它们手动删", base, err))
			continue
		}
		if other != nil {
			msgs = append(msgs, s.handToOther(ctx, base, prior, *other))
			s.recordGone(ctx, *it.prior, "换了条目")
			continue
		}
		kept, err := s.removeLibraryFiles(ctx, prior)
		for _, k := range kept {
			msgs = append(msgs, fmt.Sprintf("%s：换条目以后旧位置的这个文件留着：%s", base, k))
		}
		if err != nil {
			msgs = append(msgs, fmt.Sprintf("%s：换条目以后旧位置的文件没删干净（%v），可以在整理历史里按旧条目找到它们手动删", base, err))
		}
		s.recordGone(ctx, *it.prior, "换了条目")
	}
	// 共用的文件在最后一条删完以后才删：前面几条的目录（多季时的各季目录）这时才空
	s.removeEmptyDirsOf(ctx, priors)
	for _, it := range set.inPlace {
		s.recordGone(ctx, *it.prior, "换了条目")
	}
	if len(set.inPlace) > 0 && p.Library != nil && p.Library.Scrape {
		// 一个种子的文件是同一个条目、同一个库：重新刮削一次就把这个库里这一部的记录都刮了
		it := set.inPlace[0]
		rs, err := s.rescrapeWith(ctx, firstNonZero(it.HistoryID, it.prior.ID), false)
		switch {
		case err != nil:
			msgs = append(msgs, fmt.Sprintf("%s：换了条目，重新刮削没成：%v", filepath.Base(it.Source), err))
		case rs != nil:
			msgs = append(msgs, prefixed("刮削：", rs.Messages)...)
		}
	}
	return msgs
}

// gatherShared 返回这批换了位置的旧记录（拷贝），把目录里共用的刮削文件都并到同一个媒体库里最后一条上：共用文件只记在当初写出它们的
// 那几条记录里，前面几条删的时候目录里还有这批别的旧视频、要留下，而转交又找不到人（这批的记录已经改到了新位置）。并到最后一条上，
// 删到它时这批的旧视频都删了，共用的一并删掉；目录里还有别的种子的视频时照常转给那边的记录。
func gatherShared(moved []*PlanItem) []models.MediaTransferHistory {
	priors := make([]models.MediaTransferHistory, len(moved))
	last := map[uint]int{}
	for i, it := range moved {
		priors[i] = *it.prior
		last[priors[i].LibraryID] = i
	}
	for i := range priors {
		j := last[priors[i].LibraryID]
		if j == i {
			continue
		}
		var own, shared []extra
		for _, e := range decodeExtras(priors[i].Extras) {
			if dirShared(e, false) {
				shared = append(shared, e)
			} else {
				own = append(own, e)
			}
		}
		if len(shared) == 0 {
			continue
		}
		priors[i].Extras = encodeExtras(own)
		priors[j].Extras = encodeExtras(mergeShared(decodeExtras(priors[j].Extras), shared))
	}
	return priors
}

// mergeShared 把共用的刮削文件并进去：同一个文件记了两次（后来重写过）时留文件编号和现在对得上的那条。
func mergeShared(dst, add []extra) []extra {
	for _, a := range add {
		i := slices.IndexFunc(dst, func(e extra) bool { return e.Target == a.Target })
		switch {
		case i < 0:
			dst = append(dst, a)
		case dst[i].FileID != a.FileID:
			if id, err := fileIDOf(a.Target); err == nil && id == a.FileID {
				dst[i] = a
			}
		}
	}
	return dst
}

func encodeExtras(es []extra) string {
	if len(es) == 0 {
		return ""
	}
	b, _ := json.Marshal(es)
	return string(b)
}

// otherUser 是另一条还指向这个旧位置的已整理记录（同一份数据的辅种保存在别的目录、各自整理过时会这样）。没有时为 nil。
func (s *Service) otherUser(ctx context.Context, prior models.MediaTransferHistory) (*models.MediaTransferHistory, error) {
	if prior.TargetPath == "" {
		return nil, nil
	}
	var rows []models.MediaTransferHistory
	if err := s.cfg.DB.WithContext(ctx).
		Where("target_path = ? AND status = ? AND id <> ?", prior.TargetPath, models.MediaTransferDone, prior.ID).
		Order("id").Limit(1).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取整理记录失败: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// handToOther 把旧位置的文件留给另一条还用着它们的记录：旧记录里的字幕与刮削文件并进那一条，那一条以后换条目或删除时一并处理。
func (s *Service) handToOther(ctx context.Context, base string, prior, other models.MediaTransferHistory) string {
	note := fmt.Sprintf("%s：旧位置的文件还被另一条整理记录（%s）用着，留着；那一条也换过去以后才删", base, filepath.Base(other.SourcePath))
	add := decodeExtras(prior.Extras)
	if len(add) == 0 {
		return note
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).Where("id = ?", other.ID).
		Update("extras", encodeExtras(mergeShared(decodeExtras(other.Extras), add))).Error; err != nil {
		s.cfg.Logger.Warnf("[整理入库] 把换了条目的记录 %d 的文件转给记录 %d 失败: %v", prior.ID, other.ID, err)
	}
	return note
}

// removeEmptyDirsOf 删掉这些旧记录所在的空目录（季目录、剧集目录），到媒体库目录为止。
func (s *Service) removeEmptyDirsOf(ctx context.Context, priors []models.MediaTransferHistory) {
	libs := map[uint]string{}
	for _, r := range priors {
		root, ok := libs[r.LibraryID]
		if !ok {
			var lib models.MediaLibrary
			if err := s.cfg.DB.WithContext(ctx).Where("id = ?", r.LibraryID).Limit(1).Find(&lib).Error; err == nil {
				root = lib.Path
			}
			libs[r.LibraryID] = root
		}
		if root == "" || r.TargetPath == "" {
			continue
		}
		dirs := []string{filepath.Dir(r.TargetPath)}
		if sd := showDir(root, r.TargetPath); sd != "" && sd != dirs[0] {
			dirs = append(dirs, sd)
		}
		for _, d := range dirs {
			if transfer.Within(root, d) {
				transfer.RemoveEmptyDirs(d, root)
			}
		}
	}
}

func firstNonZero(a, b uint) uint {
	if a != 0 {
		return a
	}
	return b
}
