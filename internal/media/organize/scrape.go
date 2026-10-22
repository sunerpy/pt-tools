package organize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/cachectl"
	"github.com/sunerpy/pt-tools/internal/media/fanart"
	"github.com/sunerpy/pt-tools/internal/media/metadata"
	"github.com/sunerpy/pt-tools/internal/media/omdb"
	"github.com/sunerpy/pt-tools/internal/media/scrape"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

// scrapeTarget 是要刮削的一个视频：整理记录、库里的位置、算剧集目录与电影有没有自己的目录的起点（见 Plan.rootFor），季集。
type scrapeTarget struct {
	historyID                   uint
	path, root                  string
	season, episode, episodeEnd int
}

func (t scrapeTarget) showDir() string { return showDir(t.root, t.path) }

// scrapeFiles 是这些视频要写的刮削文件：电影每部一份（几个版本在同一个目录里时目录里的图片只写一次）；
// 剧集每个剧集目录写一次 tvshow.nfo 与图片，每一季写一次 season.nfo 与季海报（季目录用这一季第一个视频的目录），每集一份。
func scrapeFiles(kind string, items scrape.Items, targets []scrapeTarget) []scrape.File {
	var files []scrape.File
	if kind != tmdb.KindTV {
		for _, t := range targets {
			// 平铺在库目录（或分类目录）里的电影没有自己的目录：海报等带上文件名，免得几部电影共用一张
			ownDir := filepath.Clean(filepath.Dir(t.path)) != filepath.Clean(t.root)
			files = append(files, scrape.MovieFiles(t.path, ownDir, items)...)
		}
		return uniqueFiles(files)
	}
	shows, seasons := map[string]bool{}, map[string]bool{}
	for _, t := range targets {
		sd := t.showDir()
		if sd != "" && !shows[sd] {
			shows[sd] = true
			files = append(files, scrape.ShowFiles(sd, items)...)
		}
		key := fmt.Sprintf("%s|%d", sd, t.season)
		if sd != "" && !seasons[key] {
			seasons[key] = true
			files = append(files, scrape.SeasonFiles(sd, filepath.Dir(t.path), t.season, items)...)
		}
		files = append(files, scrape.EpisodeFiles(t.path, t.season, t.episode, t.episodeEnd, items)...)
	}
	return uniqueFiles(files)
}

func uniqueFiles(in []scrape.File) []scrape.File {
	seen := map[string]bool{}
	out := in[:0]
	for _, f := range in {
		if !seen[f.Path] {
			seen[f.Path] = true
			out = append(out, f)
		}
	}
	return out
}

// resolver 是这次刮削能用的来源：TMDB（c 为空时没有）、豆瓣、Bangumi，以及在「媒体识别」里填了 Key 的 Fanart.tv 与 OMDb。
// 填了 Key 却建不了客户端（例如解不开）时返回说明。
func (s *Service) resolver(ctx context.Context, c *tmdb.Client) (*metadata.Resolver, []string) {
	var ps []metadata.Provider
	var msgs []string
	if c != nil {
		ps = append(ps, metadata.NewTMDB(c))
	}
	if s.cfg.Douban != nil {
		ps = append(ps, metadata.NewDouban(s.cfg.Douban))
	}
	if s.cfg.Bangumi != nil {
		ps = append(ps, metadata.NewBangumi(s.cfg.Bangumi))
	}
	if fc, err := s.cfg.Recognizer.Fanart(ctx); err == nil {
		ps = append(ps, metadata.NewFanart(fc))
	} else if !errors.Is(err, fanart.ErrNoKey) {
		msgs = append(msgs, "Fanart.tv 不能用（"+err.Error()+"），图片用 TMDB 的")
	}
	if oc, err := s.cfg.Recognizer.OMDb(ctx); err == nil {
		ps = append(ps, metadata.NewOMDb(oc))
	} else if !errors.Is(err, omdb.ErrNoKey) {
		msgs = append(msgs, "OMDb 不能用（"+err.Error()+"），没有写 IMDb 等评分")
	}
	return metadata.NewResolver(ps...), msgs
}

