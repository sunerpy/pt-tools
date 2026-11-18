package organize

import (
	"context" //nolint:gosec // 只用来给目录监控的一项取个短标识，不涉及安全
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/utils"
)

// joinNonEmpty 用空格把非空的几段连起来。
func joinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// vars 是模板变量：条目信息来自 TMDB，画质等来自文件名（文件名里没有的取种子名里的）。
func vars(m *tmdb.Result, fm, tm meta.Meta) transfer.Vars {
	v := transfer.Vars{
		Title: firstNonEmpty(m.Title, m.OriginalTitle), OriginalTitle: firstNonEmpty(m.OriginalTitle, m.Title),
		Year: m.Year, TMDBID: m.ID, IMDbID: m.IMDbID,
		Resolution: firstNonEmpty(fm.Resolution, tm.Resolution), Source: firstNonEmpty(fm.Source, tm.Source),
		VideoCodec: firstNonEmpty(fm.VideoCodec, tm.VideoCodec), Group: firstNonEmpty(fm.Group, tm.Group),
	}
	if fm.Remux || tm.Remux {
		v.Source = joinNonEmpty(v.Source, "Remux")
	}
	hdr, audio, edition := fm.HDR, fm.Audio, fm.Edition
	if len(hdr) == 0 {
		hdr = tm.HDR
	}
	if len(audio) == 0 {
		audio = tm.Audio
	}
	if len(edition) == 0 {
		edition = tm.Edition
	}
	v.HDR, v.Audio, v.Edition = strings.Join(hdr, " "), strings.Join(audio, " "), strings.Join(edition, " ")
	v.Quality = joinNonEmpty(v.Resolution, v.Source, v.HDR, v.VideoCodec)
	return v
}

// planItems 为每个视频算出季集、目标路径与字幕，并看库里是不是已经有了。
func (s *Service) planItems(ctx context.Context, p *Plan, subs []transfer.File) {
	lib, tv := p.Library, p.Match.MediaType == tmdb.KindTV
	tpl := effectiveTemplate(*lib)
	if len(p.Items) == 1 && p.Items[0].disc != nil {
		// 原盘：一项、没有字幕（见 finishPlan）
		s.planDisc(ctx, p, tpl, &p.Items[0])
		return
	}
	seasons := map[int]*tmdb.Season{}
	season := func(n int) *tmdb.Season {
		if s, ok := seasons[n]; ok {
			return s
		}
		var out *tmdb.Season
		if p.tmdbClient != nil {
			out, _ = p.tmdbClient.Season(ctx, p.Match.ID, n)
		}
		seasons[n] = out
		return out
	}
	videos := make([]transfer.File, 0, len(p.Items))
	for i := range p.Items {
		it := &p.Items[i]
		videos = append(videos, transfer.File{Rel: it.Rel, Path: it.Source, Size: it.Size})
		fm, _ := s.cfg.Recognizer.Parse(ctx, stemOf(it.Rel), "")
		it.meta = fm
		v := vars(p.Match, fm, p.meta)
		if tv {
			it.Season = fm.Season
			if it.Season == 0 {
				it.Season = p.meta.Season
			}
			if it.Season == 0 {
				it.Season = 1
			}
			it.Episode, it.EpisodeEnd = fm.Episode, fm.EpisodeEnd
			if it.Episode == 0 && len(p.Items) == 1 {
				it.Episode, it.EpisodeEnd = p.meta.Episode, p.meta.EpisodeEnd
			}
			if it.Episode == 0 {
				it.fail(models.MediaFailRecognize, "文件名里没认出集数：可以加识别词，或改文件名后重试")
				continue
			}
			if p.group != nil {
				p.regroup(it)
			}
			if sd := season(it.Season); sd != nil {
				it.stillFor = sd
				if ep := sd.Episode(it.Episode); ep != nil && ep.Name != "" {
					it.EpisodeTitle = ep.Name
				}
			}
			v.Season, v.Episode, v.EpisodeEnd = it.Season, it.Episode, it.EpisodeEnd
			v.SeasonEpisode = transfer.SeasonEpisode(it.Season, it.Episode, it.EpisodeEnd)
			v.EpisodeTitle = it.EpisodeTitle
		}
		rel, err := transfer.Render(tpl, v)
		if err != nil {
			it.fail(models.MediaFailLibrary, err.Error())
			continue
		}
		it.Target = filepath.Join(p.base(), filepath.FromSlash(rel)) + strings.ToLower(path.Ext(it.Rel))
		if !transfer.Within(lib.Path, it.Target) {
			it.fail(models.MediaFailLibrary, "模板算出的路径不在库目录里")
			it.Target = ""
			continue
		}
	}
	// 两个文件算出了同一个目标（例如同一集的两个版本）：第二个起不整理
	for i := range p.Items {
		for j := range i {
			if p.Items[i].Target != "" && p.Items[i].Target == p.Items[j].Target && p.Items[j].Status != ItemFailed {
				p.Items[i].fail(models.MediaFailLibrary, "与另一个文件算出的目标相同（同一集或同一部的另一个版本）：在模板里加上画质或版本")
			}
		}
	}
	matched := transfer.MatchSubtitles(videos, subs, s.subtitleKey(ctx, p))
	for i := range p.Items {
		it := &p.Items[i]
		if it.Status == ItemFailed {
			continue
		}
		// 先看库里是不是已经有了（有记录时目标换成记录里的），字幕跟着最终的目标走
		it.Status = s.itemState(ctx, p, it)
		counts := map[string]int{}
		for _, sub := range matched[i] {
			lang := transfer.SubtitleLang(sub.Rel) + strings.ToLower(path.Ext(sub.Rel))
			counts[lang]++
			it.Subtitles = append(it.Subtitles, SubtitlePlan{Source: sub.Path, Target: transfer.SubtitleTarget(it.Target, sub.Rel, counts[lang])})
		}
	}
}

