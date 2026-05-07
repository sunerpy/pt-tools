package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

const (
	defaultNfoDialect = "universal"

	stageParsing          = "parsing"
	stageSearching        = "searching"
	stageFetching         = "fetching"
	stageFusing           = "fusing"
	stageWritingNFO       = "writing_nfo"
	stageDownloadingArt   = "downloading_art"
	stageRefreshingServer = "refreshing_server"
	stageDone             = "done"
)

// ScrapeService 组合所有子系统，暴露高层 Scrape* 方法。
type ScrapeService struct {
	db           *gorm.DB
	sourceReg    *core.Registry[core.MediaScraper]
	writerReg    *core.Registry[core.NfoWriter]
	connectorReg *core.Registry[core.MediaServerConnector]
	fuser        core.Fuser
	downloader   *ArtworkDownloader
	queue        *PersistentQueue
	logger       Logger
}

// Logger 抽象接口，避免硬绑定具体实现。
type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// ServiceConfig 构造 ScrapeService 的依赖。
type ServiceConfig struct {
	DB           *gorm.DB
	SourceReg    *core.Registry[core.MediaScraper]
	WriterReg    *core.Registry[core.NfoWriter]
	ConnectorReg *core.Registry[core.MediaServerConnector]
	Fuser        core.Fuser
	Downloader   *ArtworkDownloader
	Queue        *PersistentQueue
	Logger       Logger
}

// ScrapeMovieRequest 电影刮削请求。
type ScrapeMovieRequest struct {
	LibraryID    *uint
	MediaPath    string
	Title        string
	Year         int
	Providers    []string
	Locale       string
	NfoDialect   string
	ConnectorID  *uint
	OverwriteNFO bool

	taskID uint
}

// ScrapeTvShowRequest 剧集目录刮削请求。
type ScrapeTvShowRequest struct {
	LibraryID    *uint
	MediaPath    string
	Title        string
	Year         int
	Providers    []string
	Locale       string
	NfoDialect   string
	ConnectorID  *uint
	OverwriteNFO bool

	taskID uint
}

// ScrapeEpisodeRequest 单集刮削请求。
type ScrapeEpisodeRequest struct {
	LibraryID    *uint
	MediaPath    string
	Title        string
	Year         int
	Season       int
	Episode      int
	Providers    []string
	Locale       string
	NfoDialect   string
	ConnectorID  *uint
	OverwriteNFO bool

	taskID uint
}

// ScrapeResult 单次刮削的结果摘要。
type ScrapeResult struct {
	Success      bool
	Type         string
	MediaPath    string
	NfoPath      string
	PosterPath   string
	Title        string
	Year         int
	Providers    []string
	CurrentStage string
	FailedStage  string
	ErrorMessage string
	StartedAt    time.Time
	CompletedAt  time.Time

	libraryID  *uint
	taskID     uint
	fanartPath string
}

// NewScrapeService 构造业务编排服务。
func NewScrapeService(cfg ServiceConfig) (*ScrapeService, error) {
	if cfg.DB == nil {
		return nil, errors.New("nil db")
	}
	if cfg.SourceReg == nil {
		return nil, errors.New("nil SourceReg")
	}
	if cfg.WriterReg == nil {
		return nil, errors.New("nil WriterReg")
	}
	if cfg.Fuser == nil {
		return nil, errors.New("nil Fuser")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = noopLogger{}
	}

	return &ScrapeService{
		db:           cfg.DB,
		sourceReg:    cfg.SourceReg,
		writerReg:    cfg.WriterReg,
		connectorReg: cfg.ConnectorReg,
		fuser:        cfg.Fuser,
		downloader:   cfg.Downloader,
		queue:        cfg.Queue,
		logger:       logger,
	}, nil
}

