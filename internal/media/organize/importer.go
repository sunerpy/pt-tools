package organize

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

// 导入已有媒体库：扫描媒体库目录里已经有的电影与剧集（不是 pt-tools 放进去的），认出条目以后给每个视频记一条整理记录
// （方式 existing、触发方式 import，源文件与库里的文件是同一个）。导入以后它们算作库里已有：订阅不再下载、条目详情与订阅进度看得到；
// 删除记录时不跟着删文件（要另外确认）、洗版不删、同名替换不换，也不用重试（见 importRow）。
const (
	// importHashPrefix 是导入记录的 InfoHash 的前缀（见 importHash）
	importHashPrefix = "import:"
	// importSource 是导入记录里「下载器」一栏写的名字
	importSource = "导入"
	// importMaxItems 是一次扫描最多列几项（多的分子目录导入）
	importMaxItems = 1000
	// importMaxNest 是往下找放着几项的目录（分类目录、自己分的组）最多几层
	importMaxNest = 4
	// importKeep 是扫描结果在内存里留多久（从最后一次看它算起）
	importKeep = 30 * time.Minute
	// importWorkers 是扫描时同时认几项（每一项都要问 TMDB）
	importWorkers = 4
	// importMaxFiles 是每一项回给页面的视频数（数目照样按全部算）
	importMaxFiles = 200
	// importChunk 与 importSlice：导入在队列里一次最多做几项、多久，没做完的重新排到队尾，整理不用等整个导入做完
	importChunk = 20
	importSlice = 30 * time.Second
	// importMaxMessages 是一次导入最多记几条说明
	importMaxMessages = 50
)

const (
	existingDeleteNote = "这是导入时就在库里的文件（不是 pt-tools 放进去的），没有删除：确认要删它的文件以后再删一次"
	existingRetireNote = "导入的已有文件，没有自动删除"
	importRetryNote    = "导入的已有文件不用重试"
)

// ErrExistingKept 表示洗版时要删的旧版本是导入的已有文件：不删，记录照旧是已整理（见 Retire）。
var ErrExistingKept = errors.New("导入的已有文件不是 pt-tools 放进去的，洗版时不自动删除")

// importRow 报告这条记录是不是导入的已有文件。
func importRow(r models.MediaTransferHistory) bool {
	return r.Mode == models.MediaModeExisting || strings.HasPrefix(r.InfoHash, importHashPrefix)
}

// importHash 是导入的一项的标识：记在整理记录的 InfoHash 里（不是种子的 info hash）。
func importHash(item string) string {
	sum := sha256.Sum256([]byte(item))
	return importHashPrefix + hex.EncodeToString(sum[:8])
}

// 扫描与导入的状态。
const (
	ImportRunning  = "running"
	ImportDone     = "done"
	ImportFailed   = "failed"
	ImportCanceled = "canceled"
	ImportQueued   = "queued"
)

// 导入的条目来源。另外还有 manual（页面上改的）、history（视频都有整理记录，用记录里的）与识别的来源（override、imdb、search）。
const (
	importSourceNFO     = "nfo"      // NFO 里的 TMDB 编号
	importSourceNFOIMDb = "nfo-imdb" // NFO 里的 IMDb 编号
	importSourceDirname = "dirname"  // 目录名里的 TMDB 编号
)

// ImportMatch 是一项认出的条目（只带页面要用的几项）。
type ImportMatch struct {
	MediaType     string `json:"media_type"`
	TMDBID        int    `json:"tmdb_id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title,omitempty"`
	Year          int    `json:"year,omitempty"`
	PosterPath    string `json:"poster_path,omitempty"`
}

func importMatchOf(d *tmdb.Result) *ImportMatch {
	return &ImportMatch{
		MediaType: d.MediaType, TMDBID: d.ID, Title: d.Title, OriginalTitle: d.OriginalTitle, Year: d.Year, PosterPath: d.PosterPath,
	}
}

// ImportFile 是一项里的一个视频。
type ImportFile struct {
	// Rel 是相对这一项的路径（库目录里直接放的视频是文件名）
	Rel        string `json:"rel"`
	Size       int64  `json:"size"`
	Season     int    `json:"season,omitempty"`
	Episode    int    `json:"episode,omitempty"`
	EpisodeEnd int    `json:"episode_end,omitempty"`
	// Recorded 表示已经有整理记录（是某条记录的源文件，或 pt-tools 放进库的文件），导入时跳过
	Recorded bool `json:"recorded,omitempty"`
	// Problem 是这个视频导入不了的原因（例如没认出集数）
	Problem string `json:"problem,omitempty"`
}

// ImportItem 是扫描出的一项：一部电影（一个文件夹，或库目录里直接放的一个视频）或一部剧（一个剧集目录）。
type ImportItem struct {
	Path string `json:"path"`
	// Name 是相对库目录的路径（页面上显示）
	Name  string       `json:"name"`
	File  bool         `json:"file,omitempty"`
	Match *ImportMatch `json:"match,omitempty"`
	// Source 是条目从哪来（见 importSourceNFO 等）
	Source string `json:"source,omitempty"`
	// Edited 表示条目是页面上改的
	Edited bool `json:"edited,omitempty"`
	// Files 是视频（最多 importMaxFiles 个）；FileCount 与 Size 按全部视频算
	Files     []ImportFile `json:"files"`
	FileCount int          `json:"file_count"`
	Size      int64        `json:"size"`
	// Ready 是能导入的视频数（没有整理记录、没有问题的），Recorded 是已经有整理记录的
	Ready    int `json:"ready"`
	Recorded int `json:"recorded"`
	// Problem 不为空时这一项导入不了（没认出条目、类型和库不一样、原盘等）
	Problem string   `json:"problem,omitempty"`
	Notes   []string `json:"notes,omitempty"`

	// fixed 表示 Problem 是这一项本身的（原盘、太大、没有视频），改条目也没用
	fixed bool
}

