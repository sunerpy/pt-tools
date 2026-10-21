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
				it.Status, it.Message = ItemFailed, "文件名里没认出集数：可以加识别词，或改文件名后重试"
				continue
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
			it.Status, it.Message = ItemFailed, err.Error()
			continue
		}
		it.Target = filepath.Join(p.base(), filepath.FromSlash(rel)) + strings.ToLower(path.Ext(it.Rel))
		if !transfer.Within(lib.Path, it.Target) {
			it.Status, it.Message, it.Target = ItemFailed, "模板算出的路径不在库目录里", ""
			continue
		}
	}
	// 两个文件算出了同一个目标（例如同一集的两个版本）：第二个起不整理
	for i := range p.Items {
		for j := range i {
			if p.Items[i].Target != "" && p.Items[i].Target == p.Items[j].Target && p.Items[j].Status != ItemFailed {
				p.Items[i].Status, p.Items[i].Message = ItemFailed, "与另一个文件算出的目标相同（同一集或同一部的另一个版本）：在模板里加上画质或版本"
			}
		}
	}
	key := func(f transfer.File) string {
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
	if !tv {
		key = nil
	}
	matched := transfer.MatchSubtitles(videos, subs, key)
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

// itemState 看库里是不是已经有这个文件。
func (s *Service) itemState(ctx context.Context, p *Plan, it *PlanItem) string {
	if it.row != nil && it.row.Status == models.MediaTransferDone && it.row.TargetPath != "" {
		if _, err := os.Lstat(it.row.TargetPath); err == nil {
			it.Target = it.row.TargetPath
			if ours(it.row) {
				return ItemDone
			}
			it.Message = "库里的文件已经不是当初整理出的那个（换成了别的文件或改过），不会覆盖"
			return ItemExists
		}
	}
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
