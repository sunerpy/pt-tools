package organize

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

// 原盘（蓝光的 BDMV、DVD 的 VIDEO_TS）只整理电影：一张原盘一项、一条整理记录，原盘根目录里的原盘结构目录整个放进
// 模板算出的电影目录，保持相对路径。记录的 source_path 是原盘根目录，target_path 是电影目录，target_file_id 是主文件
// （最大的视频流）的编号，extras 里每个放进去的文件一项（kind 为 extraDisc，主文件标着 Main）。
const (
	// extraDisc 是整理记录 extras 里原盘的文件
	extraDisc = "disc"
	// discStem 是原盘电影目录里刮削文件的名字：movie.nfo（Jellyfin 的 MovieNfoSaver、Kodi 都认）
	discStem = "movie"
	// cloneNote 写在整理记录里：复制用了写时复制
	cloneNote = "复制用了写时复制（reflink），不多占空间"
	// isoTVNote 是剧集里的 ISO 镜像不整理的原因
	isoTVNote = "剧集里的 ISO 镜像不整理"
)

// transferDetail 是放文件的操作（测试里换成会失败的）。
var transferDetail = transfer.TransferDetail

// DiscInfo 是计划里一项原盘的种类（BDMV 或 VIDEO_TS）与要放进库的文件数。
type DiscInfo struct {
	Kind  string `json:"kind"`
	Files int    `json:"files"`
}

// videoPath 是这一项在库里的「视频」：原盘是电影目录下的 movie（刮削写 movie.nfo、媒体服务器扫电影目录），别的是目标文件。
func (it *PlanItem) videoPath() string {
	if it.disc != nil {
		return filepath.Join(it.Target, discStem)
	}
	return it.Target
}

// discMain 是原盘记录的主文件（extras 里 kind 为 extraDisc、标着 Main 的那一项）；不是原盘的记录时 ok 为假。
func discMain(row *models.MediaTransferHistory) (extra, bool) {
	if row == nil {
		return extra{}, false
	}
	for _, e := range decodeExtras(row.Extras) {
		if e.Kind == extraDisc && e.Main {
			return e, true
		}
	}
	return extra{}, false
}

// rowVideo 是整理记录在库里的「视频」（同 PlanItem.videoPath）：重新刮削、把目录里的刮削文件转给别的记录时按它认。
func rowVideo(row models.MediaTransferHistory) string {
	if _, ok := discMain(&row); ok {
		return filepath.Join(row.TargetPath, discStem)
	}
	return row.TargetPath
}

// discOurs 报告原盘记录在库里的还是当初整理出的那张：电影目录在，主文件按文件编号（软链接按指向）认得出。
func discOurs(row *models.MediaTransferHistory, main extra) bool {
	info, err := os.Lstat(row.TargetPath)
	return err == nil && info.IsDir() && extraOurs(main, row.Mode)
}

// discItem 是计划里的一张原盘。目录监控与指定的路径按主文件的编号认同一个位置上是不是换了原盘（见 markOtherFiles）。
func discItem(d transfer.Disc, p *Plan) PlanItem {
	it := PlanItem{
		Rel: d.Root, Source: d.Path, Size: d.Size, Status: ItemPending,
		Disc: &DiscInfo{Kind: d.Kind, Files: len(d.Files)}, disc: &d,
	}
	if p.WatchDirID > 0 || p.SourcePath != "" {
		it.sourceID, _ = transfer.FileID(d.Files[d.Main].Path)
	}
	return it
}

// discProblem 是种子里的原盘整理不了的原因（多张、没有自己的文件夹、没有视频流）；整理得了时为空。识别以前就定。
func discProblem(discs []transfer.Disc) string {
	switch {
	case len(discs) > 1:
		roots := make([]string, 0, len(discs))
		for _, d := range discs {
			roots = append(roots, d.Root)
		}
		if len(roots) > 3 {
			roots = append(roots[:3], "…")
		}
		return fmt.Sprintf("种子里有 %d 张原盘（%s），暂不整理", len(discs), strings.Join(roots, "、"))
	case discs[0].Root == "":
		return "原盘没有自己的文件夹（BDMV 或 VIDEO_TS 的上一级就是下载目录或监控目录），暂不整理：把它放进一个文件夹；手动整理时选含 BDMV 的那个文件夹"
	case discs[0].Main < 0:
		return "原盘里没有视频流（BDMV/STREAM 或 VIDEO_TS 里没有视频），暂不整理"
	}
	return ""
}

