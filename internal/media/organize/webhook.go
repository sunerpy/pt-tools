package organize

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/server"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

// ErrRateLimited 表示一台服务器的 Webhook 一分钟里来得太多。
var ErrRateLimited = errors.New("Webhook 一分钟里来得太多，稍后再发")

// webhookPerMinute 是每台服务器每分钟最多收几次 Webhook。
const webhookPerMinute = 60

// hookLimiter 按服务器数一分钟里收了几次（固定窗口）。
type hookLimiter struct {
	mu sync.Mutex
	m  map[uint]hookWindow
}

type hookWindow struct {
	start time.Time
	n     int
}

func (l *hookLimiter) allow(id uint, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = map[uint]hookWindow{}
	}
	w := l.m[id]
	if now.Sub(w.start) >= time.Minute || now.Before(w.start) {
		w = hookWindow{start: now}
	}
	if w.n >= webhookPerMinute {
		return false
	}
	w.n++
	l.m[id] = w
	return true
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateWebhookToken 给一台服务器生成新的 Webhook 令牌（32 字节随机数，base64url）：只存 SHA-256，令牌只在这里回一次；
// 原来的令牌作废。
func (s *Service) CreateWebhookToken(ctx context.Context, id uint) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成令牌失败: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	s.setMu.Lock()
	defer s.setMu.Unlock()
	res := s.cfg.DB.WithContext(ctx).Model(&models.MediaServer{}).Where("id = ?", id).Update("webhook_token_hash", tokenHash(token))
	if res.Error != nil {
		return "", fmt.Errorf("保存令牌失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return "", ErrNotFound
	}
	return token, nil
}

// DeleteWebhookToken 关掉一台服务器的 Webhook（令牌作废）。
func (s *Service) DeleteWebhookToken(ctx context.Context, id uint) error {
	s.setMu.Lock()
	defer s.setMu.Unlock()
	res := s.cfg.DB.WithContext(ctx).Model(&models.MediaServer{}).Where("id = ?", id).Update("webhook_token_hash", "")
	if res.Error != nil {
		return fmt.Errorf("关掉 Webhook 失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// HandleWebhook 处理媒体服务器发来的一次 Webhook（不认会话，凭令牌）：服务器不存在与令牌不对都回 ErrNotFound；
// 每台每分钟最多 60 次（ErrRateLimited）；请求体看不懂时回 server.ErrBadWebhook。新入库与删除 2 分钟后同步这台的索引
// （期间再来的合进去），开始与停止播放发播放通知（这台选了通道时），别的只记下来。服务器没启用时什么也不做。
// Webhook 本身不改索引、不删任何东西。
func (s *Service) HandleWebhook(ctx context.Context, id uint, token, contentType string, body io.Reader) error {
	var srv models.MediaServer
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&srv).Error; err != nil {
		return fmt.Errorf("读取媒体服务器失败: %w", err)
	}
	if srv.ID == 0 || srv.WebhookTokenHash == "" || token == "" ||
		subtle.ConstantTimeCompare([]byte(tokenHash(token)), []byte(srv.WebhookTokenHash)) != 1 {
		return ErrNotFound
	}
	now := s.cfg.Now()
	if !s.hooks.allow(srv.ID, now) {
		return ErrRateLimited
	}
	ev, err := server.ParseWebhook(contentType, body)
	if err != nil {
		return err
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaServer{}).Where("id = ?", srv.ID).
		Updates(map[string]any{"webhook_at": now, "webhook_event": truncate(ev.Name, 64)}).Error; err != nil {
		s.cfg.Logger.Warnf("[媒体服务器] 记下 Webhook 失败: %v", err)
	}
	if !srv.Enabled {
		return nil
	}
	switch ev.Action {
	case server.WebhookAdded, server.WebhookRemoved:
		if !srv.IndexOff {
			s.scheduleIndex(srv.ID, webhookSyncDelay)
		}
	case server.WebhookPlaybackStart, server.WebhookPlaybackStop:
		s.playbackNotice(ctx, srv, ev, now)
	}
	return nil
}

// playbackName 是播放通知里的片名：电影带年份，剧集带季集与集名。集名里已经有季集（服务器没取到元数据、
// 拿文件名当集名，如「逆袭的千金 S01E02」）时不写集名。
func playbackName(ev server.WebhookEvent) string {
	switch {
	case ev.Kind == models.MediaServerItemEpisode && ev.SeriesTitle != "":
		code := fmt.Sprintf("S%02dE%02d", ev.Season, ev.Episode)
		name := ev.SeriesTitle + " " + code
		if t := strings.TrimSpace(ev.Title); t != "" && !strings.Contains(strings.ToUpper(t), code) {
			name += "「" + t + "」"
		}
		return name
	case ev.Kind == models.MediaServerItemMovie && ev.Year > 0:
		return fmt.Sprintf("%s (%d)", ev.Title, ev.Year)
	}
	return ev.Title
}

// playbackNotice 发一条播放通知（这台服务器选了通道时）。
func (s *Service) playbackNotice(ctx context.Context, srv models.MediaServer, ev server.WebhookEvent, now time.Time) {
	channels := decodeIDs(srv.PlaybackChannels)
	if s.cfg.Notify == nil || len(channels) == 0 {
		return
	}
	action, label, icon := "start", "开始播放", "▶️"
	if ev.Action == server.WebhookPlaybackStop {
		action, label, icon = "stop", "停止播放", "⏹️"
	}
	name := playbackName(ev)
	if name == "" {
		name = "（没有名字）"
	}
	mediaType := ""
	switch ev.Kind {
	case models.MediaServerItemMovie:
		mediaType = models.MediaKindMovie
	case models.MediaServerItemEpisode, models.MediaServerItemSeries:
		mediaType = models.MediaKindTV
	}
	parts := []string{srv.Name}
	if ev.User != "" {
		parts = append(parts, ev.User)
	}
	if ev.Device != "" {
		dev := ev.Device
		if ev.Client != "" && ev.Client != ev.Device {
			dev += "（" + ev.Client + "）"
		}
		parts = append(parts, dev)
	} else if ev.Client != "" {
		parts = append(parts, ev.Client)
	}
	// Key 的「:」前面是通知日志的 Subject（这台服务器），整个是 EventKey：带上纳秒与序号（时钟粗的系统上两次可能同一刻），
	// 一次 Webhook 一条，同一部再播一次也发
	seq := s.hookSeq.Add(1)
	key := truncate("playback-"+strconv.FormatUint(uint64(srv.ID), 10)+":"+action+":"+ev.ItemID+":"+strconv.FormatInt(now.UnixNano(), 10)+
		"-"+strconv.FormatUint(seq, 10), 128)
	n := Notice{
		Key: key, Kind: NoticePlayback, ChannelIDs: channels,
		Title: icon + " " + label + "：" + name, Text: strings.Join(parts, " · "),
		Event: notify.NewEvent(notify.EventMediaPlayback, now, map[string]any{
			"server": srv.Name, "user": ev.User, "action": action, "action_label": label, "name": name, "media_type": mediaType,
			"season": ev.Season, "episode": ev.Episode, "device": ev.Device, "client": ev.Client,
		}),
	}
	if err := s.cfg.Notify(ctx, n); err != nil {
		s.cfg.Logger.Warnf("[媒体服务器] 播放通知没有发出: %v", err)
	}
}
