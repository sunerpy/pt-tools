package cookiecloud

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// MaxImportSites 是一次导入最多选几个站点。
const MaxImportSites = 500

var (
	// ErrInvalid 表示设置或请求有问题。
	ErrInvalid = errors.New("CookieCloud 设置无效")
	// ErrNotConfigured 表示还没填服务地址、UUID 或密码。
	ErrNotConfigured = errors.New("先填写 CookieCloud 服务地址、UUID 和密码")
	// ErrBusy 表示另一次预览、导入或同步正在进行。
	ErrBusy = errors.New("另一次 CookieCloud 导入正在进行，稍后再试")
)

// Cipher 加解密密码（生产环境是 ConfigStore 的 EncryptCookie / DecryptCookie）。
type Cipher interface {
	Encrypt(plain string) (string, error)
	Decrypt(cipherText string) (string, error)
}

// SiteState 是 pt-tools 支持的一个站点。BaseURL 是 pt-tools 访问它用的地址；
// Cookie 是现在保存的 Cookie（明文，只用来判断有没有变化，不出现在接口返回和日志里）。
type SiteState struct {
	Name        string
	DisplayName string
	BaseURL     string
	Enabled     bool
	Cookie      string
}

// Config 是 Service 的依赖。
type Config struct {
	DB     *gorm.DB
	Cipher Cipher
	// HTTP 为 nil 时用 30 秒超时的客户端。
	HTTP *http.Client
	// Sites 返回 pt-tools 支持的站点（含没启用的）。
	Sites func(ctx context.Context) ([]SiteState, error)
	// Apply 把 Cookie（站点名 → Cookie 请求头）写进站点并启用，走与浏览器扩展同步凭据相同的路径：保存、刷新站点实例、请求登录探测。
	// 返回写不进去的站点和原因，其余的已经写好。
	Apply  func(ctx context.Context, cookies map[string]string) map[string]error
	Now    func() time.Time
	Logger *zap.SugaredLogger
}

// Service 管理 CookieCloud 设置，预览、导入与定时同步站点 Cookie。解密只在本机做，Cookie 不缓存、不写日志。
type Service struct {
	cfg    Config
	saveMu sync.Mutex
	runMu  sync.Mutex
}

// New 构造 Service。
func New(cfg Config) *Service {
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop().Sugar()
	}
	return &Service{cfg: cfg}
}

// ---------- 设置 ----------

// Settings 是接口里的设置：密码只说有没有设置。
type Settings struct {
	ServerURL     string     `json:"server_url"`
	UUID          string     `json:"uuid"`
	HasPassword   bool       `json:"has_password"`
	AutoSync      bool       `json:"auto_sync"`
	IntervalHours int        `json:"interval_hours"`
	LastSyncAt    *time.Time `json:"last_sync_at,omitempty"`
	LastResult    string     `json:"last_result"`
}

// SettingsUpdate 是保存设置的请求（密码以外整份保存）。Password 为 nil 时保留原来的，空串时清除。
type SettingsUpdate struct {
	ServerURL     string  `json:"server_url"`
	UUID          string  `json:"uuid"`
	Password      *string `json:"password,omitempty"`
	AutoSync      bool    `json:"auto_sync"`
	IntervalHours int     `json:"interval_hours"`
}

func (s *Service) row(ctx context.Context) (models.CookieCloudSetting, error) {
	var r models.CookieCloudSetting
	err := s.cfg.DB.WithContext(ctx).First(&r, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.CookieCloudSetting{ID: 1}, nil
	}
	if err != nil {
		return r, fmt.Errorf("读取 CookieCloud 设置失败: %w", err)
	}
	return r, nil
}

// Settings 返回当前设置（0 的间隔换成默认值）。
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	r, err := s.row(ctx)
	if err != nil {
		return Settings{}, err
	}
	out := Settings{
		ServerURL: r.ServerURL, UUID: r.UUID, HasPassword: r.PasswordEncrypted != "", AutoSync: r.AutoSync,
		IntervalHours: r.IntervalHours, LastSyncAt: r.LastSyncAt, LastResult: r.LastResult,
	}
	if out.IntervalHours == 0 {
		out.IntervalHours = models.CookieCloudDefaultIntervalHours
	}
	return out, nil
}