// runScrape 按媒体库的刮削设置为这些视频写 NFO 与图片，把结果记进整理记录。match 是条目的 TMDB 详情，doubanID 是站点给的
// 豆瓣编号；replace 报告已经有的文件能不能换掉（重新刮削时，见 scrape.Writer.Replace）。
func (s *Service) runScrape(ctx context.Context, lib *models.MediaLibrary, match tmdb.Result, doubanID string, c *tmdb.Client,
	targets []scrapeTarget, replace func(string) bool,
) scrape.Report {
	files := scrapeFiles(match.MediaType, scrape.ParseItems(lib.ScrapeItems), targets)
	res, msgs := s.resolver(ctx, c)
	w := scrape.Writer{
		Resolver: res, Sources: metadata.ParseSources(lib.ScrapeSources, lib.ScrapeSource), Overwrite: lib.ScrapeOverwrite,
		Replace: replace, ImageLang: lib.ScrapeImageLang,
	}
	if c != nil {
		w.RecogLang = c.Language()
	}
	rep := w.Write(ctx, metadata.QueryFor(match, doubanID), metadata.FromTMDB(match), files)
	if rep.Fetched {
		// Fanart.tv、OMDb 建不了客户端：这次取了数据时才算来源出错
		rep.Errors = append(msgs, rep.Errors...)
	}
	for _, e := range rep.Errors {
		s.cfg.Logger.Warnf("[整理入库] 刮削「%s」：%s", match.Title, e)
	}
	for _, n := range rep.Notes {
		s.cfg.Logger.Infof("[整理入库] 刮削「%s」：%s", match.Title, n)
	}
	s.recordScraped(ctx, targets, rep)
	return rep
}

// scrape 为这次新放进库的文件写 NFO 与图片（媒体库关了刮削时不写；写哪些按媒体库选的刮削内容），返回出的问题。
func (s *Service) scrape(ctx context.Context, p *Plan, created []*PlanItem) []string {
	if !p.Library.Scrape {
		return nil
	}
	targets := make([]scrapeTarget, 0, len(created))
	for _, it := range created {
		targets = append(targets, scrapeTarget{
			historyID: it.HistoryID, path: it.Target, root: p.rootFor(it.Target), season: it.Season, episode: it.Episode, episodeEnd: it.EpisodeEnd,
		})
	}
	rep := s.runScrape(ctx, p.Library, *p.Match, p.doubanID, p.tmdbClient, targets, nil)
	return prefixed("刮削：", rep.Problems())
}

func prefixed(prefix string, in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, prefix+s)
	}
	return out
}

