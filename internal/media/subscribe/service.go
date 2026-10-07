// Package subscribe 是订阅：按电影或剧集的一季找资源（RSS 拉到的种子与定时主动搜索），按质量档案挑、推送到下载器，
// 剧集按 TMDB 的分集找缺的集；洗版时接着找更好的版本，入库后替换旧文件；豆瓣想看定时建订阅。
package subscribe

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 错误。接口层按它们回 400 / 404。
var (
	ErrInvalid  = errors.New("参数不对")
	ErrNotFound = errors.New("没有这条记录")
)

// Recognizer 是媒体识别（生产环境是 *recognize.Service）。
type Recognizer interface {
	Recognize(ctx context.Context, in recognize.Input) (*recognize.Result, error)
	Parse(ctx context.Context, title, subtitle string) (meta.Meta, error)
	TMDB(ctx context.Context) (*tmdb.Client, error)
}

// Searcher 是多站点搜索（生产环境是 CachedSearchOrchestrator）。
type Searcher interface {
	Search(ctx context.Context, q v2.MultiSiteSearchQuery) (*v2.MultiSiteSearchResult, error)
}

// SitesFunc 按站点名取共享的站点实例（下载种子文件用，与搜索、登录探测共用限速）。
type SitesFunc func(name string) (v2.Site, bool)

// PushFunc 是推送入口（生产环境是 internal.PushTorrentToDownloader，磁盘与站点容量闸门都在里面）。
type PushFunc func(ctx context.Context, req ptinternal.PushTorrentRequest) (*ptinternal.PushTorrentResult, error)

// Downloaders 按编号取下载器（洗版以后删旧种子用）。
type Downloaders interface {
	Get(ctx context.Context, id uint) (downloader.Downloader, models.DownloaderSetting, error)
}

// Library 报告条目在不在媒体服务器里（没有配媒体服务器时返回 false；生产环境是 *organize.Service）。
type Library interface {
	InLibrary(ctx context.Context, kind string, tmdbID int, imdbID, title string) (bool, error)
}

// Organizer 是整理入库里洗版要用的两个动作（生产环境是 *organize.Service）。
type Organizer interface {
	Retire(ctx context.Context, id uint, reason string) ([]string, error)
	Retry(ctx context.Context, id uint) (*organize.Result, error)
}

// Notice 是一条订阅的通知：下载了订阅的资源，或豆瓣想看拉取连续失败。
type Notice struct {
	Channels []uint
	Kind     string
	Subject  string
	EventKey string
	Title    string
	Body     string
	ImageURL string
}

// 通知的种类。
const (
	NoticeDownloaded     = "subscribe_downloaded"
	NoticeDoubanAbnormal = "douban_abnormal"
)

// Notifier 发通知（生产环境经监控通知投递器）。
type Notifier func(ctx context.Context, n Notice) error

// Config 是订阅服务的依赖。
type Config struct {
	DB          *gorm.DB
	Recognizer  Recognizer
	Search      Searcher
	Sites       SitesFunc
	SiteNames   func() []string
	Push        PushFunc
	Downloaders Downloaders
	Library     Library
	Organizer   Organizer
	Notify      Notifier
	Logger      *zap.SugaredLogger
	Now         func() time.Time
	// Jitter 是主动搜索与豆瓣拉取的随机偏移（测试里换成固定的）
	Jitter func(limit time.Duration) time.Duration
	// HTTP 拉豆瓣用（为空时用带超时的默认客户端，走系统代理）
	HTTP *http.Client
	// DoubanBase 是豆瓣的地址（测试与 QA 换成假服务）
	DoubanBase string
}

// Service 是订阅服务。
type Service struct {
	cfg Config
	// mu 串行化评估与推送：RSS 与主动搜索同时找到同一个种子时只推一次
	mu     sync.Mutex
	offers chan Candidate
	cache  parseCache

	runMu   sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
}

const offerQueue = 256

// New 建订阅服务；Start 以后才接 RSS 与定时搜索。
func New(cfg Config) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop().Sugar()
	}
	if cfg.Jitter == nil {
		cfg.Jitter = func(limit time.Duration) time.Duration {
			if limit <= 0 {
				return 0
			}
			return time.Duration(rand.Int64N(int64(limit)))
		}
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 20 * time.Second}
	}
	if cfg.DoubanBase == "" {
		cfg.DoubanBase = "https://www.douban.com"
	}
	return &Service{cfg: cfg, offers: make(chan Candidate, offerQueue), cache: parseCache{m: map[string]parsed{}}}
}

// Offer 把 RSS 拉到的种子交给订阅（不阻塞；队列满时丢掉，主动搜索还会找到）。
func (s *Service) Offer(site string, item v2.TorrentItem) {
	select {
	case s.offers <- Candidate{Site: site, Item: item, From: fromRSS}:
	default:
		s.cfg.Logger.Debugf("[订阅] RSS 队列已满，丢掉 %s", item.Title)
	}
}

// tickInterval 是后台循环的节拍。
const tickInterval = time.Minute

// Start 启动后台：处理 RSS 交来的种子，每分钟看一次到期的搜索、下载中的种子与豆瓣想看。
func (s *Service) Start(ctx context.Context) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.running {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	s.cancel, s.running = cancel, true
	s.wg.Add(2)
	go func() {
		defer s.wg.Done()
		s.offerLoop(ctx)
	}()
	go func() {
		defer s.wg.Done()
		s.loop(ctx)
	}()
}

// Stop 停掉后台并等它退出。
func (s *Service) Stop() {
	s.runMu.Lock()
	if !s.running {
		s.runMu.Unlock()
		return
	}
	s.cancel()
	s.running = false
	s.runMu.Unlock()
	s.wg.Wait()
}

func (s *Service) offerLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case c := <-s.offers:
			set, err := s.Settings(ctx)
			if err != nil || !set.Enabled {
				continue
			}
			s.considerOffer(ctx, c)
		}
	}
}

func (s *Service) loop(ctx context.Context) {
	t := time.NewTicker(tickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Tick(ctx)
		}
	}
}

// Tick 做一轮后台工作：下载中的种子入库了没有、订阅完成了没有、到期的主动搜索与豆瓣想看。总开关关着时什么都不做。
func (s *Service) Tick(ctx context.Context) {
	set, err := s.Settings(ctx)
	if err != nil || !set.Enabled {
		return
	}
	s.refresh(ctx)
	s.searchDue(ctx, set)
	s.doubanDue(ctx)
}

// parseCache 按标题缓存解析结果（RSS 一小时能拉到几百个种子，识别词要读库）。
type parseCache struct {
	mu sync.Mutex
	m  map[string]parsed
}

type parsed struct {
	m  meta.Meta
	at time.Time
}

const (
	parseCacheSize = 4096
	parseCacheTTL  = 30 * time.Minute
)

func (s *Service) parse(ctx context.Context, title, subtitle string) meta.Meta {
	key := title + "\x00" + subtitle
	now := s.cfg.Now()
	s.cache.mu.Lock()
	if p, ok := s.cache.m[key]; ok && now.Sub(p.at) < parseCacheTTL {
		s.cache.mu.Unlock()
		return p.m
	}
	s.cache.mu.Unlock()
	m, err := s.cfg.Recognizer.Parse(ctx, title, subtitle)
	if err != nil {
		m = meta.Parse(title, subtitle)
	}
	s.cache.mu.Lock()
	if len(s.cache.m) >= parseCacheSize {
		s.cache.m = map[string]parsed{}
	}
	s.cache.m[key] = parsed{m: m, at: now}
	s.cache.mu.Unlock()
	return m
}
