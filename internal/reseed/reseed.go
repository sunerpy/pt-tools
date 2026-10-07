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
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/internal/iyuu"
	"github.com/sunerpy/pt-tools/internal/transfer"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

const (
	// siteListTTL 是 IYUU 站点列表的缓存时间。
	siteListTTL = 12 * time.Hour
	// maxRateLimitWait 是限流时愿意原地等待的最长时间；更长就停下这一轮，下次再来。
	maxRateLimitWait = time.Minute
	// maxRecordsListed 是记录列表最多返回的条数。
	maxRecordsListed = 200
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
	Enabled          bool       `json:"enabled"`
	HasToken         bool       `json:"has_token"`
	IntervalHours    int        `json:"interval_hours"`
	DownloaderIDs    []uint     `json:"downloader_ids"`
	SiteNames        []string   `json:"site_names"`
	MaxPerSitePerDay int        `json:"max_per_site_per_day"`
	LastRunAt        *time.Time `json:"last_run_at,omitempty"`
	LastResult       string     `json:"last_result"`
}

// SettingsUpdate 是保存设置的请求。Token 为 nil 时保留原来的；为空串时清除。
type SettingsUpdate struct {
	Enabled          bool     `json:"enabled"`
	Token            *string  `json:"token,omitempty"`
	IntervalHours    int      `json:"interval_hours"`
	DownloaderIDs    []uint   `json:"downloader_ids"`
	SiteNames        []string `json:"site_names"`
	MaxPerSitePerDay int      `json:"max_per_site_per_day"`
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
	return view(r), nil
}