// recordScraped 把刮削写出的文件与文件编号记进对应视频的整理记录：和视频同名的记给这个视频，目录里共用的（海报、背景、
// tvshow.nfo、季海报与 season.nfo 等）记给这个目录里每个这次刮削的视频，删除时只删这些。再记下每条记录的刮削结果：
// 来源出错、或它的文件有没写成的是部分成功，它的文件一个也没成的是失败；说明（用了 TMDB 代替的一类）写进 ScrapeNote。
func (s *Service) recordScraped(ctx context.Context, targets []scrapeTarget, rep scrape.Report) {
	ids := map[string]string{}
	sums := map[string]string{}
	for _, f := range rep.Files {
		if f.Status == scrape.FileWritten {
			if id, err := transfer.FileID(f.Path); err == nil {
				ids[f.Path], sums[f.Path] = id, f.SHA256
			}
		}
	}
	now := s.cfg.Now()
	for _, t := range targets {
		if t.historyID == 0 {
			continue
		}
		stem := strings.TrimSuffix(t.path, filepath.Ext(t.path))
		dirs := []string{filepath.Dir(t.path)}
		if sd := t.showDir(); sd != "" {
			dirs = append(dirs, sd)
		}
		var add []extra
		problems := append(append([]string(nil), rep.Errors...), rep.Notes...)
		failed, fine := 0, 0
		var missing []string
		for _, f := range rep.Files {
			own := strings.HasPrefix(f.Path, stem+".") || strings.HasPrefix(f.Path, stem+"-")
			shared := scrape.IsDirArtifact(filepath.Base(f.Path)) && slices.Contains(dirs, filepath.Dir(f.Path))
			if !own && !shared {
				continue
			}
			switch f.Status {
			case scrape.FileFailed:
				failed++
				problems = append(problems, fmt.Sprintf("%s: %s", filepath.Base(f.Path), f.Err))
			case scrape.FileMissing:
				// 来源里没有这张图：没写成，算部分成功；没填 Fanart.tv 的 Key 的在 Notes 里写过了
				failed++
				if l := scrape.ArtLabel(f.Art); f.Err != scrape.NoFanartKey && !slices.Contains(missing, l) {
					missing = append(missing, l)
				}
			default:
				fine++
			}
			if id, ok := ids[f.Path]; ok {
				add = append(add, extra{Target: f.Path, FileID: id, Kind: extraMeta, SHA256: sums[f.Path]})
			}
		}
		if len(missing) > 0 {
			problems = append(problems, scrape.Join("来源里没有", strings.Join(missing, "、")))
		}
		status := models.MediaScrapeOK
		switch {
		case failed == 0 && len(rep.Errors) == 0:
		case fine == 0:
			status = models.MediaScrapeFailed
		default:
			status = models.MediaScrapePartial
		}
		cols := map[string]any{"scrape_status": status, "scrape_note": truncate(strings.Join(problems, "；"), 1024), "scraped_at": now}
		if len(add) > 0 {
			var row models.MediaTransferHistory
			if err := s.cfg.DB.WithContext(ctx).Where("id = ?", t.historyID).Limit(1).Find(&row).Error; err != nil || row.ID == 0 {
				continue
			}
			b, _ := json.Marshal(mergeExtras(decodeExtras(row.Extras), add))
			cols["extras"] = string(b)
		}
		// 不动 updated_at：重新刮削不把记录挪到整理历史的最前面
		if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).Where("id = ?", t.historyID).UpdateColumns(cols).Error; err != nil {
			s.cfg.Logger.Warnf("[整理入库] 记下刮削的结果失败 (%s): %v", t.path, err)
		}
	}
}

// RescrapeResult 是一次重新刮削的结果。
type RescrapeResult struct {
	// Records 是刮削了的整理记录数（这个媒体库里同一部电影或剧集整理好的）
	Records int `json:"records"`
	// Written 是写了的文件数（新写的或内容变了的；内容一样的不重写）
	Written  int      `json:"written"`
	Messages []string `json:"messages,omitempty"`
	// Sources 是 NFO 里文字的每一项用了哪个来源
	Sources string `json:"sources,omitempty"`
	// Queued 表示还在后台刮削（等不到结果就先回了），稍后到整理历史里看
	Queued bool `json:"queued,omitempty"`
}

// Rescrape 重新刮削一条整理记录所在的电影或剧集：这个媒体库里同一部（TMDB 编号相同）整理好、库里的文件还在的记录，
// 按媒体库现在的刮削设置写 NFO 与图片。pt-tools 写的、之后没被换过的文件照样换掉；别的（自己放的、别的工具写的）
// 只在开着覆盖时换；媒体服务器里锁定了的 NFO 不动。和整理一样排进队列（同时只做一件），等最多 20 秒。
func (s *Service) Rescrape(ctx context.Context, id uint) (*RescrapeResult, error) {
	if _, _, err := s.rescrapeBase(ctx, id); err != nil {
		return nil, err
	}
	if !s.isRunning() {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.rescrape(ctx, id)
	}
	done := make(chan jobResult, 1)
	if s.put(job{rescrape: id, done: done}) == queueFull {
		return nil, errors.New("整理队列已满，稍后再试")
	}
	timer := time.NewTimer(waitManual)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.scrape, r.err
	case <-timer.C:
		return &RescrapeResult{Queued: true}, nil
	case <-ctx.Done():
		return &RescrapeResult{Queued: true}, nil
	}
}

