package subscribe

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/models"
)

// 豆瓣想看的拉取节奏：成功后每 6 小时（加最多 30 分钟随机偏移）；失败按 1、2、4…小时退避，最长 24 小时；
// 连续 3 次失败标成异常并发一次通知。
const (
	doubanInterval     = 6 * time.Hour
	doubanJitter       = 30 * time.Minute
	doubanMaxBackoff   = 24 * time.Hour
	doubanAlertAfter   = 3
	doubanMaxBody      = 2 << 20
	doubanItemsPerPull = 50
)

var (
	doubanUserRe    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	doubanSubjectRe = regexp.MustCompile(`^https?://movie\.douban\.com/subject/(\d+)/?`)
	doubanTitleRe   = regexp.MustCompile(`title="([^"]*)"`)
)

// DoubanSourceInput 是新建或修改豆瓣来源的内容。
type DoubanSourceInput struct {
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	Confirm   bool   `json:"confirm"`
	ProfileID uint   `json:"profile_id"`
}

// DoubanSourceView 是豆瓣来源加上建过的订阅数与没找到条目的数。
type DoubanSourceView struct {
	models.MediaDoubanSource
	Subscribed int `json:"subscribed"`
	Unmatched  int `json:"unmatched"`
}

// DoubanSources 列出豆瓣来源。
func (s *Service) DoubanSources(ctx context.Context) ([]DoubanSourceView, error) {
	var rows []models.MediaDoubanSource
	if err := s.cfg.DB.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取豆瓣来源失败: %w", err)
	}
	var counts []struct {
		SourceID uint
		Status   string
		N        int
	}
	_ = s.cfg.DB.WithContext(ctx).Model(&models.MediaDoubanItem{}).Select("source_id, status, COUNT(*) AS n").Group("source_id, status").Scan(&counts).Error
	out := make([]DoubanSourceView, 0, len(rows))
	for _, r := range rows {
		v := DoubanSourceView{MediaDoubanSource: r}
		for _, c := range counts {
			if c.SourceID != r.ID {
				continue
			}
			switch c.Status {
			case models.MediaDoubanSubscribed:
				v.Subscribed = c.N
			case models.MediaDoubanUnmatched:
				v.Unmatched = c.N
			}
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) doubanRow(ctx context.Context, id uint) (models.MediaDoubanSource, error) {
	var row models.MediaDoubanSource
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&row).Error; err != nil {
		return row, fmt.Errorf("读取豆瓣来源失败: %w", err)
	}
	if row.ID == 0 {
		return row, ErrNotFound
	}
	return row, nil
}

// SaveDoubanSource 新建（id 为 0）或修改豆瓣来源。新建与重新启用的马上拉一次。
func (s *Service) SaveDoubanSource(ctx context.Context, id uint, in DoubanSourceInput) (*models.MediaDoubanSource, error) {
	in.UserID = strings.TrimSpace(in.UserID)
	in.Name = strings.TrimSpace(in.Name)
	if !doubanUserRe.MatchString(in.UserID) {
		return nil, fmt.Errorf("%w: 豆瓣用户编号只能有字母、数字、点、下划线与短横线（个人主页地址 douban.com/people/ 后面那一段）", ErrInvalid)
	}
	if utf8.RuneCountInString(in.Name) > 64 {
		return nil, fmt.Errorf("%w: 名字最多 64 个字", ErrInvalid)
	}
	if in.ProfileID != 0 {
		if _, err := s.profileRow(ctx, in.ProfileID); err != nil {
			return nil, fmt.Errorf("%w: 质量档案不存在", ErrInvalid)
		}
	}
	var dup int64
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaDoubanSource{}).Where("user_id = ? AND id <> ?", in.UserID, id).Count(&dup).Error; err != nil {
		return nil, fmt.Errorf("读取豆瓣来源失败: %w", err)
	}
	if dup > 0 {
		return nil, fmt.Errorf("%w: 已经加过这个豆瓣用户", ErrInvalid)
	}
	now := s.cfg.Now()
	if id == 0 {
		row := models.MediaDoubanSource{UserID: in.UserID, Name: in.Name, Enabled: in.Enabled, Confirm: in.Confirm, ProfileID: in.ProfileID, CreatedAt: now, UpdatedAt: now, NextFetchAt: &now}
		if err := s.cfg.DB.WithContext(ctx).Create(&row).Error; err != nil {
			return nil, fmt.Errorf("保存豆瓣来源失败: %w", err)
		}
		return &row, nil
	}
	old, err := s.doubanRow(ctx, id)
	if err != nil {
		return nil, err
	}
	upd := map[string]any{"user_id": in.UserID, "name": in.Name, "enabled": in.Enabled, "confirm": in.Confirm, "profile_id": in.ProfileID, "updated_at": now}
	if (in.Enabled && !old.Enabled) || in.UserID != old.UserID {
		upd["next_fetch_at"], upd["failures"], upd["abnormal"], upd["notified"] = now, 0, false, false
	}
	if err = s.cfg.DB.WithContext(ctx).Model(&models.MediaDoubanSource{}).Where("id = ?", id).Updates(upd).Error; err != nil {
		return nil, fmt.Errorf("保存豆瓣来源失败: %w", err)
	}
	row, err := s.doubanRow(ctx, id)
	return &row, err
}

