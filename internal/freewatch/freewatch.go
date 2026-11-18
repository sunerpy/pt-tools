// Package freewatch 是「等免费再下」（媒体中心 M2b）：记下一个现在不免费的种子要推到哪里，后台定时读它现在的优惠，
// 免费了才推送（照常过免费门禁、磁盘与站点容量闸门，免费结束时暂停），等到期了还没免费就放弃。
package freewatch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

const (
	// MaxWatching 是同时在等的最多几条
	MaxWatching = 200
	// DefaultDays、MaxDays 是等多久（天）的默认值与上限
	DefaultDays = 7
	MaxDays     = 30
	// PerSiteRound 是每一轮每个站点最多读几个种子（不给站点添负担）
	PerSiteRound = 10
	// NotFreeRetry 是还没免费（或者免费剩得太少、闸门没放行）时隔多久再看
	NotFreeRetry = 30 * time.Minute
	// MaxFailures 是连续读不到几次以后不再等
	MaxFailures = 5
	// maxDue 是一轮最多处理几条到点的
	maxDue = 500
	// ListLimit 是列表最多列几条（新建的在前）；各状态的条数照实数
	ListLimit = 500
)

// failBackoff 是第 1、2、3、4 次读不到以后隔多久再看。
var failBackoff = []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour, 4 * time.Hour}

var (
	ErrInvalid  = errors.New("参数不对")
	ErrNotFound = errors.New("没有这条记录")
)

// 通知的种类（Message.Kind）。
const (
	KindPushed  = "pushed"
	KindExpired = "expired"
	KindFailed  = "failed"
)

// Message 是要发的一条通知：推送了、到期了、读不到不再等。Event 由 NoticeEvent 生成。
type Message struct {
	Kind  string
	Watch models.FreeWatch
	// Downloader 是推送到的下载器名字（推送了时）
	Downloader string
	// Snapshot 是推送时读到的优惠（推送了时）
	Snapshot *v2.PromotionSnapshot
	// Error 是最近一次读不到的原因（不再等时）
	Error string
}

// Config 是等免费的依赖，测试里都换成假的。
type Config struct {
	DB *gorm.DB
	// Site 是注册了、启用着的站点；没有时为 nil
	Site func(name string) v2.Site
	// Fresh 读种子现在的优惠（生产是 internal.FreshPromotion），读不到时返回 nil
	Fresh func(ctx context.Context, site v2.Site, siteName, torrentID string, attemptStart time.Time) *v2.PromotionSnapshot
	// Push 推送（生产是 internal.PushTorrentToDownloader）
	Push func(ctx context.Context, req ptinternal.PushTorrentRequest) (*ptinternal.PushTorrentResult, error)
	// Notify 发通知；为空时不发
	Notify func(ctx context.Context, m Message)
	// SafetyMargin 是免费剩余时间至少要有多少（为空时用 internal.DefaultSafetyMargin）
	SafetyMargin func() time.Duration
	Now          func() time.Time
	Logger       *zap.SugaredLogger
}

// Service 管理等免费的记录并做定时检查。
type Service struct {
	cfg Config
	// createMu 让新建（查重、数条数、写入）一条一条来：同一个种子不会同时有两条在等
	createMu sync.Mutex
	// mu 让检查、取消与改到期一条一条来：检查（读优惠、推送）当中不会被取消，取消以后不会再推送
	mu sync.Mutex
}

// New 建服务。
func New(cfg Config) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	// 写进库的时间一律用本地时区（同 internal.DBTime），查询比较用 julianday()：SQLite 按文本存时间，混着时区比文本会差几个小时
	clock := cfg.Now
	cfg.Now = func() time.Time { return ptinternal.DBTime(clock()) }
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop().Sugar()
	}
	if cfg.SafetyMargin == nil {
		cfg.SafetyMargin = ptinternal.DefaultSafetyMargin
	}
	if cfg.Fresh == nil {
		cfg.Fresh = ptinternal.FreshPromotion
	}
	if cfg.Push == nil {
		cfg.Push = ptinternal.PushTorrentToDownloader
	}
	return &Service{cfg: cfg}
}

// Supports 报告这个站点能不能等免费：注册了、启用着，而且能读种子详情（读优惠要用）。
func (s *Service) Supports(name string) bool {
	if s.cfg.Site == nil {
		return false
	}
	return canRead(s.cfg.Site(name))
}

func canRead(site v2.Site) bool {
	if site == nil {
		return false
	}
	p, ok := site.(v2.DetailFetcherProvider)
	return ok && p.GetDetailFetcher() != nil
}

