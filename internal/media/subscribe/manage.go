package subscribe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// 设置的默认值与范围。
const (
	DefaultSearchIntervalHours = 12
	MinSearchIntervalHours     = 6
	maxSearchIntervalHours     = 168
)

// Settings 是订阅的设置。
type Settings struct {
	Enabled             bool     `json:"enabled"`
	SearchIntervalHours int      `json:"search_interval_hours"`
	SearchSkipSites     []string `json:"search_skip_sites"`
	DefaultProfileID    uint     `json:"default_profile_id"`
	DefaultDownloaderID uint     `json:"default_downloader_id"`
	NotifyChannels      []uint   `json:"notify_channels"`
	UpgradeOld          string   `json:"upgrade_old"`
}

func decodeIDs(raw string) []uint {
	var out []uint
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out
}

func encodeIDs(v []uint) string {
	if len(v) == 0 {
		return ""
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func (s *Service) loadSettings(ctx context.Context) (models.MediaSubscribeSetting, error) {
	var row models.MediaSubscribeSetting
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", 1).Limit(1).Find(&row).Error; err != nil {
		return row, fmt.Errorf("读取订阅设置失败: %w", err)
	}
	return row, nil
}

// Settings 读订阅设置（没有保存过时是默认值）。
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	row, err := s.loadSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	out := Settings{
		Enabled: row.Enabled, SearchIntervalHours: row.SearchIntervalHours, SearchSkipSites: decodeList(row.SearchSkipSites),
		DefaultProfileID: row.DefaultProfileID, DefaultDownloaderID: row.DefaultDownloaderID,
		NotifyChannels: decodeIDs(row.NotifyChannels), UpgradeOld: row.UpgradeOld,
	}
	if out.SearchIntervalHours <= 0 {
		out.SearchIntervalHours = DefaultSearchIntervalHours
	}
	if out.UpgradeOld == "" {
		out.UpgradeOld = models.MediaUpgradeKeep
	}
	return out, nil
}

// SaveSettings 保存订阅设置。
func (s *Service) SaveSettings(ctx context.Context, in Settings) (Settings, error) {
	if in.SearchIntervalHours == 0 {
		in.SearchIntervalHours = DefaultSearchIntervalHours
	}
	if in.SearchIntervalHours < MinSearchIntervalHours || in.SearchIntervalHours > maxSearchIntervalHours {
		return Settings{}, fmt.Errorf("%w: 主动搜索的间隔要在 %d 到 %d 小时之间", ErrInvalid, MinSearchIntervalHours, maxSearchIntervalHours)
	}
	switch in.UpgradeOld {
	case "", models.MediaUpgradeKeep, models.MediaUpgradeDelete:
	default:
		return Settings{}, fmt.Errorf("%w: 洗版以后的旧种子要选继续做种或删除", ErrInvalid)
	}
	sites, err := cleanSites(in.SearchSkipSites)
	if err != nil {
		return Settings{}, err
	}
	if in.DefaultProfileID != 0 {
		if _, err := s.profileRow(ctx, in.DefaultProfileID); err != nil {
			return Settings{}, fmt.Errorf("%w: 默认的质量档案不存在", ErrInvalid)
		}
	}
	if len(in.NotifyChannels) > 20 {
		return Settings{}, fmt.Errorf("%w: 通知通道最多 20 个", ErrInvalid)
	}
	row := models.MediaSubscribeSetting{
		ID: 1, Enabled: in.Enabled, SearchIntervalHours: in.SearchIntervalHours, SearchSkipSites: encodeList(sites),
		DefaultProfileID: in.DefaultProfileID, DefaultDownloaderID: in.DefaultDownloaderID,
		NotifyChannels: encodeIDs(in.NotifyChannels), UpgradeOld: in.UpgradeOld, UpdatedAt: s.cfg.Now(),
	}
	if err := s.cfg.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "search_interval_hours", "search_skip_sites", "default_profile_id", "default_downloader_id", "notify_channels", "upgrade_old", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return Settings{}, fmt.Errorf("保存订阅设置失败: %w", err)
	}
	return s.Settings(ctx)
}

