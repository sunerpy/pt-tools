package organize

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
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
	// watchMaxDepth 与 watchMaxFiles 是一项里最多看几层目录、几个文件
	watchMaxDepth = 6
	watchMaxFiles = 5000
	// watchSource 是目录监控整理的记录里「下载器」一栏写的名字
	watchSource = "目录监控"
)

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
			if p != root && (skipWatchEntry(d.Name()) || strings.Count(filepath.ToSlash(rel), "/") >= watchMaxDepth) {
				return filepath.SkipDir
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
			return filepath.SkipAll
		}
		files = append(files, transfer.File{Rel: filepath.ToSlash(rel), Path: p, Size: fi.Size()})
		return nil
	})
	return files, newest, err
}

// buildWatchPlan 是目录监控里一项的整理计划：这一项（一个文件或文件夹）当作一个种子，名字用来识别。
func (s *Service) buildWatchPlan(ctx context.Context, req Request) (*Plan, error) {
	var dir models.MediaWatchDir
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", req.WatchDirID).Limit(1).Find(&dir).Error; err != nil {
		return nil, fmt.Errorf("读取监控目录失败: %w", err)
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
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: 监控目录里没有「%s」", ErrNotFound, entry)
	}
	if err != nil {
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
func (s *Service) SaveWatchDir(ctx context.Context, id uint, in WatchDirInput) (models.MediaWatchDir, error) {
	path, err := cleanDir(in.Path, "监控目录")
	if err != nil {
		return models.MediaWatchDir{}, err
	}
	s.setMu.Lock()
	defer s.setMu.Unlock()
	db := s.cfg.DB.WithContext(ctx)
	var libs []models.MediaLibrary
	if err := db.Find(&libs).Error; err != nil {
		return models.MediaWatchDir{}, fmt.Errorf("读取媒体库失败: %w", err)
	}
	for _, l := range libs {
		if samePath(l.Path, path) || transfer.Within(l.Path, path) || transfer.Within(path, l.Path) {
			return models.MediaWatchDir{}, fmt.Errorf("%w: 监控目录不能是媒体库「%s」的目录，也不能在它里面或包含它", ErrInvalid, l.Name)
		}
	}
	row := models.MediaWatchDir{Path: path, Enabled: in.Enabled}
	if id == 0 {
		var n int64
		if err := db.Model(&models.MediaWatchDir{}).Count(&n).Error; err != nil {
			return models.MediaWatchDir{}, fmt.Errorf("读取监控目录失败: %w", err)
		}
		if n >= maxWatchDirs {
			return models.MediaWatchDir{}, fmt.Errorf("%w: 最多 %d 个监控目录", ErrInvalid, maxWatchDirs)
		}
		if err := db.Create(&row).Error; err != nil {
			if isUnique(err) {
				return models.MediaWatchDir{}, fmt.Errorf("%w: 已经在监控这个目录了", ErrInvalid)
			}
			return models.MediaWatchDir{}, fmt.Errorf("保存监控目录失败: %w", err)
		}
		return row, nil
	}
	res := db.Model(&models.MediaWatchDir{}).Where("id = ?", id).Select("path", "enabled", "updated_at").Updates(&row)
	if res.Error != nil {
		if isUnique(res.Error) {
			return models.MediaWatchDir{}, fmt.Errorf("%w: 已经在监控这个目录了", ErrInvalid)
		}
		return models.MediaWatchDir{}, fmt.Errorf("保存监控目录失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return models.MediaWatchDir{}, ErrNotFound
	}
	var out models.MediaWatchDir
	if err := db.First(&out, id).Error; err != nil {
		return models.MediaWatchDir{}, fmt.Errorf("读取监控目录失败: %w", err)
	}
	return out, nil
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

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

// watchEnabled 报告监控目录还在、还开着（目录监控发现的整理排队以后，目录可能被关掉或删掉了）。
func (s *Service) watchEnabled(ctx context.Context, id uint) (bool, error) {
	var dir models.MediaWatchDir
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&dir).Error; err != nil {
		return false, fmt.Errorf("读取监控目录失败: %w", err)
	}
	return dir.ID != 0 && dir.Enabled, nil
}

// watchTorrent 报告监控目录里的这一项（pt-tools 里的路径）是不是下载器里的种子的内容，或在它里面、或包含它：
// 是的话返回「下载器：种子名」。读不了哪个开着的下载器时返回错误（确认不了）。
func (s *Service) watchTorrent(ctx context.Context, path string) (string, error) {
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
			local, _ := models.MapTransferPath(maps, content)
			if samePath(local, path) || transfer.Within(local, path) || transfer.Within(path, local) {
				return ds.Name + "：" + t.Name, nil
			}
		}
	}
	return "", nil
}

// watchScan 扫描开着的监控目录，把有还没整理过的视频的项排进队列（例如新放进来的，或剧集文件夹里多了几集）。
// 每个视频都有整理记录的不排（整理过、跳过或失败的，失败的由到期重试处理）；最近还在改动的、在退避里的、
// 这个进程里排过以后没再改动的也不排。一轮最多排 maxPerRound 项。
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
			if s.backedOff(0, hash) {
				continue
			}
			files, newest, err := watchFiles(d.Path, name)
			if err != nil || s.cfg.Now().Sub(newest) < watchSettle || s.watchUnchanged(hash, newest) {
				continue
			}
			if videos := transfer.Select(files, int64(set.MinVideoMB)<<20).Videos; len(videos) > 0 {
				srcs := make([]string, 0, len(videos))
				for _, v := range videos {
					srcs = append(srcs, v.Path)
				}
				var n int64
				if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).Where("source_path IN ?", srcs).Count(&n).Error; err != nil {
					s.cfg.Logger.Warnf("[整理入库] 读取整理记录失败: %v", err)
					return
				}
				if int(n) >= len(videos) {
					s.watchSaw(hash, newest)
					continue
				}
			}
			if queued >= maxPerRound {
				return
			}
			if s.enqueue(job{req: Request{WatchDirID: d.ID, Entry: name}, trigger: models.MediaTriggerWatch}) {
				s.watchSaw(hash, newest)
				queued++
			}
		}
	}
}

// watchUnchanged 报告这一项上次排队以后没再改动过（这个进程里）：整理以后没写记录的（例如只多了张图片），不再每一轮都排。
func (s *Service) watchUnchanged(hash string, newest time.Time) bool {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	seen, ok := s.watchSeen[hash]
	return ok && !newest.After(seen)
}

func (s *Service) watchSaw(hash string, newest time.Time) {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	if s.watchSeen == nil {
		s.watchSeen = map[string]time.Time{}
	}
	s.watchSeen[hash] = newest
}