// SetQueue 在 NewScrapeService 之后注入持久化队列（解决 queue/taskBuilder
// 循环依赖：queue 构造时需要 service.TaskBuilder()，反过来 service 入队又
// 需要 queue 实例）。嵌入模式 web/server_scraper.go 会先构造 service，再
// 构造 queue 并回填。
func (s *ScrapeService) SetQueue(q *PersistentQueue) {
	s.queue = q
}

// EnqueueMovie 将电影刮削任务提交到持久化队列。
func (s *ScrapeService) EnqueueMovie(ctx context.Context, req ScrapeMovieRequest) (*store.ScrapeTask, error) {
	if s.queue == nil {
		return nil, errors.New("queue not configured")
	}
	return s.queue.Enqueue(ctx, "movie", req.MediaPath, req.LibraryID, req)
}

// EnqueueTvShow 将剧集刮削任务提交到持久化队列。
func (s *ScrapeService) EnqueueTvShow(ctx context.Context, req ScrapeTvShowRequest) (*store.ScrapeTask, error) {
	if s.queue == nil {
		return nil, errors.New("queue not configured")
	}
	return s.queue.Enqueue(ctx, "tv", req.MediaPath, req.LibraryID, req)
}

// EnqueueEpisode 将单集刮削任务提交到持久化队列。
func (s *ScrapeService) EnqueueEpisode(ctx context.Context, req ScrapeEpisodeRequest) (*store.ScrapeTask, error) {
	if s.queue == nil {
		return nil, errors.New("queue not configured")
	}
	return s.queue.Enqueue(ctx, "episode", req.MediaPath, req.LibraryID, req)
}

// TaskBuilder 返回给 PersistentQueue 使用的任务恢复构造器。
func (s *ScrapeService) TaskBuilder() func(store.ScrapeTask) core.Task {
	return func(record store.ScrapeTask) core.Task {
		switch record.TaskType {
		case "movie":
			var req ScrapeMovieRequest
			if err := json.Unmarshal([]byte(record.RequestData), &req); err != nil {
				return nil
			}
			req.taskID = record.ID
			if req.LibraryID == nil {
				req.LibraryID = record.LibraryID
			}
			return NewMovieScrapeTask(record, s, req)
		case "tv":
			var req ScrapeTvShowRequest
			if err := json.Unmarshal([]byte(record.RequestData), &req); err != nil {
				return nil
			}
			req.taskID = record.ID
			if req.LibraryID == nil {
				req.LibraryID = record.LibraryID
			}
			return NewTvShowScrapeTask(record, s, req)
		case "episode":
			var req ScrapeEpisodeRequest
			if err := json.Unmarshal([]byte(record.RequestData), &req); err != nil {
				return nil
			}
			req.taskID = record.ID
			if req.LibraryID == nil {
				req.LibraryID = record.LibraryID
			}
			return NewEpisodeScrapeTask(record, s, req)
		default:
			return nil
		}
	}
}

