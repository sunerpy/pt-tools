package scheduler

import (
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/models"
)

// RSSState 是 RSS 任务此刻的状态，网页的状态栏和提示条按它显示。
type RSSState string

const (
	// RSSRunning 有 RSS 任务在运行。
	RSSRunning RSSState = "running"
	// RSSStarting 正在按配置启动：重载还没做完（可能在等下载器），或者开着自动启动、启动后的那次重载还没开始。
	RSSStarting RSSState = "starting"
	// RSSPaused 用户手动停止了，再次启动或程序重启前不会自己恢复。
	RSSPaused RSSState = "paused"
	// RSSManual 没开自动启动，程序启动以后还没有启动过。
	RSSManual RSSState = "manual"
	// RSSNotReady 全局设置里的下载目录为空。
	RSSNotReady RSSState = "not_ready"
	// RSSNoDownloader 没有设置默认下载器，上一次重载没有启动任务。
	RSSNoDownloader RSSState = "no_downloader"
	// RSSDownloaderUnhealthy 默认下载器连不上，上一次重载没有启动任务。
	RSSDownloaderUnhealthy RSSState = "downloader_unhealthy"
	// RSSNoSubscriptions 已启用的站点下没有可以运行的 RSS 订阅。
	RSSNoSubscriptions RSSState = "no_subscriptions"
	// RSSStopped 启动过，但没有任务在运行（例如订阅所在的站点都没有注册）。
	RSSStopped RSSState = "stopped"
)

// RSSStatus 是 RSS 任务的状态快照。
type RSSStatus struct {
	State RSSState
	// Jobs 是正在运行的 RSS 任务数（每个订阅一个）。
	Jobs int
	// Subscriptions 是配置里可以运行的订阅数：已启用站点下、不是示例、地址有效的订阅（和 Reload 的口径一致）。
	Subscriptions int
	AutoStart     bool
	// StartedAt 是最早一个在运行的任务的启动时间，没有任务在运行时为零值。
	StartedAt time.Time
}

// RSSStatus 按全局设置和已启用站点下的订阅，给出 RSS 任务此刻的状态。
// rss 只需要已启用站点的订阅；示例订阅和无效地址在这里剔除。
func (m *Manager) RSSStatus(gl models.SettingsGlobal, rss []models.RSSConfig) RSSStatus {
	m.mu.Lock()
	st := RSSStatus{Jobs: len(m.jobs), AutoStart: gl.AutoStart}
	for _, j := range m.jobs {
		if st.StartedAt.IsZero() || j.startedAt.Before(st.StartedAt) {
			st.StartedAt = j.startedAt
		}
	}
	reloading := m.reloading > 0
	paused, wanted, decision := m.jobsPaused, m.jobsWanted, m.lastDecision
	m.mu.Unlock()

	for i := range rss {
		if !rss[i].ShouldSkip() && validRSS(rss[i].URL) {
			st.Subscriptions++
		}
	}

	switch {
	case st.Jobs > 0:
		st.State = RSSRunning
	case reloading:
		st.State = RSSStarting
	case paused:
		st.State = RSSPaused
	case strings.TrimSpace(gl.DownloadDir) == "":
		st.State = RSSNotReady
	case decision == RSSNoDownloader || decision == RSSDownloaderUnhealthy:
		st.State = decision
	case st.Subscriptions == 0:
		st.State = RSSNoSubscriptions
	case !gl.AutoStart && !wanted:
		st.State = RSSManual
	case gl.AutoStart && (decision == "" || decision == RSSManual):
		// 自动启动开着：程序启动时派发的那次重载，或者刚打开自动启动引起的那次重载，还没有开始
		st.State = RSSStarting
	default:
		st.State = RSSStopped
	}
	return st
}
