// Package reseed 实现 IYUU 辅种：把下载器里已经下完的种子的 info hash 交给 IYUU 查询，找出其他站点上数据相同的种子，
// 从站点下载种子文件、核对 info hash 与文件列表后，暂停加入同一台下载器（保存路径用原种子的），校验到 100% 才开始做种。
// 加入、校验与回滚复用转移做种的任务（transfer 包，任务种类 reseed）。默认关闭；token 加密保存。
package reseed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/iyuu"
	"github.com/sunerpy/pt-tools/internal/transfer"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

const (
	// siteListTTL 是 IYUU 站点列表的缓存时间。
	siteListTTL = 12 * time.Hour
	// maxRateLimitWait 是限流时愿意原地等待的最长时间；更长就停下这一轮，下次再来。
	maxRateLimitWait = time.Minute
)

var (
	// ErrInvalid 表示设置有问题。
	ErrInvalid = errors.New("辅种设置无效")
	// ErrNoSites 表示没有可以辅种的站点。
	ErrNoSites = errors.New("没有可以辅种的站点：IYUU 支持的站点里，pt-tools 还没有配置或没有选中")
)

// TokenCipher 加解密 IYUU token（生产环境是 ConfigStore 的 EncryptCookie / DecryptCookie）。
type TokenCipher interface {
	Encrypt(plain string) (string, error)
	Decrypt(cipherText string) (string, error)
}

// Config 是 Service 的依赖。
type Config struct {
	DB          *gorm.DB
	Cipher      TokenCipher
	Transfer    *transfer.Service
	Downloaders transfer.Downloaders
	// Sites 按站点名取共享的站点实例；SiteIDs 返回 pt-tools 里已配置（注册了实例）的站点。
	Sites    func(name string) (v2.Site, bool)
	SiteIDs  func() []string
	Resolver *v2.TrackerResolver
	// NewClient 按 token 建 IYUU 客户端，测试里换成假服务。
	NewClient func(token string) *iyuu.Client
	Now       func() time.Time
	// Sleep 是限流时的等待，测试里不真等。
	Sleep  func(ctx context.Context, d time.Duration) error
	Logger *zap.SugaredLogger
}

// Service 管理辅种设置并运行辅种。
type Service struct {
	cfg Config

	// saveMu 让保存设置的“读出、校验、写入”不和另一次保存交错（开启时要有 token，不能读到之后被清掉）。
	saveMu sync.Mutex

	mu        sync.Mutex
	sites     []iyuu.Site
	sitesAt   time.Time
	sitesFrom string // 缓存属于哪个 token（换 token 时作废）

	// gate 是 IYUU 调用口（最小间隔、每天的上限，见 iyuu_gate.go）
	gate iyuuGate
}

// New 构造 Service。
func New(cfg Config) *Service {
	if cfg.Resolver == nil {
		cfg.Resolver = v2.NewTrackerResolver()
	}
	if cfg.NewClient == nil {
		cfg.NewClient = iyuu.New
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Sleep == nil {
		cfg.Sleep = func(ctx context.Context, d time.Duration) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
				return nil
			}
		}
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop().Sugar()
	}
	return &Service{cfg: cfg}
}

// ---------- 设置 ----------

// Settings 是接口里的辅种设置：token 只说有没有设置，不返回内容。
type Settings struct {
	Enabled       bool     `json:"enabled"`
	HasToken      bool     `json:"has_token"`
	IntervalHours int      `json:"interval_hours"`
	DownloaderIDs []uint   `json:"downloader_ids"`
	SiteNames     []string `json:"site_names"`
	// SiteMode 是站点范围：all（全部已配置、IYUU 也支持的站点）或 selected（只算 SiteNames）
	SiteMode         string     `json:"site_mode"`
	MaxPerSitePerDay int        `json:"max_per_site_per_day"`
	LastRunAt        *time.Time `json:"last_run_at,omitempty"`
	LastResult       string     `json:"last_result"`
	// NextRunAt 是下一轮大约什么时候（开着、有 token 时才有；到点以后在下一次检查时跑）
	NextRunAt *time.Time `json:"next_run_at,omitempty"`
	// IYUUMinIntervalSeconds 是两次请求 IYUU 之间至少隔几秒（没设置时是默认的 3）；IYUUMaxCallsPerDay 是每天最多请求几次，0 是不限
	IYUUMinIntervalSeconds int `json:"iyuu_min_interval_seconds"`
	IYUUMaxCallsPerDay     int `json:"iyuu_max_calls_per_day"`
	// IYUUCooldownUntil 是限流冷却到什么时候（冷却中才有）；IYUULastLimit 是最近一次限流的情况
	IYUUCooldownUntil *time.Time `json:"iyuu_cooldown_until,omitempty"`
	IYUULastLimit     *IYUULimit `json:"iyuu_last_limit,omitempty"`
	// IYUUToday 是今天各接口的请求计数
	IYUUToday []models.ReseedIYUUDaily `json:"iyuu_today"`
}

