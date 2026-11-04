package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/internal/health"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 系统体检（媒体中心 M8）：一次检查 TMDB、站点、下载器、媒体服务器、路径映射、媒体库、磁盘、时间、备份与密钥（见 internal/health），
// 结果存进 app_meta。只认会话。

// systemCheckResponse 是体检的结果；从没体检过时 Report 为 null。
type systemCheckResponse struct {
	Report *health.Report `json:"report"`
}

// registerSystemCheckRoutes 挂上系统体检：POST 马上体检一次（同时只跑一次，跑着的时候再点等它跑完），GET 是上一次的结果。
func (s *Server) registerSystemCheckRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/system/check", s.auth(s.apiSystemCheckRun))
	mux.HandleFunc("GET /api/system/check", s.auth(s.apiSystemCheckLast))
}

func (s *Server) apiSystemCheckRun(w http.ResponseWriter, r *http.Request) {
	db, ok := s.healthDB(w)
	if !ok {
		return
	}
	rep, err := s.healthRunner(db).Run(r.Context())
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// 首页的待处理与体检那一行用这一次的结果：缓存里那份作废
	s.homeCache.stale.Add(1)
	writeJSON(w, systemCheckResponse{Report: rep})
}

func (s *Server) apiSystemCheckLast(w http.ResponseWriter, r *http.Request) {
	db, ok := s.healthDB(w)
	if !ok {
		return
	}
	rep, err := health.Last(r.Context(), db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, systemCheckResponse{Report: rep})
}

func (s *Server) healthDB(w http.ResponseWriter) (*gorm.DB, bool) {
	if global.GlobalDB == nil || global.GlobalDB.DB == nil {
		http.Error(w, "数据库未初始化", http.StatusServiceUnavailable)
		return nil, false
	}
	return global.GlobalDB.DB, true
}

// healthRunner 是体检的执行器（整个进程一个：同时只跑一次）。
func (s *Server) healthRunner(db *gorm.DB) *health.Runner {
	s.healthOnce.Do(func() {
		s.health = &health.Runner{DB: db, Checks: func() []health.Check { return s.healthChecks(db) }}
	})
	return s.health
}

// healthChecks 是体检的各组（每次体检时建一遍，设置改了马上生效）。
func (s *Server) healthChecks(db *gorm.DB) []health.Check {
	return []health.Check{
		health.TMDB(s.healthTMDB, func(ctx context.Context) string { return subscriptionPoster(ctx, db) }, time.Now),
		health.Sites(db),
		health.Downloaders(s.healthDownloaders),
		health.Servers(db, s.healthServerTest),
		health.PathMaps(db),
		health.Libraries(db, s.healthLibraryCheck, func(ctx context.Context) []health.SavePath { return s.savePaths(ctx, db) }),
		health.Disks(func(ctx context.Context) []health.DiskPath { return s.diskPaths(ctx, db) }),
		health.Backup(s.backupState, time.Now),
		health.Secret(crypto.KeyStatus),
	}
}

func (s *Server) healthTMDB(ctx context.Context) (*tmdb.Client, error) {
	if s.media == nil {
		return nil, errors.New("媒体识别服务没有启动")
	}
	return s.media.TMDB(ctx)
}

// subscriptionPoster 是体检拿来试图片地址的海报：最近改过的订阅的；没有订阅时为空。
func subscriptionPoster(ctx context.Context, db *gorm.DB) string {
	var paths []string
	if err := db.WithContext(ctx).Model(&models.MediaSubscription{}).Where("poster_path <> ''").
		Order("updated_at DESC").Limit(1).Pluck("poster_path", &paths).Error; err != nil || len(paths) == 0 {
		return ""
	}
	return paths[0]
}

// healthDownloaders 是启用的下载器；取实例在体检里几台同时取。
func (s *Server) healthDownloaders(context.Context) ([]health.DownloaderTarget, error) {
	records, err := s.listEnabledDownloaderRecords(nil)
	if err != nil {
		return nil, err
	}
	dm := s.getDownloaderManager()
	out := make([]health.DownloaderTarget, 0, len(records))
	for _, rec := range records {
		out = append(out, health.DownloaderTarget{ID: rec.ID, Name: rec.Name, Get: func(ctx context.Context) (health.DownloaderClient, error) {
			if dm == nil {
				return nil, errors.New("下载器管理器没有初始化")
			}
			return acquireDownloader(ctx, dm, rec.Name)
		}})
	}
	return out, nil
}

func (s *Server) healthServerTest(ctx context.Context, srv models.MediaServer) (health.ServerInfo, error) {
	if s.organizer == nil {
		return health.ServerInfo{}, errors.New("整理入库服务没有启动")
	}
	info, err := s.organizer.TestServer(ctx, organize.ServerTestInput{ID: srv.ID, Kind: srv.Kind, URL: srv.URL})
	if err != nil {
		return health.ServerInfo{}, err
	}
	return health.ServerInfo{Version: info.Version}, nil
}

func (s *Server) healthLibraryCheck(ctx context.Context, path, mode string) ([]health.Probe, error) {
	if s.organizer == nil {
		return nil, errors.New("整理入库服务没有启动")
	}
	items, err := s.organizer.CheckLibrary(ctx, organize.LibraryCheckInput{Path: path, Mode: mode})
	if err != nil {
		return nil, err
	}
	out := make([]health.Probe, 0, len(items))
	for _, it := range items {
		out = append(out, health.Probe{Name: it.Name, OK: it.OK, Message: it.Message})
	}
	return out, nil
}

