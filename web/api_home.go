package web

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/health"
	"github.com/sunerpy/pt-tools/internal/home"
	"github.com/sunerpy/pt-tools/internal/media/server"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 首页（媒体中心 M8）：今天播出与这周还要播、待处理、继续观看、最近入库与 NAS 状态，每块单独给数据或错误（见 internal/home）。
// 只认会话。结果缓存 30 秒（?refresh=1 不用缓存）；速度柱的新采样跟着外壳每 30 秒取的 transfer-stats 来。

const (
	// homeCacheTTL 是首页结果缓存多久
	homeCacheTTL = 30 * time.Second
	// homePartTimeout 是一块最多等多久
	homePartTimeout = 15 * time.Second
	// homeListTimeout 是取一台下载器的种子列表最多等多久
	homeListTimeout = 10 * time.Second
)

// homeResponse 是首页的各块：每块是 {data, error}。
type homeResponse struct {
	At          time.Time  `json:"at"`
	Calendar    home.Block `json:"calendar"`
	Todo        home.Block `json:"todo"`
	Resume      home.Block `json:"resume"`
	Recent      home.Block `json:"recent"`
	Downloaders home.Block `json:"downloaders"`
	Storage     home.Block `json:"storage"`
	Status      home.Block `json:"status"`
}

// homeCache 是上一次的首页结果；同时来的几个请求等同一次算完。体检跑完时 stale 加一：在这之前算的那份不再用
// （待处理与体检那一行要用新结果），不用等正在算的那一次。
type homeCache struct {
	mu    sync.Mutex
	at    time.Time
	resp  *homeResponse
	gen   uint64
	stale atomic.Uint64
}

// homeDownloader 是首页 NAS 状态里的一台下载器：速度同 transfer-stats，下载中与做种是种子列表里的个数。
type homeDownloader struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Online      bool   `json:"online"`
	Down        int64  `json:"down"`
	Up          int64  `json:"up"`
	Downloading int    `json:"downloading"`
	Seeding     int    `json:"seeding"`
	Error       string `json:"error,omitempty"`
}

// homeDownloaders 是首页 NAS 状态的速度与下载器：总速度、下载中与做种的总数，以及最近的总速度采样。
type homeDownloaders struct {
	Downloaders  []homeDownloader `json:"downloaders"`
	Down         int64            `json:"down"`
	Up           int64            `json:"up"`
	Downloading  int              `json:"downloading"`
	Seeding      int              `json:"seeding"`
	SpeedHistory []SpeedSample    `json:"speed_history"`
}

// registerHomeRoutes 挂上首页与系统体检的接口。
func (s *Server) registerHomeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/home", s.auth(s.apiHome))
	s.registerSystemCheckRoutes(mux)
}

func (s *Server) apiHome(w http.ResponseWriter, r *http.Request) {
	db, ok := s.healthDB(w)
	if !ok {
		return
	}
	writeJSON(w, s.homeData(r.Context(), db, r.URL.Query().Get("refresh") == "1"))
}

// homeData 算首页的各块（30 秒内算过的直接给）。算的时候不受这一个请求的取消影响：同时来的请求在等它。
func (s *Server) homeData(ctx context.Context, db *gorm.DB, refresh bool) *homeResponse {
	c := &s.homeCache
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	gen := c.stale.Load()
	if !refresh && c.resp != nil && c.gen == gen && now.Sub(c.at) < homeCacheTTL {
		return c.resp
	}
	ctx = context.WithoutCancel(ctx)
	last, err := health.Last(ctx, db)
	if err != nil {
		last = nil
	}
	blocks := home.Collect(ctx, []home.Part{
		{Name: "calendar", Get: func(ctx context.Context) (any, error) {
			if s.subscriber == nil {
				return nil, errors.New("订阅服务没有启动")
			}
			return s.subscriber.HomeCalendar(ctx)
		}},
		{Name: "todo", Get: func(ctx context.Context) (any, error) {
			return home.BuildTodo(ctx, home.TodoInput{DB: db, Now: now, Backup: s.backupState, Health: last})
		}},
		{Name: "resume", Get: func(ctx context.Context) (any, error) {
			if s.organizer == nil {
				return nil, errors.New("整理入库服务没有启动")
			}
			return s.organizer.Resume(ctx)
		}},
		{Name: "recent", Get: func(ctx context.Context) (any, error) {
			if s.organizer == nil {
				return nil, errors.New("整理入库服务没有启动")
			}
			return s.organizer.Recent(ctx)
		}},
		{Name: "downloaders", Get: func(ctx context.Context) (any, error) { return s.homeDownloaders(ctx) }},
		{Name: "storage", Get: func(ctx context.Context) (any, error) { return s.homeStorage(ctx, db) }},
		{Name: "status", Get: func(ctx context.Context) (any, error) {
			return home.BuildStatus(ctx, home.StatusInput{
				DB: db, Now: now, Health: last, Backup: s.backupState, Test: s.healthServerTest, HomeURL: serverHomeURL,
			})
		}},
	}, homePartTimeout)
	resp := &homeResponse{
		At: now, Calendar: blocks["calendar"], Todo: blocks["todo"], Resume: blocks["resume"], Recent: blocks["recent"],
		Downloaders: blocks["downloaders"], Storage: blocks["storage"], Status: blocks["status"],
	}
	c.at, c.resp, c.gen = now, resp, gen
	return resp
}

