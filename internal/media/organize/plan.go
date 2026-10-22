package organize

import (
	"context"
	"crypto/sha1" //nolint:gosec // 只用来给目录监控的一项取个短标识，不涉及安全
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
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
	// WatchDirID 与 Entry 是目录监控里的一项（目录下的一个文件或文件夹）：不为 0 时不看下载器与种子
	WatchDirID uint   `json:"watch_dir_id,omitempty"`
	Entry      string `json:"entry,omitempty"`
	MediaType  string `json:"media_type,omitempty"`
	TMDBID     int    `json:"tmdb_id,omitempty"`
	LibraryID  uint   `json:"library_id,omitempty"`
}

// queueHash 是排队去重、退避与重试分组用的种子标识：目录监控的一项用 watchHash。
func (r Request) queueHash() string {
	if r.WatchDirID > 0 {
		return watchHash(r.WatchDirID, r.Entry)
	}
	return r.Hash
}

// watchHash 是目录监控里一项的标识：记在整理记录的 InfoHash 里（不是种子的 info hash），重试、去重按它分组。
func watchHash(dirID uint, entry string) string {
	sum := sha1.Sum([]byte(entry))
	return fmt.Sprintf("watch:%d:%x", dirID, sum[:8])
}

// 计划里每个文件的状态。
const (
	ItemPending = "pending" // 要整理
	ItemDone    = "done"    // 库里已经有了（之前整理过）
	ItemExists  = "exists"  // 目标位置被别的文件占着，不覆盖
	ItemReplace = "replace" // 目标位置是 pt-tools 之前放进库的旧版本，按覆盖策略换成这个
	ItemFailed  = "failed"  // 整理不了，Message 写明原因
)

// 覆盖策略（MediaLibrary.OverwriteMode）：目标位置已经有 pt-tools 之前放进这个库的同一部（同一集）的旧版本时怎么办。
// 用户自己放的文件、别的条目、移动整理的旧版本（库里唯一的一份）一律不替换。只按大小换：小的不会把大的换回去，
// 两个版本不会来回换。
const (
	OverwriteNever  = ""       // 不替换（默认）
	OverwriteLarger = "larger" // 新文件更大时替换
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
	// mode 是这个文件实际的整理方式，与计划不同时才有：移动时删不掉源文件，库里那份按硬链接或复制记
	mode string
	// replaces 是要替换的旧版本的整理记录（状态为 ItemReplace 时）
	replaces *models.MediaTransferHistory
	// otherFile 表示监控目录里这个位置上换成了另一个文件，记录管着库里的文件，不整理（见 markOtherFiles）
	otherFile bool
	// sourceID 是源文件现在的编号（目录监控的项才读，记进整理记录的 SourceFileID）
	sourceID string
}

// Plan 是一个种子的整理计划，预览与执行共用。Problem 不为空时整个种子整理不了。
type Plan struct {
	DownloaderID   uint   `json:"downloader_id"`
	DownloaderName string `json:"downloader_name"`
	Hash           string `json:"hash"`
	TaskID         string `json:"task_id"`
	Name           string `json:"name"`
	SavePath       string `json:"save_path"`
	LocalPath      string `json:"local_path"`
	Mapped         bool   `json:"mapped"`
	// WatchDirID 是目录监控的目录（这次整理的不是下载器里的种子时）
	WatchDirID uint                 `json:"watch_dir_id,omitempty"`
	Match      *tmdb.Result         `json:"match,omitempty"`
	Source     string               `json:"source,omitempty"`
	Library    *models.MediaLibrary `json:"library,omitempty"`
	// Category 是放进的二级分类（媒体库开了「按二级分类放进子目录」、规则对得上时才有）
	Category string          `json:"category,omitempty"`
	Mode     string          `json:"mode,omitempty"`
	Items    []PlanItem      `json:"items"`
	Skipped  []transfer.Skip `json:"skipped,omitempty"`
	Problem  string          `json:"problem,omitempty"`

	// retryable 表示过一会儿再试可能就好了（TMDB 暂时不能访问、文件操作失败）
	retryable bool
	// noRecord 表示不写整理记录（种子没下载完）
	noRecord   bool
	contentSrc string
	meta       meta.Meta
	siteName   string
	// doubanID 是站点给的豆瓣编号（pt-tools 推送的种子才有），刮削补充豆瓣的内容时电影直接用它
	doubanID   string
	tmdbClient *tmdb.Client
	// root 是这次整理放进的目录：库目录，按二级分类放进子目录时是库目录下的分类目录
	root string
}

// base 是这次整理放进的目录（见 root）。
func (p *Plan) base() string {
	if p.root != "" {
		return p.root
	}
	return p.Library.Path
}

// rootFor 是算剧集目录、电影有没有自己的目录时的起点：按二级分类放进子目录时是分类目录；
// 文件不在分类目录里（之前整理过，目标沿用记录里的位置）时是库目录。
func (p *Plan) rootFor(target string) string {
	if root := p.base(); transfer.Within(root, target) {
		return root
	}
	return p.Library.Path
}