// splitTVISO 把剧集里的 ISO 镜像挪出计划：列进没选上的文件，另记一条跳过的记录（目录监控按记录认它处理过了）。
func (p *Plan) splitTVISO() {
	kept := p.Items[:0]
	for _, it := range p.Items {
		if it.disc == nil && transfer.IsISO(it.Rel) {
			p.Skipped = append(p.Skipped, transfer.Skip{File: transfer.File{Rel: it.Rel, Path: it.Source, Size: it.Size}, Reason: isoTVNote})
			p.isoSkips = append(p.isoSkips, it)
			continue
		}
		kept = append(kept, it)
	}
	p.Items = kept
}

// recordISOSkips 给剧集里每个 ISO 镜像写一条跳过的记录。
func (s *Service) recordISOSkips(ctx context.Context, p *Plan, trigger string, res *Result) {
	for i := range p.isoSkips {
		it := &p.isoSkips[i]
		it.fail(models.MediaFailDisc, isoTVNote)
		s.record(ctx, p, it, trigger, models.MediaTransferSkipped, isoTVNote, false, "", nil)
		res.Skipped++
	}
}

// planDisc 算出原盘放进哪个电影目录：命名模板的目录部分（模板里电影没有自己的目录时整理不了），再看库里是不是已经有了。
func (s *Service) planDisc(ctx context.Context, p *Plan, tpl string, it *PlanItem) {
	// 原盘根目录的名字整个拿来认画质（不当作文件名去掉「扩展名」）
	fm, _ := s.cfg.Recognizer.Parse(ctx, path.Base(it.Rel), "")
	it.meta = fm
	rel, err := transfer.Render(tpl, vars(p.Match, fm, p.meta))
	if err != nil {
		it.fail(models.MediaFailLibrary, err.Error())
		return
	}
	dir := path.Dir(rel)
	if dir == "." {
		it.fail(models.MediaFailLibrary, "原盘要放在单独的目录里：电影的命名模板把电影直接放在库目录里（模板里没有「/」），改成先建电影目录的模板（例如默认模板）再整理")
		return
	}
	it.Target = filepath.Join(p.base(), filepath.FromSlash(dir))
	if !transfer.Within(p.Library.Path, it.Target) {
		it.fail(models.MediaFailLibrary, "模板算出的路径不在库目录里")
		it.Target = ""
		return
	}
	it.Status = discState(it)
}

// discState 看库里是不是已经有这张原盘：记录整理好了、库里还是那张时已在库里；电影目录不在或是空的时要整理；
// 电影目录里每个原盘文件都是源文件的硬链接（或指向源文件的软链接）时也算已在库里（记录删了、文件留着时认回来）；
// 其余（另一个版本、别的文件）不放进去。原盘不参与同名文件的替换。
func discState(it *PlanItem) string {
	if it.row != nil && it.row.Status == models.MediaTransferDone && it.row.TargetPath != "" {
		if _, err := os.Lstat(it.row.TargetPath); err == nil {
			it.Target = it.row.TargetPath
			if ours(it.row) {
				return ItemDone
			}
			it.Message = "库里的原盘已经不是当初整理出的那份（换成了别的文件或改过），不会覆盖"
			return ItemExists
		}
	}
	entries, err := os.ReadDir(it.Target)
	switch {
	case errors.Is(err, os.ErrNotExist), err == nil && len(entries) == 0:
		return ItemPending
	case err != nil:
		it.Message, it.failKind = "读不了电影目录："+err.Error(), models.MediaFailFilesystem
		return ItemFailed
	}
	if mode := adoptMode(it); mode != "" {
		it.mode = mode
		return ItemDone
	}
	it.Message = fmt.Sprintf("电影目录「%s」里已经有文件（这部电影的另一个版本或别的文件），原盘要单独放一个目录，没有整理", filepath.Base(it.Target))
	return ItemExists
}

