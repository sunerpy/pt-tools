package organize

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/utils"
)

// 目录监控的限制与默认值。
const (
	maxWatchDirs = 20
	// DefaultWatchIntervalMin 是扫描监控目录的默认间隔（分钟）
	DefaultWatchIntervalMin = 10
	MinWatchIntervalMin     = 1
	MaxWatchIntervalMin     = 24 * 60
	// watchSettle 是一项最近改动以后要等多久才整理：文件可能还在复制
	watchSettle = 2 * time.Minute
	// watchMaxDepth 与 watchMaxFiles 是一项里最多几层目录、几个文件：超过时整项不整理（写一条跳过的记录），不只整理其中一部分
	watchMaxDepth = 6
	watchMaxFiles = 5000
	// watchSource 是目录监控整理的记录里「下载器」一栏写的名字
	watchSource = "目录监控"
	// otherFileNote 写在整理记录的消息前面：监控目录里这个位置上的文件换过了（见 noteOtherFile）
	otherFileNote = "监控目录里这个位置上的文件换过了"
)

// errWatchTooBig 表示一项里的目录太深或文件太多。
var errWatchTooBig = fmt.Errorf("目录超过 %d 层或文件超过 %d 个", watchMaxDepth, watchMaxFiles)

// skipWatchEntry 报告目录下这一项要不要跳过：隐藏的、NAS 的系统目录、还没下载完或 pt-tools 自己的临时文件。
func skipWatchEntry(name string) bool {
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(name, "."), strings.HasPrefix(name, "@"), strings.HasPrefix(name, "#"), strings.HasPrefix(name, "$"):
		return true
	case lower == "lost+found", lower == "system volume information":
		return true
	case strings.Contains(lower, ".pt-tools-"):
		return true
	}
	for _, suf := range []string{".part", ".!qb", ".!ut", ".tmp", ".crdownload", ".download", ".partial"} {
		if strings.HasSuffix(lower, suf) {
			return true
		}
	}
	return false
}

// watchFiles 列出一项里的文件（Rel 带上这一项的名字，和种子的文件列表一样），以及最近一次改动的时间。
// 目录太深或文件太多时返回 errWatchTooBig。
func watchFiles(dirPath, entry string) ([]transfer.File, time.Time, error) {
	root := filepath.Join(dirPath, entry)
	info, err := os.Lstat(root)
	if err != nil {
		return nil, time.Time{}, err
	}
	if info.Mode().IsRegular() {
		return []transfer.File{{Rel: entry, Path: root, Size: info.Size()}}, info.ModTime(), nil
	}
	if !info.IsDir() {
		return nil, time.Time{}, fmt.Errorf("%s 不是文件也不是文件夹", entry)
	}
	var files []transfer.File
	newest := info.ModTime()
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		rel, _ := filepath.Rel(dirPath, p)
		if d.IsDir() {
			if p != root && skipWatchEntry(d.Name()) {
				return filepath.SkipDir
			}
			if strings.Count(filepath.ToSlash(rel), "/") >= watchMaxDepth {
				return errWatchTooBig
			}
			if fi, ierr := d.Info(); ierr == nil && fi.ModTime().After(newest) {
				newest = fi.ModTime()
			}
			return nil
		}
		if !d.Type().IsRegular() || skipWatchEntry(d.Name()) {
			return nil
		}
		fi, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		if fi.ModTime().After(newest) {
			newest = fi.ModTime()
		}
		if len(files) >= watchMaxFiles {
			return errWatchTooBig
		}
		files = append(files, transfer.File{Rel: filepath.ToSlash(rel), Path: p, Size: fi.Size()})
		return nil
	})
	return files, newest, err
}