func view(r models.ReseedSetting) Settings {
	out := Settings{
		Enabled: r.Enabled, HasToken: r.TokenEncrypted != "", IntervalHours: r.IntervalHours,
		DownloaderIDs: decodeUints(r.DownloaderIDs), SiteNames: decodeStrings(r.SiteNames),
		MaxPerSitePerDay: r.MaxPerSitePerDay, LastRunAt: r.LastRunAt, LastResult: r.LastResult,
	}
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
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	r, err := s.row(ctx)
	if err != nil {
		return Settings{}, err
	}
	cols := map[string]any{
		"enabled":              u.Enabled,
		"interval_hours":       u.IntervalHours,
		"max_per_site_per_day": u.MaxPerSitePerDay,
		"downloader_ids":       encode(dedupeUints(u.DownloaderIDs)),
		"site_names":           encode(dedupeStrings(u.SiteNames)),
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
		return true
	}
	interval := r.IntervalHours
	if interval == 0 {
		interval = models.ReseedDefaultIntervalHours
	}
	return !s.cfg.Now().Before(r.LastRunAt.Add(time.Duration(interval) * time.Hour))
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

func (s *Service) iyuuSites(ctx context.Context, client *iyuu.Client, token string) ([]iyuu.Site, error) {
	s.mu.Lock()
	if s.sites != nil && s.sitesFrom == token && s.cfg.Now().Sub(s.sitesAt) < siteListTTL {
		out := s.sites
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Unlock()
	sites, err := client.Sites(ctx)
	if err != nil {
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
	sites, err := s.iyuuSites(ctx, s.cfg.NewClient(token), token)
	if err != nil {
		return nil, err
	}
	return s.mapSites(sites, decodeStrings(r.SiteNames)), nil
}

func (s *Service) mapSites(sites []iyuu.Site, selected []string) []SiteMapItem {
	configured := map[string]bool{}
	if s.cfg.SiteIDs != nil {
		for _, id := range s.cfg.SiteIDs() {
			configured[id] = true
		}
	}
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
		item.Selected = item.Configured && (len(pick) == 0 || pick[name])
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

// ---------- 运行 ----------

// RunResult 是一轮辅种的结果。
type RunResult struct {
	Downloaders int            `json:"downloaders"`
	Hashes      int            `json:"hashes"`
	Candidates  int            `json:"candidates"`
	Created     int            `json:"created"`
	Failed      int            `json:"failed"`
	Skipped     map[string]int `json:"skipped,omitempty"`
	Errors      []string       `json:"errors,omitempty"`
	// Stopped 说明这一轮为什么提前停下（限流等）。
	Stopped string `json:"stopped,omitempty"`
}

// 跳过的原因。
const (
	SkipNotSelected = "站点不在辅种范围"
	SkipExists      = "下载器里已经有"
	SkipTried       = "已经尝试过"
	SkipDailyLimit  = "今天这个站点已经加满"
	SkipActive      = "已有进行中的任务"
)

func (r *RunResult) skip(why string) {
	if r.Skipped == nil {
		r.Skipped = map[string]int{}
	}
	r.Skipped[why]++
}

// Summary 是写进设置 last_result 的一句话。
func (r RunResult) Summary() string {
	parts := []string{fmt.Sprintf("查询 %d 个种子，找到 %d 个可辅种，加入 %d 个", r.Hashes, r.Candidates, r.Created)}
	if r.Failed > 0 {
		parts = append(parts, fmt.Sprintf("失败 %d 个", r.Failed))
	}
	if n := r.skippedTotal(); n > 0 {
		parts = append(parts, fmt.Sprintf("跳过 %d 个", n))
	}
	if r.Stopped != "" {
		parts = append(parts, r.Stopped)
	}
	if len(r.Errors) > 0 {
		parts = append(parts, r.Errors[0])
	}
	return strings.Join(parts, "，")
}

func (r RunResult) skippedTotal() int {
	n := 0
	for _, v := range r.Skipped {
		n += v
	}
	return n
}

// source 是下载器里一个已经下完的种子。
type source struct {
	dlID    uint
	dl      downloader.Downloader
	torrent downloader.Torrent
}

// Run 运行一轮辅种。返回的错误是这一轮根本没法开始（没有 token、没有站点等）；单个下载器或站点的问题记在结果里。
func (s *Service) Run(ctx context.Context) (RunResult, error) {
	res := RunResult{}
	r, err := s.row(ctx)
	if err != nil {
		return res, err
	}
	token, err := s.token(ctx, r)
	if err != nil {
		return res, err
	}
	client := s.cfg.NewClient(token)
	sites, err := s.iyuuSites(ctx, client, token)
	if err != nil {
		return res, fmt.Errorf("读取 IYUU 站点列表失败: %w", err)
	}
	bySID := map[int]string{}
	sids := make([]int, 0)
	for _, it := range s.mapSites(sites, decodeStrings(r.SiteNames)) {
		if it.Selected {
			bySID[it.SID] = it.SiteName
			sids = append(sids, it.SID)
		}
	}
	if len(sids) == 0 {
		return res, ErrNoSites
	}
	sort.Ints(sids)
	sidSha1, err := s.sidSha1(ctx, client, &r, sids, false)
	if err != nil {
		return res, fmt.Errorf("向 IYUU 报告站点失败: %w", err)
	}

	sources, present := s.collect(ctx, r, &res)
	hashes := make([]string, 0, len(sources))
	for h := range sources {
		hashes = append(hashes, h)
	}
	sort.Strings(hashes)
	res.Hashes = len(hashes)

	limit := r.MaxPerSitePerDay
	if limit == 0 {
		limit = models.ReseedDefaultMaxPerSitePerDay
	}
	today, err := s.todayCounts(ctx)
	if err != nil {
		return res, err
	}
	refreshed := false
	for start := 0; start < len(hashes); start += iyuu.MaxBatch {
		if ctx.Err() != nil {
			res.Stopped = "这一轮超过时限，已停下"
			break
		}
		// token 在这一轮里被清除或更换时停下，不再用旧 token 发送 info hash
		if cur, err := s.row(ctx); err != nil {
			res.Errors = append(res.Errors, err.Error())
			break
		} else if cur.TokenEncrypted != r.TokenEncrypted {
			res.Stopped = "IYUU token 已清除或更换，这一轮停下"
			break
		}
		batch := hashes[start:min(start+iyuu.MaxBatch, len(hashes))]
		found, err := s.query(ctx, client, batch, &sidSha1, &r, sids, &refreshed)
		if err != nil {
			var rl *iyuu.RateLimitError
			if errors.As(err, &rl) {
				res.Stopped = rl.Error()
			} else {
				res.Errors = append(res.Errors, "查询 IYUU 失败: "+err.Error())
			}
			break
		}
		s.handle(ctx, found, sources, present, bySID, today, limit, &res)
	}
	return res, nil
}

// collect 读出范围内各下载器里已经下完的种子（按小写 info hash），以及全部下载器里已经有的 info hash。
func (s *Service) collect(ctx context.Context, r models.ReseedSetting, res *RunResult) (map[string]source, map[string]bool) {
	ids := decodeUints(r.DownloaderIDs)
	if len(ids) == 0 {
		var rows []models.DownloaderSetting
		if err := s.cfg.DB.WithContext(ctx).Where("enabled = ?", true).Order("id").Find(&rows).Error; err != nil {
			res.Errors = append(res.Errors, "读取下载器失败: "+err.Error())
		}
		for _, d := range rows {
			ids = append(ids, d.ID)
		}
	}
	sources := map[string]source{}
	present := map[string]bool{}
	for _, id := range ids {
		dl, set, err := s.cfg.Downloaders.TransferDownloader(ctx, id)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("下载器 %d 不可用: %v", id, err))
			continue
		}
		torrents, err := dl.GetAllTorrents()
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("读取 %s 的种子失败: %v", set.Name, err))
			continue
		}
		res.Downloaders++
		for _, t := range torrents {
			h := strings.ToLower(t.InfoHash)
			if h == "" {
				continue
			}
			present[h] = true
			if !(t.IsCompleted || t.Progress >= 1) || t.State == downloader.TorrentError || t.State == downloader.TorrentChecking {
				continue
			}
			if _, ok := sources[h]; !ok {
				sources[h] = source{dlID: id, dl: dl, torrent: t}
			}
		}
	}
	return sources, present
}