// adoptMode 报告电影目录里的原盘文件是不是全是源文件的硬链接（返回 hardlink）或全是指向源文件的软链接（返回 symlink）；
// 不是时返回空串。
func adoptMode(it *PlanItem) string {
	mode := ""
	for _, f := range it.disc.Files {
		dst := filepath.Join(it.Target, filepath.FromSlash(f.In))
		info, err := os.Lstat(dst)
		if err != nil {
			return ""
		}
		m := ""
		if info.Mode()&os.ModeSymlink != 0 {
			if to, err := os.Readlink(dst); err == nil && filepath.Clean(to) == filepath.Clean(f.Path) {
				m = models.MediaModeSymlink
			}
		} else if src, err := os.Stat(f.Path); err == nil && os.SameFile(src, info) {
			m = models.MediaModeHardlink
		}
		if m == "" || (mode != "" && m != mode) {
			return ""
		}
		mode = m
	}
	return mode
}

// libraryExtras 是电影目录里现在的原盘文件（认回来时补记录用）。
func libraryExtras(it *PlanItem) []extra {
	out := make([]extra, 0, len(it.disc.Files))
	for i, f := range it.disc.Files {
		dst := filepath.Join(it.Target, filepath.FromSlash(f.In))
		id, _ := transfer.FileID(dst)
		out = append(out, extra{Source: f.Path, Target: dst, FileID: id, Kind: extraDisc, Main: i == it.disc.Main})
	}
	return out
}

// discPlaced 是放原盘的结果。
type discPlaced struct {
	// extras 是电影目录里的原盘文件（这次放的与之前就在的）；出错时是撤不回、留在库里的文件
	extras []extra
	// fresh 是这次新放进库的文件（记录写不进去时撤回它们）
	fresh []extra
	// cloned 表示主文件复制时用了写时复制
	cloned bool
	// kept 是移动时删不掉源文件、源文件留在原处的文件数（库里那份照样放好了）
	kept int
}

func mainOf(extras []extra) string {
	for _, e := range extras {
		if e.Main {
			return e.FileID
		}
	}
	return ""
}

// placeDisc 把原盘的文件逐个放进电影目录。中途失败时撤回这次放进去的（见 undoDisc），返回错误；撤不回、留在库里的文件
// 在结果的 extras 里。
func placeDisc(it *PlanItem, mode, libRoot string) (discPlaced, error) {
	var out discPlaced
	for i, f := range it.disc.Files {
		dst := filepath.Join(it.Target, filepath.FromSlash(f.In))
		res, d, err := transferDetail(f.Path, dst, mode)
		if kept := (*transfer.SourceKeptError)(nil); errors.As(err, &kept) {
			out.kept++
			res, err = transfer.Created, nil
		}
		if err != nil {
			left := undoDisc(out.fresh, mode, it.Target, libRoot)
			err = fmt.Errorf("%s：%w", f.In, err)
			if len(left) > 0 {
				err = fmt.Errorf("%w；%d 个文件没能挪回下载目录，留在库里的「%s」", err, len(left), filepath.Base(it.Target))
			}
			return discPlaced{extras: left}, err
		}
		id, _ := transfer.FileID(dst)
		e := extra{Source: f.Path, Target: dst, FileID: id, Kind: extraDisc, Main: i == it.disc.Main}
		out.extras = append(out.extras, e)
		if res == transfer.Created {
			out.fresh = append(out.fresh, e)
		}
		if e.Main && d.Cloned {
			out.cloned = true
		}
	}
	return out, nil
}

// undoDisc 撤回这次放进电影目录的原盘文件：硬链接、复制、软链接确认是自己放的才删；移动的挪回源位置，挪不回的返回
// （留在库里）。最后删掉空了的目录（到库目录为止）。
func undoDisc(placed []extra, mode, target, libRoot string) []extra {
	var left []extra
	for i := len(placed) - 1; i >= 0; i-- {
		e := placed[i]
		if mode != models.MediaModeMove {
			_ = transfer.RemoveIfOurs(e.Target, mode, e.FileID, e.Source)
			continue
		}
		if _, _, err := transferDetail(e.Target, e.Source, models.MediaModeMove); err != nil {
			left = append([]extra{e}, left...)
		}
	}
	removeDiscDirs(placed, target, libRoot)
	return left
}