// cleanSites 规整站点名列表：去空白、去重，最多 200 个。
func cleanSites(in []string) ([]string, error) {
	var out []string
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || slices.Contains(out, v) {
			continue
		}
		if len(v) > 64 {
			return nil, fmt.Errorf("%w: 站点名太长", ErrInvalid)
		}
		out = append(out, v)
	}
	if len(out) > 200 {
		return nil, fmt.Errorf("%w: 站点最多 200 个", ErrInvalid)
	}
	return out, nil
}

// ---- 质量档案 ----

// ProfileInput 是新建或修改质量档案的内容。
type ProfileInput struct {
	Name        string   `json:"name"`
	Resolutions []string `json:"resolutions"`
	Sources     []string `json:"sources"`
	Codecs      []string `json:"codecs"`
	Remux       string   `json:"remux"`
	HDR         string   `json:"hdr"`
	ChineseSubs string   `json:"chinese_subs"`
	Free        string   `json:"free"`
	Groups      []string `json:"groups"`
	MinSizeGB   float64  `json:"min_size_gb"`
	MaxSizeGB   float64  `json:"max_size_gb"`
	MinSeeders  int      `json:"min_seeders"`
	ExcludeHR   bool     `json:"exclude_hr"`
}

// pick 检查列表里的值都在可选值里，按给的顺序去重。
func pick(name string, in, allowed []string) ([]string, error) {
	var out []string
	for _, v := range in {
		i := slices.IndexFunc(allowed, func(a string) bool { return strings.EqualFold(a, strings.TrimSpace(v)) })
		if i < 0 {
			return nil, fmt.Errorf("%w: %s里没有 %q", ErrInvalid, name, v)
		}
		if !slices.Contains(out, allowed[i]) {
			out = append(out, allowed[i])
		}
	}
	return out, nil
}

func checkPref(name, v string, avoid bool) error {
	switch v {
	case "", models.MediaPrefPrefer, models.MediaPrefRequire:
		return nil
	case models.MediaPrefAvoid:
		if avoid {
			return nil
		}
	}
	return fmt.Errorf("%w: %s要选不限、优先或必须有%s", ErrInvalid, name, map[bool]string{true: "、不要", false: ""}[avoid])
}

func (in ProfileInput) row() (models.MediaQualityProfile, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return models.MediaQualityProfile{}, fmt.Errorf("%w: 名字要填，最多 64 个字", ErrInvalid)
	}
	res, err := pick("分辨率", in.Resolutions, Resolutions)
	if err != nil {
		return models.MediaQualityProfile{}, err
	}
	src, err := pick("来源", in.Sources, Sources)
	if err != nil {
		return models.MediaQualityProfile{}, err
	}
	codecs, err := pick("编码", in.Codecs, Codecs)
	if err != nil {
		return models.MediaQualityProfile{}, err
	}
	for _, c := range []struct {
		name, v string
		avoid   bool
	}{{"Remux", in.Remux, true}, {"HDR", in.HDR, true}, {"中字", in.ChineseSubs, false}, {"免费", in.Free, false}} {
		if err := checkPref(c.name, c.v, c.avoid); err != nil {
			return models.MediaQualityProfile{}, err
		}
	}
	var groups []string
	for _, g := range in.Groups {
		g = strings.TrimSpace(g)
		if g == "" || slices.ContainsFunc(groups, func(x string) bool { return strings.EqualFold(x, g) }) {
			continue
		}
		if utf8.RuneCountInString(g) > 64 {
			return models.MediaQualityProfile{}, fmt.Errorf("%w: 制作组名字太长", ErrInvalid)
		}
		groups = append(groups, g)
	}
	if len(groups) > 50 {
		return models.MediaQualityProfile{}, fmt.Errorf("%w: 制作组最多 50 个", ErrInvalid)
	}
	if in.MinSizeGB < 0 || in.MaxSizeGB < 0 || in.MaxSizeGB > 10000 || (in.MaxSizeGB > 0 && in.MinSizeGB > in.MaxSizeGB) {
		return models.MediaQualityProfile{}, fmt.Errorf("%w: 体积区间不对", ErrInvalid)
	}
	if in.MinSeeders < 0 || in.MinSeeders > 100000 {
		return models.MediaQualityProfile{}, fmt.Errorf("%w: 最少做种数不对", ErrInvalid)
	}
	return models.MediaQualityProfile{
		Name: name, Resolutions: encodeList(res), Sources: encodeList(src), Codecs: encodeList(codecs),
		Remux: in.Remux, HDR: in.HDR, ChineseSubs: in.ChineseSubs, Free: in.Free, Groups: encodeList(groups),
		MinSizeGB: in.MinSizeGB, MaxSizeGB: in.MaxSizeGB, MinSeeders: in.MinSeeders, ExcludeHR: in.ExcludeHR,
	}, nil
}