// DeleteDoubanSource 删除豆瓣来源与它见过的条目（建过的订阅留着）。
func (s *Service) DeleteDoubanSource(ctx context.Context, id uint) error {
	if _, err := s.doubanRow(ctx, id); err != nil {
		return err
	}
	if err := s.cfg.DB.WithContext(ctx).Where("source_id = ?", id).Delete(&models.MediaDoubanItem{}).Error; err != nil {
		return fmt.Errorf("删除豆瓣条目失败: %w", err)
	}
	if err := s.cfg.DB.WithContext(ctx).Delete(&models.MediaDoubanSource{}, id).Error; err != nil {
		return fmt.Errorf("删除豆瓣来源失败: %w", err)
	}
	return nil
}

// DoubanItems 是一个来源见过的条目（新的在前）。
func (s *Service) DoubanItems(ctx context.Context, id uint) ([]models.MediaDoubanItem, error) {
	if _, err := s.doubanRow(ctx, id); err != nil {
		return nil, err
	}
	var rows []models.MediaDoubanItem
	if err := s.cfg.DB.WithContext(ctx).Where("source_id = ?", id).Order("id DESC").Limit(500).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取豆瓣条目失败: %w", err)
	}
	return rows, nil
}

// FetchDouban 立即拉一次豆瓣来源，返回新建的订阅数。
func (s *Service) FetchDouban(ctx context.Context, id uint) (int, error) {
	src, err := s.doubanRow(ctx, id)
	if err != nil {
		return 0, err
	}
	return s.pullDouban(ctx, &src)
}

// doubanDue 拉到期的豆瓣来源。
func (s *Service) doubanDue(ctx context.Context) {
	var rows []models.MediaDoubanSource
	if err := s.cfg.DB.WithContext(ctx).Where("enabled = ? AND (next_fetch_at IS NULL OR next_fetch_at <= ?)", true, s.cfg.Now()).Find(&rows).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 读取豆瓣来源失败: %v", err)
		return
	}
	for i := range rows {
		if ctx.Err() != nil {
			return
		}
		if n, err := s.pullDouban(ctx, &rows[i]); err != nil {
			s.cfg.Logger.Warnf("[订阅] 拉取豆瓣想看失败 (%s): %v", rows[i].UserID, err)
		} else if n > 0 {
			s.cfg.Logger.Infof("[订阅] 豆瓣想看 (%s) 建了 %d 个订阅", rows[i].UserID, n)
		}
	}
}

// doubanWish 是 RSS 里的一条想看。
type doubanWish struct {
	ID       string
	Name     string
	Original string
}

type doubanRSS struct {
	Items []struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
	} `xml:"channel>item"`
}

