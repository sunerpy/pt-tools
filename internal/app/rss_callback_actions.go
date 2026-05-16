package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
)

// TorrentDataFetcher fetches the raw .torrent payload for a (siteName,
// torrentID) pair. Callers wire in a closure backed by the v2 site
// registry so this package stays free of web / site/v2 imports.
type TorrentDataFetcher func(ctx context.Context, siteName, torrentID string) ([]byte, error)

// RSSCallbackActions resolves Telegram inline-button callbacks (dl/ig)
// against the rss_notification_log row referenced by the button payload.
type RSSCallbackActions struct {
	db      *gorm.DB
	fetcher TorrentDataFetcher
	now     func() time.Time
}

// NewRSSCallbackActions wires the dependencies needed by Telegram's
// CallbackActionHandler. fetcher may be nil; OnRSSDownload will then
// short-circuit with an explanatory error so callers see the wiring gap.
func NewRSSCallbackActions(db *gorm.DB, fetcher TorrentDataFetcher) *RSSCallbackActions {
	return &RSSCallbackActions{db: db, fetcher: fetcher, now: time.Now}
}

// OnRSSIgnore marks the underlying log row as suppressed. It is idempotent
// and safe to call against a row that is already in any terminal state.
func (a *RSSCallbackActions) OnRSSIgnore(ctx context.Context, logID uint, userID int64) error {
	if a == nil || a.db == nil {
		return errors.New("rss callback actions: db 未初始化")
	}
	res := a.db.WithContext(ctx).Model(&models.RSSNotificationLog{}).
		Where("id = ?", logID).
		Updates(map[string]any{
			"result":     "suppressed",
			"updated_at": a.now(),
		})
	if res.Error != nil {
		return fmt.Errorf("更新 rss_notification_log#%d 为 suppressed 失败: %w", logID, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("rss_notification_log#%d 不存在", logID)
	}
	return nil
}

// OnRSSDownload looks up the torrent referenced by logID and pushes it via
// the existing internal.PushTorrentToDownloader path. The downloader is
// resolved from RSSSubscription.DownloaderID; when nil, the default
// downloader is used. last_error is cleared on success and populated on
// failure so the row in /chatops/rss-notifications reflects reality.
func (a *RSSCallbackActions) OnRSSDownload(ctx context.Context, logID uint, userID int64) error {
	if a == nil || a.db == nil {
		return errors.New("rss callback actions: db 未初始化")
	}

	var row models.RSSNotificationLog
	if err := a.db.WithContext(ctx).First(&row, logID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("rss_notification_log#%d 不存在", logID)
		}
		return fmt.Errorf("查询 rss_notification_log#%d 失败: %w", logID, err)
	}

	if a.fetcher == nil {
		return a.recordPushError(ctx, &row, errors.New("种子下载链路未接线"))
	}

	var rss models.RSSSubscription
	if err := a.db.WithContext(ctx).First(&rss, row.RSSID).Error; err != nil {
		return a.recordPushError(ctx, &row, fmt.Errorf("查询 RSS 订阅 #%d 失败: %w", row.RSSID, err))
	}

	downloaderID, err := a.resolveDownloaderID(ctx, rss.DownloaderID)
	if err != nil {
		return a.recordPushError(ctx, &row, err)
	}

	data, err := a.fetcher(ctx, row.SiteName, row.TorrentID)
	if err != nil {
		return a.recordPushError(ctx, &row, fmt.Errorf("下载种子文件失败: %w", err))
	}
	if len(data) == 0 {
		return a.recordPushError(ctx, &row, errors.New("下载种子文件返回空数据"))
	}

	pushReq := internal.PushTorrentRequest{
		SiteID:       row.SiteName,
		TorrentID:    row.TorrentID,
		TorrentData:  data,
		Title:        row.SiteName + "/" + row.TorrentID,
		Category:     rss.Category,
		Tags:         rss.Tag,
		SavePath:     rss.DownloadPath,
		DownloaderID: downloaderID,
	}
	res, err := internal.PushTorrentToDownloader(ctx, pushReq)
	if err != nil {
		return a.recordPushError(ctx, &row, err)
	}
	if res == nil || (!res.Success && !res.Skipped) {
		msg := "推送失败"
		if res != nil && res.Message != "" {
			msg = res.Message
		}
		return a.recordPushError(ctx, &row, errors.New(msg))
	}

	if uerr := a.db.WithContext(ctx).Model(&models.RSSNotificationLog{}).
		Where("id = ?", row.ID).
		Updates(map[string]any{
			"last_error": "",
			"updated_at": a.now(),
		}).Error; uerr != nil {
		return fmt.Errorf("更新 rss_notification_log#%d 成功状态失败: %w", row.ID, uerr)
	}
	return nil
}

func (a *RSSCallbackActions) resolveDownloaderID(ctx context.Context, configured *uint) (uint, error) {
	if configured != nil && *configured != 0 {
		return *configured, nil
	}
	var dl models.DownloaderSetting
	if err := a.db.WithContext(ctx).Where("is_default = ? AND enabled = ?", true, true).First(&dl).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, errors.New("未配置默认下载器")
		}
		return 0, fmt.Errorf("查询默认下载器失败: %w", err)
	}
	return dl.ID, nil
}

func (a *RSSCallbackActions) recordPushError(ctx context.Context, row *models.RSSNotificationLog, cause error) error {
	if cause == nil {
		return nil
	}
	_ = a.db.WithContext(ctx).Model(&models.RSSNotificationLog{}).
		Where("id = ?", row.ID).
		Updates(map[string]any{
			"last_error": cause.Error(),
			"updated_at": a.now(),
		}).Error
	return cause
}