// ScrapeMovie 执行电影端到端刮削。
func (s *ScrapeService) ScrapeMovie(ctx context.Context, req ScrapeMovieRequest) (*ScrapeResult, error) {
	result := s.newResult("movie", req.MediaPath, req.LibraryID, req.taskID)

	if err := s.resolveMovieDefaults(ctx, &req); err != nil {
		return s.fail(ctx, result, stageParsing, err)
	}
	if strings.TrimSpace(req.MediaPath) == "" {
		return s.fail(ctx, result, stageParsing, errors.New("empty media path"))
	}

	if req.Title == "" {
		s.setStage(ctx, result, stageParsing)
		parsed, err := ParseFilename(req.MediaPath)
		if err != nil {
			return s.fail(ctx, result, stageParsing, err)
		}
		req.Title = parsed.Title
		if req.Year == 0 {
			req.Year = parsed.Year
		}
	}
	if strings.TrimSpace(req.Title) == "" {
		return s.fail(ctx, result, stageParsing, errors.New("empty title after parsing"))
	}

	result.Title = req.Title
	result.Year = req.Year

	rawResults, usedProviders, err := s.collectMovieMetadata(ctx, result, req)
	if err != nil {
		return s.fail(ctx, result, stageSearching, err)
	}
	result.Providers = usedProviders

	s.setStage(ctx, result, stageFusing)
	fused, err := s.fuser.Merge(ctx, rawResults)
	if err != nil {
		return s.fail(ctx, result, stageFusing, err)
	}
	if fused != nil {
		fused.Provider = "fused"
		if fused.ScrapedAt.IsZero() {
			fused.ScrapedAt = time.Now()
		}
	}

	if err := s.writeMovieNFO(ctx, result, req, fused); err != nil {
		return s.fail(ctx, result, stageWritingNFO, err)
	}
	if err := s.downloadMovieArtwork(ctx, result, req.MediaPath, fused); err != nil {
		s.logger.Warnf("download movie artwork: %v", err)
	}
	if err := s.refreshConnector(ctx, result, req.ConnectorID); err != nil {
		return s.fail(ctx, result, stageRefreshingServer, err)
	}

	result.Success = true
	s.setStage(ctx, result, stageDone)
	result.CompletedAt = time.Now()
	if err := s.persistResult(ctx, result, fused); err != nil {
		s.logger.Warnf("persist scrape result: %v", err)
	}
	return result, nil
}

// ScrapeTvShow 执行剧集目录端到端刮削。
func (s *ScrapeService) ScrapeTvShow(ctx context.Context, req ScrapeTvShowRequest) (*ScrapeResult, error) {
	result := s.newResult("tv", req.MediaPath, req.LibraryID, req.taskID)

	if err := s.resolveTvDefaults(ctx, &req); err != nil {
		return s.fail(ctx, result, stageParsing, err)
	}
	if strings.TrimSpace(req.MediaPath) == "" {
		return s.fail(ctx, result, stageParsing, errors.New("empty media path"))
	}
	if req.Title == "" {
		s.setStage(ctx, result, stageParsing)
		parsed, err := ParseFilename(req.MediaPath)
		if err != nil {
			return s.fail(ctx, result, stageParsing, err)
		}
		req.Title = parsed.Title
		if req.Year == 0 {
			req.Year = parsed.Year
		}
	}
	if strings.TrimSpace(req.Title) == "" {
		return s.fail(ctx, result, stageParsing, errors.New("empty title after parsing"))
	}

	result.Title = req.Title
	result.Year = req.Year

	rawResults, usedProviders, err := s.collectTvMetadata(ctx, result, req)
	if err != nil {
		return s.fail(ctx, result, stageSearching, err)
	}
	result.Providers = usedProviders

	s.setStage(ctx, result, stageFusing)
	fused, err := s.fuser.MergeTv(ctx, rawResults)
	if err != nil {
		return s.fail(ctx, result, stageFusing, err)
	}
	if fused != nil {
		fused.Provider = "fused"
		if fused.ScrapedAt.IsZero() {
			fused.ScrapedAt = time.Now()
		}
	}

	if err := s.writeTvShowNFO(ctx, result, req, fused); err != nil {
		return s.fail(ctx, result, stageWritingNFO, err)
	}
	if err := s.downloadShowArtwork(ctx, result, req.MediaPath, fused); err != nil {
		s.logger.Warnf("download tv artwork: %v", err)
	}
	if err := s.refreshConnector(ctx, result, req.ConnectorID); err != nil {
		return s.fail(ctx, result, stageRefreshingServer, err)
	}

	result.Success = true
	s.setStage(ctx, result, stageDone)
	result.CompletedAt = time.Now()
	if err := s.persistResult(ctx, result, fused); err != nil {
		s.logger.Warnf("persist scrape result: %v", err)
	}
	return result, nil
}