// serverHomeURL 是媒体服务器网页的首页（浏览器打开用的地址，没填时用连接地址）。
func serverHomeURL(srv models.MediaServer) string {
	base := srv.PlayURL
	if base == "" {
		base = srv.URL
	}
	return server.HomeURL(srv.Kind, base)
}

// homeDownloaders 读各台启用的下载器的速度（同 transfer-stats，也记一条速度采样）与下载中、做种的个数。
func (s *Server) homeDownloaders(ctx context.Context) (*homeDownloaders, error) {
	records, err := s.listEnabledDownloaderRecords(nil)
	if err != nil {
		return nil, err
	}
	dm := s.getDownloaderManager()
	if dm == nil {
		return nil, errors.New("下载器管理器没有初始化")
	}
	stats := s.collectTransferStats(ctx, dm, records)
	byID := make(map[uint]DownloaderTransferStatItem, len(stats.Downloaders))
	for _, it := range stats.Downloaders {
		byID[it.DownloaderID] = it
	}
	out := &homeDownloaders{Downloaders: make([]homeDownloader, len(records)), Down: stats.TotalDownloadSpeed, Up: stats.TotalUploadSpeed}
	var wg sync.WaitGroup
	for i, rec := range records {
		d := homeDownloader{ID: rec.ID, Name: rec.Name, Type: rec.Type}
		it, ok := byID[rec.ID]
		switch {
		case !ok:
			d.Error = "取不到下载器"
		case !it.Reachable:
			d.Error = it.Error
		default:
			d.Online, d.Down, d.Up = true, it.DownloadSpeed, it.UploadSpeed
		}
		out.Downloaders[i] = d
		if !d.Online {
			continue
		}
		wg.Go(func() {
			downloading, seeding, err := torrentCounts(ctx, dm, rec.Name)
			if err != nil {
				out.Downloaders[i].Error = "读不到种子列表：" + err.Error()
				return
			}
			out.Downloaders[i].Downloading, out.Downloaders[i].Seeding = downloading, seeding
		})
	}
	wg.Wait()
	for _, d := range out.Downloaders {
		out.Downloading += d.Downloading
		out.Seeding += d.Seeding
	}
	out.SpeedHistory = s.speeds.snapshot()
	return out, nil
}

// torrentCounts 数一台下载器里下载中与做种的种子（最多等 10 秒）。
func torrentCounts(ctx context.Context, dm *downloader.DownloaderManager, name string) (downloading, seeding int, err error) {
	ctx, cancel := context.WithTimeout(ctx, homeListTimeout)
	defer cancel()
	type result struct {
		list []downloader.Torrent
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		dl, err := acquireDownloader(ctx, dm, name)
		if err != nil {
			ch <- result{err: err}
			return
		}
		list, err := dl.GetAllTorrents()
		ch <- result{list, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return 0, 0, r.err
		}
		for _, t := range r.list {
			switch t.State {
			case downloader.TorrentDownloading:
				downloading++
			case downloader.TorrentSeeding:
				seeding++
			}
		}
		return downloading, seeding, nil
	case <-ctx.Done():
		return 0, 0, errors.New("下载器没有回应")
	}
}

// homeStorage 是首页 NAS 状态的存储：各下载器的默认保存路径与各媒体库所在的盘。
func (s *Server) homeStorage(ctx context.Context, db *gorm.DB) ([]home.Disk, error) {
	var libs []models.MediaLibrary
	if err := db.WithContext(ctx).Order("id").Find(&libs).Error; err != nil {
		return nil, err
	}
	dirs := s.downloadDirs(ctx, db, true)
	downloads := make([]home.DownloadDir, 0, len(dirs))
	for _, d := range dirs {
		downloads = append(downloads, home.DownloadDir{Downloader: d.name, Path: d.path, Local: d.local, Free: d.free, Err: d.err})
	}
	list := make([]home.LibraryDir, 0, len(libs))
	for _, l := range libs {
		list = append(list, home.LibraryDir{Name: l.Name, Path: l.Path})
	}
	return home.Storage(downloads, list), nil
}