// CreateInput 是新建一条等免费：要推的种子、推到哪里（同推送弹窗）、等多久。
type CreateInput struct {
	Site         string `json:"site"`
	TorrentID    string `json:"torrent_id"`
	Title        string `json:"title"`
	Subtitle     string `json:"subtitle"`
	SizeBytes    int64  `json:"size_bytes"`
	HasHR        bool   `json:"has_hr"`
	HRSeedTimeH  int    `json:"hr_seed_time_h"`
	DownloaderID uint   `json:"downloader_id"`
	SavePath     string `json:"save_path"`
	Category     string `json:"category"`
	Tags         string `json:"tags"`
	// MediaType、TMDBID、Season 是按条目搜索里带上的条目（可选）
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
	Season    int    `json:"season"`
	// Days 是等多久（1–30 天，0 是 7 天）
	Days int `json:"days"`
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}

func (in *CreateInput) normalize() error {
	in.Site, in.TorrentID = strings.TrimSpace(in.Site), strings.TrimSpace(in.TorrentID)
	if in.Site == "" || in.TorrentID == "" || len(in.Site) > 64 || len(in.TorrentID) > 128 {
		return fmt.Errorf("%w: 种子的站点或编号不对", ErrInvalid)
	}
	in.Title, in.Subtitle = clip(in.Title, 512), clip(in.Subtitle, 512)
	in.SavePath, in.Category, in.Tags = clip(in.SavePath, 1024), clip(in.Category, 128), clip(in.Tags, 255)
	if in.DownloaderID == 0 {
		return fmt.Errorf("%w: 要选下载器", ErrInvalid)
	}
	if in.Days == 0 {
		in.Days = DefaultDays
	}
	if in.Days < 1 || in.Days > MaxDays {
		return fmt.Errorf("%w: 等多久要在 1 到 %d 天之间", ErrInvalid, MaxDays)
	}
	in.SizeBytes, in.HRSeedTimeH = max(in.SizeBytes, 0), max(in.HRSeedTimeH, 0)
	switch {
	case in.TMDBID <= 0:
		in.MediaType, in.TMDBID, in.Season = "", 0, 0
	case in.MediaType == models.MediaKindMovie:
		in.Season = 0
	case in.MediaType == models.MediaKindTV:
		in.Season = max(in.Season, 0)
	default:
		return fmt.Errorf("%w: 条目的类型要是电影或剧集", ErrInvalid)
	}
	return nil
}

