package organize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/scrape"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// Request 是一次整理的请求。Hash 是种子在下载器里的编号（qBittorrent 是 info hash）。
// 手动整理时可以指定 TMDB 条目与媒体库；不指定时自动识别、按类型选库。
type Request struct {
	DownloaderID uint   `json:"downloader_id"`
	Hash         string `json:"hash"`
	MediaType    string `json:"media_type,omitempty"`
	TMDBID       int    `json:"tmdb_id,omitempty"`
	LibraryID    uint   `json:"library_id,omitempty"`
}

// 计划里每个文件的状态。
const (
	ItemPending = "pending" // 要整理
	ItemDone    = "done"    // 库里已经有了（之前整理过）
	ItemExists  = "exists"  // 目标位置被别的文件占着，不覆盖
	ItemFailed  = "failed"  // 整理不了，Message 写明原因
)

// SubtitlePlan 是一个跟着视频整理的字幕。
type SubtitlePlan struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// PlanItem 是计划里的一个视频文件。
type PlanItem struct {
	Rel          string         `json:"rel"`
	Source       string         `json:"source"`
	Size         int64          `json:"size"`
	Target       string         `json:"target,omitempty"`
	Season       int            `json:"season,omitempty"`
	Episode      int            `json:"episode,omitempty"`
	EpisodeEnd   int            `json:"episode_end,omitempty"`
	EpisodeTitle string         `json:"episode_title,omitempty"`
	Subtitles    []SubtitlePlan `json:"subtitles,omitempty"`
	Status       string         `json:"status"`
	Message      string         `json:"message,omitempty"`
	HistoryID    uint           `json:"history_id,omitempty"`

	meta     meta.Meta
	row      *models.MediaTransferHistory
	stillFor *tmdb.Season
}

// Plan 是一个种子的整理计划，预览与执行共用。Problem 不为空时整个种子整理不了。
type Plan struct {
	DownloaderID   uint                 `json:"downloader_id"`
	DownloaderName string               `json:"downloader_name"`
	Hash           string               `json:"hash"`
	TaskID         string               `json:"task_id"`
	Name           string               `json:"name"`
	SavePath       string               `json:"save_path"`
	LocalPath      string               `json:"local_path"`
	Mapped         bool                 `json:"mapped"`
	Match          *tmdb.Result         `json:"match,omitempty"`
	Source         string               `json:"source,omitempty"`
	Library        *models.MediaLibrary `json:"library,omitempty"`
	Mode           string               `json:"mode,omitempty"`
	Items          []PlanItem           `json:"items"`
	Skipped        []transfer.Skip      `json:"skipped,omitempty"`
	Problem        string               `json:"problem,omitempty"`

	// retryable 表示过一会儿再试可能就好了（TMDB 暂时不能访问、文件操作失败）
	retryable bool
	// noRecord 表示不写整理记录（种子没下载完）
	noRecord   bool
	contentSrc string
	meta       meta.Meta
	siteName   string
	tmdbClient *tmdb.Client
}

// Result 是一次整理的结果。
type Result struct {
	Plan     *Plan    `json:"plan"`
	Created  int      `json:"created"`
	Done     int      `json:"done"`
	Skipped  int      `json:"skipped"`
	Failed   int      `json:"failed"`
	Messages []string `json:"messages,omitempty"`
	// Queued 表示整理还在后台进行（等不到结果就先回了），稍后到整理历史里看
	Queued bool `json:"queued,omitempty"`
}

// 来源：手动指定的 TMDB 条目；沿用之前整理这些文件时用的条目。
const (
	sourceManual  = "manual"
	sourceHistory = "history"
)

