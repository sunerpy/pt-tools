package events

// EvtTorrentAdded: Published when a torrent is successfully added to a downloader.
// Triggered by the RSS pipeline after pushing to a downloader via internal/push.go or manual download.
const EvtTorrentAdded EventType = "torrent.added"

type TorrentAddedPayload struct {
	TorrentID      string `json:"torrent_id"`
	SiteName       string `json:"site_name"`
	Title          string `json:"title"`
	Size           int64  `json:"size"`
	DownloaderName string `json:"downloader_name"`
}

// EvtTorrentCompleted: Published when a torrent download completes (100% downloaded).
// Triggered by downloader status update monitors (future integration with downloader APIs).
const EvtTorrentCompleted EventType = "torrent.completed"

type TorrentCompletedPayload struct {
	TorrentID string `json:"torrent_id"`
	SiteName  string `json:"site_name"`
	Title     string `json:"title"`
}

// EvtTorrentFailed: Published when a torrent fails to download (error state in downloader).
// Triggered by downloader status monitors and error handlers.
const EvtTorrentFailed EventType = "torrent.failed"

type TorrentFailedPayload struct {
	TorrentID string `json:"torrent_id"`
	ErrorMsg  string `json:"error_msg"`
}

// EvtFreeEndingSoon: Published when a free torrent's free period is ending soon (< 1 hour remaining).
// Triggered by scheduler/free_end_monitor.go before deadline.
const EvtFreeEndingSoon EventType = "free.ending_soon"

type FreeEndingSoonPayload struct {
	TorrentID  string `json:"torrent_id"`
	SiteName   string `json:"site_name"`
	FreeEndsAt int64  `json:"free_ends_at"` // Unix timestamp
}

// EvtFreeEnded: Published when a free torrent's free period has ended.
// Triggered by scheduler/free_end_monitor.go at deadline; may trigger auto-pause or auto-delete.
const EvtFreeEnded EventType = "free.ended"

type FreeEndedPayload struct {
	TorrentID string `json:"torrent_id"`
	SiteName  string `json:"site_name"`
	Title     string `json:"title"`
}

// EvtDiskLow: Published when disk space falls below minimum threshold (CleanupMinDiskSpaceGB).
// Triggered by internal/push.go disk protection check and scheduler/cleanup_monitor.go.
const EvtDiskLow EventType = "disk.low"

type DiskLowPayload struct {
	FreeSpaceGB   float64 `json:"free_space_gb"`
	MinRequiredGB float64 `json:"min_required_gb"`
	Message       string  `json:"message"`
}

// EvtCleanupTriggered: Published when auto-cleanup (auto-delete) is triggered.
// Triggered by scheduler/cleanup_monitor.go after removing torrents.
const EvtCleanupTriggered EventType = "cleanup.triggered"

type CleanupTriggeredPayload struct {
	RemovedCount int64   `json:"removed_count"`
	FreedSpaceGB float64 `json:"freed_space_gb"`
}

// EvtSiteLoginExpired: Published when a site login/cookie has expired or is invalid.
// Triggered by site drivers when authentication fails (HTTP 401/403 or login page redirect).
const EvtSiteLoginExpired EventType = "site.login_expired"

type SiteLoginExpiredPayload struct {
	SiteName string `json:"site_name"`
	Message  string `json:"message"`
}

// EvtSiteScrapedDaily: Published as a daily summary event after scraping a site.
// Triggered by internal/common.go RSS fetch pipeline (if implemented) or site drivers.
const EvtSiteScrapedDaily EventType = "site.scraped_daily"

type SiteScrapedDailyPayload struct {
	SiteName      string `json:"site_name"`
	TorrentsCount int64  `json:"torrents_count"`
}

// EvtNotificationDelivered: Published when a notification is successfully delivered via a channel.
// Triggered by internal/notify/outbox.go after successful send to Telegram/QQ/Webhook/etc.
const EvtNotificationDelivered EventType = "notification.delivered"

type NotificationDeliveredPayload struct {
	NotifID   string `json:"notif_id"`
	Channel   string `json:"channel"` // "telegram", "qq", "webhook", "wecom", etc.
	Recipient string `json:"recipient"`
}

// EvtNotificationFailed: Published when a notification fails to deliver.
// Triggered by internal/notify/outbox.go after exhausting retries or immediate send error.
const EvtNotificationFailed EventType = "notification.failed"

type NotificationFailedPayload struct {
	NotifID  string `json:"notif_id"`
	Channel  string `json:"channel"`
	ErrorMsg string `json:"error_msg"`
}