// parseDouban 从豆瓣的「收藏」RSS 里取想看的电影与剧集：标题以「想看」开头、链接是 movie.douban.com 的条目。
// 描述里图片链接的 title 是原名（外语片是外文名），一起取出来帮助对 TMDB。
func parseDouban(body []byte) ([]doubanWish, error) {
	var feed doubanRSS
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("豆瓣返回的不是 RSS: %w", err)
	}
	var out []doubanWish
	for _, it := range feed.Items {
		title := strings.TrimSpace(it.Title)
		if !strings.HasPrefix(title, "想看") {
			continue
		}
		m := doubanSubjectRe.FindStringSubmatch(strings.TrimSpace(it.Link))
		if m == nil {
			continue
		}
		w := doubanWish{ID: m[1], Name: strings.TrimSpace(strings.TrimPrefix(title, "想看"))}
		if t := doubanTitleRe.FindStringSubmatch(it.Description); t != nil {
			w.Original = strings.TrimSpace(html.UnescapeString(t[1]))
		}
		if w.Name != "" {
			out = append(out, w)
		}
	}
	return out, nil
}

func (s *Service) fetchDoubanFeed(ctx context.Context, uid string) ([]byte, error) {
	u := strings.TrimRight(s.cfg.DoubanBase, "/") + "/feed/people/" + url.PathEscape(uid) + "/interests"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, errors.New("豆瓣地址格式不对")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; pt-tools)")
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml")
	resp, err := s.cfg.HTTP.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("连不上豆瓣: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("豆瓣返回 HTTP %d（用户不存在、没有公开收藏，或者被限流）", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, doubanMaxBody))
	if err != nil {
		return nil, fmt.Errorf("读取豆瓣响应失败: %w", err)
	}
	return body, nil
}

// pullDouban 拉一个豆瓣来源：新的想看对上 TMDB 条目后建订阅（已经订阅过的只记下）；对不上的记成没找到。
// TMDB 暂时不能访问时整次算失败，之后再拉，不把条目记成没找到。
func (s *Service) pullDouban(ctx context.Context, src *models.MediaDoubanSource) (int, error) {
	created, err := s.pullDoubanOnce(ctx, src)
	now := s.cfg.Now()
	upd := map[string]any{"last_fetch_at": now, "updated_at": now}
	if err == nil {
		next := now.Add(doubanInterval + s.cfg.Jitter(doubanJitter))
		upd["next_fetch_at"], upd["failures"], upd["last_error"], upd["abnormal"], upd["notified"] = next, 0, "", false, false
	} else {
		fails := src.Failures + 1
		backoff := min(time.Hour<<min(fails-1, 5), doubanMaxBackoff)
		upd["next_fetch_at"], upd["failures"], upd["last_error"] = now.Add(backoff), fails, truncate(err.Error(), 1024)
		if fails >= doubanAlertAfter {
			upd["abnormal"] = true
			if !src.Notified {
				upd["notified"] = true
				s.notifyDouban(ctx, src, err, fails)
			}
		}
	}
	if uerr := s.cfg.DB.WithContext(ctx).Model(&models.MediaDoubanSource{}).Where("id = ?", src.ID).Updates(upd).Error; uerr != nil {
		s.cfg.Logger.Warnf("[订阅] 更新豆瓣来源失败: %v", uerr)
	}
	return created, err
}

func (s *Service) pullDoubanOnce(ctx context.Context, src *models.MediaDoubanSource) (int, error) {
	// 没填 TMDB API Key 时整次算失败：不然所有条目都会记成没找到，填了 Key 以后也不再试
	if _, err := s.cfg.Recognizer.TMDB(ctx); err != nil {
		return 0, fmt.Errorf("TMDB 不能用: %w", err)
	}
	body, err := s.fetchDoubanFeed(ctx, src.UserID)
	if err != nil {
		return 0, err
	}
	wishes, err := parseDouban(body)
	if err != nil {
		return 0, err
	}
	created := 0
	for i, w := range wishes {
		if i >= doubanItemsPerPull {
			break
		}
		var seen int64
		if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaDoubanItem{}).Where("source_id = ? AND douban_id = ?", src.ID, w.ID).Count(&seen).Error; err != nil {
			return created, fmt.Errorf("读取豆瓣条目失败: %w", err)
		}
		if seen > 0 {
			continue
		}
		ok, err := s.subscribeWish(ctx, src, w)
		if err != nil {
			return created, err
		}
		if ok {
			created++
		}
	}
	return created, nil
}