// ScrapeEpisode 执行单集端到端刮削。
func (s *ScrapeService) ScrapeEpisode(ctx context.Context, req ScrapeEpisodeRequest) (*ScrapeResult, error) {
	result := s.newResult("episode", req.MediaPath, req.LibraryID, req.taskID)

	if err := s.resolveEpisodeDefaults(ctx, &req); err != nil {
		return s.fail(ctx, result, stageParsing, err)
	}
	if strings.TrimSpace(req.MediaPath) == "" {
		return s.fail(ctx, result, stageParsing, errors.New("empty media path"))
	}
	if req.Title == "" || req.Season == 0 || req.Episode == 0 {
		s.setStage(ctx, result, stageParsing)
		parsed, err := ParseFilename(req.MediaPath)
		if err != nil {
			return s.fail(ctx, result, stageParsing, err)
		}
		if req.Title == "" {
			req.Title = parsed.Title
		}
		if req.Year == 0 {
			req.Year = parsed.Year
		}
		if req.Season == 0 {
			req.Season = parsed.Season
		}
		if req.Episode == 0 {
			req.Episode = parsed.Episode
		}
	}
	if strings.TrimSpace(req.Title) == "" {
		return s.fail(ctx, result, stageParsing, errors.New("empty title after parsing"))
	}
	if req.Season <= 0 || req.Episode <= 0 {
		return s.fail(ctx, result, stageParsing, errors.New("season/episode required"))
	}

	result.Title = req.Title
	result.Year = req.Year

	rawResults, usedProviders, err := s.collectEpisodeMetadata(ctx, result, req)
	if err != nil {
		return s.fail(ctx, result, stageSearching, err)
	}
	result.Providers = usedProviders

	s.setStage(ctx, result, stageFusing)
	fused, err := s.fuser.MergeEpisode(ctx, rawResults)
	if err != nil {
		return s.fail(ctx, result, stageFusing, err)
	}
	if fused != nil {
		fused.Provider = "fused"
		if fused.ScrapedAt.IsZero() {
			fused.ScrapedAt = time.Now()
		}
	}

	if err := s.writeEpisodeNFO(ctx, result, req, fused); err != nil {
		return s.fail(ctx, result, stageWritingNFO, err)
	}
	if err := s.refreshConnector(ctx, result, req.ConnectorID); err != nil {
		return s.fail(ctx, result, stageRefreshingServer, err)
	}

	result.Success = true
	s.setStage(ctx, result, stageDone)
	result.CompletedAt = time.Now()
	if err := s.persistResult(ctx, result, fused); err != nil {
		s.logger.Warnf("persist scrape result: %v", err)
	}
	return result, nil
}

func (s *ScrapeService) collectMovieMetadata(
	ctx context.Context,
	result *ScrapeResult,
	req ScrapeMovieRequest,
) (map[string]*core.RawMediaInfo, []string, error) {
	s.setStage(ctx, result, stageSearching)
	rawResults := make(map[string]*core.RawMediaInfo, len(req.Providers))
	usedProviders := make([]string, 0, len(req.Providers))

	for _, name := range req.Providers {
		scraper, err := s.sourceReg.Get(name)
		if err != nil {
			s.logger.Warnf("scraper %s not found: %v", name, err)
			continue
		}
		if !scraper.IsActive() {
			s.logger.Warnf("scraper %s inactive", name)
			continue
		}
		movie, ok := scraper.(core.MovieMetadataScraper)
		if !ok {
			continue
		}
		candidates, err := movie.SearchMovie(ctx, core.MovieSearchOptions{
			Query:    req.Title,
			Year:     req.Year,
			Language: req.Locale,
		})
		if err != nil {
			s.logger.Warnf("search %s: %v", name, err)
			continue
		}
		if len(candidates) == 0 {
			continue
		}

		s.setStage(ctx, result, stageFetching)
		candidate := candidates[0]
		metadata, err := movie.GetMovieMetadata(ctx, core.MovieSearchOptions{
			Query:    req.Title,
			Year:     req.Year,
			Language: req.Locale,
			TMDBID:   parseIntID(candidate.ID),
		})
		if err != nil {
			s.logger.Warnf("get %s: %v", name, err)
			continue
		}
		rawResults[name] = &core.RawMediaInfo{Provider: name, Data: metadata, SearchResult: candidate}
		usedProviders = append(usedProviders, name)
	}

	if len(rawResults) == 0 {
		return nil, nil, errors.New("no provider returned results")
	}
	return rawResults, usedProviders, nil
}