// rescrapeBase 读出要重新刮削的记录与它所在的媒体库，检查能不能重新刮削：记录整理好了、媒体库还在且开着刮削。
func (s *Service) rescrapeBase(ctx context.Context, id uint) (models.MediaTransferHistory, models.MediaLibrary, error) {
	var lib models.MediaLibrary
	row, err := s.historyRow(ctx, id)
	if err != nil {
		return row, lib, err
	}
	if row.Status != models.MediaTransferDone || row.TargetPath == "" || row.TMDBID == 0 {
		return row, lib, fmt.Errorf("%w: 只有整理好的记录能重新刮削", ErrInvalid)
	}
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", row.LibraryID).Limit(1).Find(&lib).Error; err != nil {
		return row, lib, fmt.Errorf("读取媒体库失败: %w", err)
	}
	if lib.ID == 0 {
		return row, lib, fmt.Errorf("%w: 这条记录的媒体库已经删了", ErrInvalid)
	}
	if !lib.Scrape {
		return row, lib, fmt.Errorf("%w: 媒体库「%s」关着刮削：先在媒体库里打开", ErrInvalid, lib.Name)
	}
	return row, lib, nil
}

// skippedRow 是重新刮削时没有刮削的一条记录与原因。
type skippedRow struct {
	row    models.MediaTransferHistory
	reason string
}

// rescrapeRows 是这个媒体库里同一部整理好的记录：库里的视频还在、还是当初整理出的那个（见 ours）的要刮削，
// 其余的连同原因放进 skipped（不给换掉了的视频写旧条目的 NFO 与图片）。
func (s *Service) rescrapeRows(ctx context.Context, row models.MediaTransferHistory, lib models.MediaLibrary) ([]models.MediaTransferHistory, []skippedRow, error) {
	var all []models.MediaTransferHistory
	if err := s.cfg.DB.WithContext(ctx).
		Where("library_id = ? AND media_type = ? AND tmdb_id = ? AND status = ?", lib.ID, row.MediaType, row.TMDBID, models.MediaTransferDone).
		Order("id").Find(&all).Error; err != nil {
		return nil, nil, fmt.Errorf("读取整理记录失败: %w", err)
	}
	var rows []models.MediaTransferHistory
	var skipped []skippedRow
	for _, r := range all {
		switch {
		case r.TargetPath == "":
			continue
		case !transfer.Within(lib.Path, r.TargetPath):
			skipped = append(skipped, skippedRow{r, "不在媒体库的目录里，没有刮削"})
		case !present(r.TargetPath):
			skipped = append(skipped, skippedRow{r, "库里的视频已经不在了，没有刮削"})
		case !ours(&r):
			skipped = append(skipped, skippedRow{r, "库里的视频换成了别的文件（不是当初整理出的那个），没有刮削"})
		default:
			rows = append(rows, r)
		}
	}
	return rows, skipped, nil
}

// present 报告路径上有没有东西（软链接看它自己）。
func present(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// markScrape 记下这些记录这次刮削的结果（不动 updated_at）：没能刮削的写明原因。
func (s *Service) markScrape(ctx context.Context, rows []models.MediaTransferHistory, status, note string) {
	if len(rows) == 0 {
		return
	}
	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	cols := map[string]any{"scrape_status": status, "scrape_note": truncate(note, 1024), "scraped_at": s.cfg.Now()}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).Where("id IN ?", ids).UpdateColumns(cols).Error; err != nil {
		s.cfg.Logger.Warnf("[整理入库] 记下刮削的结果失败: %v", err)
	}
}