// subtitleKey 是把字幕分给剧集的哪一集时用的季集（「季:集」，文件名里没认出集数时为空）；电影为 nil（只按文件名前缀分）。
func (s *Service) subtitleKey(ctx context.Context, p *Plan) func(transfer.File) string {
	if p.Match == nil || p.Match.MediaType != tmdb.KindTV {
		return nil
	}
	return func(f transfer.File) string {
		m, _ := s.cfg.Recognizer.Parse(ctx, stemOf(f.Rel), "")
		if m.Episode == 0 {
			return ""
		}
		sn := m.Season
		if sn == 0 {
			sn = max(p.meta.Season, 1)
		}
		return fmt.Sprintf("%d:%d", sn, m.Episode)
	}
}

// itemState 看库里是不是已经有这个文件。已经整理过、这次条目变了的，见 changeState。
func (s *Service) itemState(ctx context.Context, p *Plan, it *PlanItem) string {
	if it.row != nil && it.row.Status == models.MediaTransferDone && it.row.TargetPath != "" {
		if _, err := os.Lstat(it.row.TargetPath); err == nil {
			if !ours(it.row) {
				it.Target = it.row.TargetPath
				it.Message = "库里的文件已经不是当初整理出的那个（换成了别的文件或改过），不会覆盖"
				return ItemExists
			}
			if entryChanged(it.row, p) && (p.explicit || p.corrected && !pinnedEntry(*it.row)) {
				return s.changeState(ctx, p, it)
			}
			it.Target = it.row.TargetPath
			return ItemDone
		}
	}
	return s.targetState(ctx, p, it)
}

// entryChanged 报告已整理的这条记录的条目和这次的不同。只看条目（类型与 TMDB 编号），不看媒体库：请求里不带媒体库时
// 选出的库可能和记录的不同，只换了库的文件不动。换不换还要看条目是怎么来的（见 itemState）：请求里明确指定的才换明确指定过的
// （手动、订阅、推送时说明的）记录；没有明确指定时只有手动纠正给的条目才换，而且只换自动识别来的记录。
func entryChanged(row *models.MediaTransferHistory, p *Plan) bool {
	if row.TMDBID <= 0 || p.Match == nil || p.Library == nil {
		return false
	}
	return row.MediaType != p.Match.MediaType || row.TMDBID != p.Match.ID
}

// sameEntry 报告两个路径是不是库里同一个位置：字符串相同，或者在同一个目录里（媒体库路径是软链接、挂载别名）、
// 文件名只差大小写、而且是同一个文件。判断不了时当作不同。硬链接方式整理的新旧位置本来就是同一个文件，所以还要比目录。
func sameEntry(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	if !strings.EqualFold(filepath.Base(a), filepath.Base(b)) {
		return false
	}
	da, aerr := os.Stat(filepath.Dir(a))
	db, berr := os.Stat(filepath.Dir(b))
	if aerr != nil || berr != nil || !os.SameFile(da, db) {
		return false
	}
	fa, aerr := os.Lstat(a)
	fb, berr := os.Lstat(b)
	return aerr == nil && berr == nil && os.SameFile(fa, fb)
}