func (s *ScrapeService) collectTvMetadata(
	ctx context.Context,
	result *ScrapeResult,
	req ScrapeTvShowRequest,
) (map[string]*core.RawMediaInfo, []string, error) {
	s.setStage(ctx, result, stageSearching)
	rawResults := make(map[string]*core.RawMediaInfo, len(req.Providers))
	usedProviders := make([]string, 0, len(req.Providers))

	for _, name := range req.Providers {
		scraper, err := s.sourceReg.Get(name)
		if err != nil {
			s.logger.Warnf("scraper %s not found: %v", name, err)
			continue
		}
		if !scraper.IsActive() {
			continue
		}
		show, ok := scraper.(core.TvShowMetadataScraper)
		if !ok {
			continue
		}
		candidates, err := show.SearchTvShow(ctx, core.TvShowSearchOptions{
			Query:    req.Title,
			Year:     req.Year,
			Language: req.Locale,
		})
		if err != nil {
			s.logger.Warnf("search %s: %v", name, err)
			continue
		}
		if len(candidates) == 0 {
			continue
		}

		s.setStage(ctx, result, stageFetching)
		candidate := candidates[0]
		metadata, err := show.GetTvShowMetadata(ctx, core.TvShowSearchOptions{
			Query:    req.Title,
			Year:     req.Year,
			Language: req.Locale,
			TMDBID:   parseIntID(candidate.ID),
		})
		if err != nil {
			s.logger.Warnf("get %s: %v", name, err)
			continue
		}
		rawResults[name] = &core.RawMediaInfo{Provider: name, Data: metadata, SearchResult: candidate}
		usedProviders = append(usedProviders, name)
	}

	if len(rawResults) == 0 {
		return nil, nil, errors.New("no provider returned results")
	}
	return rawResults, usedProviders, nil
}

func (s *ScrapeService) collectEpisodeMetadata(
	ctx context.Context,
	result *ScrapeResult,
	req ScrapeEpisodeRequest,
) (map[string]*core.RawMediaInfo, []string, error) {
	s.setStage(ctx, result, stageSearching)
	rawResults := make(map[string]*core.RawMediaInfo, len(req.Providers))
	usedProviders := make([]string, 0, len(req.Providers))

	for _, name := range req.Providers {
		scraper, err := s.sourceReg.Get(name)
		if err != nil {
			s.logger.Warnf("scraper %s not found: %v", name, err)
			continue
		}
		if !scraper.IsActive() {
			continue
		}
		show, ok := scraper.(core.TvShowMetadataScraper)
		if !ok {
			continue
		}
		candidates, err := show.SearchTvShow(ctx, core.TvShowSearchOptions{
			Query:    req.Title,
			Year:     req.Year,
			Language: req.Locale,
		})
		if err != nil {
			s.logger.Warnf("search %s: %v", name, err)
			continue
		}
		if len(candidates) == 0 {
			continue
		}

		s.setStage(ctx, result, stageFetching)
		candidate := candidates[0]
		metadata, err := show.GetEpisodeMetadata(ctx, core.TvShowEpisodeSearchOptions{
			TvShowID: parseIntID(candidate.ID),
			Season:   req.Season,
			Episode:  req.Episode,
			Language: req.Locale,
		})
		if err != nil {
			s.logger.Warnf("get episode %s: %v", name, err)
			continue
		}
		rawResults[name] = &core.RawMediaInfo{Provider: name, Data: metadata, SearchResult: candidate}
		usedProviders = append(usedProviders, name)
	}

	if len(rawResults) == 0 {
		return nil, nil, errors.New("no provider returned results")
	}
	return rawResults, usedProviders, nil
}