// SaveSettings 校验并保存设置，只写配置列（上次同步的结果不受影响）。打开定时同步时地址、UUID、密码都要有。
func (s *Service) SaveSettings(ctx context.Context, u SettingsUpdate) (Settings, error) {
	server, err := normalizeServer(u.ServerURL)
	if err != nil {
		return Settings{}, err
	}
	uuid := strings.TrimSpace(u.UUID)
	if u.IntervalHours != 0 && (u.IntervalHours < models.CookieCloudMinIntervalHours || u.IntervalHours > models.CookieCloudMaxIntervalHours) {
		return Settings{}, fmt.Errorf("%w：间隔要在 %d 到 %d 小时之间", ErrInvalid, models.CookieCloudMinIntervalHours, models.CookieCloudMaxIntervalHours)
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	r, err := s.row(ctx)
	if err != nil {
		return Settings{}, err
	}
	cols := map[string]any{"server_url": server, "uuid": uuid, "auto_sync": u.AutoSync, "interval_hours": u.IntervalHours}
	pwEnc := r.PasswordEncrypted
	if u.Password != nil {
		pwEnc = ""
		if pw := *u.Password; pw != "" {
			if s.cfg.Cipher == nil {
				return Settings{}, errors.New("没有可用的加密密钥，不能保存密码")
			}
			if pwEnc, err = s.cfg.Cipher.Encrypt(pw); err != nil {
				return Settings{}, fmt.Errorf("加密密码失败: %w", err)
			}
		}
		cols["password_encrypted"] = pwEnc
	}
	if u.AutoSync && (server == "" || uuid == "" || pwEnc == "") {
		return Settings{}, fmt.Errorf("%w：打开定时同步前要填写服务地址、UUID 和密码", ErrInvalid)
	}
	db := s.cfg.DB.WithContext(context.WithoutCancel(ctx))
	if err := ensureRow(db); err != nil {
		return Settings{}, fmt.Errorf("保存 CookieCloud 设置失败: %w", err)
	}
	if err := db.Model(&models.CookieCloudSetting{}).Where("id = 1").Updates(cols).Error; err != nil {
		return Settings{}, fmt.Errorf("保存 CookieCloud 设置失败: %w", err)
	}
	return s.Settings(ctx)
}

// normalizeServer 去掉空白和末尾的 /；不为空时必须是 http(s) 地址。
func normalizeServer(raw string) (string, error) {
	server := strings.TrimRight(strings.TrimSpace(raw), "/")
	if server == "" {
		return "", nil
	}
	u, err := url.Parse(server)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", fmt.Errorf("%w：服务地址要以 http:// 或 https:// 开头", ErrInvalid)
	}
	return server, nil
}

// ensureRow 建出唯一的那一行设置（已有时不动）。
func ensureRow(db *gorm.DB) error {
	var r models.CookieCloudSetting
	return db.FirstOrCreate(&r, models.CookieCloudSetting{ID: 1}).Error
}

// Due 报告这一刻该不该定时同步：打开了、填全了、距上次同步满了间隔。
func (s *Service) Due(ctx context.Context) bool {
	r, err := s.row(ctx)
	if err != nil || !r.AutoSync || r.ServerURL == "" || r.UUID == "" || r.PasswordEncrypted == "" {
		return false
	}
	if r.LastSyncAt == nil {
		return true
	}
	interval := r.IntervalHours
	if interval == 0 {
		interval = models.CookieCloudDefaultIntervalHours
	}
	return !s.cfg.Now().Before(r.LastSyncAt.Add(time.Duration(interval) * time.Hour))
}

// RecordSync 记下这一次同步的时间和结果。
func (s *Service) RecordSync(ctx context.Context, at time.Time, summary string) error {
	db := s.cfg.DB.WithContext(context.WithoutCancel(ctx))
	if err := ensureRow(db); err != nil {
		return fmt.Errorf("保存 CookieCloud 设置失败: %w", err)
	}
	if err := db.Model(&models.CookieCloudSetting{}).Where("id = 1").
		Updates(map[string]any{"last_sync_at": at, "last_result": summary}).Error; err != nil {
		return fmt.Errorf("记录同步结果失败: %w", err)
	}
	return nil
}

// ---------- 取数与匹配 ----------