func (s *Service) profileRow(ctx context.Context, id uint) (models.MediaQualityProfile, error) {
	var row models.MediaQualityProfile
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&row).Error; err != nil {
		return row, fmt.Errorf("读取质量档案失败: %w", err)
	}
	if row.ID == 0 {
		return row, ErrNotFound
	}
	return row, nil
}

// Profiles 列出质量档案。
func (s *Service) Profiles(ctx context.Context) ([]Profile, error) {
	var rows []models.MediaQualityProfile
	if err := s.cfg.DB.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取质量档案失败: %w", err)
	}
	out := make([]Profile, 0, len(rows))
	for _, r := range rows {
		out = append(out, profileOf(r))
	}
	return out, nil
}

// SaveProfile 新建（id 为 0）或修改质量档案。
func (s *Service) SaveProfile(ctx context.Context, id uint, in ProfileInput) (Profile, error) {
	row, err := in.row()
	if err != nil {
		return Profile{}, err
	}
	var dup int64
	if err = s.cfg.DB.WithContext(ctx).Model(&models.MediaQualityProfile{}).Where("name = ? AND id <> ?", row.Name, id).Count(&dup).Error; err != nil {
		return Profile{}, fmt.Errorf("读取质量档案失败: %w", err)
	}
	if dup > 0 {
		return Profile{}, fmt.Errorf("%w: 已经有叫「%s」的质量档案", ErrInvalid, row.Name)
	}
	now := s.cfg.Now()
	row.UpdatedAt = now
	if id == 0 {
		row.CreatedAt = now
		if err = s.cfg.DB.WithContext(ctx).Create(&row).Error; err != nil {
			return Profile{}, fmt.Errorf("保存质量档案失败: %w", err)
		}
		return profileOf(row), nil
	}
	old, err := s.profileRow(ctx, id)
	if err != nil {
		return Profile{}, err
	}
	row.ID, row.CreatedAt = old.ID, old.CreatedAt
	if err := s.cfg.DB.WithContext(ctx).Select("*").Omit("created_at").Save(&row).Error; err != nil {
		return Profile{}, fmt.Errorf("保存质量档案失败: %w", err)
	}
	return profileOf(row), nil
}

// DeleteProfile 删除质量档案：还有订阅、豆瓣来源在用或是默认档案时不删。
func (s *Service) DeleteProfile(ctx context.Context, id uint) error {
	if _, err := s.profileRow(ctx, id); err != nil {
		return err
	}
	var subs, sources int64
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("profile_id = ?", id).Count(&subs).Error; err != nil {
		return fmt.Errorf("读取订阅失败: %w", err)
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaDoubanSource{}).Where("profile_id = ?", id).Count(&sources).Error; err != nil {
		return fmt.Errorf("读取豆瓣来源失败: %w", err)
	}
	set, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	switch {
	case subs > 0:
		return fmt.Errorf("%w: 还有 %d 个订阅在用这个档案", ErrInvalid, subs)
	case sources > 0:
		return fmt.Errorf("%w: 还有 %d 个豆瓣来源在用这个档案", ErrInvalid, sources)
	case set.DefaultProfileID == id:
		return fmt.Errorf("%w: 这是默认的质量档案，先在设置里换一个", ErrInvalid)
	}
	if err := s.cfg.DB.WithContext(ctx).Delete(&models.MediaQualityProfile{}, id).Error; err != nil {
		return fmt.Errorf("删除质量档案失败: %w", err)
	}
	return nil
}