// sidSha1 返回查询要带的站点哈希：报告的站点没变、7 天之内就用缓存的；force 时重新报告。
func (s *Service) sidSha1(ctx context.Context, client *iyuu.Client, r *models.ReseedSetting, sids []int, force bool) (string, error) {
	key := joinInts(sids)
	now := s.cfg.Now()
	if !force && r.SidSha1 != "" && r.SidKey == key && r.SidAt != nil && now.Sub(*r.SidAt) < models.ReseedSidTTL {
		return r.SidSha1, nil
	}
	sum, err := client.ReportExisting(ctx, sids)
	if err != nil {
		return "", err
	}
	r.SidSha1, r.SidKey, r.SidAt = sum, key, &now
	// 只写缓存列，并且只在 token 没换过时写：这一轮读出设置之后，token 可能已经被清除或更换
	if err := s.cfg.DB.WithContext(context.WithoutCancel(ctx)).Model(&models.ReseedSetting{}).
		Where("id = 1 AND token_encrypted = ?", r.TokenEncrypted).
		Updates(map[string]any{"sid_sha1": sum, "sid_key": key, "sid_at": now}).Error; err != nil {
		s.cfg.Logger.Warnf("[辅种] 保存 sid_sha1 失败: %v", err)
	}
	return sum, nil
}

// query 查询一批：限流且等待不长时等一次再查；IYUU 报错时重新报告站点、再查一次（每轮最多一次）。
func (s *Service) query(ctx context.Context, client *iyuu.Client, batch []string, sidSha1 *string,
	r *models.ReseedSetting, sids []int, refreshed *bool,
) (map[string][]iyuu.Candidate, error) {
	found, err := client.Query(ctx, batch, *sidSha1)
	var rl *iyuu.RateLimitError
	if errors.As(err, &rl) && rl.RetryAfter <= maxRateLimitWait {
		if serr := s.cfg.Sleep(ctx, rl.RetryAfter); serr != nil {
			return nil, err
		}
		found, err = client.Query(ctx, batch, *sidSha1)
	}
	var apiErr *iyuu.APIError
	if errors.As(err, &apiErr) && !*refreshed {
		*refreshed = true
		sum, rerr := s.sidSha1(ctx, client, r, sids, true)
		if rerr != nil {
			return nil, err
		}
		*sidSha1 = sum
		found, err = client.Query(ctx, batch, *sidSha1)
	}
	return found, err
}

// todayCounts 返回今天（进程时区）每个站点已经尝试过的个数。
func (s *Service) todayCounts(ctx context.Context) (map[string]int, error) {
	now := s.cfg.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var rows []struct {
		SiteName string
		N        int
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.ReseedRecord{}).Select("site_name, COUNT(*) AS n").
		Where("updated_at >= ?", start).Group("site_name").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("统计今天的辅种失败: %w", err)
	}
	out := map[string]int{}
	for _, r := range rows {
		out[r.SiteName] = r.N
	}
	return out, nil
}