// ImportScan 是一次扫描（见 StartImportScan）：进度、每一项，以及之后的导入。
type ImportScan struct {
	ID        string `json:"id"`
	LibraryID uint   `json:"library_id"`
	Path      string `json:"path"`
	Status    string `json:"status"`
	// Scanned 是认完了几项，Total 是一共几项（找完目录以后才有）
	Scanned int `json:"scanned"`
	Total   int `json:"total"`
	// Truncated 表示多于 importMaxItems 项：只列了前这么多项，其余的分子目录扫描
	Truncated  bool         `json:"truncated,omitempty"`
	Error      string       `json:"error,omitempty"`
	Items      []ImportItem `json:"items,omitempty"`
	StartedAt  time.Time    `json:"started_at"`
	FinishedAt *time.Time   `json:"finished_at,omitempty"`
	// Import 是这次扫描最近一次导入的进度与结果
	Import *ImportRun `json:"import,omitempty"`
}

// ImportRun 是一次导入：排进整理的队列，和整理一样一次只做一件。
type ImportRun struct {
	Status string `json:"status"`
	// Items 是要导入的项，Done 是做完了几项
	Items int `json:"items"`
	Done  int `json:"done"`
	// Created 是记下的视频，Skipped 是已经有整理记录的视频，Failed 是导入不了的项或视频
	Created int `json:"created"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
	// Scraped 是补刮了的条目
	Scraped    int        `json:"scraped"`
	Messages   []string   `json:"messages,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// importer 是导入的状态：最近一次扫描（新开一次时取消上一次）与后台在跑的扫描。
type importer struct {
	mu   sync.Mutex
	scan *importScan
	wg   sync.WaitGroup
}

type importScan struct {
	ImportScan
	lib    models.MediaLibrary
	cancel context.CancelFunc
	done   chan struct{}
	// seen 是最后一次看它的时间（见 importKeep）
	seen time.Time
}

// importing 报告这次扫描的导入还没做完。
func (sc *importScan) importing() bool {
	return sc.Import != nil && (sc.Import.Status == ImportQueued || sc.Import.Status == ImportRunning)
}

// snapshot 是扫描现在的样子（调用方持有 imp.mu）；items 为假时不带每一项（页面轮询进度时用）。
func (sc *importScan) snapshot(items bool) ImportScan {
	out := sc.ImportScan
	out.Items = nil
	if items && sc.Status == ImportDone {
		out.Items = slices.Clone(sc.Items)
	}
	if sc.Import != nil {
		run := *sc.Import
		run.Messages = slices.Clone(run.Messages)
		out.Import = &run
	}
	return out
}

func newScanID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成扫描编号失败: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func kindName(kind string) string {
	if kind == tmdb.KindTV {
		return "剧集"
	}
	return "电影"
}

// importLibrary 读要导入的媒体库。
func (s *Service) importLibrary(ctx context.Context, id uint) (models.MediaLibrary, error) {
	var lib models.MediaLibrary
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&lib).Error; err != nil {
		return lib, fmt.Errorf("读取媒体库失败: %w", err)
	}
	if lib.ID == 0 {
		return lib, fmt.Errorf("%w: 媒体库不存在", ErrNotFound)
	}
	return lib, nil
}

// importPath 检查扫描的路径：绝对路径，是库目录或在它里面（解开软链接以后也是），是一个文件夹。空串是库目录。
func importPath(lib models.MediaLibrary, p string) (string, error) {
	if p = strings.TrimSpace(p); p == "" {
		p = lib.Path
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("%w: 路径要写绝对路径", ErrInvalid)
	}
	p = filepath.Clean(p)
	in := func(root, q string) bool { return samePath(root, q) || transfer.Within(root, q) }
	if !in(lib.Path, p) || !in(realPath(lib.Path), realPath(p)) {
		return "", fmt.Errorf("%w: 路径要是媒体库「%s」的目录（%s）或在它里面", ErrInvalid, lib.Name, lib.Path)
	}
	info, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("%w: 读不了 %s：%v", ErrInvalid, p, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s 不是文件夹", ErrInvalid, p)
	}
	return p, nil
}

// StartImportScan 开始扫描媒体库 libID 里的路径 path（空串是库目录）：找出电影与剧集，看哪些视频还没有整理记录，认出条目。
// 扫描在后台进行（每一项都要问 TMDB），马上回扫描的状态，之后用 ImportScanStatus 看进度与结果。同时只有一次扫描，
// 再开一次时取消上一次；扫描结果在内存里留 30 分钟（从最后一次看它算起）。
func (s *Service) StartImportScan(ctx context.Context, libID uint, path string) (ImportScan, error) {
	lib, err := s.importLibrary(ctx, libID)
	if err != nil {
		return ImportScan{}, err
	}
	if path, err = importPath(lib, path); err != nil {
		return ImportScan{}, err
	}
	c, err := s.cfg.Recognizer.TMDB(ctx)
	switch {
	case errors.Is(err, tmdb.ErrNoKey):
		return ImportScan{}, fmt.Errorf("%w: 没有填写 TMDB API Key：先在「媒体识别」里填写", ErrInvalid)
	case err != nil:
		return ImportScan{}, fmt.Errorf("TMDB 不能用：%w", err)
	}
	id, err := newScanID()
	if err != nil {
		return ImportScan{}, err
	}
	now := s.cfg.Now()
	sctx, cancel := context.WithCancel(context.Background())
	sc := &importScan{
		ImportScan: ImportScan{ID: id, LibraryID: lib.ID, Path: path, Status: ImportRunning, StartedAt: now},
		lib:        lib, cancel: cancel, done: make(chan struct{}), seen: now,
	}
	s.imp.mu.Lock()
	if prev := s.imp.scan; prev != nil {
		prev.cancel()
	}
	s.imp.scan = sc
	snap := sc.snapshot(false)
	s.imp.mu.Unlock()
	s.imp.wg.Go(func() {
		defer close(sc.done)
		defer cancel()
		s.runImportScan(sctx, sc, c)
	})
	return snap, nil
}

// importScanLocked 是编号为 id 的扫描，记下看它的时间；过了保留时间的清掉（调用方持有 imp.mu）。
func (s *Service) importScanLocked(id string) *importScan {
	sc := s.imp.scan
	if sc == nil {
		return nil
	}
	now := s.cfg.Now()
	if sc.Status != ImportRunning && !sc.importing() && now.Sub(sc.seen) > importKeep {
		s.imp.scan = nil
		return nil
	}
	if sc.ID != id {
		return nil
	}
	sc.seen = now
	return sc
}

var errScanGone = fmt.Errorf("%w: 扫描结果不在了（只留 30 分钟，或者开始了新的扫描）：重新扫描", ErrNotFound)