// showDir 是这次整理里一个文件的剧集目录（从 rootFor 算起）。
func (p *Plan) showDir(target string) string {
	return showDir(p.rootFor(target), target)
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
	if err != nil {
		return p, err
	}
	if p.Problem != "" {
		for i := range p.Items {
			it := &p.Items[i]
			if it.row != nil && it.row.Status == models.MediaTransferDone && ours(it.row) {
				it.Status, it.Target = ItemDone, it.row.TargetPath
				continue
			}
			it.Status = ItemFailed
		}
	}
	markOtherFiles(p)
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
	if req.TMDBID < 0 || (req.TMDBID > 0 && req.MediaType != tmdb.KindMovie && req.MediaType != tmdb.KindTV) {
		return nil, fmt.Errorf("%w: 指定条目时要写明类型（movie 或 tv）与 TMDB 编号", ErrInvalid)
	}
	if req.WatchDirID > 0 {
		return s.buildWatchPlan(ctx, req)
	}
	if req.DownloaderID == 0 || strings.TrimSpace(req.Hash) == "" {
		return nil, fmt.Errorf("%w: 要指定下载器与种子", ErrInvalid)
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
		tf := transfer.File{Rel: rel, Path: filepath.Join(p.LocalPath, filepath.FromSlash(rel)), Size: f.Size}
		if f.Progress < 1 {
			// 种子下载完了、这个文件没下完：是在下载器里没选（不下载）的，例如季包里只下了几集
			p.Skipped = append(p.Skipped, transfer.Skip{File: tf, Reason: "在下载器里没选这个文件，没有下载"})
			continue
		}
		tfs = append(tfs, tf)
	}
	return s.finishPlan(ctx, p, req, t, tfs, settings)
}

// finishPlan 是下载器的种子与目录监控的一项共用的部分：选出视频、读整理记录、识别、选媒体库、算目标。
func (s *Service) finishPlan(ctx context.Context, p *Plan, req Request, t downloader.Torrent, tfs []transfer.File, settings Settings) (*Plan, error) {
	sel := transfer.Select(tfs, int64(settings.MinVideoMB)<<20)
	p.Skipped = append(p.Skipped, sel.Skipped...)
	switch {
	case sel.Disc != "":
		p.Problem = fmt.Sprintf("原盘（%s 目录）暂不整理", sel.Disc)
		return p, nil
	case len(sel.Videos) == 0:
		p.Problem = "没有可整理的视频文件"
		return p, nil
	}
	for _, v := range sel.Videos {
		it := PlanItem{Rel: v.Rel, Source: v.Path, Size: v.Size, Status: ItemPending}
		if p.WatchDirID > 0 {
			// 目录监控按源文件的编号认同一个位置上是不是换了文件（见 markOtherFiles）；移动整理以后就读不到了，先读
			it.sourceID, _ = transfer.FileID(v.Path)
		}
		p.Items = append(p.Items, it)
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
	switch {
	case p.Mode != models.MediaModeMove:
	case p.WatchDirID > 0:
		// 目录监控的一项可能是下载器里的种子（监控了下载目录）：移走了种子就没法做种
		owner, err := s.watchTorrent(ctx, p.contentSrc, p.Name)
		if err != nil {
			p.Problem, p.retryable = "移动整理以前要确认「"+p.Name+"」不是下载器里的种子："+err.Error(), true
			return p, nil
		}
		if owner != "" {
			p.Problem = "「" + p.Name + "」是下载器里的种子（" + owner + "），目录监控不移动下载器里的种子：把媒体库改用硬链接，或不要监控下载目录"
			return p, nil
		}
	case t.State != downloader.TorrentPaused && t.State != downloader.TorrentStopped:
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
	if p.WatchDirID == 0 {
		// 目录监控的一项不是种子：没有站点给的标题与 IMDb 编号
		_ = s.cfg.DB.WithContext(ctx).Where("LOWER(torrent_hash) = ?", p.Hash).Order("id DESC").Limit(1).Find(&info).Error
	}
	p.siteName, p.doubanID = info.SiteName, info.DoubanID
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

// pickLibrary 选媒体库：指定了就用指定的（要启用、类型相同）；追剧推送或认领的种子用追剧指定的库（同样当作指定的）；
// 否则在同类型里选，动画优先进只收动画的库。
func (s *Service) pickLibrary(ctx context.Context, p *Plan, req Request) bool {
	var libs []models.MediaLibrary
	if err := s.cfg.DB.WithContext(ctx).Where("enabled = ?", true).Order("id").Find(&libs).Error; err != nil {
		p.Problem, p.retryable = "读取媒体库失败："+err.Error(), true
		return false
	}
	kind := p.Match.MediaType
	wanted, follow := req.LibraryID, false
	if wanted == 0 && p.WatchDirID == 0 {
		if f := s.followLibrary(); f != nil {
			id, found, err := f(ctx, p.DownloaderID, p.Hash)
			if err != nil {
				// 查不到时不自动选库：选错了库，追剧要的库里就缺了这一集
				p.Problem, p.retryable = "查不到追剧指定的媒体库，稍后重试："+err.Error(), true
				return false
			}
			if found && id > 0 {
				wanted, follow = id, true
			}
		}
	}
	var lib *models.MediaLibrary
	if wanted > 0 {
		for i := range libs {
			if libs[i].ID == wanted && libs[i].Kind == kind {
				lib = &libs[i]
			}
		}
		if lib == nil {
			p.Problem = "指定的媒体库不存在、没有启用，或类型与条目不同"
			if follow {
				p.Problem = "追剧指定的媒体库不存在、没有启用，或类型与条目不同：在追剧里改选媒体库"
			}
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
	cat, ok := s.classify(ctx, p, lib)
	if !ok {
		return false
	}
	if dir := transfer.CleanName(cat); dir != "" {
		p.Category, p.root = cat, filepath.Join(lib.Path, dir)
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

// showDir 是剧集目录：模板算出的路径（从 root 算起）至少有两级时取第一级；平铺时为空。
func showDir(root, target string) string {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return ""
	}
	segs := strings.Split(filepath.ToSlash(rel), "/")
	if len(segs) < 2 || segs[0] == ".." {
		return ""
	}
	return filepath.Join(root, segs[0])
}