// SettingsUpdate 是保存设置的请求。Token 为 nil 时保留原来的；为空串时清除。
// SiteNames 为 nil 时保留原来的站点范围（网页在「站点」页用开关改，见 SetSiteSelected）；给了就按名单保存，空名单是全部站点。
type SettingsUpdate struct {
	Enabled          bool      `json:"enabled"`
	Token            *string   `json:"token,omitempty"`
	IntervalHours    int       `json:"interval_hours"`
	DownloaderIDs    []uint    `json:"downloader_ids"`
	SiteNames        *[]string `json:"site_names,omitempty"`
	MaxPerSitePerDay int       `json:"max_per_site_per_day"`
	// IYUUMinIntervalSeconds 为 nil 时保留原来的（没设置过就是默认的 3 秒）；0 是不隔
	IYUUMinIntervalSeconds *int `json:"iyuu_min_interval_seconds,omitempty"`
	IYUUMaxCallsPerDay     int  `json:"iyuu_max_calls_per_day"`
}

func (s *Service) row(ctx context.Context) (models.ReseedSetting, error) {
	var r models.ReseedSetting
	err := s.cfg.DB.WithContext(ctx).First(&r, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.ReseedSetting{ID: 1}, nil
	}
	if err != nil {
		return r, fmt.Errorf("读取辅种设置失败: %w", err)
	}
	return r, nil
}

// Settings 返回当前设置（0 的间隔与上限换成默认值）。
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	r, err := s.row(ctx)
	if err != nil {
		return Settings{}, err
	}
	out := view(r)
	now := s.cfg.Now()
	out.NextRunAt = nextRun(r.Enabled && r.TokenEncrypted != "", r.LastRunAt, out.IntervalHours, now)
	if ce := cooling(r, now); ce != nil {
		until := ce.Until
		out.IYUUCooldownUntil = &until
	}
	out.IYUULastLimit = lastLimit(r)
	if out.IYUUToday, err = s.IYUUCallsToday(ctx); err != nil {
		return Settings{}, err
	}
	return out, nil
}

// nextRun 是下一轮大约什么时候：上一轮加间隔，没跑过或已经到点时是现在（下一次检查时跑）；不会自动跑时为空。
func nextRun(on bool, last *time.Time, intervalHours int, now time.Time) *time.Time {
	if !on {
		return nil
	}
	next := now
	if last != nil {
		if t := last.Add(time.Duration(intervalHours) * time.Hour); t.After(now) {
			next = t
		}
	}
	return &next
}

func view(r models.ReseedSetting) Settings {
	out := Settings{
		Enabled: r.Enabled, HasToken: r.TokenEncrypted != "", IntervalHours: r.IntervalHours,
		DownloaderIDs: decodeUints(r.DownloaderIDs), SiteNames: decodeStrings(r.SiteNames),
		MaxPerSitePerDay: r.MaxPerSitePerDay, LastRunAt: r.LastRunAt, LastResult: r.LastResult,
		IYUUMinIntervalSeconds: int(minInterval(r) / time.Second), IYUUMaxCallsPerDay: r.IYUUMaxCallsPerDay,
	}
	out.SiteMode = models.ReseedSiteModeOf(r.SiteMode, out.SiteNames)
	if out.IntervalHours == 0 {
		out.IntervalHours = models.ReseedDefaultIntervalHours
	}
	if out.MaxPerSitePerDay == 0 {
		out.MaxPerSitePerDay = models.ReseedDefaultMaxPerSitePerDay
	}
	return out
}

