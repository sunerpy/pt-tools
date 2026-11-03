package organize

import (
	"context"
	"crypto/sha1" //nolint:gosec // 只用来给目录监控的一项与指定的路径取个短标识，不涉及安全
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
	// SourcePath 是手动指定的路径：pt-tools 里看到的文件或文件夹；给了它就不看下载器与监控目录
	SourcePath string `json:"source_path,omitempty"`
	MediaType  string `json:"media_type,omitempty"`
	TMDBID     int    `json:"tmdb_id,omitempty"`
	LibraryID  uint   `json:"library_id,omitempty"`
	// Mode 是这次用的整理方式（hardlink、copy、symlink、move），手动整理时可以选；空串跟媒体库
	Mode string `json:"mode,omitempty"`
}

// queueHash 是排队去重、退避与重试分组用的种子标识：目录监控的一项用 watchHash，指定的路径用 pathHash。
func (r Request) queueHash() string {
	if r.SourcePath != "" {
		return pathHash(r.SourcePath)
	}
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

// pathHash 是手动指定的路径（规整过的）的标识：记在整理记录的 InfoHash 里（不是种子的 info hash），重试、去重按它分组。
func pathHash(p string) string {
	sum := sha1.Sum([]byte(p))
	return fmt.Sprintf("%s%x", pathHashPrefix, sum[:8])
}

// normalized 检查并规整请求：指定条目时类型与编号要对得上；整理方式要认得；来源（下载器里的种子、监控目录里的一项、一个路径）
// 只能有一种；源路径按 cleanSource 检查，写回规整后的路径。
func (r Request) normalized() (Request, error) {
	if r.TMDBID < 0 || (r.TMDBID > 0 && r.MediaType != tmdb.KindMovie && r.MediaType != tmdb.KindTV) {
		return r, fmt.Errorf("%w: 指定条目时要写明类型（movie 或 tv）与 TMDB 编号", ErrInvalid)
	}
	if r.Mode = strings.TrimSpace(r.Mode); r.Mode != "" {
		if _, err := transfer.NormalizeMode(r.Mode); err != nil {
			return r, fmt.Errorf("%w: 整理方式只能是 hardlink、copy、symlink 或 move（空着跟媒体库）: %w", ErrInvalid, err)
		}
	}
	r.SourcePath = strings.TrimSpace(r.SourcePath)
	sources := 0
	if r.DownloaderID != 0 || strings.TrimSpace(r.Hash) != "" {
		sources++
	}
	if r.WatchDirID != 0 || strings.TrimSpace(r.Entry) != "" {
		sources++
	}
	if r.SourcePath != "" {
		sources++
	}
	if sources > 1 {
		return r, fmt.Errorf("%w: 整理的来源只能选一种：下载器里的种子、监控目录里的一项，或一个路径", ErrInvalid)
	}
	if r.SourcePath != "" {
		p, err := cleanSource(r.SourcePath)
		if err != nil {
			return r, err
		}
		r.SourcePath = p
	}
	return r, nil
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
	// Disc 不为空时这一项是一张原盘：Source 是原盘根目录，Target 是电影目录
	Disc *DiscInfo `json:"disc,omitempty"`

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
	// disc 是这一项的原盘（Disc 不为空时）
	disc *transfer.Disc
	// failKind 是这一项整理不了的分类（models.MediaFailExists 等）：写进整理记录
	failKind string
}

// fail 把这一项标成整理不了，记下原因与分类。
func (it *PlanItem) fail(kind, msg string) {
	it.Status, it.Message, it.failKind = ItemFailed, msg, kind
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
	WatchDirID uint `json:"watch_dir_id,omitempty"`
	// SourcePath 是手动指定的路径（规整过的；这次整理的是一个路径、不是下载器里的种子或监控目录里的一项时）
	SourcePath string               `json:"source_path,omitempty"`
	Match      *tmdb.Result         `json:"match,omitempty"`
	Source     string               `json:"source,omitempty"`
	Library    *models.MediaLibrary `json:"library,omitempty"`
	// Category 是放进的二级分类（媒体库开了「按二级分类放进子目录」、规则对得上时才有）
	Category string `json:"category,omitempty"`
	// Classify 说明二级分类怎么定的（媒体库开了「按二级分类放进子目录」、这一类有规则时才有）
	Classify *ClassifyNote   `json:"classify,omitempty"`
	Mode     string          `json:"mode,omitempty"`
	Items    []PlanItem      `json:"items"`
	Skipped  []transfer.Skip `json:"skipped,omitempty"`
	Problem  string          `json:"problem,omitempty"`
	// FailKind 是整理不了的分类（models.MediaFailRecognize 等，有 Problem 时才有）：写进整理记录，页面按它给出处理办法
	FailKind string `json:"fail_kind,omitempty"`
	// Notes 是不挡整理的说明（例如指定的路径是下载器里的种子、改成了按种子整理）
	Notes []string `json:"notes,omitempty"`

	// retryable 表示过一会儿再试可能就好了（TMDB 暂时不能访问、文件操作失败）
	retryable bool
	// nameHit 是和指定的路径同名的下载器里的种子（pt-tools 看不到那个种子的文件，按名字认的）；
	// torrentErr 是认不了的原因（读不了某个下载器）。移动整理时看它们，见 finishPlan
	nameHit    *torrentHit
	torrentErr error
	// noRecord 表示不写整理记录（种子没下载完）
	noRecord   bool
	contentSrc string
	meta       meta.Meta
	siteName   string
	// doubanID 是站点给的豆瓣编号（pt-tools 推送的种子才有），刮削补充豆瓣的内容时电影直接用它
	doubanID string
	// siteTitle 是站点上的标题（pt-tools 推送的种子才有）：二级分类的关键字也看它
	siteTitle string
	// recognizeIn 是按名字识别时用的输入（走到这一步才有）：「让 AI 认」用它搜候选
	recognizeIn recognize.Input
	tmdbClient  *tmdb.Client
	// root 是这次整理放进的目录：库目录，按二级分类放进子目录时是库目录下的分类目录
	root string
	// chosenMode 是请求里选的整理方式（空串是跟媒体库）：写进整理记录，重试按它整理
	chosenMode string
	// follow 是订阅推送或认领的种子的订阅的条目与媒体库（不是订阅的种子、没有查时为空）
	follow *MediaHint
	// matchNote 是条目来源上要写进整理记录的说明（订阅或推送时说明的条目在 TMDB 上找不到了、改成自动识别）
	matchNote string
	// skip 表示 Problem 不是出错（例如剧集原盘）：记成跳过，不重试
	skip bool
	// isoSkips 是剧集里不整理的 ISO 镜像：每个写一条跳过的记录
	isoSkips []PlanItem
}

// otherNote 是同一个位置上换了文件时写在整理记录消息前面的说明（见 markOtherFiles）。
func (p *Plan) otherNote() string {
	if p.SourcePath != "" {
		return pathOtherFileNote
	}
	return otherFileNote
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

// 条目的来源，按这个顺序取：手动指定的 TMDB 条目；沿用之前整理这些文件时用的条目；订阅的条目；推送时说明的条目。
// 都没有时自动识别，来源是媒体识别给的（纠正、IMDb 编号、搜索等）。
const (
	sourceManual       = "manual"
	sourceHistory      = "history"
	sourceSubscription = "subscription"
	sourcePush         = "push"
)

// validKind 报告类型是不是电影或剧集。
func validKind(kind string) bool { return kind == tmdb.KindMovie || kind == tmdb.KindTV }

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
	if main, ok := discMain(row); ok {
		return discOurs(row, main)
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

// followHint 问订阅：这个种子是订阅推送或认领的时，回订阅的条目与媒体库；不是时回 nil。没有接上订阅服务时也是 nil。
func (s *Service) followHint(ctx context.Context, p *Plan) (*MediaHint, error) {
	f := s.followMedia()
	if f == nil {
		return nil, nil
	}
	h, found, err := f(ctx, p.DownloaderID, p.Hash)
	if err != nil || !found {
		return nil, err
	}
	return &h, nil
}

// torrentRecord 是 pt-tools 推送这个种子时记下的记录（站点、标题、外部编号、推送时说明的条目）：先查种子表，
// 没有时查归档表（完成两周后记录挪进归档表，种子照样在下载器里做种）。都没有时是空的。
func (s *Service) torrentRecord(ctx context.Context, hash string) models.TorrentInfo {
	var info models.TorrentInfo
	_ = s.cfg.DB.WithContext(ctx).Where("LOWER(torrent_hash) = ?", hash).Order("id DESC").Limit(1).Find(&info).Error
	if info.ID != 0 {
		return info
	}
	var a models.TorrentInfoArchive
	_ = s.cfg.DB.WithContext(ctx).Where("LOWER(torrent_hash) = ?", hash).Order("id DESC").Limit(1).Find(&a).Error
	return models.TorrentInfo{
		SiteName: a.SiteName, Title: a.Title, IMDbID: a.IMDbID, DoubanID: a.DoubanID,
		MediaType: a.MediaType, TMDBID: a.TMDBID, MediaSeason: a.MediaSeason,
	}
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

// buildPlan 检查请求，按来源分给指定的路径、目录监控的一项或下载器里的种子。
func (s *Service) buildPlan(ctx context.Context, req Request) (*Plan, error) {
	req, err := req.normalized()
	if err != nil {
		return nil, err
	}
	var p *Plan
	switch {
	case req.SourcePath != "":
		p, err = s.buildPathPlan(ctx, req)
	case req.WatchDirID > 0:
		p, err = s.buildWatchPlan(ctx, req)
	default:
		p, err = s.buildTorrentPlan(ctx, req)
	}
	if p != nil {
		// 还没选到媒体库就整理不了的（例如识别失败）也记下：重试时还按选的方式
		p.chosenMode = req.Mode
	}
	return p, err
}

// hardlinkHint 是移动整理不了时请用户改用硬链接的说法：这次选了整理方式时改选，跟媒体库时改媒体库。
func hardlinkHint(req Request) string {
	if req.Mode != "" {
		return "整理方式改选硬链接"
	}
	return "把媒体库改用硬链接"
}

// buildTorrentPlan 是下载器里一个种子的整理计划：读种子与文件列表，按路径映射换成 pt-tools 里的路径。
func (s *Service) buildTorrentPlan(ctx context.Context, req Request) (*Plan, error) {
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
		p.FailKind = models.MediaFailOther
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

// finishPlan 是下载器的种子、目录监控的一项与指定的路径共用的部分：选出视频、读整理记录、识别、选媒体库、算目标。
func (s *Service) finishPlan(ctx context.Context, p *Plan, req Request, t downloader.Torrent, tfs []transfer.File, settings Settings) (*Plan, error) {
	sel := transfer.Select(tfs, int64(settings.MinVideoMB)<<20)
	p.Skipped = append(p.Skipped, sel.Skipped...)
	switch {
	case len(sel.Discs) > 0:
		if p.Problem = discProblem(sel.Discs); p.Problem != "" {
			p.FailKind = models.MediaFailDisc
			return p, nil
		}
		p.Items = append(p.Items, discItem(sel.Discs[0], p))
	case len(sel.Videos) == 0:
		p.FailKind = models.MediaFailOther
		p.Problem = "没有可整理的视频文件"
		return p, nil
	}
	for _, v := range sel.Videos {
		it := PlanItem{Rel: v.Rel, Source: v.Path, Size: v.Size, Status: ItemPending}
		if p.WatchDirID > 0 || p.SourcePath != "" {
			// 目录监控与指定的路径按源文件的编号认同一个位置上是不是换了文件（见 markOtherFiles）；移动整理以后就读不到了，先读
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
			p.FailKind = models.MediaFailPath
			p.Problem = fmt.Sprintf("pt-tools 里找不到 %s：下载器与 pt-tools 看到的路径不同时（例如在 Docker 里），在「路径映射」里添加对应关系", it.Source)
			return p, nil
		}
		break
	}
	if !s.recognize(ctx, p, req, t) {
		return p, nil
	}
	if p.Match.MediaType == tmdb.KindTV {
		if p.Items[0].disc != nil {
			p.FailKind = models.MediaFailDisc
			p.Problem, p.skip = "剧集原盘暂不整理", true
			return p, nil
		}
		if p.splitTVISO(); len(p.Items) == 0 {
			p.FailKind = models.MediaFailDisc
			p.Problem, p.skip = "没有可整理的视频文件（"+isoTVNote+"）", true
			return p, nil
		}
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
			p.FailKind = models.MediaFailOther
			p.Problem, p.retryable = "移动整理以前要确认「"+p.Name+"」不是下载器里的种子："+err.Error(), true
			return p, nil
		}
		if owner != "" {
			p.FailKind = models.MediaFailOther
			p.Problem = "「" + p.Name + "」是下载器里的种子（" + owner + "），目录监控不移动下载器里的种子：" + hardlinkHint(req) + "，或不要监控下载目录"
			return p, nil
		}
	case p.SourcePath != "":
		// 指定的路径认种子时只剩按名字认出的或确认不了的（见 buildPathPlan）：可能就是下载器里的种子，移走了没法做种
		if p.torrentErr != nil {
			p.FailKind = models.MediaFailOther
			p.Problem, p.retryable = "移动整理以前要确认「"+p.Name+"」不是下载器里的种子："+p.torrentErr.Error(), true
			return p, nil
		}
		if p.nameHit != nil {
			p.FailKind = models.MediaFailOther
			p.Problem = fmt.Sprintf("「%s」和下载器里的种子同名（%s：%s），可能就是它：移动整理会让它没法做种。%s，或配好路径映射后在「下载器 Web UI」里按种子整理",
				p.Name, p.nameHit.DownloaderName, p.nameHit.Name, hardlinkHint(req))
			return p, nil
		}
	case t.State != downloader.TorrentPaused && t.State != downloader.TorrentStopped:
		p.FailKind = models.MediaFailOther
		p.Problem = "种子还在做种，不能用移动整理：暂停种子后再整理，或" + hardlinkHint(req)
		return p, nil
	}
	if p.SourcePath != "" && p.Mode != models.MediaModeMove {
		// 不移动时照常整理，写明可能是下载器里的种子
		if p.torrentErr != nil {
			p.Notes = append(p.Notes, fmt.Sprintf("没能确认「%s」是不是下载器里的种子（%v）：是的话建议在「下载器 Web UI」里按种子整理", p.Name, p.torrentErr))
		}
		if p.nameHit != nil {
			p.Notes = append(p.Notes, fmt.Sprintf("「%s」和下载器「%s」里的种子「%s」同名（pt-tools 里看不到那个种子的文件，按名字认的）：如果是同一份数据，建议配好路径映射后按种子整理",
				p.Name, p.nameHit.DownloaderName, p.nameHit.Name))
		}
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

// recognize 定种子对应的条目：手动指定的、之前整理用过的、订阅的、推送时说明的条目依次直接取详情；
// 都没有时用种子名（站点标题作副标题）与 IMDb 编号识别。
func (s *Service) recognize(ctx context.Context, p *Plan, req Request, t downloader.Torrent) bool {
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if errors.Is(err, tmdb.ErrNoKey) {
		p.FailKind = models.MediaFailRecognize
		p.Problem = "没有填写 TMDB API Key：先在「媒体识别」里填写"
		return false
	}
	if err == nil {
		p.tmdbClient = c
	}
	var info models.TorrentInfo
	if p.WatchDirID == 0 && p.SourcePath == "" {
		// 目录监控的一项与指定的路径不是种子：没有站点给的标题、编号与推送时说明的条目
		info = s.torrentRecord(ctx, p.Hash)
		// 条目或媒体库没有指定时问订阅：查不到时不自动识别、不自动选库，识别成别的条目或放错了库，订阅那边就缺了这几集
		if req.TMDBID == 0 || req.LibraryID == 0 {
			hint, ferr := s.followHint(ctx, p)
			if ferr != nil {
				p.FailKind = models.MediaFailRecognize
				p.Problem, p.retryable = "查不到追剧的条目与媒体库，稍后重试："+ferr.Error(), true
				return false
			}
			p.follow = hint
		}
	}
	p.siteName, p.doubanID, p.siteTitle = info.SiteName, info.DoubanID, info.Title
	source := sourceManual
	if req.TMDBID == 0 {
		// 之前整理过（比如手动指定过条目）：沿用那个条目，库里的文件名与重新整理时一致
		if kind, id := priorEntry(p); id > 0 {
			req.MediaType, req.TMDBID, source = kind, id, sourceHistory
		}
	}
	if f := p.follow; req.TMDBID == 0 && f != nil && validKind(f.MediaType) && f.TMDBID > 0 {
		req.MediaType, req.TMDBID, source = f.MediaType, f.TMDBID, sourceSubscription
	}
	if req.TMDBID == 0 && validKind(info.MediaType) && info.TMDBID > 0 {
		req.MediaType, req.TMDBID, source = info.MediaType, info.TMDBID, sourcePush
	}
	if req.TMDBID > 0 {
		if err != nil {
			p.FailKind = models.MediaFailRecognize
			p.Problem = "识别失败：" + err.Error()
			return false
		}
		d, detailErr := c.Details(ctx, req.MediaType, req.TMDBID)
		switch {
		case detailErr == nil:
			p.meta, _ = s.cfg.Recognizer.Parse(ctx, t.Name, "")
			p.Match, p.Source = d, source
			return true
		case errors.Is(detailErr, tmdb.ErrNotFound) && (source == sourceSubscription || source == sourcePush):
			// 订阅或推送时说明的条目在 TMDB 上不在了（被合并、删除）：改成自动识别，写明
			what := "订阅的条目"
			if source == sourcePush {
				what = "推送时说明的条目"
			}
			p.matchNote = fmt.Sprintf("%s（TMDB %s %d）在 TMDB 上找不到了，改成自动识别", what, req.MediaType, req.TMDBID)
			p.Notes = append(p.Notes, p.matchNote)
		default:
			p.FailKind = models.MediaFailRecognize
			p.Problem, p.retryable = "识别失败："+detailErr.Error(), !errors.Is(detailErr, tmdb.ErrNotFound)
			return false
		}
	}
	in := recognize.Input{Title: t.Name, IMDbID: info.IMDbID}
	if info.Title != "" && info.Title != t.Name {
		in.Subtitle = info.Title
	}
	p.recognizeIn = in
	res, err := s.cfg.Recognizer.Recognize(ctx, in)
	if err != nil {
		p.FailKind = models.MediaFailRecognize
		p.Problem = "识别失败：" + err.Error()
		return false
	}
	p.meta = res.Meta
	switch {
	case res.Error != "":
		p.FailKind = models.MediaFailRecognize
		p.Problem, p.retryable = "识别失败："+res.Error, true
		return false
	case res.Match == nil:
		p.FailKind = models.MediaFailRecognize
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

// pickLibrary 选媒体库：指定了就用指定的（要启用、类型相同）；订阅推送或认领的种子用订阅指定的库（同样当作指定的）；
// 否则在同类型里选，动画优先进只收动画的库。
func (s *Service) pickLibrary(ctx context.Context, p *Plan, req Request) bool {
	var libs []models.MediaLibrary
	if err := s.cfg.DB.WithContext(ctx).Where("enabled = ?", true).Order("id").Find(&libs).Error; err != nil {
		p.FailKind = models.MediaFailLibrary
		p.Problem, p.retryable = "读取媒体库失败："+err.Error(), true
		return false
	}
	kind := p.Match.MediaType
	wanted, follow := req.LibraryID, false
	if wanted == 0 && p.follow != nil && p.follow.LibraryID > 0 {
		wanted, follow = p.follow.LibraryID, true
	}
	var lib *models.MediaLibrary
	if wanted > 0 {
		for i := range libs {
			if libs[i].ID == wanted && libs[i].Kind == kind {
				lib = &libs[i]
			}
		}
		if lib == nil {
			p.FailKind = models.MediaFailLibrary
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
		p.FailKind = models.MediaFailLibrary
		p.Problem = fmt.Sprintf("没有启用的%s媒体库：先在「媒体库」里添加", what)
		return false
	}
	// 手动整理时选了整理方式就用选的，否则用库的
	p.Library, p.Mode = lib, firstNonEmpty(req.Mode, lib.Mode)
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