// ImportScanStatus 是一次扫描的进度与结果；items 为真时带上每一项（扫描完了才有）。
func (s *Service) ImportScanStatus(id string, items bool) (ImportScan, error) {
	s.imp.mu.Lock()
	defer s.imp.mu.Unlock()
	sc := s.importScanLocked(id)
	if sc == nil {
		return ImportScan{}, errScanGone
	}
	return sc.snapshot(items), nil
}

// CancelImportScan 取消还在进行的扫描；扫描完了的丢掉结果（导入还在进行时留着）。
func (s *Service) CancelImportScan(id string) error {
	s.imp.mu.Lock()
	defer s.imp.mu.Unlock()
	sc := s.importScanLocked(id)
	if sc == nil {
		return errScanGone
	}
	switch {
	case sc.Status == ImportRunning:
		sc.cancel()
	case !sc.importing():
		s.imp.scan = nil
	}
	return nil
}

// stopImportScan 取消在跑的扫描，等它停下（服务停止时）。
func (s *Service) stopImportScan() {
	s.imp.mu.Lock()
	if sc := s.imp.scan; sc != nil {
		sc.cancel()
	}
	s.imp.mu.Unlock()
	s.imp.wg.Wait()
}

// importScanStarted 在扫描开始时调用（测试里用它卡住扫描）。
var importScanStarted = func(context.Context) {}

// runImportScan 扫描：先找出每一项，再同时认几项。取消了的（再开一次扫描、停止服务）记成已取消。
func (s *Service) runImportScan(ctx context.Context, sc *importScan, c *tmdb.Client) {
	finish := func(status, msg string, items []ImportItem) {
		if ctx.Err() != nil {
			status, msg, items = ImportCanceled, "", nil
		}
		s.imp.mu.Lock()
		defer s.imp.mu.Unlock()
		now := s.cfg.Now()
		sc.Status, sc.Error, sc.FinishedAt, sc.seen = status, msg, &now, now
		if items != nil {
			sc.Items = items
		}
	}
	importScanStarted(ctx)
	set, err := s.Settings(ctx)
	if err != nil {
		finish(ImportFailed, err.Error(), nil)
		return
	}
	minSize := int64(set.MinVideoMB) << 20
	cats, err := s.categoryNames(ctx, sc.lib.Kind)
	if err != nil {
		finish(ImportFailed, err.Error(), nil)
		return
	}
	entries, truncated, err := discoverImport(ctx, sc.lib, sc.Path, cats, minSize)
	if err != nil {
		finish(ImportFailed, "读取目录失败："+err.Error(), nil)
		return
	}
	s.imp.mu.Lock()
	sc.Total, sc.Truncated = len(entries), truncated
	s.imp.mu.Unlock()
	items := make([]ImportItem, len(entries))
	next := make(chan int)
	var wg sync.WaitGroup
	for range min(importWorkers, len(entries)) {
		wg.Go(func() {
			for i := range next {
				items[i] = s.scanImportItem(ctx, sc.lib, c, entries[i], minSize)
				s.imp.mu.Lock()
				sc.Scanned++
				s.imp.mu.Unlock()
			}
		})
	}
feed:
	for i := range entries {
		select {
		case next <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(next)
	wg.Wait()
	finish(ImportDone, "", items)
}

// categoryNames 是二级分类的目录名（现在的规则与常用的预设，不分大小写）：扫描时这些目录当作放着几项的分类目录。
func (s *Service) categoryNames(ctx context.Context, kind string) (map[string]bool, error) {
	row, err := s.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	rules, err := decodeClassify(row.ClassifyRules)
	if err != nil {
		// 规则读不出来时只用预设：扫描不靠它放文件，少认出几个分类目录也只是多几项
		rules = ClassifyRules{}
	}
	names := map[string]bool{}
	for _, set := range []ClassifyRules{rules, DefaultClassifyRules} {
		list := set.Movie
		if kind == tmdb.KindTV {
			list = set.TV
		}
		for _, r := range list {
			names[strings.ToLower(r.Name)] = true
		}
	}
	return names, nil
}

// importEntry 是扫描路径下找到的一项：一个文件夹，或放着几项的目录里直接放的一个视频（file 为真）。
type importEntry struct {
	path string
	file bool
}

// discoverImport 找出扫描路径下的每一项（按名字排）：库目录、分类目录与自己分的组（见 importContainer）往下找，最多 importMaxNest 层；
// 别的文件夹是一项，这些目录里直接放的视频各是一项。扫描路径本身不是库目录、也不像放着几项的目录时，它就是一项。
// 超过 importMaxItems 项时只回前这么多项，truncated 为真。
func discoverImport(ctx context.Context, lib models.MediaLibrary, scanPath string, cats map[string]bool, minSize int64) ([]importEntry, bool, error) {
	var out []importEntry
	full := false
	add := func(e importEntry) {
		if len(out) >= importMaxItems {
			full = true
			return
		}
		out = append(out, e)
	}
	var walk func(dir string, depth int) error
	walk = func(dir string, depth int) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if depth == 0 {
				return err
			}
			// 读不了的子目录当作一项：读它的视频时写明原因
			add(importEntry{path: dir})
			return nil
		}
		for _, d := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if full {
				return nil
			}
			name := d.Name()
			if skipWatchEntry(name) {
				continue
			}
			p := filepath.Join(dir, name)
			switch {
			case d.IsDir() && importContainer(lib.Kind, p, depth+1, cats, minSize):
				if err := walk(p, depth+1); err != nil {
					return err
				}
			case d.IsDir():
				add(importEntry{path: p})
			case d.Type().IsRegular() && importFileVideo(p, name, minSize):
				add(importEntry{path: p, file: true})
			}
		}
		return nil
	}
	var err error
	if samePath(scanPath, lib.Path) || importContainer(lib.Kind, scanPath, 0, cats, minSize) {
		err = walk(scanPath, 0)
	} else {
		add(importEntry{path: scanPath})
	}
	return out, full, err
}