// buildWatchPlan 是目录监控里一项的整理计划：这一项（一个文件或文件夹）当作一个种子，名字用来识别。
func (s *Service) buildWatchPlan(ctx context.Context, req Request) (*Plan, error) {
	dir, err := s.watchDir(ctx, req.WatchDirID)
	if err != nil {
		return nil, err
	}
	if dir.ID == 0 {
		return nil, fmt.Errorf("%w: 监控目录不存在", ErrNotFound)
	}
	entry := strings.TrimSpace(req.Entry)
	if entry == "" || entry == "." || entry == ".." || entry != filepath.Base(entry) || strings.ContainsAny(entry, `/\`) {
		return nil, fmt.Errorf("%w: 监控目录下那一项的名字不对", ErrInvalid)
	}
	p := &Plan{
		WatchDirID: dir.ID, DownloaderName: watchSource, Hash: watchHash(dir.ID, entry), Name: entry,
		SavePath: dir.Path, LocalPath: dir.Path, Mapped: true, contentSrc: filepath.Join(dir.Path, entry), Items: []PlanItem{},
	}
	files, newest, err := watchFiles(dir.Path, entry)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("%w: 监控目录里没有「%s」", ErrNotFound, entry)
	case errors.Is(err, errWatchTooBig):
		// 只整理一部分的话剩下的永远不会整理：整项不整理，写一条跳过的记录说明
		p.Problem = "「" + entry + "」里的" + errWatchTooBig.Error() + "，不整理：拆成几项放进监控目录"
		return p, nil
	case err != nil:
		return nil, fmt.Errorf("读取「%s」失败: %w", entry, err)
	}
	if s.cfg.Now().Sub(newest) < watchSettle {
		p.Problem, p.noRecord = fmt.Sprintf("「%s」最近 %d 分钟里还有改动（可能还在复制），等写完再整理", entry, int(watchSettle/time.Minute)), true
		return p, nil
	}
	settings, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	// 不是下载器里的种子：没有做种这回事，可以移动整理
	t := downloader.Torrent{Name: entry, State: downloader.TorrentPaused, Progress: 1}
	return s.finishPlan(ctx, p, req, t, files, settings)
}

// WatchDirs 列出监控的目录。
func (s *Service) WatchDirs(ctx context.Context) ([]models.MediaWatchDir, error) {
	var rows []models.MediaWatchDir
	if err := s.cfg.DB.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取监控目录失败: %w", err)
	}
	return rows, nil
}

// WatchDirInput 是新建或修改监控目录的请求。
type WatchDirInput struct {
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
}

// SaveWatchDir 新建（id 为 0）或修改监控目录。目录不能在媒体库里，也不能包含媒体库（不然会把库里的文件再整理一遍）。
// 修改只能开关，路径不能改：排着的整理与整理记录只记着目录的编号和里面那一项的名字，改了路径会去整理新目录里同名的文件。
func (s *Service) SaveWatchDir(ctx context.Context, id uint, in WatchDirInput) (models.MediaWatchDir, error) {
	path, err := cleanDir(in.Path, "监控目录")
	if err != nil {
		return models.MediaWatchDir{}, err
	}
	s.setMu.Lock()
	defer s.setMu.Unlock()
	db := s.cfg.DB.WithContext(ctx)
	if id != 0 {
		cur, err := s.watchDir(ctx, id)
		if err != nil {
			return models.MediaWatchDir{}, err
		}
		if cur.ID == 0 {
			return models.MediaWatchDir{}, ErrNotFound
		}
		if !samePath(cur.Path, path) {
			return models.MediaWatchDir{}, fmt.Errorf("%w: 监控目录的路径不能改，要换目录就删掉再添加", ErrInvalid)
		}
		if err := db.Model(&cur).Select("enabled", "updated_at").Updates(&models.MediaWatchDir{Enabled: in.Enabled}).Error; err != nil {
			return models.MediaWatchDir{}, fmt.Errorf("保存监控目录失败: %w", err)
		}
		return s.watchDir(ctx, id)
	}
	var libs []models.MediaLibrary
	if err := db.Find(&libs).Error; err != nil {
		return models.MediaWatchDir{}, fmt.Errorf("读取媒体库失败: %w", err)
	}
	for _, l := range libs {
		if overlaps(l.Path, path) {
			return models.MediaWatchDir{}, fmt.Errorf("%w: 监控目录不能是媒体库「%s」的目录，也不能在它里面或包含它", ErrInvalid, l.Name)
		}
	}
	var dirs []models.MediaWatchDir
	if err := db.Find(&dirs).Error; err != nil {
		return models.MediaWatchDir{}, fmt.Errorf("读取监控目录失败: %w", err)
	}
	if len(dirs) >= maxWatchDirs {
		return models.MediaWatchDir{}, fmt.Errorf("%w: 最多 %d 个监控目录", ErrInvalid, maxWatchDirs)
	}
	for _, d := range dirs {
		if samePath(d.Path, path) {
			return models.MediaWatchDir{}, fmt.Errorf("%w: 已经在监控这个目录了", ErrInvalid)
		}
	}
	row := models.MediaWatchDir{Path: path, Enabled: in.Enabled}
	if err := db.Create(&row).Error; err != nil {
		if isUnique(err) {
			return models.MediaWatchDir{}, fmt.Errorf("%w: 已经在监控这个目录了", ErrInvalid)
		}
		return models.MediaWatchDir{}, fmt.Errorf("保存监控目录失败: %w", err)
	}
	return row, nil
}

// DeleteWatchDir 删除监控目录（整理过的文件与记录不动）。
func (s *Service) DeleteWatchDir(ctx context.Context, id uint) error {
	s.setMu.Lock()
	defer s.setMu.Unlock()
	res := s.cfg.DB.WithContext(ctx).Delete(&models.MediaWatchDir{}, id)
	if res.Error != nil {
		return fmt.Errorf("删除监控目录失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// samePath 报告两个路径是不是同一个（Windows 上不分大小写）。
func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// overlaps 报告两个路径是同一个位置，或一个在另一个里面：照写的比一次，解开软链接（经另一个名字指过来的）再比一次。
// Windows 上不分大小写（filepath.Rel 本来就不分）。
func overlaps(a, b string) bool {
	near := func(a, b string) bool { return samePath(a, b) || transfer.Within(a, b) || transfer.Within(b, a) }
	return near(a, b) || near(realPath(a), realPath(b))
}

// realPath 解开路径里的软链接：路径还不存在时解开存在的那一段，后面的照原样接上。
func realPath(p string) string {
	p = filepath.Clean(p)
	rest := ""
	for q := p; ; {
		if r, err := filepath.EvalSymlinks(q); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(q)
		if parent == q {
			return p
		}
		rest = filepath.Join(filepath.Base(q), rest)
		q = parent
	}
}

// baseName 是路径的最后一段，「/」与「\」都当分隔符（下载器可能在别的系统上）。
func baseName(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// watchDir 读一个监控目录；不存在时返回零值（目录监控发现的整理排队以后，目录可能被关掉或删掉了）。
func (s *Service) watchDir(ctx context.Context, id uint) (models.MediaWatchDir, error) {
	var dir models.MediaWatchDir
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&dir).Error; err != nil {
		return models.MediaWatchDir{}, fmt.Errorf("读取监控目录失败: %w", err)
	}
	return dir, nil
}

// watchTorrent 报告监控目录里的这一项（pt-tools 里的路径 path，名字 entry）是不是下载器里的种子：种子的内容就是它、
// 在它里面或包含它（按路径映射换成 pt-tools 里的路径，见 overlaps）。种子的路径在 pt-tools 里找不到（没配路径映射，
// 或者下载器在别的机器上）时比不了路径，按名字认：同名的（不分大小写）当成是它。是的话返回「下载器：种子名」；
// 读不了哪个开着的下载器时返回错误（确认不了）。
func (s *Service) watchTorrent(ctx context.Context, path, entry string) (string, error) {
	settings, err := s.cfg.Downloaders.List(ctx)
	if err != nil {
		return "", fmt.Errorf("读取下载器失败: %w", err)
	}
	for _, ds := range settings {
		dl, _, err := s.cfg.Downloaders.Get(ctx, ds.ID)
		if err != nil {
			return "", fmt.Errorf("连不上下载器「%s」: %w", ds.Name, err)
		}
		all, err := dl.GetAllTorrents()
		if err != nil {
			return "", fmt.Errorf("读取下载器「%s」的种子失败: %w", ds.Name, err)
		}
		maps, err := s.pathMaps(ctx, ds.ID)
		if err != nil {
			return "", err
		}
		for _, t := range all {
			content := t.ContentPath
			if content == "" {
				content = strings.TrimRight(t.SavePath, `/\`) + "/" + t.Name
			}
			local, mapped := models.MapTransferPath(maps, content)
			if !mapped {
				if _, err := os.Lstat(local); err != nil {
					if strings.EqualFold(t.Name, entry) || strings.EqualFold(baseName(content), entry) {
						return ds.Name + "：" + t.Name, nil
					}
					continue
				}
			}
			if overlaps(local, path) {
				return ds.Name + "：" + t.Name, nil
			}
		}
	}
	return "", nil
}

// watchScan 扫描开着的监控目录，把有还没整理过的视频的项排进队列（例如新放进来的，或剧集文件夹里多了几集）。
// 每一轮都按整理记录与文件编号看（见 watchCovered），不靠修改时间记住扫过的项：视频都有记录、还是记录里那个文件的不排；
// 最近还在改动的、在退避里的、排上了还没整理完的也不排。没有视频、原盘或太大的项排一次：整理时写一条跳过的记录说明原因，
// 之后看到这条记录就不再排。一轮最多排 maxPerRound 项。
func (s *Service) watchScan(ctx context.Context, set Settings) {
	var dirs []models.MediaWatchDir
	if err := s.cfg.DB.WithContext(ctx).Where("enabled = ?", true).Order("id").Find(&dirs).Error; err != nil {
		s.cfg.Logger.Warnf("[整理入库] 读取监控目录失败: %v", err)
		return
	}
	queued := 0
	for _, d := range dirs {
		entries, err := os.ReadDir(d.Path)
		if err != nil {
			s.cfg.Logger.Warnf("[整理入库] 读取监控目录 %s 失败: %v", d.Path, err)
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if skipWatchEntry(name) {
				continue
			}
			hash := watchHash(d.ID, name)
			if s.backedOff(0, hash) || s.watchBusyOn(hash) {
				continue
			}
			files, newest, err := watchFiles(d.Path, name)
			if err != nil && !errors.Is(err, errWatchTooBig) {
				continue
			}
			if err == nil && s.cfg.Now().Sub(newest) < watchSettle {
				continue
			}
			covered, cerr := s.watchCovered(ctx, filepath.Join(d.Path, name), files, err != nil, set)
			if cerr != nil {
				s.cfg.Logger.Warnf("[整理入库] 读取整理记录失败: %v", cerr)
				return
			}
			if covered {
				continue
			}
			if queued >= maxPerRound {
				return
			}
			s.setWatchBusy(hash, true)
			if !s.enqueue(job{req: Request{WatchDirID: d.ID, Entry: name}, trigger: models.MediaTriggerWatch}) {
				s.setWatchBusy(hash, false)
				continue
			}
			queued++
		}
	}
}

// watchCovered 报告这一项不用再排：里面的视频都有整理记录（整理过、跳过或失败的；失败的由到期重试处理）。
// 没有视频、原盘或太大（tooBig）的项看有没有给这一项本身写的记录。现在的文件不是记录里的那个时（见 sameSource），
// 这个位置上换成了另一个文件：记录不管库里的文件时要排（按新文件整理）；管着时整理不了它，记录前面写过说明
// （见 noteOtherFile）才不再排。
func (s *Service) watchCovered(ctx context.Context, entryPath string, files []transfer.File, tooBig bool, set Settings) (bool, error) {
	db := s.cfg.DB.WithContext(ctx)
	var videos []transfer.File
	if !tooBig {
		if sel := transfer.Select(files, int64(set.MinVideoMB)<<20); sel.Disc == "" {
			videos = sel.Videos
		}
	}
	if len(videos) == 0 {
		var n int64
		if err := db.Model(&models.MediaTransferHistory{}).Where("source_path = ?", entryPath).Count(&n).Error; err != nil {
			return false, err
		}
		return n > 0, nil
	}
	srcs := make([]string, 0, len(videos))
	for _, v := range videos {
		srcs = append(srcs, v.Path)
	}
	var rows []models.MediaTransferHistory
	if err := db.Where("source_path IN ?", srcs).Find(&rows).Error; err != nil {
		return false, err
	}
	by := make(map[string]*models.MediaTransferHistory, len(rows))
	for i := range rows {
		by[rows[i].SourcePath] = &rows[i]
	}
	for _, v := range videos {
		r := by[v.Path]
		if r == nil {
			return false, nil
		}
		// 换成了另一个文件：记录管着库里的文件、已经写过说明的才算处理过
		if id, _ := transfer.FileID(v.Path); !sameSource(r, id, v.Size) && (!hasLibraryFiles(r) || !strings.HasPrefix(r.Message, otherFileNote)) {
			return false, nil
		}
	}
	return true, nil
}

// sameSource 报告监控目录里这个位置上现在的文件（编号 id、大小 size）就是整理记录里的那个：记录记着源文件的编号时
// 按编号比（同一个文件，大小与修改时间也没变）；没记着（以前的记录）或现在读不出编号时按大小比。
func sameSource(row *models.MediaTransferHistory, id string, size int64) bool {
	if row.SourceFileID != "" && id != "" {
		return row.SourceFileID == id
	}
	return row.Size == size
}

// markOtherFiles 处理监控目录里同一个位置上换成了另一个文件（见 sameSource，例如移动整理以后又放进来一个同名的）：
// 记录不管库里的文件时当成新的整理（记录改成新文件的）；管着时不整理，免得那条记录改成新文件以后库里的文件没人管了。
// 一个位置只能有一条整理记录：要整理新文件，先删除那条记录。
func markOtherFiles(p *Plan) {
	if p.WatchDirID == 0 {
		return
	}
	for i := range p.Items {
		it := &p.Items[i]
		if it.row == nil || sameSource(it.row, it.sourceID, it.Size) {
			continue
		}
		if !hasLibraryFiles(it.row) {
			it.row = nil
			continue
		}
		it.otherFile = true
		it.Status, it.Target, it.Subtitles = ItemExists, it.row.TargetPath, nil
		it.Message = fmt.Sprintf("%s（换成了另一个文件或改过，现在 %s，记录里是 %s），没有整理：一个位置只能有一条整理记录，要整理它先删除这条记录（库里的文件可以留着）",
			otherFileNote, utils.FormatBytes(it.Size), utils.FormatBytes(it.row.Size))
	}
}

// noteOtherFile 把 markOtherFiles 的说明写在那条整理记录的消息前面（写过了不再写）。
func (s *Service) noteOtherFile(ctx context.Context, it *PlanItem) {
	if it.row == nil || strings.HasPrefix(it.row.Message, otherFileNote) {
		return
	}
	msg := it.Message
	if it.row.Message != "" {
		msg += "；" + it.row.Message
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).Where("id = ?", it.row.ID).
		Update("message", truncate(msg, 1024)).Error; err != nil {
		s.cfg.Logger.Warnf("[整理入库] 更新整理记录失败: %v", err)
	}
}

func (s *Service) watchBusyOn(hash string) bool {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	return s.watchBusy[hash]
}

func (s *Service) setWatchBusy(hash string, on bool) {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	if !on {
		delete(s.watchBusy, hash)
		return
	}
	if s.watchBusy == nil {
		s.watchBusy = map[string]bool{}
	}
	s.watchBusy[hash] = true
}
