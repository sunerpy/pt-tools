// Package organize 是整理入库服务：种子下载完成后识别出电影或剧集，按媒体库的模板与整理方式放进库里，
// 写 NFO 与图片，通知媒体服务器扫描，发入库通知。触发方式有三种：pt-tools 推送的种子下载完成（事件，
// 另有定时补查）、按范围定期扫描下载器、在任务列表与下载器页面手动整理。整理记录按源文件去重。
package organize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

var (
	// ErrInvalid 表示参数不对（错误信息写明哪一项）。
	ErrInvalid = errors.New("参数无效")
	// ErrNotFound 表示要改或删的记录不存在。
	ErrNotFound = errors.New("记录不存在")
)

// Cipher 加解密媒体服务器的 Token（生产环境是 ConfigStore 的 EncryptCookie / DecryptCookie）。
type Cipher interface {
	Encrypt(plain string) (string, error)
	Decrypt(cipherText string) (string, error)
}

// Recognizer 是识别服务（生产环境是 *recognize.Service）。
type Recognizer interface {
	Recognize(ctx context.Context, in recognize.Input) (*recognize.Result, error)
	Parse(ctx context.Context, title, subtitle string) (meta.Meta, error)
	TMDB(ctx context.Context) (*tmdb.Client, error)
}

// Downloaders 取下载器（生产环境包装 DownloaderManager 与下载器设置表）。
type Downloaders interface {
	// Get 按下载器 ID 取实例与设置。
	Get(ctx context.Context, id uint) (downloader.Downloader, models.DownloaderSetting, error)
	// ByName 按名字取（完成事件里只有名字）。
	ByName(ctx context.Context, name string) (downloader.Downloader, models.DownloaderSetting, error)
	// List 是启用的下载器。
	List(ctx context.Context) ([]models.DownloaderSetting, error)
}

// Notice 是一条入库通知。Key 相同的只发一次。
type Notice struct {
	Key        string
	Title      string
	Text       string
	ChannelIDs []uint
}

// Notifier 发入库通知（生产环境写 MonitorNotificationLog，由现有的投递器发出）。
type Notifier func(ctx context.Context, n Notice) error

// Config 是服务的依赖。
type Config struct {
	DB          *gorm.DB
	Cipher      Cipher
	Recognizer  Recognizer
	Downloaders Downloaders
	Notify      Notifier
	Logger      *zap.SugaredLogger
	// Now 为空时用 time.Now（测试里换成假的时钟）
	Now func() time.Time
	// ServerHTTP 为空时用 server.NewHTTPClient（测试里换成 httptest 的）
	ServerHTTP *http.Client
}

// Service 是整理入库服务。
type Service struct {
	cfg Config
	// mu 串行化整理：一次只整理一个种子，同一个文件不会被两种触发方式同时整理
	mu sync.Mutex
	// setMu 串行化设置、媒体库、路径映射与媒体服务器的写入
	setMu sync.Mutex

	jobs    chan job
	qmu     sync.Mutex
	queued  map[string]bool
	backoff map[string]backoffState
	// retrying 是到期重试排上队、还没整理完的记录（retryDue 先清掉了它们的重试时间，见 Retrying）
	retrying map[uint]bool

	runMu   sync.Mutex
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// New 建一个服务（还没开始后台循环，见 Start）。
func New(cfg Config) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop().Sugar()
	}
	return &Service{
		cfg:      cfg,
		jobs:     make(chan job, jobQueueSize),
		queued:   map[string]bool{},
		backoff:  map[string]backoffState{},
		retrying: map[uint]bool{},
	}
}

// 默认值与上限。
const (
	DefaultScanIntervalMin = 60
	MinScanIntervalMin     = 10
	MaxScanIntervalMin     = 1440
	DefaultMinVideoMB      = 50
	MaxMinVideoMB          = 10240
	maxScopeItems          = 50
	maxScopeItemLen        = 512
)

