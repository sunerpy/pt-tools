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

// 通知通道的投递统计：最近 7 天里每个通道投了多少、送达多少，按来源分（RSS 与各个后台监控：保号、签到、战报、
// 失效种子、订阅与入库、远程访问、刷流）。业务通知都经 RSS 通知日志与监控通知日志投递，两张表合起来就是全部；
// 通道测试、ChatOps 与 AI 助手的回复不记日志，不算在内。

// NotifyStatsSource 是一个来源在这个通道上的条数。
type NotifyStatsSource struct {
	Source string `json:"source"`
	Total  int64  `json:"total"`
	Sent   int64  `json:"sent"`
}

// NotifyStatsChannel 是一个通道的统计。Pending 是还在等（安静时段挂起、等重试），Skipped 是按规则没发的 RSS
// 通知（被同一种子后来的命中通知取代，或超出每小时上限）。
type NotifyStatsChannel struct {
	ConfID  uint                `json:"conf_id"`
	Total   int64               `json:"total"`
	Sent    int64               `json:"sent"`
	Failed  int64               `json:"failed"`
	Pending int64               `json:"pending"`
	Skipped int64               `json:"skipped"`
	Sources []NotifyStatsSource `json:"sources"`
}

// NotifyStatsView 是 GET /api/chatops/notifications/stats 的回应。
type NotifyStatsView struct {
	Since    time.Time            `json:"since"`
	Days     int                  `json:"days"`
	Channels []NotifyStatsChannel `json:"channels"`
}

type notifyStatsRow struct {
	ConfID uint
	Source string
	Result string
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
		Select("notification_conf_id AS conf_id, 'rss' AS source, result, COUNT(*) AS n").
		Where("created_at >= ?", since).Group("notification_conf_id, result").Scan(&rss).Error; err != nil {
		writeChatopsErr(w, http.StatusInternalServerError, "query_failed", "读取 RSS 通知日志失败")
		return
	}
	if err := db.Model(&models.MonitorNotificationLog{}).
		Select("notification_conf_id AS conf_id, source, result, COUNT(*) AS n").
		Where("created_at >= ?", since).Group("notification_conf_id, source, result").Scan(&mon).Error; err != nil {
		writeChatopsErr(w, http.StatusInternalServerError, "query_failed", "读取监控通知日志失败")
		return
	}
	writeJSON(w, NotifyStatsView{Since: since, Days: notifyStatsDays, Channels: aggregateNotifyStats(append(rss, mon...))})
}

// aggregateNotifyStats 按通道合起来（条数多的在前），每个通道里的来源也按条数排。
func aggregateNotifyStats(rows []notifyStatsRow) []NotifyStatsChannel {
	byConf := map[uint]*NotifyStatsChannel{}
	bySrc := map[uint]map[string]*NotifyStatsSource{}
	for _, row := range rows {
		ch := byConf[row.ConfID]
		if ch == nil {
			ch = &NotifyStatsChannel{ConfID: row.ConfID}
			byConf[row.ConfID], bySrc[row.ConfID] = ch, map[string]*NotifyStatsSource{}
		}
		src := bySrc[row.ConfID][row.Source]
		if src == nil {
			src = &NotifyStatsSource{Source: row.Source}
			bySrc[row.ConfID][row.Source] = src
		}
		ch.Total += row.N
		src.Total += row.N
		switch row.Result {
		case "sent":
			ch.Sent += row.N
			src.Sent += row.N
		case "failed":
			ch.Failed += row.N
		case "pending":
			ch.Pending += row.N
		case "suppressed", "throttled":
			ch.Skipped += row.N
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