// profileFor 是订阅用的档案：订阅自己的，没有时用设置里的默认档案，都没有时不限（按分辨率与来源从好到差比）。
func (s *Service) profileFor(ctx context.Context, sub *models.MediaSubscription, set Settings) Profile {
	for _, id := range []uint{sub.ProfileID, set.DefaultProfileID} {
		if id == 0 {
			continue
		}
		if row, err := s.profileRow(ctx, id); err == nil {
			return profileOf(row)
		}
	}
	return Profile{}
}

// ---- 订阅 ----

// SubscriptionInput 是新建或修改订阅的内容。新建时要条目（类型、TMDB 编号，剧集还要季）；修改时这几项不能改。
type SubscriptionInput struct {
	MediaType    string   `json:"media_type"`
	TMDBID       int      `json:"tmdb_id"`
	Season       int      `json:"season"`
	ProfileID    uint     `json:"profile_id"`
	Sites        []string `json:"sites"`
	DownloaderID uint     `json:"downloader_id"`
	Category     string   `json:"category"`
	Tags         string   `json:"tags"`
	SavePath     string   `json:"save_path"`
	Upgrade      bool     `json:"upgrade"`
	// Pending 为真时建成待确认
	Pending bool `json:"pending"`
}

// SubscriptionView 是订阅加上进度与已下载的种子数。
type SubscriptionView struct {
	models.MediaSubscription
	Sites    []string  `json:"sites"`
	Progress *Progress `json:"progress,omitempty"`
	Torrents int       `json:"torrents"`
}

// SubscriptionDetail 是订阅详情：订阅、进度与下载过的种子。
type SubscriptionDetail struct {
	SubscriptionView
	TorrentList []models.MediaSubscriptionTorrent `json:"torrent_list"`
}

func (in SubscriptionInput) check(s *Service, ctx context.Context) error {
	if in.ProfileID != 0 {
		if _, err := s.profileRow(ctx, in.ProfileID); err != nil {
			return fmt.Errorf("%w: 质量档案不存在", ErrInvalid)
		}
	}
	for _, v := range []struct{ name, v string }{{"分类", in.Category}, {"标签", in.Tags}} {
		if utf8.RuneCountInString(v.v) > 128 {
			return fmt.Errorf("%w: %s太长", ErrInvalid, v.name)
		}
	}
	if len(in.SavePath) > 1024 {
		return fmt.Errorf("%w: 保存目录太长", ErrInvalid)
	}
	return nil
}

func (s *Service) subRow(ctx context.Context, id uint) (models.MediaSubscription, error) {
	var row models.MediaSubscription
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&row).Error; err != nil {
		return row, fmt.Errorf("读取订阅失败: %w", err)
	}
	if row.ID == 0 {
		return row, ErrNotFound
	}
	return row, nil
}