// Settings 是给界面看与保存用的整理设置。
type Settings struct {
	AutoEnabled         bool       `json:"auto_enabled"`
	AutoSince           *time.Time `json:"auto_since,omitempty"`
	ScanEnabled         bool       `json:"scan_enabled"`
	ScanIntervalMin     int        `json:"scan_interval_min"`
	Downloaders         []uint     `json:"downloaders"`
	Categories          []string   `json:"categories"`
	Tags                []string   `json:"tags"`
	SavePaths           []string   `json:"save_paths"`
	MinVideoMB          int        `json:"min_video_mb"`
	NotifyChannels      []uint     `json:"notify_channels"`
	DeleteLinksOnRemove bool       `json:"delete_links_on_remove"`
}

// SettingsInput 是保存设置的请求（AutoSince 由服务在打开自动整理时记下）。
type SettingsInput struct {
	AutoEnabled         bool     `json:"auto_enabled"`
	ScanEnabled         bool     `json:"scan_enabled"`
	ScanIntervalMin     int      `json:"scan_interval_min"`
	Downloaders         []uint   `json:"downloaders"`
	Categories          []string `json:"categories"`
	Tags                []string `json:"tags"`
	SavePaths           []string `json:"save_paths"`
	MinVideoMB          int      `json:"min_video_mb"`
	NotifyChannels      []uint   `json:"notify_channels"`
	DeleteLinksOnRemove bool     `json:"delete_links_on_remove"`
}

func (s *Service) loadSettings(ctx context.Context) (models.MediaOrganizeSetting, error) {
	var row models.MediaOrganizeSetting
	err := s.cfg.DB.WithContext(ctx).Where("id = ?", 1).Limit(1).Find(&row).Error
	if err != nil {
		return row, fmt.Errorf("读取整理设置失败: %w", err)
	}
	row.ID = 1
	return row, nil
}

func decodeIDs(raw string) []uint {
	var ids []uint
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &ids)
	}
	if ids == nil {
		ids = []uint{}
	}
	return ids
}

func splitList(raw, sep string) []string {
	out := []string{}
	for _, p := range strings.Split(raw, sep) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func viewSettings(row models.MediaOrganizeSetting) Settings {
	v := Settings{
		AutoEnabled: row.AutoEnabled, AutoSince: row.AutoSince, ScanEnabled: row.ScanEnabled,
		ScanIntervalMin: row.ScanIntervalMin, Downloaders: decodeIDs(row.ScopeDownloaders),
		Categories: splitList(row.ScopeCategories, ","), Tags: splitList(row.ScopeTags, ","),
		SavePaths: splitList(row.ScopeSavePaths, "\n"), MinVideoMB: row.MinVideoMB,
		NotifyChannels: decodeIDs(row.NotifyChannels), DeleteLinksOnRemove: row.DeleteLinksOnRemove,
	}
	if v.ScanIntervalMin <= 0 {
		v.ScanIntervalMin = DefaultScanIntervalMin
	}
	if v.MinVideoMB <= 0 {
		v.MinVideoMB = DefaultMinVideoMB
	}
	return v
}

// Settings 返回当前的整理设置（没有保存过时是默认值）。
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	row, err := s.loadSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	return viewSettings(row), nil
}

func cleanStrings(in []string, what, sep string) ([]string, error) {
	out := []string{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || slices.Contains(out, v) {
			continue
		}
		if len(v) > maxScopeItemLen || strings.Contains(v, sep) {
			return nil, fmt.Errorf("%w: %s「%s」太长或含有分隔符", ErrInvalid, what, v)
		}
		out = append(out, v)
	}
	if len(out) > maxScopeItems {
		return nil, fmt.Errorf("%w: %s最多 %d 项", ErrInvalid, what, maxScopeItems)
	}
	return out, nil
}

