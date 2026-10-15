package web

import (
	"net/http"
	"sort"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// notifyStatsDays 是投递统计的时间范围。
const notifyStatsDays = 7

// 通知通道的投递统计：最近 7 天里每个通道发了多少，按来源分（RSS 与各个后台监控：保号、签到、战报、失效种子、
// 订阅与入库、远程访问）。业务通知都经 RSS 通知日志与监控通知日志投递，两张表合起来就是全部；
// 通道测试、ChatOps 与 AI 助手的回复不记日志，不算在内。
//
// 两张表的 sent 不完全一样：监控通知的 sent 是真的发出去了；RSS 通知实时发不出去时转进重发队列
// （notification_outbox，只有 RSS 通知会进），日志照样记 sent。所以重发队列的结果单独数，页面上写明。

// NotifyStatsSource 是一个来源在这个通道上的条数。
type NotifyStatsSource struct {
	Source string `json:"source"`
	Total  int64  `json:"total"`
	Sent   int64  `json:"sent"`
}

// NotifyStatsRequeue 是实时发不出去、转进重发队列的 RSS 通知（按发送的消息数，合并发送的一组算一条）。
type NotifyStatsRequeue struct {
	Total   int64 `json:"total"`
	Sent    int64 `json:"sent"`
	Pending int64 `json:"pending"`
	Dead    int64 `json:"dead"`
}

// NotifyStatsChannel 是一个通道的统计。Sent 是已发出（含转进重发队列的 RSS 通知，以及发出后在消息里点了「忽略」的）；
// Pending 是还在等（安静时段挂起、等重试）；Canceled 是在 RSS 通知日志里取消的、或被同一种子后来的通知取代的；
// Throttled 是超出 RSS 每小时上限没发的。
type NotifyStatsChannel struct {
	ConfID    uint                `json:"conf_id"`
	Total     int64               `json:"total"`
	Sent      int64               `json:"sent"`
	Failed    int64               `json:"failed"`
	Pending   int64               `json:"pending"`
	Canceled  int64               `json:"canceled"`
	Throttled int64               `json:"throttled"`
	Requeued  NotifyStatsRequeue  `json:"requeued"`
	Sources   []NotifyStatsSource `json:"sources"`
}

// NotifyStatsView 是 GET /api/chatops/notifications/stats 的回应。
type NotifyStatsView struct {
	Since    time.Time            `json:"since"`
	Days     int                  `json:"days"`
	Channels []NotifyStatsChannel `json:"channels"`
}

type notifyStatsRow struct {
	ConfID    uint
	Source    string
	Result    string
	Delivered bool
	N         int64
}

type notifyRequeueRow struct {
	ConfID uint
	Status string
	N      int64
}

func (h *chatopsHandlers) notifyStats(w http.ResponseWriter, r *http.Request) {
	if global.GlobalDB == nil || global.GlobalDB.DB == nil {
		writeChatopsErr(w, http.StatusServiceUnavailable, "not_wired", "database not initialized")
		return
	}
	since := time.Now().Add(-notifyStatsDays * 24 * time.Hour)
	db := global.GlobalDB.DB.WithContext(r.Context())
	var rss, mon []notifyStatsRow
	if err := db.Model(&models.RSSNotificationLog{}).
		Select("notification_conf_id AS conf_id, 'rss' AS source, result, delivered_at IS NOT NULL AS delivered, COUNT(*) AS n").
		Where("created_at >= ?", since).
		Group("notification_conf_id, result, delivered_at IS NOT NULL").Scan(&rss).Error; err != nil {
		writeChatopsErr(w, http.StatusInternalServerError, "query_failed", "读取 RSS 通知日志失败")
		return
	}
	if err := db.Model(&models.MonitorNotificationLog{}).
		Select("notification_conf_id AS conf_id, source, result, COUNT(*) AS n").
		Where("created_at >= ?", since).Group("notification_conf_id, source, result").Scan(&mon).Error; err != nil {
		writeChatopsErr(w, http.StatusInternalServerError, "query_failed", "读取监控通知日志失败")
		return
	}
	var requeued []notifyRequeueRow
	if err := db.Model(&models.NotificationOutbox{}).
		Select("notification_conf_id AS conf_id, status, COUNT(*) AS n").
		Where("created_at >= ?", since).Group("notification_conf_id, status").Scan(&requeued).Error; err != nil {
		writeChatopsErr(w, http.StatusInternalServerError, "query_failed", "读取重发队列失败")
		return
	}
	writeJSON(w, NotifyStatsView{Since: since, Days: notifyStatsDays, Channels: aggregateNotifyStats(append(rss, mon...), requeued)})
}

// aggregateNotifyStats 按通道合起来（条数多的在前），每个通道里的来源也按条数排。
func aggregateNotifyStats(rows []notifyStatsRow, requeued []notifyRequeueRow) []NotifyStatsChannel {
	byConf := map[uint]*NotifyStatsChannel{}
	bySrc := map[uint]map[string]*NotifyStatsSource{}
	channel := func(id uint) *NotifyStatsChannel {
		ch := byConf[id]
		if ch == nil {
			ch = &NotifyStatsChannel{ConfID: id, Sources: []NotifyStatsSource{}}
			byConf[id], bySrc[id] = ch, map[string]*NotifyStatsSource{}
		}
		return ch
	}
	for _, row := range rows {
		ch := channel(row.ConfID)
		src := bySrc[row.ConfID][row.Source]
		if src == nil {
			src = &NotifyStatsSource{Source: row.Source}
			bySrc[row.ConfID][row.Source] = src
		}
		ch.Total += row.N
		src.Total += row.N
		switch {
		// 发出以后在消息里点了「忽略」的也是 suppressed，但它已经送到了
		case row.Result == "sent", row.Result == "suppressed" && row.Delivered:
			ch.Sent += row.N
			src.Sent += row.N
		case row.Result == "failed":
			ch.Failed += row.N
		case row.Result == "pending":
			ch.Pending += row.N
		case row.Result == "suppressed":
			ch.Canceled += row.N
		case row.Result == "throttled":
			ch.Throttled += row.N
		}
	}
	for _, row := range requeued {
		q := &channel(row.ConfID).Requeued
		q.Total += row.N
		switch row.Status {
		case "sent":
			q.Sent += row.N
		case "pending":
			q.Pending += row.N
		case "dead":
			q.Dead += row.N
		}
	}
	out := make([]NotifyStatsChannel, 0, len(byConf))
	for id, ch := range byConf {
		for _, src := range bySrc[id] {
			ch.Sources = append(ch.Sources, *src)
		}
		sort.Slice(ch.Sources, func(i, j int) bool {
			if ch.Sources[i].Total != ch.Sources[j].Total {
				return ch.Sources[i].Total > ch.Sources[j].Total
			}
			return ch.Sources[i].Source < ch.Sources[j].Source
		})
		out = append(out, *ch)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].ConfID < out[j].ConfID
	})
	return out
}