// cleanRel 检查下载器给的相对路径：不能是绝对路径，不能有 .. 这一级。
func cleanRel(name string) (string, bool) {
	rel := strings.ReplaceAll(name, `\`, "/")
	if rel == "" || strings.HasPrefix(rel, "/") || (len(rel) >= 2 && rel[1] == ':') {
		return "", false
	}
	for seg := range strings.SplitSeq(rel, "/") {
		if seg == ".." {
			return "", false
		}
	}
	return path.Clean(rel), true
}

func stemOf(name string) string {
	base := path.Base(strings.ReplaceAll(name, `\`, "/"))
	return strings.TrimSuffix(base, path.Ext(base))
}

// plan 算出一个种子怎么整理，不改动任何文件。只有请求本身不对、下载器或数据库出错时返回错误；
// 种子整理不了的原因写在 Plan.Problem 里，这时库里已经有的文件标成「已在库里」，其余标成「整理不了」。
func (s *Service) plan(ctx context.Context, req Request) (*Plan, error) {
	p, err := s.buildPlan(ctx, req)
	if err != nil || p.Problem == "" {
		return p, err
	}
	for i := range p.Items {
		it := &p.Items[i]
		if it.row != nil && it.row.Status == models.MediaTransferDone && ours(it.row) {
			it.Status, it.Target = ItemDone, it.row.TargetPath
			continue
		}
		it.Status = ItemFailed
	}
	return p, nil
}

// ours 报告记录里的目标还是不是当初整理出的那个文件：软链接看指向；其余看文件编号，硬链接也可以看和源文件是不是同一个。
func ours(row *models.MediaTransferHistory) bool {
	if row == nil || row.TargetPath == "" {
		return false
	}
	info, err := os.Lstat(row.TargetPath)
	if err != nil {
		return false
	}
	if row.Mode == models.MediaModeSymlink {
		to, err := os.Readlink(row.TargetPath)
		return err == nil && filepath.Clean(to) == filepath.Clean(row.SourcePath)
	}
	if !info.Mode().IsRegular() {
		return false
	}
	if id, err := transfer.FileID(row.TargetPath); err == nil && row.TargetFileID != "" && id == row.TargetFileID {
		return true
	}
	if row.Mode == models.MediaModeHardlink || row.Mode == "" {
		src, err := os.Stat(row.SourcePath)
		return err == nil && os.SameFile(src, info)
	}
	return false
}

// extraOurs 报告字幕还是不是当初整理出的那个：软链接看指向；其余看文件编号，硬链接也可以看和源文件是不是同一个。
func extraOurs(e extra, mode string) bool {
	info, err := os.Lstat(e.Target)
	if err != nil {
		return false
	}
	if mode == models.MediaModeSymlink {
		to, err := os.Readlink(e.Target)
		return err == nil && filepath.Clean(to) == filepath.Clean(e.Source)
	}
	if !info.Mode().IsRegular() {
		return false
	}
	if id, err := transfer.FileID(e.Target); err == nil && e.FileID != "" && id == e.FileID {
		return true
	}
	if mode == models.MediaModeHardlink || mode == "" {
		src, err := os.Stat(e.Source)
		return err == nil && os.SameFile(src, info)
	}
	return false
}

// priorEntry 是之前整理这些文件时用的条目：先看已经整理好的记录，再看手动整理时指定过条目的记录。
func priorEntry(p *Plan) (string, int) {
	for _, done := range []bool{true, false} {
		for _, it := range p.Items {
			r := it.row
			if r == nil || r.TMDBID <= 0 || (r.MediaType != tmdb.KindMovie && r.MediaType != tmdb.KindTV) {
				continue
			}
			if done && r.Status == models.MediaTransferDone || !done && r.Trigger == models.MediaTriggerManual {
				return r.MediaType, r.TMDBID
			}
		}
	}
	return "", 0
}

func (s *Service) buildPlan(ctx context.Context, req Request) (*Plan, error) {
	if req.DownloaderID == 0 || strings.TrimSpace(req.Hash) == "" {
		return nil, fmt.Errorf("%w: 要指定下载器与种子", ErrInvalid)
	}
	if req.TMDBID < 0 || (req.TMDBID > 0 && req.MediaType != tmdb.KindMovie && req.MediaType != tmdb.KindTV) {
		return nil, fmt.Errorf("%w: 指定条目时要写明类型（movie 或 tv）与 TMDB 编号", ErrInvalid)
	}
	dl, setting, err := s.cfg.Downloaders.Get(ctx, req.DownloaderID)
	if err != nil {
		return nil, err
	}
	t, err := dl.GetTorrent(strings.TrimSpace(req.Hash))
	if err != nil {
		if errors.Is(err, downloader.ErrTorrentNotFound) {
			return nil, fmt.Errorf("%w: 下载器「%s」里没有这个种子", ErrNotFound, setting.Name)
		}
		return nil, fmt.Errorf("读取种子失败: %w", err)
	}
	p := &Plan{
		DownloaderID: setting.ID, DownloaderName: setting.Name, Hash: strings.ToLower(t.InfoHash), TaskID: t.ID,
		Name: t.Name, SavePath: t.SavePath, Items: []PlanItem{},
	}
	if p.Hash == "" {
		p.Hash = strings.ToLower(strings.TrimSpace(req.Hash))
	}
	if t.Progress < 1 {
		p.Problem, p.noRecord = "种子还没下载完", true
		return p, nil
	}
	settings, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	files, err := dl.GetTorrentFiles(t.ID)
	if err != nil {
		return nil, fmt.Errorf("读取种子的文件列表失败: %w", err)
	}
	if p.LocalPath, p.Mapped, err = s.localPath(ctx, setting.ID, t.SavePath); err != nil {
		return nil, err
	}
	content := t.ContentPath
	if content == "" {
		content = strings.TrimRight(t.SavePath, `/\`) + "/" + t.Name
	}
	if p.contentSrc, _, err = s.localPath(ctx, setting.ID, content); err != nil {
		return nil, err
	}
	tfs := make([]transfer.File, 0, len(files))
	for _, f := range files {
		rel, ok := cleanRel(f.Name)
		if !ok {
			continue
		}
		tfs = append(tfs, transfer.File{Rel: rel, Path: filepath.Join(p.LocalPath, filepath.FromSlash(rel)), Size: f.Size})
	}
	sel := transfer.Select(tfs, int64(settings.MinVideoMB)<<20)
	p.Skipped = sel.Skipped
	switch {
	case sel.Disc != "":
		p.Problem = fmt.Sprintf("原盘（%s 目录）暂不整理", sel.Disc)
		return p, nil
	case len(sel.Videos) == 0:
		p.Problem = "没有可整理的视频文件"
		return p, nil
	}
	for _, v := range sel.Videos {
		p.Items = append(p.Items, PlanItem{Rel: v.Rel, Source: v.Path, Size: v.Size, Status: ItemPending})
	}
	if err := s.loadRows(ctx, p); err != nil {
		return nil, err
	}
	// 看第一个还没整理好的文件在不在（整理好的跳过：移动整理以后源文件本来就不在了）
	for _, it := range p.Items {
		if it.row != nil && it.row.Status == models.MediaTransferDone && ours(it.row) {
			continue
		}
		if _, err := os.Stat(it.Source); err != nil {
			p.Problem = fmt.Sprintf("pt-tools 里找不到 %s：下载器与 pt-tools 看到的路径不同时（例如在 Docker 里），在「路径映射」里添加对应关系", it.Source)
			return p, nil
		}
		break
	}
	if !s.recognize(ctx, p, req, t) {
		return p, nil
	}
	if !s.pickLibrary(ctx, p, req) {
		return p, nil
	}
	if p.Mode == models.MediaModeMove && t.State != downloader.TorrentPaused && t.State != downloader.TorrentStopped {
		p.Problem = "种子还在做种，不能用移动整理：暂停种子后再整理，或把媒体库改用硬链接"
		return p, nil
	}
	s.planItems(ctx, p, sel.Subtitles)
	return p, nil
}

// loadRows 读出这些文件已有的整理记录。
func (s *Service) loadRows(ctx context.Context, p *Plan) error {
	srcs := make([]string, 0, len(p.Items))
	for _, it := range p.Items {
		srcs = append(srcs, it.Source)
	}
	var rows []models.MediaTransferHistory
	if err := s.cfg.DB.WithContext(ctx).Where("source_path IN ?", srcs).Find(&rows).Error; err != nil {
		return fmt.Errorf("读取整理记录失败: %w", err)
	}
	for i := range p.Items {
		for j := range rows {
			if rows[j].SourcePath == p.Items[i].Source {
				p.Items[i].row = &rows[j]
				p.Items[i].HistoryID = rows[j].ID
			}
		}
	}
	return nil
}

// recognize 识别种子对应的条目：指定了 TMDB 编号时直接取详情，否则用种子名（站点标题作副标题）与 IMDb 编号识别。
func (s *Service) recognize(ctx context.Context, p *Plan, req Request, t downloader.Torrent) bool {
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if errors.Is(err, tmdb.ErrNoKey) {
		p.Problem = "没有填写 TMDB API Key：先在「媒体识别」里填写"
		return false
	}
	if err == nil {
		p.tmdbClient = c
	}
	var info models.TorrentInfo
	_ = s.cfg.DB.WithContext(ctx).Where("LOWER(torrent_hash) = ?", p.Hash).Order("id DESC").Limit(1).Find(&info).Error
	p.siteName = info.SiteName
	source := sourceManual
	if req.TMDBID == 0 {
		// 之前整理过（比如手动指定过条目）：沿用那个条目，库里的文件名与重新整理时一致
		if kind, id := priorEntry(p); id > 0 {
			req.MediaType, req.TMDBID, source = kind, id, sourceHistory
		}
	}
	if req.TMDBID > 0 {
		if err != nil {
			p.Problem = "识别失败：" + err.Error()
			return false
		}
		d, detailErr := c.Details(ctx, req.MediaType, req.TMDBID)
		if detailErr != nil {
			p.Problem, p.retryable = "识别失败："+detailErr.Error(), !errors.Is(detailErr, tmdb.ErrNotFound)
			return false
		}
		p.meta, _ = s.cfg.Recognizer.Parse(ctx, t.Name, "")
		p.Match, p.Source = d, source
		return true
	}
	in := recognize.Input{Title: t.Name, IMDbID: info.IMDbID}
	if info.Title != "" && info.Title != t.Name {
		in.Subtitle = info.Title
	}
	res, err := s.cfg.Recognizer.Recognize(ctx, in)
	if err != nil {
		p.Problem = "识别失败：" + err.Error()
		return false
	}
	p.meta = res.Meta
	switch {
	case res.Error != "":
		p.Problem, p.retryable = "识别失败："+res.Error, true
		return false
	case res.Match == nil:
		p.Problem = "没有识别出来：TMDB 上没有可靠的匹配"
		if len(res.Candidates) > 0 {
			c := res.Candidates[0]
			p.Problem += fmt.Sprintf("（最接近的是「%s」）", firstNonEmpty(c.Title, c.OriginalTitle))
		}
		p.Problem += "。手动整理时填 TMDB 编号，或到「媒体识别」里加纠正、识别词"
		return false
	}
	p.Match, p.Source = res.Match, res.Source
	return true
}

// pickLibrary 选媒体库：指定了就用指定的（要启用、类型相同）；否则在同类型里选，动画优先进只收动画的库。
func (s *Service) pickLibrary(ctx context.Context, p *Plan, req Request) bool {
	var libs []models.MediaLibrary
	if err := s.cfg.DB.WithContext(ctx).Where("enabled = ?", true).Order("id").Find(&libs).Error; err != nil {
		p.Problem, p.retryable = "读取媒体库失败："+err.Error(), true
		return false
	}
	kind := p.Match.MediaType
	var lib *models.MediaLibrary
	if req.LibraryID > 0 {
		for i := range libs {
			if libs[i].ID == req.LibraryID && libs[i].Kind == kind {
				lib = &libs[i]
			}
		}
		if lib == nil {
			p.Problem = "指定的媒体库不存在、没有启用，或类型与条目不同"
			return false
		}
	} else {
		lib = chooseLibrary(libs, kind, p.Match.IsAnimation())
	}
	if lib == nil {
		what := "电影"
		if kind == tmdb.KindTV {
			what = "剧集"
		}
		p.Problem = fmt.Sprintf("没有启用的%s媒体库：先在「媒体库」里添加", what)
		return false
	}
	p.Library, p.Mode = lib, lib.Mode
	if p.Mode == "" {
		p.Mode = models.MediaModeHardlink
	}
	return true
}

func chooseLibrary(libs []models.MediaLibrary, kind string, anime bool) *models.MediaLibrary {
	var plain, animeLib *models.MediaLibrary
	for i := range libs {
		l := &libs[i]
		if !l.Enabled || l.Kind != kind {
			continue
		}
		if l.Anime {
			if animeLib == nil {
				animeLib = l
			}
		} else if plain == nil {
			plain = l
		}
	}
	if anime && animeLib != nil {
		return animeLib
	}
	return plain
}

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
		it.Target = filepath.Join(lib.Path, filepath.FromSlash(rel)) + strings.ToLower(path.Ext(it.Rel))
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
		it.Status = s.itemState(it)
		counts := map[string]int{}
		for _, sub := range matched[i] {
			lang := transfer.SubtitleLang(sub.Rel) + strings.ToLower(path.Ext(sub.Rel))
			counts[lang]++
			it.Subtitles = append(it.Subtitles, SubtitlePlan{Source: sub.Path, Target: transfer.SubtitleTarget(it.Target, sub.Rel, counts[lang])})
		}
	}
}

// itemState 看库里是不是已经有这个文件。
func (s *Service) itemState(it *PlanItem) string {
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
	it.Message = "目标位置已经有同名文件（不是同一个文件），不会覆盖"
	return ItemExists
}

// Preview 算出整理计划，不改动任何文件。
func (s *Service) Preview(ctx context.Context, req Request) (*Plan, error) {
	return s.plan(ctx, req)
}

// retryDelays 是失败后第 1 到 5 次自动重试前的等待时间；之后不再自动重试。
var retryDelays = []time.Duration{10 * time.Minute, 30 * time.Minute, time.Hour, 3 * time.Hour, 12 * time.Hour}

// extra 是跟着一个视频整理或写出的文件：Kind 为空是字幕，meta 是刮削写的 NFO 与图片。
type extra struct {
	Source string `json:"source,omitempty"`
	Target string `json:"target"`
	FileID string `json:"file_id,omitempty"`
	Kind   string `json:"kind,omitempty"`
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
		switch it.Status {
		case ItemFailed:
			res.Failed++
			s.record(ctx, p, it, trigger, models.MediaTransferFailed, it.Message, false, "", nil)
			continue
		case ItemExists:
			res.Skipped++
			s.record(ctx, p, it, trigger, models.MediaTransferSkipped, it.Message, false, "", nil)
			continue
		case ItemDone:
			// 库里已经是这个文件：补上之前没整理成的字幕；没有记录时（例如记录被删了）补一条
			subs, msgs, added := s.transferSubtitles(it, p.Mode, rowExtras(it.row))
			res.Messages = append(res.Messages, msgs...)
			if it.row == nil || it.row.Status != models.MediaTransferDone || added > 0 || len(msgs) > 0 {
				fileID := ""
				if it.row != nil && it.row.Status == models.MediaTransferDone {
					fileID = it.row.TargetFileID
				}
				if fileID == "" {
					fileID, _ = transfer.FileID(it.Target)
				}
				s.record(ctx, p, it, trigger, models.MediaTransferDone, strings.Join(msgs, "；"), false, fileID, mergeExtras(rowExtras(it.row), subs))
			}
			res.Done++
			continue
		}
		// 自动整理不重做记录为已整理、但库里的文件不在了的（多半是用户自己删的）
		if it.row != nil && it.row.Status == models.MediaTransferDone && trigger != models.MediaTriggerManual {
			res.Done++
			continue
		}
		out, terr := transfer.Transfer(it.Source, it.Target, p.Mode)
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
		fileID, _ := transfer.FileID(it.Target)
		extras, msgs, _ := s.transferSubtitles(it, p.Mode, rowExtras(it.row))
		res.Messages = append(res.Messages, msgs...)
		it.Status = ItemDone
		s.record(ctx, p, it, trigger, models.MediaTransferDone, strings.Join(msgs, "；"), false, fileID, mergeExtras(rowExtras(it.row), extras))
		if out == transfer.AlreadyDone {
			res.Done++
			continue
		}
		res.Created++
		created = append(created, it)
	}
	if len(created) > 0 {
		msgs, written := s.scrape(ctx, p, created)
		res.Messages = append(res.Messages, msgs...)
		s.recordScraped(ctx, p, created, written)
		res.Messages = append(res.Messages, s.refreshServers(ctx, p, created)...)
		if msg := s.notify(ctx, p, created); msg != "" {
			res.Messages = append(res.Messages, msg)
		}
	}
	return res, nil
}

// transferSubtitles 用同样的方式整理字幕；失败的记在消息里（写进整理记录），不影响视频，下次整理时再试。
// 返回整理好的字幕（含之前就在的）、失败的消息与这次新放进库的个数。
func (s *Service) transferSubtitles(it *PlanItem, mode string, prior []extra) ([]extra, []string, int) {
	var out []extra
	var msgs []string
	added := 0
	for _, sub := range it.Subtitles {
		// 之前整理好的（复制与移动认不出来历，按记录里的文件编号认）
		if i := slices.IndexFunc(prior, func(e extra) bool { return e.Kind == "" && e.Target == sub.Target }); i >= 0 && extraOurs(prior[i], mode) {
			out = append(out, prior[i])
			continue
		}
		res, err := transfer.Transfer(sub.Source, sub.Target, mode)
		if err != nil {
			msgs = append(msgs, fmt.Sprintf("字幕 %s：%v", filepath.Base(sub.Source), err))
			continue
		}
		if res == transfer.Created {
			added++
		}
		id, _ := transfer.FileID(sub.Target)
		out = append(out, extra{Source: sub.Source, Target: sub.Target, FileID: id})
	}
	return out, msgs, added
}

// recordScraped 把刮削写出的文件与文件编号记进对应视频的整理记录：和视频同名的记给这个视频，
// 目录里的（海报、背景、tvshow.nfo、季海报与 season.nfo）记给这个目录里每个新整理的视频。删除时只删这些。
func (s *Service) recordScraped(ctx context.Context, p *Plan, created []*PlanItem, written []string) {
	if len(written) == 0 {
		return
	}
	ids := make(map[string]string, len(written))
	for _, w := range written {
		if id, err := transfer.FileID(w); err == nil {
			ids[w] = id
		}
	}
	for _, it := range created {
		if it.HistoryID == 0 {
			continue
		}
		stem := strings.TrimSuffix(it.Target, filepath.Ext(it.Target))
		dirs := []string{filepath.Dir(it.Target)}
		if sd := showDir(p.Library, it.Target); sd != "" {
			dirs = append(dirs, sd)
		}
		var add []extra
		for _, w := range written {
			id, ok := ids[w]
			if !ok {
				continue
			}
			own := strings.HasPrefix(w, stem+".") || strings.HasPrefix(w, stem+"-")
			shared := isDirArtifact(filepath.Base(w)) && slices.Contains(dirs, filepath.Dir(w))
			if own || shared {
				add = append(add, extra{Target: w, FileID: id, Kind: extraMeta})
			}
		}
		if len(add) == 0 {
			continue
		}
		var row models.MediaTransferHistory
		if err := s.cfg.DB.WithContext(ctx).Where("id = ?", it.HistoryID).Limit(1).Find(&row).Error; err != nil || row.ID == 0 {
			continue
		}
		b, _ := json.Marshal(mergeExtras(decodeExtras(row.Extras), add))
		if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).Where("id = ?", row.ID).Update("extras", string(b)).Error; err != nil {
			s.cfg.Logger.Warnf("[整理入库] 记下刮削的文件失败 (%s): %v", it.Target, err)
		}
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
		// 已经整理好的文件不因为这次识别失败改成失败
		if it.row != nil && it.row.Status == models.MediaTransferDone {
			res.Done++
			continue
		}
		it.Status, it.Message = ItemFailed, p.Problem
		s.record(ctx, p, it, trigger, status, p.Problem, p.retryable, "", nil)
		res.Failed++
	}
}

// record 写一个文件的整理记录（按源文件更新或新建）。
func (s *Service) record(ctx context.Context, p *Plan, it *PlanItem, trigger, status, msg string, retry bool, fileID string, extras []extra) {
	now := s.cfg.Now()
	row := models.MediaTransferHistory{
		DownloaderID: p.DownloaderID, DownloaderName: p.DownloaderName, InfoHash: p.Hash, TaskID: p.TaskID,
		TorrentName: truncate(p.Name, 512), SourcePath: it.Source, SaveRoot: p.LocalPath, TargetPath: it.Target, Mode: p.Mode,
		Season: it.Season, Episode: it.Episode, EpisodeEnd: it.EpisodeEnd, Size: it.Size, Status: status,
		Message: truncate(msg, 1024), Trigger: trigger, TargetFileID: fileID, CreatedAt: now, UpdatedAt: now,
	}
	if p.Library != nil {
		row.LibraryID = p.Library.ID
	}
	if p.Match != nil {
		row.MediaType, row.TMDBID, row.Title, row.Year = p.Match.MediaType, p.Match.ID, truncate(p.Match.Title, 255), p.Match.Year
	}
	if len(extras) > 0 {
		b, _ := json.Marshal(extras)
		row.Extras = string(b)
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
	err := s.cfg.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "source_path"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"downloader_id", "downloader_name", "info_hash", "task_id", "torrent_name", "library_id", "save_root", "target_path",
			"extras", "mode", "media_type", "tmdb_id", "title", "year", "season", "episode", "episode_end", "size",
			"status", "message", "attempts", "next_retry_at", "target_file_id", "trigger", "updated_at",
		}),
	}).Create(&row).Error
	if err != nil {
		s.cfg.Logger.Errorf("[整理入库] 写整理记录失败 (%s): %v", it.Source, err)
		return
	}
	if it.HistoryID == 0 {
		var saved models.MediaTransferHistory
		if s.cfg.DB.WithContext(ctx).Select("id").Where("source_path = ?", it.Source).Limit(1).Find(&saved).Error == nil {
			it.HistoryID = saved.ID
		}
	}
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

// showDir 是剧集目录：模板算出的路径至少有两级时取第一级；平铺时为空。
func showDir(lib *models.MediaLibrary, target string) string {
	rel, err := filepath.Rel(lib.Path, target)
	if err != nil {
		return ""
	}
	segs := strings.Split(filepath.ToSlash(rel), "/")
	if len(segs) < 2 {
		return ""
	}
	return filepath.Join(lib.Path, segs[0])
}

// scrape 为这次新放进库的文件写 NFO 与图片（媒体库关了刮削时不写）。返回出错的消息与写出的文件。
func (s *Service) scrape(ctx context.Context, p *Plan, created []*PlanItem) ([]string, []string) {
	if !p.Library.Scrape {
		return nil, nil
	}
	w := scrape.Writer{Overwrite: p.Library.ScrapeOverwrite}
	if p.tmdbClient != nil {
		w.Images = p.tmdbClient
	}
	var rep scrape.Report
	if p.Match.MediaType != tmdb.KindTV {
		for _, it := range created {
			ownDir := filepath.Clean(filepath.Dir(it.Target)) != filepath.Clean(p.Library.Path)
			rep.Merge(w.Movie(ctx, it.Target, ownDir, *p.Match))
		}
	} else {
		shows, seasons := map[string]bool{}, map[string]bool{}
		for _, it := range created {
			sd := showDir(p.Library, it.Target)
			if sd != "" && !shows[sd] {
				shows[sd] = true
				rep.Merge(w.Show(ctx, sd, *p.Match))
			}
			key := fmt.Sprintf("%s|%d", sd, it.Season)
			if sd != "" && !seasons[key] {
				seasons[key] = true
				rep.Merge(w.Season(ctx, sd, filepath.Dir(it.Target), it.Season, it.stillFor))
			}
			rep.Merge(w.Episode(ctx, it.Target, *p.Match, it.Season, it.Episode, it.EpisodeEnd, it.stillFor))
		}
	}
	msgs := make([]string, 0, len(rep.Errors))
	for _, e := range rep.Errors {
		msgs = append(msgs, "刮削："+e)
	}
	return msgs, rep.Written
}

// refreshDirs 是要通知媒体服务器扫描的目录：电影是文件所在目录，剧集是剧集目录（平铺时是文件所在目录）。
func refreshDirs(p *Plan, created []*PlanItem) []string {
	var dirs []string
	for _, it := range created {
		d := filepath.Dir(it.Target)
		if p.Match.MediaType == tmdb.KindTV {
			if sd := showDir(p.Library, it.Target); sd != "" {
				d = sd
			}
		}
		if !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// refreshServers 通知启用的媒体服务器扫描新目录，结果记在服务器上。
func (s *Service) refreshServers(ctx context.Context, p *Plan, created []*PlanItem) []string {
	var servers []models.MediaServer
	if err := s.cfg.DB.WithContext(ctx).Where("enabled = ?", true).Order("id").Find(&servers).Error; err != nil {
		return []string{"读取媒体服务器失败：" + err.Error()}
	}
	dirs := refreshDirs(p, created)
	var msgs []string
	for _, srv := range servers {
		c, err := s.serverClient(srv)
		if err == nil {
			if srv.RefreshMode == models.MediaRefreshLibrary {
				err = c.RefreshAll(ctx)
			} else {
				paths := make([]string, 0, len(dirs))
				var unmapped []string
				for _, d := range dirs {
					if sp, ok := serverPath(srv, d); ok {
						paths = append(paths, sp)
					} else {
						unmapped = append(unmapped, d)
					}
				}
				if len(paths) > 0 {
					err = c.RefreshPaths(ctx, paths)
				}
				if err == nil && len(unmapped) > 0 {
					err = fmt.Errorf("%s 不在路径映射 %s → %s 里，没有通知", strings.Join(unmapped, "、"), srv.LocalPrefix, srv.ServerPrefix)
				}
			}
		}
		now := s.cfg.Now()
		upd := map[string]any{"last_refresh_at": &now, "last_error": ""}
		if err != nil {
			upd["last_error"] = truncate(err.Error(), 1024)
			msgs = append(msgs, fmt.Sprintf("通知媒体服务器「%s」失败：%v", srv.Name, err))
		}
		s.cfg.DB.WithContext(ctx).Model(&models.MediaServer{}).Where("id = ?", srv.ID).Updates(upd)
	}
	return msgs
}

// notify 发一条入库通知（设置里选了通道时）。返回给结果看的错误；成功时为空。
func (s *Service) notify(ctx context.Context, p *Plan, created []*PlanItem) string {
	if s.cfg.Notify == nil {
		return ""
	}
	settings, err := s.Settings(ctx)
	if err != nil || len(settings.NotifyChannels) == 0 {
		return ""
	}
	n := Notice{Key: fmt.Sprintf("%s:%d", p.Hash, created[0].HistoryID), ChannelIDs: settings.NotifyChannels}
	n.Title, n.Text = noticeText(p, created)
	if err := s.cfg.Notify(ctx, n); err != nil {
		return "入库通知没有发出：" + err.Error()
	}
	return ""
}

// noticeText 是入库通知的标题与正文：条目、季集、画质、媒体库、来源种子，最后是海报地址。
func noticeText(p *Plan, created []*PlanItem) (string, string) {
	m := p.Match
	name := m.Title
	if m.Year > 0 {
		name = fmt.Sprintf("%s (%d)", m.Title, m.Year)
	}
	var lines []string
	if m.MediaType == tmdb.KindTV {
		var eps []string
		for _, it := range created {
			eps = append(eps, transfer.SeasonEpisode(it.Season, it.Episode, it.EpisodeEnd))
		}
		if len(eps) > 6 {
			eps = append(eps[:3], "…", eps[len(eps)-1])
		}
		lines = append(lines, fmt.Sprintf("剧集 %s（%d 个文件）", strings.Join(eps, "、"), len(created)))
	} else {
		lines = append(lines, "电影")
	}
	if q := vars(m, created[0].meta, p.meta).Quality; q != "" {
		lines = append(lines, "画质："+q)
	}
	lines = append(lines, "媒体库："+p.Library.Name, "种子："+p.Name)
	if poster := tmdb.PosterURL(m.PosterPath, "w500"); poster != "" {
		lines = append(lines, "海报："+poster)
	}
	return "入库：" + name, strings.Join(lines, "\n")
}