func (s *ScrapeService) writeMovieNFO(ctx context.Context, result *ScrapeResult, req ScrapeMovieRequest, fused *core.Movie) error {
	s.setStage(ctx, result, stageWritingNFO)
	dialect := req.NfoDialect
	if dialect == "" {
		dialect = defaultNfoDialect
	}
	writer, err := s.writerReg.Get(dialect)
	if err != nil {
		return fmt.Errorf("writer %s: %w", dialect, err)
	}

	nfoPath := strings.TrimSuffix(req.MediaPath, filepath.Ext(req.MediaPath)) + ".nfo"
	movieNfoPath := filepath.Join(filepath.Dir(req.MediaPath), "movie.nfo")
	if err := ensureWritable(req.OverwriteNFO, nfoPath, movieNfoPath); err != nil {
		return err
	}
	if err := writer.WriteMovieNfo(ctx, fused, []string{nfoPath, movieNfoPath}); err != nil {
		return err
	}
	result.NfoPath = nfoPath
	return nil
}

func (s *ScrapeService) writeTvShowNFO(ctx context.Context, result *ScrapeResult, req ScrapeTvShowRequest, fused *core.TvShow) error {
	s.setStage(ctx, result, stageWritingNFO)
	dialect := req.NfoDialect
	if dialect == "" {
		dialect = defaultNfoDialect
	}
	writer, err := s.writerReg.Get(dialect)
	if err != nil {
		return fmt.Errorf("writer %s: %w", dialect, err)
	}
	showDir := filepath.Dir(req.MediaPath)
	nfoPath := filepath.Join(showDir, "tvshow.nfo")
	if err := ensureWritable(req.OverwriteNFO, nfoPath); err != nil {
		return err
	}
	if err := writer.WriteTvShowNfo(ctx, fused, showDir); err != nil {
		return err
	}
	result.NfoPath = nfoPath
	return nil
}

func (s *ScrapeService) writeEpisodeNFO(ctx context.Context, result *ScrapeResult, req ScrapeEpisodeRequest, fused *core.TvShowEpisode) error {
	s.setStage(ctx, result, stageWritingNFO)
	dialect := req.NfoDialect
	if dialect == "" {
		dialect = defaultNfoDialect
	}
	writer, err := s.writerReg.Get(dialect)
	if err != nil {
		return fmt.Errorf("writer %s: %w", dialect, err)
	}
	nfoPath := strings.TrimSuffix(req.MediaPath, filepath.Ext(req.MediaPath)) + ".nfo"
	if err := ensureWritable(req.OverwriteNFO, nfoPath); err != nil {
		return err
	}
	if err := writer.WriteEpisodeNfo(ctx, fused, nfoPath); err != nil {
		return err
	}
	result.NfoPath = nfoPath
	return nil
}

func (s *ScrapeService) downloadMovieArtwork(ctx context.Context, result *ScrapeResult, mediaPath string, fused *core.Movie) error {
	if s.downloader == nil || fused == nil || len(fused.ArtworkURLs) == 0 {
		return nil
	}
	s.setStage(ctx, result, stageDownloadingArt)
	artworkDir := filepath.Dir(mediaPath)
	if err := s.downloader.DownloadArtworks(ctx, artworkMapToSlice(fused.ArtworkURLs), artworkDir); err != nil {
		return err
	}
	result.PosterPath = filepath.Join(artworkDir, "poster.jpg")
	result.fanartPath = filepath.Join(artworkDir, "fanart.jpg")
	return nil
}