// rescrape 重新刮削（调用方持有 s.mu）。各来源都不读缓存，要现在的数据（取到的照样写进缓存）。没能刮削的记录
// （视频不在了、换成了别的文件、认不出分类目录、TMDB 不能用）记成刮削失败并写明原因。
func (s *Service) rescrape(ctx context.Context, id uint) (*RescrapeResult, error) {
	row, lib, err := s.rescrapeBase(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, skipped, err := s.rescrapeRows(ctx, row, lib)
	if err != nil {
		return nil, err
	}
	ctx = cachectl.Bypass(ctx)
	out := &RescrapeResult{}
	// 先认出每条记录在哪：没能刮削的在问 TMDB 之前就记下原因
	tpl := effectiveTemplate(lib)
	var targets []scrapeTarget
	var scraping []models.MediaTransferHistory
	for _, r := range rows {
		root := scrapeRoot(r, lib, tpl)
		if root == "" {
			skipped = append(skipped, skippedRow{r, "升级前整理的、媒体库按二级分类放进了子目录，认不出它在哪个分类目录里，没有刮削"})
			continue
		}
		targets = append(targets, scrapeTarget{historyID: r.ID, path: r.TargetPath, root: root, season: r.Season, episode: r.Episode, episodeEnd: r.EpisodeEnd})
		scraping = append(scraping, r)
	}
	for _, k := range skipped {
		s.markScrape(ctx, []models.MediaTransferHistory{k.row}, models.MediaScrapeFailed, k.reason)
		out.Messages = append(out.Messages, filepath.Base(k.row.TargetPath)+"："+k.reason)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrInvalid, strings.Join(out.Messages, "；"))
	}
	var match *tmdb.Result
	c, err := s.cfg.Recognizer.TMDB(ctx)
	switch {
	case errors.Is(err, tmdb.ErrNoKey):
		err = fmt.Errorf("%w: 没有填写 TMDB API Key：先在「媒体识别」里填写", ErrInvalid)
	case err != nil:
		err = fmt.Errorf("TMDB 不能用：%w", err)
	default:
		if match, err = c.Details(ctx, row.MediaType, row.TMDBID); err != nil {
			err = fmt.Errorf("取 TMDB 详情失败：%w", err)
		}
	}
	if err != nil {
		s.markScrape(ctx, scraping, models.MediaScrapeFailed, err.Error())
		return nil, err
	}
	rep := s.runScrape(ctx, &lib, *match, s.siteDoubanID(ctx, row), c, targets, ownedBy(rows))
	out.Records, out.Written, out.Sources = len(targets), len(rep.Written()), rep.Sources
	out.Messages = append(out.Messages, rep.Problems()...)
	out.Messages = append(out.Messages, untouched(rep)...)
	return out, nil
}

// untouched 写明重新刮削时没换的文件：改过的或不是 pt-tools 写的（媒体库没开覆盖）、媒体服务器里锁定了的 NFO。
func untouched(rep scrape.Report) []string {
	var kept, locked []string
	for _, f := range rep.Files {
		switch f.Status {
		case scrape.FileKept:
			kept = append(kept, filepath.Base(f.Path))
		case scrape.FileLocked:
			locked = append(locked, filepath.Base(f.Path))
		}
	}
	names := func(list []string) string {
		if len(list) > 3 {
			return fmt.Sprintf("%s 等 %d 个", strings.Join(list[:3], "、"), len(list))
		}
		return strings.Join(list, "、")
	}
	var out []string
	if len(kept) > 0 {
		out = append(out, "改过的或不是 pt-tools 写的文件没换（媒体库开着覆盖时才换）："+names(kept))
	}
	if len(locked) > 0 {
		out = append(out, "媒体服务器里锁定了的 NFO 没动："+names(locked))
	}
	return out
}

// siteDoubanID 是站点给这条记录的种子的豆瓣编号（pt-tools 推送的种子才有）。
func (s *Service) siteDoubanID(ctx context.Context, row models.MediaTransferHistory) string {
	if row.WatchDirID > 0 || row.InfoHash == "" {
		return ""
	}
	var info models.TorrentInfo
	_ = s.cfg.DB.WithContext(ctx).Where("LOWER(torrent_hash) = ?", strings.ToLower(row.InfoHash)).Order("id DESC").Limit(1).Find(&info).Error
	return info.DoubanID
}