// savePathTimeout 是取一台下载器的默认保存路径最多等多久
const savePathTimeout = 10 * time.Second

// downloadDir 是一台下载器的默认保存路径与它报的剩余空间（体检与首页的存储共用）。
type downloadDir struct {
	name  string
	path  string
	local string
	free  int64
	err   error
}

// downloadDirs 取各台启用的下载器的默认保存路径（按路径映射换成 pt-tools 里的路径）；withFree 时也取剩余空间。几台同时取。
func (s *Server) downloadDirs(ctx context.Context, db *gorm.DB, withFree bool) []downloadDir {
	records, err := s.listEnabledDownloaderRecords(nil)
	if err != nil {
		return nil
	}
	dm := s.getDownloaderManager()
	out := make([]downloadDir, len(records))
	var wg sync.WaitGroup
	for i, rec := range records {
		wg.Go(func() {
			out[i] = s.downloadDirOf(ctx, db, dm, rec, withFree)
		})
	}
	wg.Wait()
	return out
}

func (s *Server) downloadDirOf(ctx context.Context, db *gorm.DB, dm *downloader.DownloaderManager, rec downloaderRecord, withFree bool) downloadDir {
	d := downloadDir{name: rec.Name, free: -1}
	ctx, cancel := context.WithTimeout(ctx, savePathTimeout)
	defer cancel()
	type result struct {
		path string
		free int64
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		if dm == nil {
			ch <- result{err: errors.New("下载器管理器没有初始化")}
			return
		}
		dl, err := dm.GetDownloaderContext(ctx, rec.Name)
		if err != nil {
			ch <- result{err: err}
			return
		}
		paths, err := dl.GetClientPaths()
		if err != nil {
			ch <- result{err: err}
			return
		}
		r := result{free: -1}
		if len(paths) > 0 {
			r.path = strings.TrimSpace(paths[0])
		}
		if withFree {
			if f, ferr := dl.GetClientFreeSpace(ctx); ferr == nil {
				r.free = f
			}
		}
		ch <- r
	}()
	select {
	case r := <-ch:
		d.path, d.free, d.err = r.path, r.free, r.err
	case <-ctx.Done():
		d.err = errors.New("下载器没有回应")
		return d
	}
	if d.err == nil && d.path == "" {
		d.err = errors.New("下载器没给默认保存路径")
	}
	if d.err != nil {
		return d
	}
	d.local = d.path
	var maps []models.MediaPathMap
	if err := db.WithContext(ctx).Where("downloader_id = ?", rec.ID).Find(&maps).Error; err == nil {
		conv := make([]models.DownloaderPathMap, 0, len(maps))
		for _, m := range maps {
			conv = append(conv, models.DownloaderPathMap{SourcePrefix: m.DownloaderPrefix, TargetPrefix: m.LocalPrefix})
		}
		d.local, _ = models.MapTransferPath(conv, d.path)
	}
	return d
}

// savePaths 是体检比硬链接库用的各下载器的默认保存路径（取不到的不列）。
func (s *Server) savePaths(ctx context.Context, db *gorm.DB) []health.SavePath {
	var out []health.SavePath
	for _, d := range s.downloadDirs(ctx, db, false) {
		if d.err == nil {
			out = append(out, health.SavePath{Downloader: d.name, Path: d.local})
		}
	}
	return out
}

// diskPaths 是体检看剩余空间的目录：数据目录、备份目录、各媒体库、路径映射的本地目录与各下载器的默认保存路径。
func (s *Server) diskPaths(ctx context.Context, db *gorm.DB) []health.DiskPath {
	var out []health.DiskPath
	if dir, err := core.DataDir(); err == nil {
		out = append(out, health.DiskPath{Label: "数据目录", Path: dir})
	}
	if s.store != nil {
		if cfg, err := s.store.BackupSettings(); err == nil {
			out = append(out, health.DiskPath{Label: "备份目录", Path: cfg.BackupDir()})
		}
	}
	var libs []models.MediaLibrary
	if err := db.WithContext(ctx).Order("id").Find(&libs).Error; err == nil {
		for _, l := range libs {
			out = append(out, health.DiskPath{Label: "媒体库「" + l.Name + "」", Path: l.Path})
		}
	}
	names := map[uint]string{}
	var dls []models.DownloaderSetting
	if err := db.WithContext(ctx).Select("id", "name").Find(&dls).Error; err == nil {
		for _, d := range dls {
			names[d.ID] = d.Name
		}
	}
	var maps []models.MediaPathMap
	if err := db.WithContext(ctx).Order("downloader_id, id").Find(&maps).Error; err == nil {
		for _, m := range maps {
			out = append(out, health.DiskPath{Label: "下载目录（" + names[m.DownloaderID] + "）", Path: m.LocalPrefix})
		}
	}
	for _, d := range s.downloadDirs(ctx, db, false) {
		if d.err == nil {
			out = append(out, health.DiskPath{Label: "下载目录（" + d.name + "）", Path: d.local})
		}
	}
	return out
}

// backupState 是备份的设置与最近一次的结果。
func (s *Server) backupState() (health.BackupState, error) {
	if s.store == nil {
		return health.BackupState{}, errors.New("配置存储未初始化")
	}
	cfg, err := s.store.BackupSettings()
	if err != nil {
		return health.BackupState{}, err
	}
	st := health.BackupState{Enabled: cfg.Enabled}
	if last := cfg.Last; last != nil {
		at := last.At
		st.LastAt, st.LastName, st.LastError, st.LastOKAt = &at, last.Name, last.Error, last.LastOKAt
	}
	return st, nil
}