// CreateSubscription 按 TMDB 条目建订阅：取详情（名字、年份、IMDb、海报、别名），剧集取这一季的集数。
// 同一个条目的同一季已经订阅过时返回 ErrInvalid。
func (s *Service) CreateSubscription(ctx context.Context, in SubscriptionInput, source string) (*models.MediaSubscription, error) {
	if in.MediaType != models.MediaKindMovie && in.MediaType != models.MediaKindTV {
		return nil, fmt.Errorf("%w: 类型要选电影或剧集", ErrInvalid)
	}
	if in.TMDBID <= 0 {
		return nil, fmt.Errorf("%w: 要指定 TMDB 编号", ErrInvalid)
	}
	if in.MediaType == models.MediaKindMovie {
		in.Season = 0
	} else if in.Season < 0 || in.Season > 200 {
		return nil, fmt.Errorf("%w: 季号不对", ErrInvalid)
	}
	if err := in.check(s, ctx); err != nil {
		return nil, err
	}
	sites, err := cleanSites(in.Sites)
	if err != nil {
		return nil, err
	}
	var exist int64
	if err = s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).
		Where("media_type = ? AND tmdb_id = ? AND season = ?", in.MediaType, in.TMDBID, in.Season).Count(&exist).Error; err != nil {
		return nil, fmt.Errorf("读取订阅失败: %w", err)
	}
	if exist > 0 {
		return nil, fmt.Errorf("%w: 已经订阅过了", ErrInvalid)
	}
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return nil, err
	}
	d, err := c.Details(ctx, in.MediaType, in.TMDBID)
	if err != nil {
		return nil, fmt.Errorf("取 TMDB 条目失败: %w", err)
	}
	row := models.MediaSubscription{
		MediaType: in.MediaType, TMDBID: in.TMDBID, Season: in.Season, Title: d.Title, OriginalTitle: d.OriginalTitle, Year: d.Year,
		IMDbID: d.IMDbID, PosterPath: d.PosterPath, ProfileID: in.ProfileID, Sites: encodeList(sites), DownloaderID: in.DownloaderID,
		Category: strings.TrimSpace(in.Category), Tags: strings.TrimSpace(in.Tags), SavePath: strings.TrimSpace(in.SavePath),
		Status: models.MediaSubActive, Upgrade: in.Upgrade, Source: source,
	}
	if in.Pending {
		row.Status = models.MediaSubPending
	}
	if titles, err := c.Titles(ctx, in.MediaType, in.TMDBID); err == nil {
		row.Aliases = encodeList(titles)
	}
	if in.MediaType == models.MediaKindTV {
		if in.Season == 0 {
			in.Season = max(d.Seasons, 1)
			row.Season = in.Season
		}
		season, err := c.Season(ctx, in.TMDBID, in.Season)
		if err != nil {
			if errors.Is(err, tmdb.ErrNotFound) {
				return nil, fmt.Errorf("%w: TMDB 上「%s」没有第 %d 季", ErrInvalid, d.Title, in.Season)
			}
			return nil, fmt.Errorf("取 TMDB 季详情失败: %w", err)
		}
		row.TotalEpisodes = len(season.Episodes)
		if y := yearOf(season.AirDate); y > 0 {
			row.Year = y // 这一季的年份（种子标题里写的多半是它）
		}
	}
	now := s.cfg.Now()
	row.CreatedAt, row.UpdatedAt, row.NextSearchAt = now, now, &now
	if err := s.cfg.DB.WithContext(ctx).Create(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "UNIQUE") {
			return nil, fmt.Errorf("%w: 已经订阅过了", ErrInvalid)
		}
		return nil, fmt.Errorf("保存订阅失败: %w", err)
	}
	return &row, nil
}

// UpdateSubscription 修改订阅（条目与季不能改）。
func (s *Service) UpdateSubscription(ctx context.Context, id uint, in SubscriptionInput) (*models.MediaSubscription, error) {
	row, err := s.subRow(ctx, id)
	if err != nil {
		return nil, err
	}
	if err = in.check(s, ctx); err != nil {
		return nil, err
	}
	sites, err := cleanSites(in.Sites)
	if err != nil {
		return nil, err
	}
	upd := map[string]any{
		"profile_id": in.ProfileID, "sites": encodeList(sites), "downloader_id": in.DownloaderID,
		"category": strings.TrimSpace(in.Category), "tags": strings.TrimSpace(in.Tags), "save_path": strings.TrimSpace(in.SavePath),
		"upgrade": in.Upgrade, "updated_at": s.cfg.Now(),
	}
	// 打开洗版的已完成订阅重新开始找；关掉洗版的不变
	if in.Upgrade && !row.Upgrade && row.Status == models.MediaSubDone {
		upd["status"] = models.MediaSubActive
		upd["next_search_at"] = s.cfg.Now()
	}
	if err = s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", id).Updates(upd).Error; err != nil {
		return nil, fmt.Errorf("保存订阅失败: %w", err)
	}
	row, err = s.subRow(ctx, id)
	return &row, err
}