// SaveSettings 校验并保存设置（token 以外的项全量保存）。开启时必须有 token；换 token 时作废缓存的站点列表与 sid_sha1。
// 只写配置列：运行中的一轮会写上次运行结果和 sid_sha1 缓存，整行写回会把它们改回旧值。
func (s *Service) SaveSettings(ctx context.Context, u SettingsUpdate) (Settings, error) {
	switch {
	case u.IntervalHours != 0 && (u.IntervalHours < models.ReseedMinIntervalHours || u.IntervalHours > models.ReseedMaxIntervalHours):
		return Settings{}, fmt.Errorf("%w：间隔要在 %d 到 %d 小时之间", ErrInvalid, models.ReseedMinIntervalHours, models.ReseedMaxIntervalHours)
	case u.MaxPerSitePerDay < 0 || u.MaxPerSitePerDay > models.ReseedMaxPerSitePerDay:
		return Settings{}, fmt.Errorf("%w：每站每天最多 %d 个", ErrInvalid, models.ReseedMaxPerSitePerDay)
	case u.IYUUMinIntervalSeconds != nil && (*u.IYUUMinIntervalSeconds < 0 || *u.IYUUMinIntervalSeconds > models.ReseedMaxIYUUMinInterval):
		return Settings{}, fmt.Errorf("%w：两次请求 IYUU 之间隔 0 到 %d 秒", ErrInvalid, models.ReseedMaxIYUUMinInterval)
	case u.IYUUMaxCallsPerDay < 0 || u.IYUUMaxCallsPerDay > models.ReseedMaxIYUUCallsPerDay:
		return Settings{}, fmt.Errorf("%w：每天最多请求 IYUU %d 次（0 是不限）", ErrInvalid, models.ReseedMaxIYUUCallsPerDay)
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	r, err := s.row(ctx)
	if err != nil {
		return Settings{}, err
	}
	cols := map[string]any{
		"enabled":                u.Enabled,
		"interval_hours":         u.IntervalHours,
		"max_per_site_per_day":   u.MaxPerSitePerDay,
		"downloader_ids":         encode(dedupeUints(u.DownloaderIDs)),
		"iyuu_max_calls_per_day": u.IYUUMaxCallsPerDay,
	}
	if u.IYUUMinIntervalSeconds != nil {
		cols["iyuu_min_interval_seconds"] = *u.IYUUMinIntervalSeconds
	}
	if u.SiteNames != nil {
		names := dedupeStrings(*u.SiteNames)
		cols["site_names"], cols["site_mode"] = encode(names), models.ReseedSiteModeOf("", names)
	}
	tokenEnc := r.TokenEncrypted
	if u.Token != nil {
		tokenEnc = ""
		if token := strings.TrimSpace(*u.Token); token != "" {
			if s.cfg.Cipher == nil {
				return Settings{}, errors.New("没有可用的加密密钥，不能保存 token")
			}
			if tokenEnc, err = s.cfg.Cipher.Encrypt(token); err != nil {
				return Settings{}, fmt.Errorf("加密 token 失败: %w", err)
			}
		}
		cols["token_encrypted"] = tokenEnc
		cols["sid_sha1"], cols["sid_key"], cols["sid_at"] = "", "", nil
		// 限流是按 token 算的：换了 token 不再冷却
		cols["iyuu_cooldown_until"] = nil
	}
	if u.Enabled && tokenEnc == "" {
		return Settings{}, fmt.Errorf("%w：开启辅种前要先填写 IYUU token", ErrInvalid)
	}
	db := s.cfg.DB.WithContext(context.WithoutCancel(ctx))
	if err := ensureRow(db); err != nil {
		return Settings{}, fmt.Errorf("保存辅种设置失败: %w", err)
	}
	if err := db.Model(&models.ReseedSetting{}).Where("id = 1").Updates(cols).Error; err != nil {
		return Settings{}, fmt.Errorf("保存辅种设置失败: %w", err)
	}
	if u.Token != nil {
		s.mu.Lock()
		s.sites, s.sitesAt, s.sitesFrom = nil, time.Time{}, ""
		s.mu.Unlock()
	}
	return s.Settings(ctx)
}

// ensureRow 建出唯一的那一行设置（已有时不动）。
func ensureRow(db *gorm.DB) error {
	var r models.ReseedSetting
	return db.FirstOrCreate(&r, models.ReseedSetting{ID: 1}).Error
}

func (s *Service) token(ctx context.Context, r models.ReseedSetting) (string, error) {
	if r.TokenEncrypted == "" {
		return "", iyuu.ErrNoToken
	}
	if s.cfg.Cipher == nil {
		return "", errors.New("没有可用的加密密钥，读不出 token")
	}
	plain, err := s.cfg.Cipher.Decrypt(r.TokenEncrypted)
	if err != nil {
		return "", fmt.Errorf("解密 token 失败: %w", err)
	}
	_ = ctx
	return plain, nil
}

// Due 报告这一刻该不该定时运行：开启了、有 token、距上次运行满了间隔。
func (s *Service) Due(ctx context.Context) bool {
	r, err := s.row(ctx)
	if err != nil || !r.Enabled || r.TokenEncrypted == "" {
		return false
	}
	if r.LastRunAt == nil {
		return s.IYUUReady(ctx) == nil
	}
	interval := r.IntervalHours
	if interval == 0 {
		interval = models.ReseedDefaultIntervalHours
	}
	if s.cfg.Now().Before(r.LastRunAt.Add(time.Duration(interval) * time.Hour)) {
		return false
	}
	// 限流冷却中、今天的请求到了上限：不开始（开始了也是马上停下）
	return s.IYUUReady(ctx) == nil
}

// RecordRun 记下这一轮的时间和结果。
func (s *Service) RecordRun(ctx context.Context, at time.Time, summary string) error {
	db := s.cfg.DB.WithContext(context.WithoutCancel(ctx))
	if err := ensureRow(db); err != nil {
		return fmt.Errorf("保存辅种设置失败: %w", err)
	}
	if err := db.Model(&models.ReseedSetting{}).Where("id = 1").
		Updates(map[string]any{"last_run_at": at, "last_result": summary}).Error; err != nil {
		return fmt.Errorf("记录辅种结果失败: %w", err)
	}
	return nil
}

// ---------- 站点对照 ----------

// SiteMapItem 是 IYUU 的一个站点，以及它在 pt-tools 里对应哪个站点。
type SiteMapItem struct {
	SID        int    `json:"sid"`
	IYUUSite   string `json:"iyuu_site"`
	Nickname   string `json:"nickname"`
	Host       string `json:"host"`
	SiteName   string `json:"site_name,omitempty"`
	Configured bool   `json:"configured"`
	Selected   bool   `json:"selected"`
}

// iyuuSites 返回 IYUU 支持的站点（缓存 12 小时）。限流冷却中、到了每天的上限时，有旧的缓存就用旧的。
func (s *Service) iyuuSites(ctx context.Context, res *RunResult, client *iyuu.Client, token string) ([]iyuu.Site, error) {
	s.mu.Lock()
	cached := s.sites
	if s.sitesFrom != token {
		cached = nil
	}
	if cached != nil && s.cfg.Now().Sub(s.sitesAt) < siteListTTL {
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()
	var sites []iyuu.Site
	err := s.callIYUU(ctx, res, models.IYUUEndpointSites, 0, func(ctx context.Context) error {
		var cerr error
		sites, cerr = client.Sites(ctx)
		return cerr
	})
	if err != nil {
		if cached != nil && IYUULimited(err) {
			return cached, nil
		}
		return nil, err
	}
	s.mu.Lock()
	s.sites, s.sitesAt, s.sitesFrom = sites, s.cfg.Now(), token
	s.mu.Unlock()
	return sites, nil
}

// SiteMap 列出 IYUU 支持的站点和它们在 pt-tools 里对应的站点（按站点地址的主机名与可注册域识别）。
func (s *Service) SiteMap(ctx context.Context) ([]SiteMapItem, error) {
	r, err := s.row(ctx)
	if err != nil {
		return nil, err
	}
	token, err := s.token(ctx, r)
	if err != nil {
		return nil, err
	}
	sites, err := s.iyuuSites(ctx, nil, s.cfg.NewClient(token), token)
	if err != nil {
		return nil, err
	}
	return s.mapSites(sites, r), nil
}

// SiteSelection 是站点范围：Mode 为 all 时全部已配置、IYUU 也支持的站点都参与（以后新配置的也算），为 selected 时只算 SiteNames。
type SiteSelection struct {
	Mode      string   `json:"site_mode"`
	SiteNames []string `json:"site_names"`
}

// SetSiteSelected 打开或关掉一个站点。从「全部站点」关掉一个时，名单换成现在已配置的站点去掉它（以后新配置的站点默认不参与）；
// 打开的站点必须已经在 pt-tools 里配置。只写站点范围的两列，与保存设置互斥。
func (s *Service) SetSiteSelected(ctx context.Context, site string, selected bool) (SiteSelection, error) {
	site = strings.TrimSpace(site)
	if site == "" {
		return SiteSelection{}, fmt.Errorf("%w：缺少站点", ErrInvalid)
	}
	configured := s.configuredSites()
	if selected && !configured[site] {
		return SiteSelection{}, fmt.Errorf("%w：站点 %s 还没有在 pt-tools 里配置", ErrInvalid, site)
	}
	return s.updateSites(ctx, func(mode string, names []string) (string, []string) {
		if mode == models.ReseedSitesAll {
			if selected {
				return mode, names
			}
			names = names[:0]
			for id := range configured {
				if id != site {
					names = append(names, id)
				}
			}
			return models.ReseedSitesSelected, names
		}
		out := make([]string, 0, len(names)+1)
		for _, n := range names {
			if n != site {
				out = append(out, n)
			}
		}
		if selected {
			out = append(out, site)
		}
		return models.ReseedSitesSelected, out
	})
}

// SetAllSitesSelected 全部打开（以后新配置的站点也自动参与）或全部关掉。
func (s *Service) SetAllSitesSelected(ctx context.Context, selected bool) (SiteSelection, error) {
	return s.updateSites(ctx, func(string, []string) (string, []string) {
		if selected {
			return models.ReseedSitesAll, []string{}
		}
		return models.ReseedSitesSelected, []string{}
	})
}

func (s *Service) configuredSites() map[string]bool {
	out := map[string]bool{}
	if s.cfg.SiteIDs != nil {
		for _, id := range s.cfg.SiteIDs() {
			out[id] = true
		}
	}
	return out
}

// updateSites 在 saveMu 下读出站点范围、按 change 改、写回（只写 site_mode 与 site_names 两列）。
func (s *Service) updateSites(ctx context.Context, change func(mode string, names []string) (string, []string)) (SiteSelection, error) {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	r, err := s.row(ctx)
	if err != nil {
		return SiteSelection{}, err
	}
	names := decodeStrings(r.SiteNames)
	mode, names := change(models.ReseedSiteModeOf(r.SiteMode, names), names)
	names = dedupeStrings(names)
	sort.Strings(names)
	db := s.cfg.DB.WithContext(context.WithoutCancel(ctx))
	if err := ensureRow(db); err != nil {
		return SiteSelection{}, fmt.Errorf("保存辅种站点失败: %w", err)
	}
	if err := db.Model(&models.ReseedSetting{}).Where("id = 1").
		Updates(map[string]any{"site_mode": mode, "site_names": encode(names)}).Error; err != nil {
		return SiteSelection{}, fmt.Errorf("保存辅种站点失败: %w", err)
	}
	return SiteSelection{Mode: mode, SiteNames: names}, nil
}

// mapSites 按设置里的站点范围标出每个 IYUU 站点在不在辅种范围。
func (s *Service) mapSites(sites []iyuu.Site, r models.ReseedSetting) []SiteMapItem {
	selected := decodeStrings(r.SiteNames)
	all := models.ReseedSiteModeOf(r.SiteMode, selected) == models.ReseedSitesAll
	configured := s.configuredSites()
	pick := map[string]bool{}
	for _, n := range selected {
		pick[n] = true
	}
	out := make([]SiteMapItem, 0, len(sites))
	for _, st := range sites {
		host := strings.TrimSpace(st.BaseURL)
		raw := host
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		name, _ := s.cfg.Resolver.Resolve(raw)
		item := SiteMapItem{SID: st.SID, IYUUSite: st.Site, Nickname: st.Nickname, Host: host, SiteName: name}
		item.Configured = name != "" && configured[name]
		item.Selected = item.Configured && (all || pick[name])
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Configured != out[j].Configured {
			return out[i].Configured
		}
		return out[i].IYUUSite < out[j].IYUUSite
	})
	return out
}

// ---------- 小工具 ----------

func encode(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func decodeUints(s string) []uint {
	out := []uint{}
	if strings.TrimSpace(s) != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	return out
}

func decodeStrings(s string) []string {
	out := []string{}
	if strings.TrimSpace(s) != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	return out
}

func dedupeUints(in []uint) []uint {
	seen := map[uint]bool{}
	out := make([]uint, 0, len(in))
	for _, v := range in {
		if v != 0 && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func joinInts(v []int) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}