func cleanIDs(in []uint, what string) ([]uint, error) {
	out := []uint{}
	for _, id := range in {
		if id == 0 {
			return nil, fmt.Errorf("%w: %s编号无效", ErrInvalid, what)
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	if len(out) > maxScopeItems {
		return nil, fmt.Errorf("%w: %s最多 %d 项", ErrInvalid, what, maxScopeItems)
	}
	return out, nil
}

// SaveSettings 保存整理设置。打开自动整理时记下时间：只整理之后下载完成的种子。
func (s *Service) SaveSettings(ctx context.Context, in SettingsInput) (Settings, error) {
	if in.ScanIntervalMin == 0 {
		in.ScanIntervalMin = DefaultScanIntervalMin
	}
	if in.ScanIntervalMin < MinScanIntervalMin || in.ScanIntervalMin > MaxScanIntervalMin {
		return Settings{}, fmt.Errorf("%w: 扫描间隔要在 %d 到 %d 分钟之间", ErrInvalid, MinScanIntervalMin, MaxScanIntervalMin)
	}
	if in.MinVideoMB == 0 {
		in.MinVideoMB = DefaultMinVideoMB
	}
	if in.MinVideoMB < 1 || in.MinVideoMB > MaxMinVideoMB {
		return Settings{}, fmt.Errorf("%w: 视频最小体积要在 1 到 %d MB 之间", ErrInvalid, MaxMinVideoMB)
	}
	dls, err := cleanIDs(in.Downloaders, "下载器")
	if err != nil {
		return Settings{}, err
	}
	chans, err := cleanIDs(in.NotifyChannels, "通知通道")
	if err != nil {
		return Settings{}, err
	}
	cats, err := cleanStrings(in.Categories, "分类", ",")
	if err != nil {
		return Settings{}, err
	}
	tags, err := cleanStrings(in.Tags, "标签", ",")
	if err != nil {
		return Settings{}, err
	}
	paths, err := cleanStrings(in.SavePaths, "保存路径", "\n")
	if err != nil {
		return Settings{}, err
	}
	s.setMu.Lock()
	defer s.setMu.Unlock()
	row, err := s.loadSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	switch {
	case !in.AutoEnabled:
		row.AutoSince = nil
	case !row.AutoEnabled || row.AutoSince == nil:
		now := s.cfg.Now()
		row.AutoSince = &now
	}
	dlJSON, _ := json.Marshal(dls)
	chJSON, _ := json.Marshal(chans)
	row.AutoEnabled, row.ScanEnabled, row.ScanIntervalMin = in.AutoEnabled, in.ScanEnabled, in.ScanIntervalMin
	row.ScopeDownloaders, row.NotifyChannels = string(dlJSON), string(chJSON)
	row.ScopeCategories, row.ScopeTags, row.ScopeSavePaths = strings.Join(cats, ","), strings.Join(tags, ","), strings.Join(paths, "\n")
	row.MinVideoMB, row.DeleteLinksOnRemove = in.MinVideoMB, in.DeleteLinksOnRemove
	row.UpdatedAt = s.cfg.Now()
	if err := s.cfg.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"auto_enabled", "auto_since", "scan_enabled", "scan_interval_min", "scope_downloaders", "scope_categories",
			"scope_tags", "scope_save_paths", "min_video_mb", "notify_channels", "delete_links_on_remove", "updated_at",
		}),
	}).Create(&row).Error; err != nil {
		return Settings{}, fmt.Errorf("保存整理设置失败: %w", err)
	}
	return viewSettings(row), nil
}

// scope 是自动整理与定期扫描的范围。
type scope struct {
	downloaders []uint
	categories  []string
	tags        []string
	savePaths   []string
}

func scopeOf(v Settings) scope {
	return scope{downloaders: v.Downloaders, categories: v.Categories, tags: v.Tags, savePaths: v.SavePaths}
}

// match 报告种子在不在范围里：每一项为空时不限制，不为空时要满足其中之一。
func (sc scope) match(dlID uint, t downloader.Torrent) bool {
	if len(sc.downloaders) > 0 && !slices.Contains(sc.downloaders, dlID) {
		return false
	}
	if len(sc.categories) > 0 && !slices.ContainsFunc(sc.categories, func(c string) bool { return strings.EqualFold(c, t.Category) }) {
		return false
	}
	if len(sc.tags) > 0 {
		have := splitList(t.Tags, ",")
		if !slices.ContainsFunc(sc.tags, func(want string) bool {
			return slices.ContainsFunc(have, func(h string) bool { return strings.EqualFold(h, want) })
		}) {
			return false
		}
	}
	if len(sc.savePaths) > 0 {
		maps := make([]models.DownloaderPathMap, 0, len(sc.savePaths))
		for _, p := range sc.savePaths {
			maps = append(maps, models.DownloaderPathMap{SourcePrefix: p, TargetPrefix: p})
		}
		if _, ok := models.MapTransferPath(maps, t.SavePath); !ok {
			return false
		}
	}
	return true
}