// Create 新建一条等免费，建好以后马上可以检查（NextCheckAt 是现在）。站点要能读种子详情，下载器要存在并启用；
// 同一个种子已经在等时报错；在等的最多 MaxWatching 条。
func (s *Service) Create(ctx context.Context, in CreateInput) (*models.FreeWatch, error) {
	if err := in.normalize(); err != nil {
		return nil, err
	}
	if !s.Supports(in.Site) {
		return nil, fmt.Errorf("%w: 站点 %s 读不了种子详情（或者没有启用），不能等免费", ErrInvalid, in.Site)
	}
	db := s.cfg.DB.WithContext(ctx)
	var dl models.DownloaderSetting
	if err := db.Where("id = ?", in.DownloaderID).Limit(1).Find(&dl).Error; err != nil {
		return nil, fmt.Errorf("读取下载器失败: %w", err)
	}
	if dl.ID == 0 {
		return nil, fmt.Errorf("%w: 下载器不存在", ErrInvalid)
	}
	if !dl.Enabled {
		return nil, fmt.Errorf("%w: 下载器 %s 没有启用", ErrInvalid, dl.Name)
	}
	s.createMu.Lock()
	defer s.createMu.Unlock()
	var n int64
	if err := db.Model(&models.FreeWatch{}).Where("status = ?", models.FreeWatchWatching).Count(&n).Error; err != nil {
		return nil, fmt.Errorf("读取等免费失败: %w", err)
	}
	if n >= MaxWatching {
		return nil, fmt.Errorf("%w: 在等的已经有 %d 条了，先取消一些", ErrInvalid, MaxWatching)
	}
	var dup int64
	if err := db.Model(&models.FreeWatch{}).Where("status = ? AND lower(site_name) = ? AND torrent_id = ?",
		models.FreeWatchWatching, strings.ToLower(in.Site), in.TorrentID).Count(&dup).Error; err != nil {
		return nil, fmt.Errorf("读取等免费失败: %w", err)
	}
	if dup > 0 {
		return nil, fmt.Errorf("%w: 这个种子已经在等免费了", ErrInvalid)
	}
	now := s.cfg.Now()
	row := &models.FreeWatch{
		SiteName: in.Site, TorrentID: in.TorrentID, Title: in.Title, Subtitle: in.Subtitle, SizeBytes: in.SizeBytes,
		HasHR: in.HasHR, HRSeedTimeH: in.HRSeedTimeH, DownloaderID: in.DownloaderID, SavePath: in.SavePath,
		Category: in.Category, Tags: in.Tags, MediaType: in.MediaType, TMDBID: in.TMDBID, Season: in.Season,
		Status: models.FreeWatchWatching, NextCheckAt: &now, ExpiresAt: now.Add(time.Duration(in.Days) * 24 * time.Hour),
		Message: "等第一次检查", CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(row).Error; err != nil {
		return nil, fmt.Errorf("保存等免费失败: %w", err)
	}
	return row, nil
}

// Counts 是各状态的条数。
type Counts map[string]int64

// List 列出等免费的记录（status 为空时全部），新建的在前，并给出各状态的条数。
func (s *Service) List(ctx context.Context, status string) ([]models.FreeWatch, Counts, error) {
	db := s.cfg.DB.WithContext(ctx)
	q := db.Model(&models.FreeWatch{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var rows []models.FreeWatch
	if err := q.Order("created_at DESC, id DESC").Limit(ListLimit).Find(&rows).Error; err != nil {
		return nil, nil, fmt.Errorf("读取等免费失败: %w", err)
	}
	var groups []struct {
		Status string
		N      int64
	}
	if err := db.Model(&models.FreeWatch{}).Select("status, count(*) AS n").Group("status").Scan(&groups).Error; err != nil {
		return nil, nil, fmt.Errorf("读取等免费失败: %w", err)
	}
	counts := Counts{}
	for _, g := range groups {
		counts[g.Status] = g.N
	}
	if rows == nil {
		rows = []models.FreeWatch{}
	}
	return rows, counts, nil
}

// watching 读一条在等的记录。
func (s *Service) watching(ctx context.Context, id uint) (models.FreeWatch, error) {
	var row models.FreeWatch
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&row).Error; err != nil {
		return row, fmt.Errorf("读取等免费失败: %w", err)
	}
	if row.ID == 0 {
		return row, ErrNotFound
	}
	if row.Status != models.FreeWatchWatching {
		return row, fmt.Errorf("%w: 这一条已经不在等了", ErrInvalid)
	}
	return row, nil
}

// Cancel 取消一条在等的。
func (s *Service) Cancel(ctx context.Context, id uint) (*models.FreeWatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.watching(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.finish(ctx, row, models.FreeWatchCanceled, "取消了", "", false)
}

// SetExpiry 改一条在等的到期时间：要晚于现在，最多是现在起 MaxDays 天。
func (s *Service) SetExpiry(ctx context.Context, id uint, at time.Time) (*models.FreeWatch, error) {
	now := s.cfg.Now()
	at = ptinternal.DBTime(at)
	if !at.After(now) {
		return nil, fmt.Errorf("%w: 到期时间要晚于现在", ErrInvalid)
	}
	if at.After(now.Add(MaxDays * 24 * time.Hour)) {
		return nil, fmt.Errorf("%w: 最多等到 %d 天以后", ErrInvalid, MaxDays)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.watching(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.FreeWatch{}).Where("id = ?", row.ID).
		Updates(map[string]any{"expires_at": at, "updated_at": now}).Error; err != nil {
		return nil, fmt.Errorf("保存等免费失败: %w", err)
	}
	row.ExpiresAt, row.UpdatedAt = at, now
	return &row, nil
}

// RunOnce 做一轮检查：先把到期的记成过期，再按下次检查时间取到点的，每个站点最多 PerSiteRound 个。返回读了几个种子。
func (s *Service) RunOnce(ctx context.Context) int {
	now := s.cfg.Now()
	db := s.cfg.DB.WithContext(ctx)
	var expired []models.FreeWatch
	if err := db.Where("status = ? AND julianday(expires_at) <= julianday(?)", models.FreeWatchWatching, now).Order("id").Find(&expired).Error; err != nil {
		s.cfg.Logger.Warnf("[等免费] 读取记录失败: %v", err)
		return 0
	}
	for _, w := range expired {
		s.expire(ctx, w.ID)
	}
	var due []models.FreeWatch
	if err := db.Where("status = ? AND julianday(next_check_at) <= julianday(?) AND julianday(expires_at) > julianday(?)", models.FreeWatchWatching, now, now).
		Order("julianday(next_check_at), id").Limit(maxDue).Find(&due).Error; err != nil {
		s.cfg.Logger.Warnf("[等免费] 读取记录失败: %v", err)
		return 0
	}
	perSite := map[string]int{}
	checked := 0
	for _, w := range due {
		if ctx.Err() != nil {
			break
		}
		k := strings.ToLower(w.SiteName)
		if perSite[k] >= PerSiteRound {
			continue
		}
		perSite[k]++
		checked++
		s.check(ctx, w.ID)
	}
	return checked
}

// expire 把到期的记成过期并通知。
func (s *Service) expire(ctx context.Context, id uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.watching(ctx, id)
	if err != nil || w.ExpiresAt.After(s.cfg.Now()) {
		return
	}
	if row, ferr := s.finish(ctx, w, models.FreeWatchExpired, "等到期了还没免费", "", false); ferr == nil {
		s.notify(ctx, Message{Kind: KindExpired, Watch: *row})
	}
}

// check 检查一条：读优惠，免费了就推送。
func (s *Service) check(ctx context.Context, id uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.watching(ctx, id)
	if err != nil {
		return
	}
	attempt := s.cfg.Now()
	var site v2.Site
	if s.cfg.Site != nil {
		site = s.cfg.Site(w.SiteName)
	}
	if site == nil {
		s.failed(ctx, w, "站点没有启用或者不存在")
		return
	}
	snap := s.cfg.Fresh(ctx, site, w.SiteName, w.TorrentID, attempt)
	if snap == nil {
		s.failed(ctx, w, "读不到种子现在的优惠")
		return
	}
	policy := ptinternal.AdmissionPolicy{
		RequireFree: true, PauseOnFreeEnd: true, SafetyMargin: s.cfg.SafetyMargin(),
		SourceKind: ptinternal.SourceKindFreeWatch, AttemptStart: attempt, Clock: s.cfg.Now,
	}
	if d := ptinternal.CheckAdmission(policy, snap, s.cfg.Now()); !d.Allow {
		s.later(ctx, w, notFreeText(snap, d))
		return
	}
	data, err := site.Download(ctx, w.TorrentID)
	if err != nil {
		s.failed(ctx, w, "免费了，但下载种子文件失败："+err.Error())
		return
	}
	req := ptinternal.PushTorrentRequest{
		SiteID: w.SiteName, TorrentID: w.TorrentID, TorrentData: data, Title: w.Title, Category: w.Category, Tags: w.Tags,
		SavePath: w.SavePath, DownloaderID: w.DownloaderID, Source: ptinternal.SourceKindFreeWatch,
		Meta:   &ptinternal.PushTorrentMeta{SizeBytes: w.SizeBytes, HasHR: w.HasHR, HRSeedTimeH: w.HRSeedTimeH},
		Policy: policy, Snapshot: snap,
	}
	if w.TMDBID > 0 {
		req.Media = &ptinternal.PushMedia{Type: w.MediaType, TMDBID: w.TMDBID, Season: w.Season}
	}
	res, err := s.cfg.Push(ctx, req)
	switch {
	case err != nil:
		s.failed(ctx, w, "免费了，但推送失败："+err.Error())
	case res == nil:
		s.failed(ctx, w, "免费了，但推送没有结果")
	case res.Status == ptinternal.PushStatusDuplicate || res.Skipped:
		_, _ = s.finish(ctx, w, models.FreeWatchPushed, "下载器里已经有这个种子，不用再推", res.TorrentHash, true)
	case res.Success:
		msg := "免费了，已推送"
		if res.Status == ptinternal.PushStatusMonitorDegraded {
			msg += "（免费结束监控没登记上：" + res.Message + "）"
		}
		row, ferr := s.finish(ctx, w, models.FreeWatchPushed, msg, res.TorrentHash, true)
		if ferr == nil {
			s.notify(ctx, Message{Kind: KindPushed, Watch: *row, Downloader: s.downloaderName(ctx, w.DownloaderID), Snapshot: snap})
		}
	default:
		// 门禁或闸门没放行（免费剩得太少、磁盘不够、站点做种上限）：过一会儿再看
		s.later(ctx, w, "免费了，但这次没推送："+res.Message)
	}
}

// notFreeText 是还没免费（或免费剩得太少）时的说明。
func notFreeText(snap *v2.PromotionSnapshot, d ptinternal.AdmissionDecision) string {
	if d.Status == ptinternal.PushStatusRejectedNotFree {
		if t := levelText(snap.Level); t != "" {
			return "还没免费（现在是 " + t + "）"
		}
		return "还没免费"
	}
	return d.Reason
}

// levelText 是不免费的优惠的写法；没有优惠时为空。
func levelText(l v2.DiscountLevel) string {
	switch l {
	case v2.DiscountPercent50:
		return "50%"
	case v2.DiscountPercent30:
		return "30%"
	case v2.DiscountPercent70:
		return "70%"
	case v2.Discount2xUp:
		return "2X 上传"
	case v2.Discount2x50:
		return "2X 50%"
	}
	return ""
}

// later 是还没免费：NotFreeRetry 以后再看，连续失败清零。
func (s *Service) later(ctx context.Context, w models.FreeWatch, msg string) {
	now := s.cfg.Now()
	next := now.Add(NotFreeRetry)
	if err := s.cfg.DB.WithContext(ctx).Model(&models.FreeWatch{}).Where("id = ?", w.ID).Updates(map[string]any{
		"next_check_at": next, "last_check_at": now, "check_count": w.CheckCount + 1, "failures": 0,
		"message": clip(msg, 1024), "updated_at": now,
	}).Error; err != nil {
		s.cfg.Logger.Warnf("[等免费] 保存「%s」的检查结果失败: %v", w.Title, err)
	}
}

// failed 是读不到（站点、详情、下种子、推送出错）：按 failBackoff 退避，连续 MaxFailures 次以后不再等并通知。
func (s *Service) failed(ctx context.Context, w models.FreeWatch, why string) {
	now := s.cfg.Now()
	fails := w.Failures + 1
	if fails >= MaxFailures {
		w.Failures = fails
		row, err := s.finish(ctx, w, models.FreeWatchFailed, fmt.Sprintf("%s（连续 %d 次，不再等）", why, fails), "", true)
		if err == nil {
			s.notify(ctx, Message{Kind: KindFailed, Watch: *row, Error: why})
		}
		return
	}
	wait := failBackoff[min(fails, len(failBackoff))-1]
	next := now.Add(wait)
	if err := s.cfg.DB.WithContext(ctx).Model(&models.FreeWatch{}).Where("id = ?", w.ID).Updates(map[string]any{
		"next_check_at": next, "last_check_at": now, "check_count": w.CheckCount + 1, "failures": fails,
		"message": clip(fmt.Sprintf("%s，%d 小时后再试", why, int(wait/time.Hour)), 1024), "updated_at": now,
	}).Error; err != nil {
		s.cfg.Logger.Warnf("[等免费] 保存「%s」的检查结果失败: %v", w.Title, err)
	}
}

// finish 把一条记成不再等的状态（推送了、过期、取消、失败），返回写好的样子。checked 为真时这次是检查的结果
// （记下检查时间、次数与连续失败次数）。
func (s *Service) finish(ctx context.Context, w models.FreeWatch, status, msg, hash string, checked bool) (*models.FreeWatch, error) {
	now := s.cfg.Now()
	fields := map[string]any{"status": status, "next_check_at": nil, "message": clip(msg, 1024), "updated_at": now}
	if checked {
		fields["last_check_at"], fields["check_count"], fields["failures"] = now, w.CheckCount+1, w.Failures
	}
	if hash != "" {
		fields["info_hash"] = strings.ToLower(hash)
	}
	res := s.cfg.DB.WithContext(ctx).Model(&models.FreeWatch{}).Where("id = ? AND status = ?", w.ID, models.FreeWatchWatching).Updates(fields)
	if res.Error != nil {
		s.cfg.Logger.Warnf("[等免费] 保存「%s」的状态失败: %v", w.Title, res.Error)
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, fmt.Errorf("%w: 这一条已经不在等了", ErrInvalid)
	}
	var row models.FreeWatch
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", w.ID).Limit(1).Find(&row).Error; err != nil {
		return nil, fmt.Errorf("读取等免费失败: %w", err)
	}
	return &row, nil
}

func (s *Service) downloaderName(ctx context.Context, id uint) string {
	var dl models.DownloaderSetting
	if err := s.cfg.DB.WithContext(ctx).Select("id", "name").Where("id = ?", id).Limit(1).Find(&dl).Error; err != nil || dl.ID == 0 {
		return fmt.Sprintf("下载器 %d", id)
	}
	return dl.Name
}

func (s *Service) notify(ctx context.Context, m Message) {
	if s.cfg.Notify != nil {
		s.cfg.Notify(ctx, m)
	}
}