// subscribeWish 给一条想看找 TMDB 条目并建订阅，记下条目；建了新订阅时返回真。
func (s *Service) subscribeWish(ctx context.Context, src *models.MediaDoubanSource, w doubanWish) (bool, error) {
	in := recognize.Input{Title: w.Name}
	if w.Original != "" && w.Original != w.Name {
		in = recognize.Input{Title: w.Original, Subtitle: w.Name}
	}
	res, err := s.cfg.Recognizer.Recognize(ctx, in)
	if err != nil {
		return false, fmt.Errorf("识别「%s」失败: %w", w.Name, err)
	}
	item := models.MediaDoubanItem{SourceID: src.ID, DoubanID: w.ID, Title: truncate(w.Name, 255), CreatedAt: s.cfg.Now(), UpdatedAt: s.cfg.Now()}
	if res.Match == nil {
		if res.Error != "" {
			// TMDB 暂时不能访问这类：整次算失败，下次再试
			return false, fmt.Errorf("识别「%s」失败: %s", w.Name, res.Error)
		}
		item.Status = models.MediaDoubanUnmatched
		return false, s.saveWish(ctx, &item)
	}
	item.MediaType, item.TMDBID, item.Year = res.Match.MediaType, res.Match.ID, res.Match.Year
	season := 0
	if res.Match.MediaType == models.MediaKindTV {
		season = max(seasonOf(res.Meta), 1)
	}
	var existing models.MediaSubscription
	if err = s.cfg.DB.WithContext(ctx).Where("media_type = ? AND tmdb_id = ? AND season = ?", res.Match.MediaType, res.Match.ID, season).Limit(1).Find(&existing).Error; err != nil {
		return false, fmt.Errorf("读取订阅失败: %w", err)
	}
	item.Status = models.MediaDoubanSubscribed
	if existing.ID != 0 {
		item.SubscriptionID = existing.ID
		return false, s.saveWish(ctx, &item)
	}
	sub, err := s.CreateSubscription(ctx, SubscriptionInput{
		MediaType: res.Match.MediaType, TMDBID: res.Match.ID, Season: season, ProfileID: src.ProfileID, Pending: src.Confirm,
	}, models.MediaSubFromDouban)
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			// 例如 TMDB 上没有这一季：记成没找到，不再试
			item.Status = models.MediaDoubanUnmatched
			return false, s.saveWish(ctx, &item)
		}
		return false, err
	}
	s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Update("douban_id", w.ID)
	item.SubscriptionID = sub.ID
	return true, s.saveWish(ctx, &item)
}

func seasonOf(m meta.Meta) int { return m.Season }

func (s *Service) saveWish(ctx context.Context, item *models.MediaDoubanItem) error {
	if err := s.cfg.DB.WithContext(ctx).Create(item).Error; err != nil {
		return fmt.Errorf("记下豆瓣条目失败: %w", err)
	}
	return nil
}

func (s *Service) notifyDouban(ctx context.Context, src *models.MediaDoubanSource, cause error, fails int) {
	set, err := s.Settings(ctx)
	if err != nil || s.cfg.Notify == nil || len(set.NotifyChannels) == 0 {
		return
	}
	name := src.Name
	if name == "" {
		name = src.UserID
	}
	n := Notice{
		Channels: set.NotifyChannels, Kind: NoticeDoubanAbnormal, Subject: "douban|" + src.UserID,
		EventKey: fmt.Sprintf("douban-%d-%d", src.ID, s.cfg.Now().Unix()/3600),
		Title:    "豆瓣想看拉取失败", Body: fmt.Sprintf("%s：连续 %d 次没拉到（%v），之后按退避继续重试", name, fails, cause),
	}
	if err := s.cfg.Notify(ctx, n); err != nil {
		s.cfg.Logger.Warnf("[订阅] 发通知失败: %v", err)
	}
}