// load 取回 CookieCloud 的数据并在本机解密，按 pt-tools 的站点挑出 Cookie。
func (s *Service) load(ctx context.Context) ([]Match, map[string]SiteState, int, error) {
	r, err := s.row(ctx)
	if err != nil {
		return nil, nil, 0, err
	}
	if r.ServerURL == "" || r.UUID == "" || r.PasswordEncrypted == "" {
		return nil, nil, 0, ErrNotConfigured
	}
	if s.cfg.Cipher == nil {
		return nil, nil, 0, errors.New("没有可用的加密密钥，读不出密码")
	}
	password, err := s.cfg.Cipher.Decrypt(r.PasswordEncrypted)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("解密保存的密码失败: %w", err)
	}
	payload, err := Fetch(ctx, s.cfg.HTTP, r.ServerURL, r.UUID)
	if err != nil {
		return nil, nil, 0, err
	}
	data, err := Decrypt(payload, r.UUID, password)
	if err != nil {
		return nil, nil, 0, err
	}
	sites, err := s.cfg.Sites(ctx)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("读取站点失败: %w", err)
	}
	byName := make(map[string]SiteState, len(sites))
	list := make([]Site, 0, len(sites))
	for _, st := range sites {
		byName[st.Name] = st
		list = append(list, Site{Name: st.Name, BaseURL: st.BaseURL})
	}
	return MatchSites(data, list, s.cfg.Now()), byName, len(data.CookieData), nil
}

// PreviewItem 是一个能导入的站点：CookieCloud 里有对它的地址有效的 Cookie。不含 Cookie 的值。
type PreviewItem struct {
	Site        string   `json:"site"`
	SiteName    string   `json:"site_name"`
	Host        string   `json:"host"`
	CookieNames []string `json:"cookie_names"`
	// Enabled 表示站点在 pt-tools 里已经启用；Changed 表示和现在保存的 Cookie 不同。
	Enabled bool `json:"enabled"`
	Changed bool `json:"changed"`
}

// Preview 是预览结果。Domains 是 CookieCloud 里一共有多少个域名。
type Preview struct {
	Items   []PreviewItem `json:"items"`
	Domains int           `json:"domains"`
}

// Preview 取回、解密并匹配，列出能导入的站点；不写任何东西。
func (s *Service) Preview(ctx context.Context) (Preview, error) {
	if !s.runMu.TryLock() {
		return Preview{}, ErrBusy
	}
	defer s.runMu.Unlock()
	matches, sites, domains, err := s.load(ctx)
	if err != nil {
		return Preview{}, err
	}
	out := Preview{Items: make([]PreviewItem, 0, len(matches)), Domains: domains}
	for _, m := range matches {
		st := sites[m.Site]
		name := st.DisplayName
		if name == "" {
			name = m.Site
		}
		out.Items = append(out.Items, PreviewItem{
			Site: m.Site, SiteName: name, Host: m.Host, CookieNames: m.Names,
			Enabled: st.Enabled, Changed: !SameCookie(m.Header, st.Cookie),
		})
	}
	return out, nil
}

// ImportResult 是一次导入或同步的结果。
type ImportResult struct {
	// Imported 是写入了 Cookie 的站点；Unchanged 是 Cookie 没有变化、没写的站点；Missing 是选了、但 CookieCloud 里没有它的 Cookie 的站点；
	// Failed 是写不进去的站点。
	Imported  []string    `json:"imported"`
	Unchanged []string    `json:"unchanged"`
	Missing   []string    `json:"missing"`
	Failed    []SiteError `json:"failed"`
}

// SiteError 是一个站点写不进去的原因。
type SiteError struct {
	Site  string `json:"site"`
	Error string `json:"error"`
}

func newResult() ImportResult {
	return ImportResult{Imported: []string{}, Unchanged: []string{}, Missing: []string{}, Failed: []SiteError{}}
}

// Summary 是一句话的结果（不含 Cookie 内容）。
func (r ImportResult) Summary() string {
	parts := []string{}
	if len(r.Imported) > 0 {
		parts = append(parts, fmt.Sprintf("更新了 %d 个站点的 Cookie（%s）", len(r.Imported), strings.Join(r.Imported, "、")))
	}
	if len(r.Unchanged) > 0 {
		parts = append(parts, fmt.Sprintf("%d 个没有变化", len(r.Unchanged)))
	}
	if len(r.Missing) > 0 {
		parts = append(parts, fmt.Sprintf("%d 个在 CookieCloud 里没有找到（%s）", len(r.Missing), strings.Join(r.Missing, "、")))
	}
	if len(r.Failed) > 0 {
		names := make([]string, 0, len(r.Failed))
		for _, f := range r.Failed {
			names = append(names, f.Site)
		}
		parts = append(parts, fmt.Sprintf("%d 个写入失败（%s）", len(r.Failed), strings.Join(names, "、")))
	}
	if len(parts) == 0 {
		return "没有可以更新的站点"
	}
	return strings.Join(parts, "，")
}