// handle 处理一批查询结果：逐个可辅种的种子判断、下载、核对，然后建任务或记下失败。
func (s *Service) handle(ctx context.Context, found map[string][]iyuu.Candidate, sources map[string]source, present map[string]bool,
	bySID map[int]string, today map[string]int, limit int, res *RunResult,
) {
	srcHashes := make([]string, 0, len(found))
	for h := range found {
		srcHashes = append(srcHashes, h)
	}
	sort.Strings(srcHashes)
	for _, srcHash := range srcHashes {
		src, ok := sources[srcHash]
		if !ok {
			continue
		}
		for _, c := range found[srcHash] {
			if ctx.Err() != nil {
				return
			}
			res.Candidates++
			site, ok := bySID[c.SID]
			switch {
			case !ok:
				res.skip(SkipNotSelected)
				continue
			case c.InfoHash == "" || present[c.InfoHash]:
				res.skip(SkipExists)
				continue
			case s.tried(ctx, c.InfoHash, site):
				res.skip(SkipTried)
				continue
			case today[site] >= limit:
				res.skip(SkipDailyLimit)
				continue
			}
			today[site]++
			present[c.InfoHash] = true
			s.attempt(ctx, src, c, site, res)
		}
	}
}

// tried 报告这个站点的这个种子是不是已经尝试过、这次不该再试：暂时失败的过了 ReseedRetryAfter 可以再试。
func (s *Service) tried(ctx context.Context, hash, site string) bool {
	var rec models.ReseedRecord
	err := s.cfg.DB.WithContext(ctx).Where("info_hash = ? AND site_name = ?", hash, site).First(&rec).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return false
	case err != nil:
		return true // 读不到就当尝试过，不冒险重复
	}
	retryNow := rec.State == models.ReseedFailed && rec.Retryable && s.cfg.Now().Sub(rec.UpdatedAt) >= models.ReseedRetryAfter
	return !retryNow
}

// attempt 下载一个可辅种的种子，核对 info hash 与文件列表，通过就建任务；结果记进 ReseedRecord。
func (s *Service) attempt(ctx context.Context, src source, c iyuu.Candidate, site string, res *RunResult) {
	rec := models.ReseedRecord{
		InfoHash: c.InfoHash, SiteName: site, TorrentID: c.TorrentID, SourceHash: strings.ToLower(src.torrent.InfoHash),
		DownloaderID: src.dlID, Name: src.torrent.Name,
	}
	fail := func(why string) {
		res.Failed++
		rec.State, rec.Message = models.ReseedFailed, why
		s.saveRecord(ctx, &rec)
	}
	// 站点不可用、下载不到种子、读不到原种子的文件列表是暂时的：记为可重试，过一段时间再试
	retryLater := func(why string) {
		rec.Retryable = true
		fail(why)
	}
	inst, ok := s.cfg.Sites(site)
	if !ok {
		retryLater("站点 " + site + " 不可用")
		return
	}
	data, err := inst.Download(ctx, c.TorrentID)
	if err != nil {
		retryLater("下载种子失败: " + err.Error())
		return
	}
	if h, hashErr := qbit.ComputeTorrentHash(data); hashErr != nil || !strings.EqualFold(h, c.InfoHash) {
		fail("下载到的种子与 IYUU 给的 info hash 不符")
		return
	}
	newFiles, err := TorrentFiles(data)
	if err != nil {
		fail(err.Error())
		return
	}
	oldFiles, err := src.dl.GetTorrentFiles(src.torrent.ID)
	if err != nil {
		// 下载器暂时读不到（重启中、连不上）：还没核对，过一段时间再试
		retryLater("读取原种子的文件列表失败: " + err.Error())
		return
	}
	if same, why := SameFiles(newFiles, DownloaderFiles(oldFiles)); !same {
		fail("文件列表与原种子不一致：" + why)
		return
	}
	job := models.TorrentTransferJob{
		TargetDownloaderID: src.dlID, InfoHash: c.InfoHash, Name: src.torrent.Name, TotalSize: src.torrent.TotalSize,
		SiteName: site, TorrentID: c.TorrentID, SourceSavePath: src.torrent.SavePath, TargetSavePath: src.torrent.SavePath,
		Category: src.torrent.Category, Tags: site, TorrentData: data,
	}
	if err := s.cfg.Transfer.EnqueueReseed(ctx, &job); err != nil {
		if errors.Is(err, transfer.ErrJobActive) {
			res.skip(SkipActive)
			return
		}
		if errors.Is(err, transfer.ErrInvalid) {
			fail(err.Error())
		} else {
			retryLater("建辅种任务失败: " + err.Error()) // 写库失败等暂时的问题
		}
		return
	}
	res.Created++
	rec.State, rec.JobID = models.ReseedQueued, &job.ID
	s.saveRecord(ctx, &rec)
}