// SetStatus 暂停、恢复或确认订阅：status 要是 active 或 paused。
func (s *Service) SetStatus(ctx context.Context, id uint, status string) (*models.MediaSubscription, error) {
	if status != models.MediaSubActive && status != models.MediaSubPaused {
		return nil, fmt.Errorf("%w: 状态要选 active 或 paused", ErrInvalid)
	}
	row, err := s.subRow(ctx, id)
	if err != nil {
		return nil, err
	}
	upd := map[string]any{"status": status, "updated_at": s.cfg.Now()}
	if status == models.MediaSubActive && row.Status != models.MediaSubActive {
		upd["next_search_at"] = s.cfg.Now()
	}
	if err = s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", id).Updates(upd).Error; err != nil {
		return nil, fmt.Errorf("保存订阅失败: %w", err)
	}
	row, err = s.subRow(ctx, id)
	return &row, err
}

// DeleteSubscription 删除订阅与它下载过的种子记录（下载器里的种子与库里的文件不动）。
func (s *Service) DeleteSubscription(ctx context.Context, id uint) error {
	if _, err := s.subRow(ctx, id); err != nil {
		return err
	}
	return s.cfg.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("subscription_id = ?", id).Delete(&models.MediaSubscriptionTorrent{}).Error; err != nil {
			return fmt.Errorf("删除订阅的种子记录失败: %w", err)
		}
		if err := tx.Delete(&models.MediaSubscription{}, id).Error; err != nil {
			return fmt.Errorf("删除订阅失败: %w", err)
		}
		return nil
	})
}

// SubscriptionQuery 是订阅列表的筛选。
type SubscriptionQuery struct {
	Status  string
	Keyword string
}

// Subscriptions 列出订阅（新的在前），带进度。
func (s *Service) Subscriptions(ctx context.Context, q SubscriptionQuery) ([]SubscriptionView, error) {
	db := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{})
	switch q.Status {
	case "":
	case models.MediaSubActive, models.MediaSubPaused, models.MediaSubPending, models.MediaSubDone:
		db = db.Where("status = ?", q.Status)
	default:
		return nil, fmt.Errorf("%w: 状态要选 active、paused、pending 或 done", ErrInvalid)
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		if len(kw) > 200 {
			return nil, fmt.Errorf("%w: 关键字太长", ErrInvalid)
		}
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(kw) + "%"
		db = db.Where(`title LIKE ? ESCAPE '\' OR original_title LIKE ? ESCAPE '\'`, like, like)
	}
	var rows []models.MediaSubscription
	if err := db.Order("id DESC").Limit(500).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取订阅失败: %w", err)
	}
	counts := map[uint]int{}
	var pairs []struct {
		SubscriptionID uint
		N              int
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscriptionTorrent{}).Select("subscription_id, COUNT(*) AS n").Group("subscription_id").Scan(&pairs).Error; err == nil {
		for _, p := range pairs {
			counts[p.SubscriptionID] = p.N
		}
	}
	out := make([]SubscriptionView, 0, len(rows))
	for i := range rows {
		v := SubscriptionView{MediaSubscription: rows[i], Sites: decodeList(rows[i].Sites), Torrents: counts[rows[i].ID]}
		if p, err := s.progress(ctx, &rows[i], false); err == nil {
			v.Progress = p
		}
		out = append(out, v)
	}
	return out, nil
}

// Subscription 是一个订阅的详情。
func (s *Service) Subscription(ctx context.Context, id uint) (*SubscriptionDetail, error) {
	row, err := s.subRow(ctx, id)
	if err != nil {
		return nil, err
	}
	var torrents []models.MediaSubscriptionTorrent
	if err := s.cfg.DB.WithContext(ctx).Where("subscription_id = ?", id).Order("id DESC").Find(&torrents).Error; err != nil {
		return nil, fmt.Errorf("读取订阅的种子失败: %w", err)
	}
	d := &SubscriptionDetail{SubscriptionView: SubscriptionView{MediaSubscription: row, Sites: decodeList(row.Sites), Torrents: len(torrents)}, TorrentList: torrents}
	if p, err := s.progress(ctx, &row, true); err == nil {
		d.Progress = p
	}
	return d, nil
}