func (s *ScrapeService) downloadShowArtwork(ctx context.Context, result *ScrapeResult, mediaPath string, fused *core.TvShow) error {
	if s.downloader == nil || fused == nil || len(fused.ArtworkURLs) == 0 {
		return nil
	}
	s.setStage(ctx, result, stageDownloadingArt)
	artworkDir := filepath.Dir(mediaPath)
	if err := s.downloader.DownloadArtworks(ctx, artworkMapToSlice(fused.ArtworkURLs), artworkDir); err != nil {
		return err
	}
	result.PosterPath = filepath.Join(artworkDir, "poster.jpg")
	result.fanartPath = filepath.Join(artworkDir, "fanart.jpg")
	return nil
}

func (s *ScrapeService) refreshConnector(ctx context.Context, result *ScrapeResult, connectorID *uint) error {
	if connectorID == nil {
		return nil
	}
	if s.connectorReg == nil {
		return errors.New("connector registry not configured")
	}
	var cfg store.ConnectorConfig
	if err := s.db.WithContext(ctx).First(&cfg, *connectorID).Error; err != nil {
		return fmt.Errorf("load connector %d: %w", *connectorID, err)
	}
	connector, err := s.connectorReg.Get(cfg.Type)
	if err != nil {
		return fmt.Errorf("connector %s: %w", cfg.Type, err)
	}
	s.setStage(ctx, result, stageRefreshingServer)
	return connector.RefreshLibrary(ctx, "")
}

func (s *ScrapeService) resolveMovieDefaults(ctx context.Context, req *ScrapeMovieRequest) error {
	if req == nil {
		return errors.New("nil request")
	}
	lib, err := s.resolveLibrary(ctx, req.LibraryID)
	if err != nil {
		return err
	}
	if len(req.Providers) == 0 {
		if lib != nil {
			req.Providers = splitCSV(lib.ProviderIDs)
		}
		if len(req.Providers) == 0 {
			req.Providers = s.sourceReg.List()
		}
	}
	if req.NfoDialect == "" && lib != nil && lib.NfoDialect != "" {
		req.NfoDialect = lib.NfoDialect
	}
	if req.ConnectorID == nil && lib != nil {
		req.ConnectorID = lib.ConnectorID
	}
	req.Providers = dedupeNonEmpty(req.Providers)
	return nil
}

func (s *ScrapeService) resolveTvDefaults(ctx context.Context, req *ScrapeTvShowRequest) error {
	if req == nil {
		return errors.New("nil request")
	}
	lib, err := s.resolveLibrary(ctx, req.LibraryID)
	if err != nil {
		return err
	}
	if len(req.Providers) == 0 {
		if lib != nil {
			req.Providers = splitCSV(lib.ProviderIDs)
		}
		if len(req.Providers) == 0 {
			req.Providers = s.sourceReg.List()
		}
	}
	if req.NfoDialect == "" && lib != nil && lib.NfoDialect != "" {
		req.NfoDialect = lib.NfoDialect
	}
	if req.ConnectorID == nil && lib != nil {
		req.ConnectorID = lib.ConnectorID
	}
	req.Providers = dedupeNonEmpty(req.Providers)
	return nil
}

func (s *ScrapeService) resolveEpisodeDefaults(ctx context.Context, req *ScrapeEpisodeRequest) error {
	if req == nil {
		return errors.New("nil request")
	}
	lib, err := s.resolveLibrary(ctx, req.LibraryID)
	if err != nil {
		return err
	}
	if len(req.Providers) == 0 {
		if lib != nil {
			req.Providers = splitCSV(lib.ProviderIDs)
		}
		if len(req.Providers) == 0 {
			req.Providers = s.sourceReg.List()
		}
	}
	if req.NfoDialect == "" && lib != nil && lib.NfoDialect != "" {
		req.NfoDialect = lib.NfoDialect
	}
	if req.ConnectorID == nil && lib != nil {
		req.ConnectorID = lib.ConnectorID
	}
	req.Providers = dedupeNonEmpty(req.Providers)
	return nil
}