// saveRecord 写入（或在重试时更新）这个站点这个种子的记录。
func (s *Service) saveRecord(ctx context.Context, rec *models.ReseedRecord) {
	now := s.cfg.Now()
	rec.CreatedAt, rec.UpdatedAt = now, now
	if err := s.cfg.DB.WithContext(context.WithoutCancel(ctx)).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "info_hash"}, {Name: "site_name"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"torrent_id", "source_hash", "downloader_id", "name", "state", "message", "retryable", "job_id", "updated_at",
		}),
	}).Create(rec).Error; err != nil {
		s.cfg.Logger.Warnf("[辅种] 记录 %s@%s 失败: %v", rec.InfoHash, rec.SiteName, err)
	}
}

// ---------- 记录 ----------

// RecordView 是记录列表里的一项：记录加上任务的状态。JobMissing 表示记录指向的任务已经不在了（结果不知道）。
type RecordView struct {
	models.ReseedRecord
	JobState   string `json:"job_state,omitempty"`
	JobMessage string `json:"job_message,omitempty"`
	JobMissing bool   `json:"job_missing,omitempty"`
}

// Records 返回最近尝试的辅种记录（重试过的按最后一次尝试的时间排）。
func (s *Service) Records(ctx context.Context) ([]RecordView, error) {
	var recs []models.ReseedRecord
	if err := s.cfg.DB.WithContext(ctx).Order("updated_at DESC, id DESC").Limit(maxRecordsListed).Find(&recs).Error; err != nil {
		return nil, fmt.Errorf("读取辅种记录失败: %w", err)
	}
	ids := make([]uint, 0)
	for _, r := range recs {
		if r.JobID != nil {
			ids = append(ids, *r.JobID)
		}
	}
	jobs := map[uint]models.TorrentTransferJob{}
	if len(ids) > 0 {
		var rows []models.TorrentTransferJob
		if err := s.cfg.DB.WithContext(ctx).Omit("torrent_data").Where("id IN ?", ids).Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("读取辅种任务失败: %w", err)
		}
		for _, j := range rows {
			jobs[j.ID] = j
		}
	}
	out := make([]RecordView, 0, len(recs))
	for _, r := range recs {
		v := RecordView{ReseedRecord: r}
		if r.JobID != nil {
			if j, ok := jobs[*r.JobID]; ok {
				v.JobState, v.JobMessage = j.State, j.Message
			} else {
				v.JobMissing = true
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// finalJobStates 是辅种任务结束时的状态。
var finalJobStates = []string{models.TransferDone, models.TransferRolledBack, models.TransferFailed, models.TransferCanceled}

// ClearFinishedJobs 删除已经结束的辅种任务，返回删除的条数。删除前把任务的结果写进引用它的辅种记录，
// 记录不再指向任务；不改记录的更新时间（重试间隔和每天的上限按它算）。
func (s *Service) ClearFinishedJobs(ctx context.Context) (int64, error) {
	var n int64
	err := s.cfg.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var jobs []models.TorrentTransferJob
		if err := tx.Omit("torrent_data").Where("kind = ? AND state IN ?", models.JobKindReseed, finalJobStates).
			Find(&jobs).Error; err != nil {
			return err
		}
		if len(jobs) == 0 {
			return nil
		}
		ids := make([]uint, 0, len(jobs))
		for _, j := range jobs {
			ids = append(ids, j.ID)
			state, msg := recordResult(j)
			if err := tx.Model(&models.ReseedRecord{}).Where("job_id = ?", j.ID).
				UpdateColumns(map[string]any{"state": state, "message": msg, "job_id": nil}).Error; err != nil {
				return err
			}
		}
		res := tx.Where("id IN ? AND state IN ?", ids, finalJobStates).Delete(&models.TorrentTransferJob{})
		n = res.RowsAffected
		return res.Error
	})
	if err != nil {
		return 0, fmt.Errorf("清除已结束的辅种任务失败: %w", err)
	}
	return n, nil
}

// recordResult 是结束的辅种任务写进记录里的结果和说明。
func recordResult(j models.TorrentTransferJob) (string, string) {
	switch j.State {
	case models.TransferDone:
		return models.ReseedDone, j.Message
	case models.TransferRolledBack:
		return models.ReseedRolledBack, j.Message
	case models.TransferCanceled:
		return models.ReseedCanceled, j.Message
	default:
		return models.ReseedFailed, j.Message
	}
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