var (
	// partDirRe 是一部电影分成几段放的目录（CD1、Disc 2）
	partDirRe = regexp.MustCompile(`(?i)^(?:cd|dvd|disc|disk|part|pt)[ ._-]*\d{1,2}$`)
	// seasonDirRe 与 cnSeasonDirRe 是季目录（Season 1、S01、第一季），specialsDirRe 是特别篇目录（第 0 季）
	seasonDirRe   = regexp.MustCompile(`(?i)^(?:season|series|s)[ ._-]*(\d{1,3})(?:[^0-9].*)?$`)
	cnSeasonDirRe = regexp.MustCompile(`^第\s*[\d一二三四五六七八九十]{1,4}\s*季`)
	specialsDirRe = regexp.MustCompile(`(?i)^(?:specials?|特别篇|特別篇)$`)
	// season0Re 是文件名里写明的第 0 季（S00E01）：解析出的季号是 0 时和没写季号分不开
	season0Re = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s0{1,3}[ .]?e\d`)
)

// seasonDir 是季目录的季号：不是季目录时回 -1，特别篇是 0。
func seasonDir(name string) int {
	switch {
	case specialsDirRe.MatchString(name):
		return 0
	case cnSeasonDirRe.MatchString(name):
		if n := meta.Parse(name, "").Season; n > 0 {
			return n
		}
	default:
		if mm := seasonDirRe.FindStringSubmatch(name); mm != nil {
			n, _ := strconv.Atoi(mm[1])
			return n
		}
	}
	return -1
}

// importContainer 报告文件夹是放着几项的目录（库目录下的分类目录、自己分的组），不是一项（一部电影或一部剧）：
// 名字是二级分类的目录名的是；有 movie.nfo / tvshow.nfo、原盘的不是；电影直接放着视频的不是（几个视频是几部电影时是，
// 例如平铺在分类目录里的电影），分段的目录（CD1、Disc 2）也不是；剧集直接放着视频或有季目录的不是；
// 剩下的有子目录的是。太深（depth 超过 importMaxNest）的当作一项。
func importContainer(kind, dir string, depth int, cats map[string]bool, minSize int64) bool {
	if cats[strings.ToLower(filepath.Base(dir))] {
		return true
	}
	if depth > importMaxNest {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	tv := kind == tmdb.KindTV
	var videos []string
	subdirs, seasons, parts := 0, false, false
	for _, d := range entries {
		n := d.Name()
		if skipWatchEntry(n) {
			continue
		}
		lower := strings.ToLower(n)
		switch {
		case d.IsDir():
			if lower == "bdmv" || lower == "video_ts" {
				return false
			}
			subdirs++
			if tv && seasonDir(n) >= 0 {
				seasons = true
			}
			if !tv && partDirRe.MatchString(n) {
				parts = true
			}
		case d.Type().IsRegular():
			if (!tv && lower == "movie.nfo") || (tv && lower == "tvshow.nfo") {
				return false
			}
			if importFileVideo(filepath.Join(dir, n), n, minSize) {
				videos = append(videos, n)
			}
		}
	}
	switch {
	case tv && (len(videos) > 0 || seasons):
		return false
	case !tv && len(videos) > 0:
		return distinctTitles(videos)
	case !tv && parts:
		return false
	}
	return subdirs > 0
}

// distinctTitles 报告这几个视频的名字解析出的片名或年份不一样：是几部电影，不是一部的几个版本或几段。
func distinctTitles(names []string) bool {
	key := ""
	for i, n := range names {
		m := meta.Parse(stemOf(n), "")
		k := strings.ToLower(strings.TrimSpace(m.NameEN)) + "|" + strings.TrimSpace(m.NameCN) + "|" + strconv.Itoa(m.Year)
		if i == 0 {
			key = k
		} else if k != key {
			return true
		}
	}
	return false
}

// importFileVideo 报告文件是不是要导入的视频：视频或 ISO 镜像，不是样片、预告（按整理挑视频的规则）。
func importFileVideo(p, name string, minSize int64) bool {
	if !transfer.IsVideo(name) && !transfer.IsISO(name) {
		return false
	}
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	return len(transfer.Select([]transfer.File{{Rel: name, Path: p, Size: info.Size()}}, minSize).Videos) == 1
}

// importName 是一项相对库目录的路径（页面上显示）。
func importName(libPath, p string) string {
	if rel, err := filepath.Rel(libPath, p); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.Base(p)
}

// importVideo 是一项里的一个视频：页面上看的（ImportFile）加上 pt-tools 里的路径。
type importVideo struct {
	ImportFile
	path string
}

// importVideos 读一项里的视频：按整理挑视频的规则（花絮、样片去掉），剧集认出季集，标出已经有整理记录的。
// 这一项本身导入不了时（原盘、太大、没有视频、还在改动、剧集的视频没放在剧集目录里）Problem 写明，fixed 为真。
// rec 是这些视频已有的一条已整理记录（视频都有记录时页面上用它的条目）。
func (s *Service) importVideos(ctx context.Context, lib models.MediaLibrary, e importEntry, minSize int64) (ImportItem, []importVideo, *models.MediaTransferHistory) {
	parent, name := filepath.Dir(e.path), filepath.Base(e.path)
	it := ImportItem{Path: e.path, Name: importName(lib.Path, e.path), File: e.file, Files: []ImportFile{}}
	fixed := func(msg string) (ImportItem, []importVideo, *models.MediaTransferHistory) {
		it.Problem, it.fixed = msg, true
		return it, nil, nil
	}
	tv := lib.Kind == tmdb.KindTV
	if e.file && tv {
		return fixed("剧集的视频要放在剧集目录里（例如「剧名/Season 1/」），没有导入")
	}
	files, _, err := watchFiles(parent, name)
	switch {
	case errors.Is(err, errWatchTooBig):
		return fixed("里面的" + errWatchTooBig.Error() + "，没有导入：分开扫描里面的文件夹")
	case err != nil:
		return fixed("读不了：" + err.Error())
	}
	sel := transfer.Select(files, minSize)
	switch {
	case len(sel.Discs) > 0:
		return fixed("原盘暂不导入")
	case len(sel.Videos) == 0:
		return fixed("没有可导入的视频文件")
	case s.cfg.Now().Sub(newestFile(sel.Videos)) < watchSettle:
		// 只看视频：刮削刚写进来的 NFO 与图片、媒体服务器改的文件不算（导入只记视频）
		return fixed(fmt.Sprintf("最近 %d 分钟里还有改动（可能还在复制），等写完再导入", int(watchSettle/time.Minute)))
	}
	videos := make([]importVideo, 0, len(sel.Videos))
	for _, v := range sel.Videos {
		rel := v.Rel
		if !e.file {
			rel = strings.TrimPrefix(rel, filepath.ToSlash(name)+"/")
		}
		iv := importVideo{ImportFile: ImportFile{Rel: rel, Size: v.Size}, path: v.Path}
		if tv {
			s.importEpisode(ctx, e.path, &iv)
		}
		videos = append(videos, iv)
	}
	rec, err := s.markRecorded(ctx, videos)
	if err != nil {
		return fixed("读取整理记录失败：" + err.Error())
	}
	for _, v := range videos {
		it.FileCount++
		it.Size += v.Size
		switch {
		case v.Recorded:
			it.Recorded++
		case v.Problem == "":
			it.Ready++
		}
		it.Files = append(it.Files, v.ImportFile)
	}
	return it, videos, rec
}

// newestFile 是这些文件里最晚的修改时间（读不了的不算）。
func newestFile(files []transfer.File) time.Time {
	var newest time.Time
	for _, f := range files {
		if info, err := os.Stat(f.Path); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest
}

// importEpisode 认出剧集里一个视频的季与集：看文件名（套用识别词）；文件名里没写季号时看上面的季目录（Season 1、S01、第一季、Specials），
// 都没有时是第 1 季。没认出集数的、ISO 镜像写进 Problem。
func (s *Service) importEpisode(ctx context.Context, item string, v *importVideo) {
	if transfer.IsISO(v.path) {
		v.Problem = "剧集里的 ISO 镜像不导入"
		return
	}
	stem := stemOf(v.Rel)
	m, _ := s.cfg.Recognizer.Parse(ctx, stem, "")
	season, known := m.Season, m.Season > 0
	if !known && season0Re.MatchString(stem) {
		season, known = 0, true
	}
	for d := filepath.Dir(v.path); !known && transfer.Within(item, d); d = filepath.Dir(d) {
		if n := seasonDir(filepath.Base(d)); n >= 0 {
			season, known = n, true
		}
	}
	if !known {
		season = 1
	}
	v.Season, v.Episode, v.EpisodeEnd = season, m.Episode, m.EpisodeEnd
	if v.Episode == 0 {
		v.Problem = "文件名里没认出集数：可以加识别词，或改文件名后重新扫描"
	}
}

// markRecorded 标出已经有整理记录的视频，回其中一条已整理的记录。源文件是它的（不管状态，一个源文件只能有一条记录），
// 或者库里的文件是它、而且是 pt-tools 放的（已整理的，或之前整理出过文件的；跳过的记录里的目标是 pt-tools 想放、被别的文件占着的位置，不算）。
func (s *Service) markRecorded(ctx context.Context, videos []importVideo) (*models.MediaTransferHistory, error) {
	var first *models.MediaTransferHistory
	for i := 0; i < len(videos); i += 400 {
		batch := videos[i:min(i+400, len(videos))]
		paths := make([]string, 0, len(batch))
		for _, v := range batch {
			paths = append(paths, v.path)
		}
		var rows []models.MediaTransferHistory
		if err := s.cfg.DB.WithContext(ctx).
			Select("id", "source_path", "target_path", "target_file_id", "status", "media_type", "tmdb_id", "title", "year").
			Where("source_path IN ? OR target_path IN ?", paths, paths).Find(&rows).Error; err != nil {
			return nil, err
		}
		have := map[string]bool{}
		for j := range rows {
			r := &rows[j]
			have[r.SourcePath] = true
			if r.Status == models.MediaTransferDone || (r.Status != models.MediaTransferRemoved && r.TargetFileID != "") {
				have[r.TargetPath] = true
			}
			if first == nil && r.Status == models.MediaTransferDone && r.TMDBID > 0 && validKind(r.MediaType) {
				first = r
			}
		}
		for j := range batch {
			batch[j].Recorded = have[batch[j].path]
		}
	}
	return first, nil
}

// scanImportItem 读一项的视频并认出条目。
func (s *Service) scanImportItem(ctx context.Context, lib models.MediaLibrary, c *tmdb.Client, e importEntry, minSize int64) ImportItem {
	it, videos, rec := s.importVideos(ctx, lib, e, minSize)
	if it.Problem == "" {
		s.identifyImport(ctx, lib, c, &it, e, videos, rec)
	}
	if len(it.Files) > importMaxFiles {
		it.Files = it.Files[:importMaxFiles]
	}
	return it
}

// identifyImport 认出一项的条目，按这个顺序：已有的 NFO（TMDB 编号直接取详情，只有 IMDb 编号时带上它识别）、
// 目录名里的编号（tmdbid=123、tmdb-123）、按目录名（库目录里直接放的电影按文件名）识别。认出的类型要和媒体库一样。
// 视频都已经有整理记录时不问 TMDB，用记录里的条目；视频都导入不了时不再认。
func (s *Service) identifyImport(ctx context.Context, lib models.MediaLibrary, c *tmdb.Client, it *ImportItem, e importEntry, videos []importVideo,
	rec *models.MediaTransferHistory,
) {
	if it.Ready == 0 {
		if rec != nil && it.Recorded == it.FileCount {
			it.Match = &ImportMatch{MediaType: rec.MediaType, TMDBID: rec.TMDBID, Title: rec.Title, Year: rec.Year}
			it.Source = sourceHistory
			return
		}
		if it.Recorded < it.FileCount {
			it.Problem, it.fixed = "没有能导入的视频（原因见每个视频）", true
		}
		return
	}
	nfo := importNFO(lib.Kind, e, videos)
	if nfo.TMDB > 0 && s.importDetails(ctx, c, lib, it, nfo.TMDB, importSourceNFO, "NFO 里") || it.Problem != "" {
		return
	}
	name := filepath.Base(e.path)
	if e.file {
		name = stemOf(name)
	}
	tags := dirTags(name)
	if tags.TMDB > 0 && s.importDetails(ctx, c, lib, it, tags.TMDB, importSourceDirname, "名字里") || it.Problem != "" {
		return
	}
	imdb := firstNonEmpty(nfo.IMDb, tags.IMDb)
	s.importRecognize(ctx, lib, it, stripTags(name), imdb, nfo.IMDb != "")
}

// importNFO 是一项已有的 NFO 里写的编号：电影看 movie.nfo 与视频同名的 .nfo，剧集看 tvshow.nfo。
func importNFO(kind string, e importEntry, videos []importVideo) nfoIDs {
	var paths []string
	switch {
	case kind == tmdb.KindTV:
		paths = []string{filepath.Join(e.path, "tvshow.nfo")}
	case e.file:
		paths = []string{strings.TrimSuffix(e.path, filepath.Ext(e.path)) + ".nfo"}
	default:
		paths = []string{filepath.Join(e.path, "movie.nfo")}
		for _, v := range videos[:min(len(videos), 5)] {
			paths = append(paths, strings.TrimSuffix(v.path, filepath.Ext(v.path))+".nfo")
		}
	}
	for _, p := range paths {
		if ids := readNFO(p, kind); ids.TMDB > 0 || ids.IMDb != "" {
			return ids
		}
	}
	return nfoIDs{}
}

// importDetails 按编号取条目详情（类型跟媒体库），报告认出来了没有。TMDB 上没有这个编号时写一条说明、回假（接着按名字认）；
// 别的错误写进 Problem。
func (s *Service) importDetails(ctx context.Context, c *tmdb.Client, lib models.MediaLibrary, it *ImportItem, id int, source, where string) bool {
	d, err := c.Details(ctx, lib.Kind, id)
	switch {
	case err == nil:
		it.Match, it.Source = importMatchOf(d), source
		return true
	case errors.Is(err, tmdb.ErrNotFound):
		it.Notes = append(it.Notes, fmt.Sprintf("%s写的 TMDB 编号 %d 在 TMDB 上不是%s，按名字识别", where, id, kindName(lib.Kind)))
	default:
		it.Problem = "取 TMDB 详情失败：" + err.Error()
	}
	return false
}

// importRecognize 按名字识别（imdb 不为空时识别先按 IMDb 编号查）。剧集目录名里多半没写季号，按剧集识别：加上「S01」。
func (s *Service) importRecognize(ctx context.Context, lib models.MediaLibrary, it *ImportItem, name, imdb string, imdbFromNFO bool) {
	title := name
	if lib.Kind == tmdb.KindTV {
		if m, err := s.cfg.Recognizer.Parse(ctx, name, ""); err == nil && m.Type != meta.TypeTV {
			title = name + " S01"
		}
	}
	res, err := s.cfg.Recognizer.Recognize(ctx, recognize.Input{Title: title, IMDbID: imdb})
	switch {
	case err != nil:
		it.Problem = "识别失败：" + err.Error()
	case res.Error != "":
		it.Problem = "识别失败：" + res.Error
	case res.Match == nil:
		it.Problem = "没有识别出来：TMDB 上没有可靠的匹配"
		if len(res.Candidates) > 0 {
			c := res.Candidates[0]
			it.Problem += fmt.Sprintf("（最接近的是「%s」）", firstNonEmpty(c.Title, c.OriginalTitle))
		}
		it.Problem += "，填 TMDB 编号"
	case res.Match.MediaType != lib.Kind:
		it.Problem = fmt.Sprintf("认成了%s「%s」，这是%s库：填%s的 TMDB 编号", kindName(res.Match.MediaType),
			firstNonEmpty(res.Match.Title, res.Match.OriginalTitle), kindName(lib.Kind), kindName(lib.Kind))
	default:
		it.Match, it.Source = importMatchOf(res.Match), res.Source
		if imdbFromNFO && res.Source == recognize.SourceIMDb {
			it.Source = importSourceNFOIMDb
		}
	}
}

// SetImportItem 改扫描里一项的条目（类型跟媒体库）：按编号取 TMDB 详情，导入时用它。回改好的这一项。
func (s *Service) SetImportItem(ctx context.Context, scanID, path string, tmdbID int) (ImportItem, error) {
	if tmdbID <= 0 {
		return ImportItem{}, fmt.Errorf("%w: 要填 TMDB 编号", ErrInvalid)
	}
	find := func() (*importScan, int, error) {
		sc := s.importScanLocked(scanID)
		switch {
		case sc == nil:
			return nil, 0, errScanGone
		case sc.Status != ImportDone:
			return nil, 0, fmt.Errorf("%w: 扫描还没完成", ErrInvalid)
		case sc.importing():
			return nil, 0, fmt.Errorf("%w: 这次扫描的导入还没做完", ErrInvalid)
		}
		i := slices.IndexFunc(sc.Items, func(it ImportItem) bool { return it.Path == path })
		if i < 0 {
			return nil, 0, fmt.Errorf("%w: 「%s」不是这次扫描出的项", ErrInvalid, path)
		}
		if it := sc.Items[i]; it.fixed {
			return nil, 0, fmt.Errorf("%w: 「%s」导入不了（%s），改条目也没用", ErrInvalid, it.Name, it.Problem)
		}
		return sc, i, nil
	}
	s.imp.mu.Lock()
	sc, _, err := find()
	s.imp.mu.Unlock()
	if err != nil {
		return ImportItem{}, err
	}
	c, err := s.cfg.Recognizer.TMDB(ctx)
	switch {
	case errors.Is(err, tmdb.ErrNoKey):
		return ImportItem{}, fmt.Errorf("%w: 没有填写 TMDB API Key：先在「媒体识别」里填写", ErrInvalid)
	case err != nil:
		return ImportItem{}, fmt.Errorf("TMDB 不能用：%w", err)
	}
	d, err := c.Details(ctx, sc.lib.Kind, tmdbID)
	switch {
	case errors.Is(err, tmdb.ErrNotFound):
		return ImportItem{}, fmt.Errorf("%w: TMDB 上没有编号是 %d 的%s", ErrInvalid, tmdbID, kindName(sc.lib.Kind))
	case err != nil:
		return ImportItem{}, fmt.Errorf("取 TMDB 详情失败：%w", err)
	}
	s.imp.mu.Lock()
	defer s.imp.mu.Unlock()
	// 问 TMDB 的时候放开了锁：重新找一次（扫描可能换了）
	again, i, err := find()
	if err != nil {
		return ImportItem{}, err
	}
	if again != sc {
		return ImportItem{}, errScanGone
	}
	it := sc.Items[i]
	it.Match, it.Source, it.Edited, it.Problem = importMatchOf(d), sourceManual, true, ""
	sc.Items[i] = it
	return it, nil
}

// importTask 是导入的一项：扫描里的这一项与选定的条目。
type importTask struct {
	path, name string
	file       bool
	tmdbID     int
	source     string
}

// importRun 是排进队列的一次导入；state 是扫描里给页面看的进度（读写都在 imp.mu 下）。
type importRun struct {
	libID   uint
	tasks   []importTask
	next    int
	scrape  bool
	minSize int64
	state   *ImportRun
}

// QueueImport 导入扫描里选中的项（paths 是每一项的路径，只认扫描里有的、认出了条目的），用扫描里的条目：排进整理的队列，
// 和整理一样一次只做一件，马上回；进度在扫描的 Import 里。scrape 为真时导入完每个条目补刮一次（只补没有的 NFO 与图片，媒体库关着刮削时不刮）。
// 后台没在跑时（例如测试）直接导入。
func (s *Service) QueueImport(ctx context.Context, scanID string, paths []string, scrape bool) (ImportRun, error) {
	if len(paths) == 0 {
		return ImportRun{}, fmt.Errorf("%w: 先选要导入的项", ErrInvalid)
	}
	set, err := s.Settings(ctx)
	if err != nil {
		return ImportRun{}, err
	}
	s.imp.mu.Lock()
	sc := s.importScanLocked(scanID)
	switch {
	case sc == nil:
		s.imp.mu.Unlock()
		return ImportRun{}, errScanGone
	case sc.Status != ImportDone:
		s.imp.mu.Unlock()
		return ImportRun{}, fmt.Errorf("%w: 扫描还没完成", ErrInvalid)
	case sc.importing():
		s.imp.mu.Unlock()
		return ImportRun{}, fmt.Errorf("%w: 这次扫描的导入还没做完", ErrInvalid)
	}
	var tasks []importTask
	seen := map[string]bool{}
	for _, p := range paths {
		if seen[p] {
			continue
		}
		seen[p] = true
		i := slices.IndexFunc(sc.Items, func(it ImportItem) bool { return it.Path == p })
		if i < 0 {
			s.imp.mu.Unlock()
			return ImportRun{}, fmt.Errorf("%w: 「%s」不是这次扫描出的项", ErrInvalid, p)
		}
		it := sc.Items[i]
		switch {
		case it.Problem != "":
			s.imp.mu.Unlock()
			return ImportRun{}, fmt.Errorf("%w: 「%s」导入不了：%s", ErrInvalid, it.Name, it.Problem)
		case it.Match == nil || it.Ready == 0:
			s.imp.mu.Unlock()
			return ImportRun{}, fmt.Errorf("%w: 「%s」没有能导入的视频", ErrInvalid, it.Name)
		}
		tasks = append(tasks, importTask{path: it.Path, name: it.Name, file: it.File, tmdbID: it.Match.TMDBID, source: it.Source})
	}
	run := &ImportRun{Status: ImportQueued, Items: len(tasks)}
	r := &importRun{libID: sc.lib.ID, tasks: tasks, scrape: scrape, minSize: int64(set.MinVideoMB) << 20, state: run}
	sc.Import = run
	running := s.isRunning()
	if running && s.put(job{imp: r}) != queueAdded {
		sc.Import = nil
		s.imp.mu.Unlock()
		return ImportRun{}, errors.New("整理队列已满，稍后再试")
	}
	s.imp.mu.Unlock()
	if !running {
		for !s.runImport(ctx, r) {
		}
	}
	s.imp.mu.Lock()
	defer s.imp.mu.Unlock()
	out := *run
	out.Messages = slices.Clone(run.Messages)
	return out, nil
}

// handleImport 做队列里的一次导入：先从去重表里拿掉，做一段（见 runImport），没做完的重新排到队尾。
func (s *Service) handleImport(ctx context.Context, j job) {
	for {
		s.qmu.Lock()
		delete(s.queued, j.key())
		s.qmu.Unlock()
		if s.runImport(ctx, j.imp) || s.put(j) == queueAdded {
			return
		}
		// 队列满了排不回去：接着做
	}
}

// updateImport 改一次导入的进度。
func (s *Service) updateImport(r *importRun, f func(*ImportRun)) {
	s.imp.mu.Lock()
	defer s.imp.mu.Unlock()
	f(r.state)
}

// runImport 接着做一次导入：最多 importChunk 项、importSlice 这么久，报告做完了没有。
func (s *Service) runImport(ctx context.Context, r *importRun) bool {
	finish := func(status, msg string) bool {
		s.updateImport(r, func(st *ImportRun) {
			now := s.cfg.Now()
			st.Status, st.FinishedAt = status, &now
			if msg != "" {
				st.Messages = append(st.Messages, msg)
			}
		})
		return true
	}
	var lib models.MediaLibrary
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", r.libID).Limit(1).Find(&lib).Error; err != nil {
		if ctx.Err() != nil {
			return finish(ImportFailed, "pt-tools 停止了，没导入完：重新扫描以后导入剩下的")
		}
		return finish(ImportFailed, "读取媒体库失败："+err.Error())
	}
	if lib.ID == 0 {
		return finish(ImportFailed, "媒体库删了，没导入完")
	}
	s.updateImport(r, func(st *ImportRun) { st.Status = ImportRunning })
	start := time.Now()
	for n := 0; r.next < len(r.tasks) && n < importChunk && time.Since(start) < importSlice; n++ {
		if ctx.Err() != nil {
			return finish(ImportFailed, "pt-tools 停止了，没导入完：重新扫描以后导入剩下的")
		}
		t := r.tasks[r.next]
		r.next++
		s.mu.Lock()
		out := s.importItem(ctx, lib, r, t)
		s.mu.Unlock()
		s.updateImport(r, func(st *ImportRun) {
			st.Done++
			st.Created, st.Skipped, st.Failed = st.Created+out.created, st.Skipped+out.skipped, st.Failed+out.failed
			if out.scraped {
				st.Scraped++
			}
			for _, m := range out.msgs {
				if len(st.Messages) < importMaxMessages {
					st.Messages = append(st.Messages, m)
				}
			}
		})
	}
	if r.next < len(r.tasks) {
		return false
	}
	created := 0
	s.updateImport(r, func(st *ImportRun) { created = st.Created })
	s.cfg.Logger.Infof("[整理入库] 导入已有文件到媒体库「%s」：%d 项，记下 %d 个视频", lib.Name, len(r.tasks), created)
	return finish(ImportDone, "")
}

// importOutcome 是导入一项的结果。
type importOutcome struct {
	created, skipped, failed int
	scraped                  bool
	msgs                     []string
}

// importItem 导入一项（调用方持有 s.mu）：再读一次这一项的视频（不用扫描时读到的），按选定的条目给每个能导入的视频记一条整理记录，
// 已经有记录的跳过。要补刮时导入完刮削一次（只补没有的文件，见 rescrapeWith）。
func (s *Service) importItem(ctx context.Context, lib models.MediaLibrary, r *importRun, t importTask) importOutcome {
	var out importOutcome
	fail := func(msg string) importOutcome {
		out.failed++
		out.msgs = append(out.msgs, t.name+"："+msg)
		return out
	}
	it, videos, _ := s.importVideos(ctx, lib, importEntry{path: t.path, file: t.file}, r.minSize)
	if it.Problem != "" {
		return fail(it.Problem)
	}
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return fail("TMDB 不能用：" + err.Error())
	}
	d, err := c.Details(ctx, lib.Kind, t.tmdbID)
	if err != nil {
		return fail("取 TMDB 详情失败：" + err.Error())
	}
	now := s.cfg.Now()
	root := filepath.Dir(t.path)
	var first uint
	for _, v := range videos {
		switch {
		case v.Recorded:
			out.skipped++
			continue
		case v.Problem != "":
			out.failed++
			out.msgs = append(out.msgs, t.name+"/"+v.Rel+"："+v.Problem)
			continue
		}
		id, err := fileIDOf(v.path)
		if err != nil {
			out.failed++
			out.msgs = append(out.msgs, fmt.Sprintf("%s/%s：读不到文件编号（%v）", t.name, v.Rel, err))
			continue
		}
		row := models.MediaTransferHistory{
			DownloaderName: importSource, InfoHash: importHash(t.path), TorrentName: truncate(filepath.Base(v.path), 512), LibraryID: lib.ID,
			SourcePath: v.path, SaveRoot: root, TargetPath: v.path, Mode: models.MediaModeExisting, MediaType: d.MediaType, TMDBID: d.ID,
			Title: truncate(d.Title, 255), Year: d.Year, Season: v.Season, Episode: v.Episode, EpisodeEnd: v.EpisodeEnd, Size: v.Size,
			Status: models.MediaTransferDone, TargetFileID: id, SourceFileID: id, RootPath: root, Trigger: models.MediaTriggerImport,
			MatchSource: truncate(t.source, 16), CreatedAt: now, UpdatedAt: now,
		}
		res := s.cfg.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		switch {
		case res.Error != nil:
			out.failed++
			out.msgs = append(out.msgs, fmt.Sprintf("%s/%s：写整理记录失败（%v）", t.name, v.Rel, res.Error))
		case res.RowsAffected == 0:
			// 刚刚有了记录（例如同时整理到了这里）
			out.skipped++
		default:
			out.created++
			if first == 0 {
				first = row.ID
			}
		}
	}
	if r.scrape && first > 0 && lib.Scrape {
		if _, err := s.rescrapeWith(ctx, first, true); err != nil {
			out.msgs = append(out.msgs, t.name+"：补刮失败："+err.Error())
		} else {
			out.scraped = true
		}
	}
	return out
}

// 每周补刮（媒体库的「每周补一次缺的刮削」）。
const (
	// fillCheck 是后台多久看一次要不要补刮
	fillCheck = time.Hour
	// fillEvery 是一个媒体库多久补刮一次
	fillEvery = 7 * 24 * time.Hour
	// fillMax 是一个媒体库一次最多补刮几个条目
	fillMax = 50
)

// scrapeFill 补刮：开着刮削与「每周补一次缺的刮削」、上次补刮在 7 天以前的媒体库，把刮削结果是部分成功或失败的已整理记录
// 所在的条目排进队列重新刮削（只补没有的文件，见 rescrapeWith），每个库一次最多 fillMax 个条目，先补刮削得最早的；记下补刮的时间。
func (s *Service) scrapeFill(ctx context.Context) {
	now := s.cfg.Now()
	var libs []models.MediaLibrary
	if err := s.cfg.DB.WithContext(ctx).
		Where("scrape = ? AND scrape_fill = ? AND (scrape_filled_at IS NULL OR julianday(scrape_filled_at) <= julianday(?))", true, true, now.Add(-fillEvery).UTC()).
		Order("id").Find(&libs).Error; err != nil {
		s.cfg.Logger.Warnf("[整理入库] 读取要补刮的媒体库失败: %v", err)
		return
	}
	for _, lib := range libs {
		var picks []struct {
			ID uint `gorm:"column:id"`
		}
		if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).Select("MIN(id) AS id").
			Where("library_id = ? AND status = ? AND tmdb_id > 0 AND target_path <> '' AND scrape_status IN ?",
				lib.ID, models.MediaTransferDone, []string{models.MediaScrapePartial, models.MediaScrapeFailed}).
			Group("media_type, tmdb_id").Order("MIN(julianday(COALESCE(scraped_at, created_at)))").Limit(fillMax).Scan(&picks).Error; err != nil {
			s.cfg.Logger.Warnf("[整理入库] 读取媒体库「%s」要补刮的记录失败: %v", lib.Name, err)
			continue
		}
		queued := 0
		for _, p := range picks {
			if s.put(job{rescrape: p.ID, fill: true}) == queueFull {
				break
			}
			queued++
		}
		if queued < len(picks) {
			// 队列满了：下一次再看（不记补刮的时间）
			s.cfg.Logger.Warnf("[整理入库] 整理队列已满，媒体库「%s」的补刮排了 %d 个条目，剩下的下次再补", lib.Name, queued)
			continue
		}
		if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaLibrary{}).Where("id = ?", lib.ID).UpdateColumn("scrape_filled_at", now).Error; err != nil {
			s.cfg.Logger.Warnf("[整理入库] 记下媒体库「%s」补刮的时间失败: %v", lib.Name, err)
			continue
		}
		if queued > 0 {
			s.cfg.Logger.Infof("[整理入库] 媒体库「%s」补刮 %d 个条目", lib.Name, queued)
		}
	}
}