func (s *ScrapeService) resolveLibrary(ctx context.Context, libraryID *uint) (*store.MediaLibraryConfig, error) {
	if libraryID == nil || *libraryID == 0 {
		return nil, nil
	}
	var lib store.MediaLibraryConfig
	if err := s.db.WithContext(ctx).First(&lib, *libraryID).Error; err != nil {
		return nil, fmt.Errorf("load library %d: %w", *libraryID, err)
	}
	return &lib, nil
}

func (s *ScrapeService) newResult(kind, mediaPath string, libraryID *uint, taskID uint) *ScrapeResult {
	return &ScrapeResult{
		Type:      kind,
		MediaPath: mediaPath,
		StartedAt: time.Now(),
		libraryID: libraryID,
		taskID:    taskID,
	}
}

func (s *ScrapeService) setStage(ctx context.Context, result *ScrapeResult, stage string) {
	if result == nil || stage == "" {
		return
	}
	result.CurrentStage = stage
	if result.taskID == 0 {
		return
	}
	if err := s.db.WithContext(ctx).
		Model(&store.ScrapeTask{}).
		Where("id = ?", result.taskID).
		Update("current_stage", stage).Error; err != nil {
		s.logger.Warnf("update task %d stage %s: %v", result.taskID, stage, err)
	}
}

func (s *ScrapeService) fail(ctx context.Context, result *ScrapeResult, stage string, err error) (*ScrapeResult, error) {
	if result == nil {
		return nil, err
	}
	result.Success = false
	result.FailedStage = stage
	if err != nil {
		result.ErrorMessage = err.Error()
	}
	result.CompletedAt = time.Now()
	s.setStage(ctx, result, stage)
	if result.taskID != 0 {
		if updateErr := s.db.WithContext(ctx).
			Model(&store.ScrapeTask{}).
			Where("id = ?", result.taskID).
			Updates(map[string]any{"current_stage": stage, "last_error": result.ErrorMessage}).Error; updateErr != nil {
			s.logger.Warnf("update failed task %d: %v", result.taskID, updateErr)
		}
	}
	return result, err
}

func (s *ScrapeService) persistResult(ctx context.Context, r *ScrapeResult, unifiedData any) error {
	if r == nil {
		return errors.New("nil result")
	}
	data, err := json.Marshal(unifiedData)
	if err != nil {
		return fmt.Errorf("marshal unified data: %w", err)
	}
	record := store.ScrapeResult{
		TaskID:      r.taskID,
		LibraryID:   r.libraryID,
		MediaType:   r.Type,
		Title:       r.Title,
		Year:        r.Year,
		FilePath:    r.MediaPath,
		NfoPath:     r.NfoPath,
		PosterPath:  r.PosterPath,
		FanartPath:  r.fanartPath,
		UnifiedData: string(data),
		Providers:   strings.Join(r.Providers, ","),
		ScrapedAt:   r.CompletedAt,
	}
	if record.ScrapedAt.IsZero() {
		record.ScrapedAt = time.Now()
	}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("create scrape result: %w", err)
	}
	return nil
}

func ensureWritable(overwrite bool, paths ...string) error {
	if overwrite {
		return nil
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("nfo already exists: %s", p)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat %s: %w", p, err)
		}
	}
	return nil
}

func artworkMapToSlice(m map[core.ArtworkType]string) []core.MediaArtwork {
	artworks := make([]core.MediaArtwork, 0, len(m))
	for artType, url := range m {
		if strings.TrimSpace(url) == "" {
			continue
		}
		artworks = append(artworks, core.MediaArtwork{Type: artType, URL: url, Provider: "fused"})
	}
	return artworks
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func dedupeNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

type noopLogger struct{}

func (noopLogger) Debugf(string, ...any) {}
func (noopLogger) Infof(string, ...any)  {}
func (noopLogger) Warnf(string, ...any)  {}
func (noopLogger) Errorf(string, ...any) {}

func parseIntID(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