// ownedBy 报告一个已经有的文件是不是这些记录里记着的刮削文件、之后没被改过：内容的摘要和记下的一样；
// 升级前记下的没有摘要，看文件编号（换成了别的文件时对不上，原地改过的认不出来）。
func ownedBy(rows []models.MediaTransferHistory) func(string) bool {
	owned := map[string][]extra{}
	for _, r := range rows {
		for _, e := range decodeExtras(r.Extras) {
			if e.Kind == extraMeta && (e.FileID != "" || e.SHA256 != "") {
				owned[e.Target] = append(owned[e.Target], e)
			}
		}
	}
	return func(p string) bool {
		for _, e := range owned[p] {
			if e.SHA256 != "" {
				if sum, err := scrape.FileSHA256(p); err == nil && sum == e.SHA256 {
					return true
				}
				continue
			}
			if id, err := fileIDOf(p); err == nil && id == e.FileID {
				return true
			}
		}
		return false
	}
}

// actionRe 是模板里的 {{…}}。
var actionRe = regexp.MustCompile(`(?s)\{\{.*?\}\}`)

// templateDepth 是模板算出的路径有几级（不算 {{…}} 里的 /）。
func templateDepth(tpl string) int {
	return strings.Count(actionRe.ReplaceAllString(tpl, ""), "/") + 1
}

// scrapeRoot 是重新刮削时一条记录算剧集目录、电影有没有自己的目录的起点：整理时记下的（RootPath）。升级前的记录没记：
// 媒体库没有按二级分类放进子目录时是库目录；否则先看记下的刮削文件（tvshow.nfo 在剧集目录里，电影目录里的海报说明电影有
// 自己的目录，带文件名的说明平铺），再看剧集目录里有没有 tvshow.nfo，最后按模板的级数认（库目录与它下面第一级目录里，
// 只有一个对得上时用它）。认不出时返回空串，不猜。
func scrapeRoot(row models.MediaTransferHistory, lib models.MediaLibrary, tpl string) string {
	if row.RootPath != "" && transfer.Within(row.RootPath, row.TargetPath) {
		return row.RootPath
	}
	if !lib.ClassifyDir {
		return lib.Path
	}
	dir := filepath.Dir(row.TargetPath)
	stem := strings.TrimSuffix(row.TargetPath, filepath.Ext(row.TargetPath))
	for _, e := range decodeExtras(row.Extras) {
		if e.Kind != extraMeta {
			continue
		}
		name := filepath.Base(e.Target)
		switch {
		case name == "tvshow.nfo" && transfer.Within(filepath.Dir(e.Target), row.TargetPath):
			return filepath.Dir(filepath.Dir(e.Target))
		case row.MediaType != tmdb.KindTV && scrape.IsDirArtifact(name) && filepath.Dir(e.Target) == dir:
			return filepath.Dir(dir)
		case row.MediaType != tmdb.KindTV && strings.HasPrefix(e.Target, stem+"-"):
			return dir
		}
	}
	rel, err := filepath.Rel(lib.Path, row.TargetPath)
	if err != nil {
		return ""
	}
	segs := strings.Split(filepath.ToSlash(rel), "/")
	if len(segs) < 2 {
		return lib.Path
	}
	cands := []string{filepath.Join(lib.Path, segs[0]), lib.Path}
	if row.MediaType == tmdb.KindTV {
		for _, root := range cands {
			if sd := showDir(root, row.TargetPath); sd != "" {
				if _, err := os.Lstat(filepath.Join(sd, "tvshow.nfo")); err == nil {
					return root
				}
			}
		}
	}
	depth := templateDepth(tpl)
	var found []string
	for _, root := range cands {
		if r, err := filepath.Rel(root, row.TargetPath); err == nil && len(strings.Split(filepath.ToSlash(r), "/")) == depth {
			found = append(found, root)
		}
	}
	if len(found) == 1 {
		return found[0]
	}
	return ""
}