// Import 把选中站点的 Cookie 写进 pt-tools（没启用的站点会被启用）。每次重新取数解密，不用预览时的结果。
func (s *Service) Import(ctx context.Context, siteNames []string) (ImportResult, error) {
	names := dedupe(siteNames)
	switch {
	case len(names) == 0:
		return ImportResult{}, fmt.Errorf("%w：至少选一个站点", ErrInvalid)
	case len(names) > MaxImportSites:
		return ImportResult{}, fmt.Errorf("%w：一次最多 %d 个站点", ErrInvalid, MaxImportSites)
	}
	if !s.runMu.TryLock() {
		return ImportResult{}, ErrBusy
	}
	defer s.runMu.Unlock()
	matches, sites, _, err := s.load(ctx)
	if err != nil {
		return ImportResult{}, err
	}
	byName := make(map[string]Match, len(matches))
	for _, m := range matches {
		byName[m.Site] = m
	}
	res := newResult()
	cookies := map[string]string{}
	for _, n := range names {
		m, ok := byName[n]
		switch {
		case !ok:
			res.Missing = append(res.Missing, n)
		case sites[n].Enabled && SameCookie(m.Header, sites[n].Cookie):
			res.Unchanged = append(res.Unchanged, n)
		default:
			cookies[n] = m.Header
			res.Imported = append(res.Imported, n)
		}
	}
	if err := s.apply(ctx, cookies, &res); err != nil {
		return ImportResult{}, err
	}
	s.cfg.Logger.Infof("[CookieCloud] 导入：%s", res.Summary())
	return res, nil
}

// Sync 是定时同步：只更新已经启用、Cookie 有变化的站点，不启用新站点。
func (s *Service) Sync(ctx context.Context) (ImportResult, error) {
	if !s.runMu.TryLock() {
		return ImportResult{}, ErrBusy
	}
	defer s.runMu.Unlock()
	matches, sites, _, err := s.load(ctx)
	if err != nil {
		return ImportResult{}, err
	}
	res := newResult()
	cookies := map[string]string{}
	for _, m := range matches {
		st := sites[m.Site]
		if !st.Enabled {
			continue
		}
		if SameCookie(m.Header, st.Cookie) {
			res.Unchanged = append(res.Unchanged, m.Site)
			continue
		}
		cookies[m.Site] = m.Header
		res.Imported = append(res.Imported, m.Site)
	}
	if err := s.apply(ctx, cookies, &res); err != nil {
		return ImportResult{}, err
	}
	s.cfg.Logger.Infof("[CookieCloud] 定时同步：%s", res.Summary())
	return res, nil
}

// apply 写入 Cookie，把写不进去的站点从 Imported 挪到 Failed。
func (s *Service) apply(ctx context.Context, cookies map[string]string, res *ImportResult) error {
	if len(cookies) == 0 {
		return nil
	}
	if s.cfg.Apply == nil {
		return errors.New("没有可用的站点写入方式")
	}
	failed := s.cfg.Apply(ctx, cookies)
	if len(failed) == 0 {
		return nil
	}
	kept := res.Imported[:0]
	for _, n := range res.Imported {
		if err, ok := failed[n]; ok {
			res.Failed = append(res.Failed, SiteError{Site: n, Error: err.Error()})
			continue
		}
		kept = append(kept, n)
	}
	res.Imported = kept
	return nil
}

// SameCookie 报告两个 Cookie 请求头是不是同一组 name=value（顺序与空白不计）。
func SameCookie(a, b string) bool {
	pa, pb := cookiePairs(a), cookiePairs(b)
	if len(pa) != len(pb) {
		return false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return false
		}
	}
	return true
}

func cookiePairs(h string) []string {
	out := []string{}
	for _, p := range strings.Split(h, ";") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