// changeState 是已整理、条目变了的文件的状态：能换的是 ItemChange（位置不变时 inPlace）；不能换的（移动整理的、
// 导入的、源文件不在、新位置被别的文件或另一个版本占着）是 ItemDone 加 keep，库里的文件与记录都不动，写明原因。
func (s *Service) changeState(ctx context.Context, p *Plan, it *PlanItem) string {
	row := *it.row
	it.prior = &row
	it.PriorTarget, it.PriorMediaType, it.PriorTMDBID, it.PriorTitle = row.TargetPath, row.MediaType, row.TMDBID, row.Title
	keep := func(msg string) string {
		it.keep, it.Target, it.Message = true, row.TargetPath, msg
		return ItemDone
	}
	switch row.Mode {
	case models.MediaModeMove:
		return keep("移动整理的文件（库里唯一的一份）不换条目，还在原来的位置")
	case models.MediaModeExisting:
		return keep("导入时就在库里的文件不换条目，还在原来的位置")
	}
	if _, err := os.Stat(it.Source); err != nil {
		return keep("下载目录里的源文件不在了，换不了条目，还在原来的位置")
	}
	if sameEntry(it.Target, row.TargetPath) {
		it.inPlace = true
		it.Message = "库里的文件位置不变：整理记录改成这个条目，重新刮削"
		return ItemChange
	}
	switch st := s.targetState(ctx, p, it); st {
	case ItemPending:
		it.Message = "换条目：先在新位置放好，再删掉旧位置里 pt-tools 放的文件"
		return ItemChange
	case ItemDone:
		it.placed = true
		it.Message = "换条目：新位置已经是这个文件，删掉旧位置里 pt-tools 放的文件"
		return ItemChange
	}
	msg := it.Message
	it.replaces = nil
	return keep("换条目：新位置已经有文件（" + msg + "），这次不换，还在原来的位置")
}

// targetState 看算出的目标位置上有没有文件、是不是这个文件，被 pt-tools 整理的旧版本占着时按覆盖策略看换不换。
func (s *Service) targetState(ctx context.Context, p *Plan, it *PlanItem) string {
	dstInfo, err := os.Lstat(it.Target)
	if err != nil {
		return ItemPending
	}
	if srcInfo, err := os.Stat(it.Source); err == nil && os.SameFile(srcInfo, dstInfo) {
		return ItemDone
	}
	if to, err := os.Readlink(it.Target); err == nil && filepath.Clean(to) == filepath.Clean(it.Source) {
		return ItemDone
	}
	old := s.occupant(ctx, p, it)
	if old == nil {
		it.Message = "目标位置已经有同名文件（不是同一个文件），不会覆盖"
		return ItemExists
	}
	if old.Mode == models.MediaModeExisting {
		// 用户原来的文件：换的话要把它挪开删掉
		it.Message = fmt.Sprintf("目标位置是导入时就在库里的文件（%s，%s，不是 pt-tools 放进去的），不替换", filepath.Base(old.TargetPath), utils.FormatBytes(old.Size))
		return ItemExists
	}
	// 目标位置是 pt-tools 之前放进库的另一个版本：按媒体库的覆盖策略看换不换
	what := fmt.Sprintf("目标位置是之前整理的另一个版本（%s，%s）", firstNonEmpty(old.TorrentName, filepath.Base(old.SourcePath)), utils.FormatBytes(old.Size))
	switch {
	case p.Library.OverwriteMode == OverwriteNever:
		it.Message = what + "，没有替换：要换成新的，在媒体库里把「同名文件」改成替换"
	case old.Mode == models.MediaModeMove:
		it.Message = what + "，是移动整理的（库里唯一的一份），不替换"
	case p.Mode == models.MediaModeMove:
		it.Message = what + "，这个媒体库用移动整理，不替换旧版本"
	case old.TargetFileID == "":
		// 替换时按文件编号确认挪开的是旧版本，记录里没有编号就没法确认
		it.Message = what + "，整理记录里没有它的文件编号，不替换"
	case it.Size <= old.Size:
		it.Message = what + fmt.Sprintf("，新文件（%s）不比它大，不替换", utils.FormatBytes(it.Size))
	default:
		it.replaces = old
		it.Message = fmt.Sprintf("替换库里的旧版本（%s，%s → %s）", firstNonEmpty(old.TorrentName, filepath.Base(old.SourcePath)),
			utils.FormatBytes(old.Size), utils.FormatBytes(it.Size))
		return ItemReplace
	}
	return ItemExists
}

// occupant 是占着目标位置的旧版本：pt-tools 整理进同一个库、同一部电影（剧集是同一季同几集）的另一条已整理记录，
// 库里还是它放的那个文件。别的条目（例如模板写成固定的文件名）、别的库的记录都不算。没有时为空。
func (s *Service) occupant(ctx context.Context, p *Plan, it *PlanItem) *models.MediaTransferHistory {
	if p.Library == nil || p.Match == nil {
		return nil
	}
	q := s.cfg.DB.WithContext(ctx).
		Where("target_path = ? AND status = ? AND source_path <> ? AND library_id = ? AND media_type = ? AND tmdb_id = ?",
			it.Target, models.MediaTransferDone, it.Source, p.Library.ID, p.Match.MediaType, p.Match.ID)
	if p.Match.MediaType == tmdb.KindTV {
		q = q.Where("season = ? AND episode = ? AND episode_end = ?", it.Season, it.Episode, it.EpisodeEnd)
	}
	var row models.MediaTransferHistory
	if err := q.Order("id DESC").Limit(1).Find(&row).Error; err != nil || row.ID == 0 || !ours(&row) {
		return nil
	}
	return &row
}

// Preview 算出整理计划，不改动任何文件。
func (s *Service) Preview(ctx context.Context, req Request) (*Plan, error) {
	return s.plan(ctx, req)
}