// removeDiscDirs 从原盘文件所在的目录往上删空目录（到 root 为止，root 本身不删），最后是 top。
func removeDiscDirs(files []extra, top, root string) {
	var dirs []string
	for _, e := range files {
		if d := filepath.Dir(e.Target); !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	dirs = append(dirs, top)
	for _, d := range dirs {
		if transfer.Within(root, d) {
			transfer.RemoveEmptyDirs(d, root)
		}
	}
}

// removeMovedDirs 在移动整理以后删掉源原盘里移空了的目录：从每个文件原来的目录往上删到原盘根目录的上一级为止
// （原盘根目录里还有别的文件时它留着）。
func removeMovedDirs(it *PlanItem) {
	root := filepath.Dir(it.Source)
	var dirs []string
	for _, f := range it.disc.Files {
		if d := filepath.Dir(f.Path); !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	for _, d := range dirs {
		transfer.RemoveEmptyDirs(d, root)
	}
}

// organizeDisc 整理计划里的一张原盘（organize 里一个视频的那一段）并写整理记录。新放进库里时返回真。
func (s *Service) organizeDisc(ctx context.Context, p *Plan, it *PlanItem, trigger string, res *Result) bool {
	switch it.Status {
	case ItemFailed:
		res.Failed++
		s.record(ctx, p, it, trigger, models.MediaTransferFailed, it.Message, false, "", nil)
		return false
	case ItemExists:
		res.Skipped++
		it.failKind = models.MediaFailExists
		s.record(ctx, p, it, trigger, models.MediaTransferSkipped, it.Message, false, "", nil)
		return false
	case ItemDone:
		// 库里已经是这张原盘；没有已整理的记录时（记录删了、文件留着）按电影目录里现在的文件补一条
		if it.row == nil || it.row.Status != models.MediaTransferDone {
			extras := libraryExtras(it)
			if err := s.writeRecord(ctx, p, it, trigger, models.MediaTransferDone, "", false, mainOf(extras), mergeExtras(rowExtras(it.row), extras)); err != nil {
				res.Failed++
				res.Messages = append(res.Messages, fmt.Sprintf("%s：%v", it.Rel, err))
				return false
			}
		}
		res.Done++
		return false
	}
	// 自动整理不重做记录为已整理、但库里的电影目录不在了的（多半是用户自己删的）
	if it.row != nil && it.row.Status == models.MediaTransferDone && trigger != models.MediaTriggerManual {
		res.Done++
		return false
	}
	placed, err := placeDisc(it, p.Mode, p.Library.Path)
	if err != nil {
		status, retry := models.MediaTransferFailed, true
		switch {
		case errors.Is(err, transfer.ErrTargetExists):
			status, retry = models.MediaTransferSkipped, false
			res.Skipped++
		case errors.Is(err, transfer.ErrCrossDevice), errors.Is(err, transfer.ErrSourceMissing):
			retry = false
			res.Failed++
		default:
			res.Failed++
		}
		it.fail(failKindOf(err), err.Error())
		s.record(ctx, p, it, trigger, status, err.Error(), retry, mainOf(placed.extras), placed.extras)
		return false
	}
	var msgs []string
	if placed.cloned {
		msgs = append(msgs, cloneNote)
	}
	if placed.kept > 0 {
		msgs = append(msgs, fmt.Sprintf("%d 个文件删不掉源文件，没有移动成：库里放好了，源文件留在下载目录里", placed.kept))
	}
	if err := s.writeRecord(ctx, p, it, trigger, models.MediaTransferDone, strings.Join(msgs, "；"), false, mainOf(placed.extras),
		mergeExtras(rowExtras(it.row), placed.extras)); err != nil {
		// 记录写不进去时库里的文件没人认领：撤回这次放的（移动过去的是唯一的一份，留着）
		if p.Mode != models.MediaModeMove {
			undoDisc(placed.fresh, p.Mode, it.Target, p.Library.Path)
		}
		it.Status, it.Message = ItemFailed, fmt.Sprintf("整理记录写不进去：%v", err)
		res.Failed++
		res.Messages = append(res.Messages, fmt.Sprintf("%s：%s", it.Rel, it.Message))
		return false
	}
	if p.Mode == models.MediaModeMove && placed.kept == 0 {
		removeMovedDirs(it)
	}
	res.Messages = append(res.Messages, msgs...)
	it.Status = ItemDone
	if len(placed.fresh) == 0 {
		res.Done++
		return false
	}
	res.Created++
	return true
}
