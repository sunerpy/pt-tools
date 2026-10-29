package web

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/redact"
	"github.com/sunerpy/pt-tools/models"
)

const (
	notifyRecentDefault = 8
	notifyRecentMax     = 50
	// notifyRecentTitleMax 是每条标题最多给多少个字（标题是种子名之类，过长的截掉）
	notifyRecentTitleMax = 200
)

// RecentNotification 是「最近的通知」里的一条：RSS 通知与后台监控通知合在一起按时间倒序。
// Title 是通知的内置标题（RSS 是种子名），投递以前存下的：配了消息模板时实际收到的标题按模板，
// 几条 RSS 合并成一条发出时这里仍逐条列出。没有存下内容的（例如超出每小时上限没发的）为空。
// Result 是 sent（已发出，含发出后点了「忽略」的）、failed、pending、canceled（取消或被取代）、throttled（超出每小时上限）。
type RecentNotification struct {
	Source    string `json:"source"`
	LogID     uint   `json:"log_id"`
	ConfID    uint   `json:"conf_id"`
	Title     string `json:"title"`
	Site      string `json:"site,omitempty"`
	TorrentID string `json:"torrent_id,omitempty"`
	Kind      string `json:"kind"`
	Result    string `json:"result"`
	Attempts  int    `json:"attempts"`
	// Error 是最近一次投递失败的原因（遮掉凭证）；没有失败过时为空
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// GET /api/chatops/notifications/recent?limit=8：两张通知日志各取最新的 limit 条，合起来再取 limit 条。
func (h *chatopsHandlers) notifyRecent(w http.ResponseWriter, r *http.Request) {
	if global.GlobalDB == nil || global.GlobalDB.DB == nil {
		writeChatopsErr(w, http.StatusServiceUnavailable, "not_wired", "database not initialized")
		return
	}
	limit := notifyRecentDefault
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > notifyRecentMax {
			writeChatopsErr(w, http.StatusBadRequest, "invalid_limit", "limit 要在 1 到 50 之间")
			return
		}
		limit = n
	}
	db := global.GlobalDB.DB.WithContext(r.Context())
	var rss []models.RSSNotificationLog
	if err := db.Select("id, site_name, torrent_id, notify_kind, notification_conf_id, result, attempts, last_error, payload_json, delivered_at, created_at").
		Order("created_at DESC, id DESC").Limit(limit).Find(&rss).Error; err != nil {
		writeChatopsErr(w, http.StatusInternalServerError, "query_failed", "读取 RSS 通知日志失败")
		return
	}
	var mon []models.MonitorNotificationLog
	if err := db.Select("id, source, kind, notification_conf_id, result, attempts, last_error, payload_json, created_at").
		Order("created_at DESC, id DESC").Limit(limit).Find(&mon).Error; err != nil {
		writeChatopsErr(w, http.StatusInternalServerError, "query_failed", "读取监控通知日志失败")
		return
	}
	items := make([]RecentNotification, 0, len(rss)+len(mon))
	for _, row := range rss {
		result := row.Result
		if result == "suppressed" {
			// 发出以后在消息里点了「忽略」的也是 suppressed，但它已经送到了
			result = "canceled"
			if row.DeliveredAt != nil {
				result = "sent"
			}
		}
		items = append(items, RecentNotification{
			Source: "rss", LogID: row.ID, ConfID: row.NotificationConfID, Title: payloadTitle(row.PayloadJSON),
			Site: row.SiteName, TorrentID: row.TorrentID, Kind: row.NotifyKind, Result: result,
			Attempts: row.Attempts, Error: redact.Secrets(row.LastError), CreatedAt: row.CreatedAt,
		})
	}
	for _, row := range mon {
		items = append(items, RecentNotification{
			Source: row.Source, LogID: row.ID, ConfID: row.NotificationConfID, Title: payloadTitle(row.PayloadJSON),
			Kind: row.Kind, Result: row.Result, Attempts: row.Attempts, Error: redact.Secrets(row.LastError), CreatedAt: row.CreatedAt,
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	if len(items) > limit {
		items = items[:limit]
	}
	writeJSON(w, map[string]any{"items": items})
}

// payloadTitle 取通知日志里存下的内置标题（RSS 的 JSON 键是 Title，监控的是 title，解码不分大小写），截到 200 字以内。
func payloadTitle(payload string) string {
	if payload == "" {
		return ""
	}
	var p struct {
		Title string `json:"title"`
	}
	if json.Unmarshal([]byte(payload), &p) != nil {
		return ""
	}
	if utf8.RuneCountInString(p.Title) > notifyRecentTitleMax {
		return string([]rune(p.Title)[:notifyRecentTitleMax]) + "…"
	}
	return p.Title
}
