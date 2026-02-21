package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

const (
	defaultCheckInterval     = 5 * time.Minute
	progressUpdateInterval   = 1 * time.Minute
	allTasksProgressInterval = 2 * time.Minute
	archiveCheckInterval     = 6 * time.Hour
	archiveRetentionDays     = 14
	maxRetryCount            = 3
	baseRetryDelay           = 30 * time.Second
	maxRetryDelay            = 10 * time.Minute
	completionCheckTimeout   = 10 * time.Second
	progressUpdateBatchSize  = 50
)

type FreeEndMonitor struct {
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	db            *gorm.DB
	downloaderMgr *downloader.DownloaderManager
	checkInterval time.Duration
	pendingTasks  map[uint]*monitorTask
	wg            sync.WaitGroup
	running       bool
}

type monitorTask struct {
	torrentID uint
	timer     *time.Timer
	cancel    context.CancelFunc
}

func NewFreeEndMonitor(db *gorm.DB, downloaderMgr *downloader.DownloaderManager) *FreeEndMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &FreeEndMonitor{
		ctx:           ctx,
		cancel:        cancel,
		db:            db,
		downloaderMgr: downloaderMgr,
		checkInterval: defaultCheckInterval,
		pendingTasks:  make(map[uint]*monitorTask),
	}
}

func (m *FreeEndMonitor) Start() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = true
	m.mu.Unlock()

	if err := m.loadPendingTasksFromDB(); err != nil {
		global.GetSlogger().Errorf("加载待处理任务失败: %v", err)
	}

	m.wg.Add(4)
	go m.periodicCheck()
	go m.periodicProgressUpdate()
	go m.periodicArchive()
	go m.periodicAllTasksProgressUpdate()

	global.GetSlogger().Info("免费结束监控器已启动")
	return nil
}

func (m *FreeEndMonitor) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	m.mu.Unlock()

	m.cancel()

	m.mu.Lock()
	for _, task := range m.pendingTasks {
		if task.timer != nil {
			task.timer.Stop()
		}
		if task.cancel != nil {
			task.cancel()
		}
	}
	m.pendingTasks = make(map[uint]*monitorTask)
	m.mu.Unlock()

	m.wg.Wait()
	global.GetSlogger().Info("免费结束监控器已停止")
}

func (m *FreeEndMonitor) loadPendingTasksFromDB() error {
	var torrents []models.TorrentInfo
	err := m.db.Where(
		"pause_on_free_end = ? AND is_paused_by_system = ? AND is_completed = ? AND free_end_time IS NOT NULL AND downloader_task_id != ''",
		true, false, false,
	).Find(&torrents).Error
	if err != nil {
		return fmt.Errorf("查询待监控种子失败: %w", err)
	}

	now := time.Now()
	for _, t := range torrents {
		if t.FreeEndTime == nil || t.FreeEndTime.Before(now) {
			m.wg.Add(1)
			go func(torrent models.TorrentInfo) {
				defer m.wg.Done()
				m.handleFreeEndedTorrent(torrent)
			}(t)
			continue
		}
		m.scheduleTask(t)
	}

	global.GetSlogger().Infof("从数据库加载了 %d 个待监控种子", len(torrents))
	return nil
}

func (m *FreeEndMonitor) ScheduleTorrent(torrent models.TorrentInfo) {
	if !torrent.PauseOnFreeEnd || torrent.FreeEndTime == nil || torrent.DownloaderTaskID == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.pendingTasks[torrent.ID]; exists {
		return
	}

	m.scheduleTaskLocked(torrent)
}

func (m *FreeEndMonitor) scheduleTask(torrent models.TorrentInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scheduleTaskLocked(torrent)
}

func (m *FreeEndMonitor) scheduleTaskLocked(torrent models.TorrentInfo) {
	if torrent.FreeEndTime == nil {
		return
	}

	delay := time.Until(*torrent.FreeEndTime)
	if delay < 0 {
		delay = 0
	}

	ctx, cancel := context.WithCancel(m.ctx)
	task := &monitorTask{
		torrentID: torrent.ID,
		cancel:    cancel,
	}

	task.timer = time.AfterFunc(delay, func() {
		select {
		case <-ctx.Done():
			return
		default:
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				m.handleFreeEndedTorrent(torrent)
			}()
		}
	})

	m.pendingTasks[torrent.ID] = task
	global.GetSlogger().Debugf("已调度种子 %s (ID:%d) 的免费结束监控，将在 %v 后检查", torrent.Title, torrent.ID, delay)
}

func (m *FreeEndMonitor) CancelTorrent(torrentID uint) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if task, exists := m.pendingTasks[torrentID]; exists {
		if task.timer != nil {
			task.timer.Stop()
		}
		if task.cancel != nil {
			task.cancel()
		}
		delete(m.pendingTasks, torrentID)
	}
}

func (m *FreeEndMonitor) periodicCheck() {
	defer m.wg.Done()

	m.checkAndProcessExpiredTorrents()

	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkAndProcessExpiredTorrents()
		}
	}
}

func (m *FreeEndMonitor) periodicProgressUpdate() {
	defer m.wg.Done()

	m.updateAllMonitoredProgress()

	ticker := time.NewTicker(progressUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.updateAllMonitoredProgress()
		}
	}
}

func (m *FreeEndMonitor) updateAllMonitoredProgress() {
	var torrents []models.TorrentInfo
	err := m.db.Where(
		"pause_on_free_end = ? AND is_paused_by_system = ? AND is_completed = ? AND downloader_task_id != ''",
		true, false, false,
	).Limit(progressUpdateBatchSize).Find(&torrents).Error
	if err != nil {
		global.GetSlogger().Errorf("查询待更新进度的种子失败: %v", err)
		return
	}

	if len(torrents) == 0 {
		return
	}

	global.GetSlogger().Debugf("开始更新 %d 个种子的下载进度", len(torrents))

	downloaderCache := make(map[string]downloader.Downloader)
	defer func() {
		for _, dl := range downloaderCache {
			dl.Close()
		}
	}()

	for _, t := range torrents {
		dl, ok := downloaderCache[t.DownloaderName]
		if !ok {
			var err error
			dl, err = m.getDownloader(t)
			if err != nil {
				global.GetSlogger().Warnf("获取下载器失败 (种子:%s): %v", t.Title, err)
				continue
			}
			downloaderCache[t.DownloaderName] = dl
		}

		info, err := dl.GetTorrent(t.DownloaderTaskID)
		if err != nil {
			if errors.Is(err, downloader.ErrTorrentNotFound) {
				global.GetSlogger().Warnf("种子已从下载器删除，标记任务状态 (种子:%s, TaskID:%s)", t.Title, t.DownloaderTaskID)
				m.markRemovedFromDownloader(t)
				continue
			}
			global.GetSlogger().Warnf("获取种子信息失败 (种子:%s, TaskID:%s): %v", t.Title, t.DownloaderTaskID, err)
			continue
		}

		progress := info.Progress * 100
		updates := map[string]any{
			"progress":        progress,
			"torrent_size":    info.TotalSize,
			"last_check_time": time.Now(),
			"check_count":     gorm.Expr("check_count + 1"),
		}

		if info.Progress >= 1.0 {
			updates["is_completed"] = true
			updates["completed_at"] = time.Now()
			m.CancelTorrent(t.ID)
			global.GetSlogger().Infof("种子已完成下载: %s (ID:%d)", t.Title, t.ID)
		}

		if err := m.db.Model(&models.TorrentInfo{}).Where("id = ?", t.ID).Updates(updates).Error; err != nil {
			global.GetSlogger().Errorf("更新种子进度失败 (种子:%s): %v", t.Title, err)
		}
	}

	global.GetSlogger().Debugf("已更新 %d 个种子的下载进度", len(torrents))
}

func (m *FreeEndMonitor) checkAndProcessExpiredTorrents() {
	var torrents []models.TorrentInfo
	now := time.Now()

	err := m.db.Where(
		"pause_on_free_end = ? AND is_paused_by_system = ? AND is_completed = ? AND free_end_time IS NOT NULL AND free_end_time <= ? AND downloader_task_id != ''",
		true, false, false, now,
	).Find(&torrents).Error
	if err != nil {
		global.GetSlogger().Errorf("查询已过期种子失败: %v", err)
		return
	}

	if len(torrents) > 0 {
		global.GetSlogger().Infof("发现 %d 个免费期已结束的种子，开始处理", len(torrents))
	}

	for _, t := range torrents {
		m.wg.Add(1)
		go func(torrent models.TorrentInfo) {
			defer m.wg.Done()
			m.handleFreeEndedTorrent(torrent)
		}(t)
	}
}

func (m *FreeEndMonitor) handleFreeEndedTorrent(torrent models.TorrentInfo) {
	global.GetSlogger().Debugf("[FreeEndMonitor] 开始处理免费期结束的种子: ID=%d, Title=%s, TaskID=%s, Downloader=%s",
		torrent.ID, torrent.Title, torrent.DownloaderTaskID, torrent.DownloaderName)

	m.mu.Lock()
	delete(m.pendingTasks, torrent.ID)
	m.mu.Unlock()

	// 使用数据库原子更新获取处理锁，防止独立定时器和周期检查同时处理同一个种子
	// 只有当种子仍处于待处理状态时才继续处理
	result := m.db.Model(&models.TorrentInfo{}).
		Where("id = ? AND is_paused_by_system = ? AND is_completed = ?", torrent.ID, false, false).
		Update("last_check_time", time.Now())
	if result.Error != nil {
		global.GetSlogger().Errorf("[FreeEndMonitor] 获取处理锁失败 (种子:%s, ID:%d): %v", torrent.Title, torrent.ID, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		// 种子已被其他 goroutine 处理（已暂停或已完成），跳过
		global.GetSlogger().Debugf("[FreeEndMonitor] 种子已被处理，跳过 (种子:%s, ID:%d)", torrent.Title, torrent.ID)
		return
	}

	dl, err := m.getDownloader(torrent)
	if err != nil {
		global.GetSlogger().Errorf("[FreeEndMonitor] 获取下载器失败 (种子:%s, ID:%d): %v", torrent.Title, torrent.ID, err)
		m.markRetry(torrent, fmt.Sprintf("获取下载器失败: %v", err))
		return
	}
	defer dl.Close()

	global.GetSlogger().Debugf("[FreeEndMonitor] 成功获取下载器: %s (类型:%s)", dl.GetName(), dl.GetType())

	ctx, cancel := context.WithTimeout(m.ctx, completionCheckTimeout)
	defer cancel()

	completed, progress, totalSize, err := m.checkTorrentCompletion(ctx, dl, torrent.DownloaderTaskID)
	if err != nil {
		global.GetSlogger().Errorf("[FreeEndMonitor] 检查种子完成状态失败 (种子:%s, TaskID:%s): %v", torrent.Title, torrent.DownloaderTaskID, err)
		m.markRetry(torrent, fmt.Sprintf("检查完成状态失败: %v", err))
		return
	}

	global.GetSlogger().Debugf("[FreeEndMonitor] 种子状态: Title=%s, Progress=%.2f%%, TotalSize=%d, Completed=%v",
		torrent.Title, progress, totalSize, completed)

	if completed {
		m.markCompleted(torrent, totalSize)
		global.GetSlogger().Infof("[FreeEndMonitor] 种子 %s 已完成下载，无需暂停", torrent.Title)
		return
	}

	if m.isAutoDeleteEnabled() {
		global.GetSlogger().Infof("[FreeEndMonitor] 准备自动删除种子: %s (进度:%.1f%%, TaskID:%s)", torrent.Title, progress, torrent.DownloaderTaskID)

		if err := dl.RemoveTorrent(torrent.DownloaderTaskID, true); err != nil {
			if !errors.Is(err, downloader.ErrTorrentNotFound) {
				global.GetSlogger().Errorf("[FreeEndMonitor] 自动删除种子失败 (种子:%s): %v", torrent.Title, err)
				m.markRetry(torrent, fmt.Sprintf("自动删除失败: %v", err))
				return
			}
		}

		m.markAutoDeleted(torrent, progress, totalSize)
		global.GetSlogger().Infof("[FreeEndMonitor] 种子 %s 已自动删除 (进度:%.1f%%, 原因:免费期结束)", torrent.Title, progress)
		return
	}

	global.GetSlogger().Infof("[FreeEndMonitor] 准备暂停种子: %s (进度:%.1f%%, TaskID:%s)", torrent.Title, progress, torrent.DownloaderTaskID)

	if err := m.pauseTorrentWithRetry(ctx, dl, torrent); err != nil {
		global.GetSlogger().Errorf("[FreeEndMonitor] 暂停种子失败 (种子:%s): %v", torrent.Title, err)
		m.markRetry(torrent, fmt.Sprintf("暂停失败: %v", err))
		return
	}

	m.markPaused(torrent, progress, totalSize)
	global.GetSlogger().Infof("[FreeEndMonitor] 种子 %s 已暂停 (进度:%.1f%%, 原因:免费期结束)", torrent.Title, progress)
}

// TestHandleFreeEndedTorrent 暴露给测试/调试命令使用
func (m *FreeEndMonitor) TestHandleFreeEndedTorrent(torrent models.TorrentInfo) {
	m.handleFreeEndedTorrent(torrent)
}

func (m *FreeEndMonitor) getDownloader(torrent models.TorrentInfo) (downloader.Downloader, error) {
	if m.downloaderMgr == nil {
		return nil, fmt.Errorf("下载器管理器未初始化")
	}

	if torrent.DownloaderName != "" {
		dl, err := m.downloaderMgr.GetDownloader(torrent.DownloaderName)
		if err == nil {
			return dl, nil
		}
	}

	if torrent.DownloaderID != nil {
		var dlSetting models.DownloaderSetting
		if err := m.db.First(&dlSetting, *torrent.DownloaderID).Error; err == nil {
			return m.downloaderMgr.GetDownloader(dlSetting.Name)
		}
	}

	return m.downloaderMgr.GetDefaultDownloader()
}

func (m *FreeEndMonitor) checkTorrentCompletion(_ context.Context, dl downloader.Downloader, taskID string) (bool, float64, int64, error) {
	info, err := dl.GetTorrent(taskID)
	if err != nil {
		return false, 0, 0, fmt.Errorf("获取种子信息失败: %w", err)
	}

	progress := info.Progress * 100
	return info.Progress >= 1.0, progress, info.TotalSize, nil
}

func (m *FreeEndMonitor) pauseTorrentWithRetry(ctx context.Context, dl downloader.Downloader, torrent models.TorrentInfo) error {
	var lastErr error
	delay := baseRetryDelay

	global.GetSlogger().Debugf("[FreeEndMonitor] 开始暂停种子: Title=%s, TaskID=%s, MaxRetry=%d",
		torrent.Title, torrent.DownloaderTaskID, maxRetryCount)

	for i := range maxRetryCount {
		select {
		case <-ctx.Done():
			global.GetSlogger().Warnf("[FreeEndMonitor] 暂停操作被取消: %v", ctx.Err())
			return ctx.Err()
		default:
		}

		global.GetSlogger().Debugf("[FreeEndMonitor] 尝试暂停种子 (尝试 %d/%d): TaskID=%s", i+1, maxRetryCount, torrent.DownloaderTaskID)

		if err := dl.PauseTorrent(torrent.DownloaderTaskID); err != nil {
			lastErr = err
			global.GetSlogger().Warnf("[FreeEndMonitor] 暂停种子 %s 失败 (尝试 %d/%d): %v", torrent.Title, i+1, maxRetryCount, err)

			if i < maxRetryCount-1 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
				delay = min(delay*2, maxRetryDelay)
			}
			continue
		}

		global.GetSlogger().Debugf("[FreeEndMonitor] 暂停种子成功: Title=%s, TaskID=%s", torrent.Title, torrent.DownloaderTaskID)
		return nil
	}
	return lastErr
}

func (m *FreeEndMonitor) markCompleted(torrent models.TorrentInfo, totalSize int64) {
	now := time.Now()
	updates := map[string]any{
		"is_completed":    true,
		"completed_at":    now,
		"progress":        100.0,
		"torrent_size":    totalSize,
		"last_check_time": now,
	}
	if err := m.db.Model(&models.TorrentInfo{}).Where("id = ?", torrent.ID).Updates(updates).Error; err != nil {
		global.GetSlogger().Errorf("更新种子完成状态失败 (种子:%s): %v", torrent.Title, err)
	}
}

func (m *FreeEndMonitor) markPaused(torrent models.TorrentInfo, progress float64, totalSize int64) {
	now := time.Now()
	updates := map[string]any{
		"is_paused_by_system": true,
		"paused_at":           now,
		"pause_reason":        "免费期结束，下载未完成",
		"progress":            progress,
		"torrent_size":        totalSize,
		"last_check_time":     now,
		"retry_count":         0,
		"last_error":          "",
	}
	if err := m.db.Model(&models.TorrentInfo{}).Where("id = ?", torrent.ID).Updates(updates).Error; err != nil {
		global.GetSlogger().Errorf("更新种子暂停状态失败 (种子:%s): %v", torrent.Title, err)
	}
}

func (m *FreeEndMonitor) markRetry(torrent models.TorrentInfo, errMsg string) {
	now := time.Now()
	updates := map[string]any{
		"retry_count":     gorm.Expr("retry_count + 1"),
		"last_error":      errMsg,
		"last_check_time": now,
	}
	if err := m.db.Model(&models.TorrentInfo{}).Where("id = ?", torrent.ID).Updates(updates).Error; err != nil {
		global.GetSlogger().Errorf("更新种子重试状态失败 (种子:%s): %v", torrent.Title, err)
	}

	var updated models.TorrentInfo
	m.db.First(&updated, torrent.ID)
	if updated.RetryCount < maxRetryCount {
		retryDelay := min(baseRetryDelay*time.Duration(1<<uint(updated.RetryCount))*2, maxRetryDelay)
		retryTime := now.Add(retryDelay)
		updated.FreeEndTime = &retryTime
		m.scheduleTask(updated)
	}
}

func (m *FreeEndMonitor) markRemovedFromDownloader(torrent models.TorrentInfo) {
	now := time.Now()
	updates := map[string]any{
		"is_completed":       true,
		"completed_at":       now,
		"last_check_time":    now,
		"last_error":         "种子已从下载器中删除",
		"downloader_task_id": "",
	}
	if err := m.db.Model(&models.TorrentInfo{}).Where("id = ?", torrent.ID).Updates(updates).Error; err != nil {
		global.GetSlogger().Errorf("更新种子删除状态失败 (种子:%s): %v", torrent.Title, err)
	}
	m.CancelTorrent(torrent.ID)
}

func (m *FreeEndMonitor) isAutoDeleteEnabled() bool {
	var cfg models.SettingsGlobal
	if err := m.db.First(&cfg).Error; err != nil {
		return false
	}
	return cfg.AutoDeleteOnFreeEnd
}

func (m *FreeEndMonitor) markAutoDeleted(torrent models.TorrentInfo, progress float64, totalSize int64) {
	now := time.Now()
	updates := map[string]any{
		"is_paused_by_system": true,
		"paused_at":           now,
		"pause_reason":        "免费期结束，自动删除（未完成）",
		"progress":            progress,
		"torrent_size":        totalSize,
		"last_check_time":     now,
		"retry_count":         0,
		"last_error":          "",
		"downloader_task_id":  "",
	}
	if err := m.db.Model(&models.TorrentInfo{}).Where("id = ?", torrent.ID).Updates(updates).Error; err != nil {
		global.GetSlogger().Errorf("更新种子自动删除状态失败 (种子:%s): %v", torrent.Title, err)
	}
}

func (m *FreeEndMonitor) periodicArchive() {
	defer m.wg.Done()

	m.archiveOldTorrents()

	ticker := time.NewTicker(archiveCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.archiveOldTorrents()
		}
	}
}

func (m *FreeEndMonitor) archiveOldTorrents() {
	cutoff := time.Now().AddDate(0, 0, -archiveRetentionDays)

	var torrentsToArchive []models.TorrentInfo
	// 归档条件：
	// 1. 已完成下载的任务
	// 2. 被系统暂停的任务
	// 3. 被跳过且未下载的任务
	// 4. 已推送但无法追踪的任务（downloader_task_id 为空）
	// 以上都需要超过保留期
	err := m.db.Where(
		"(is_completed = ? OR is_paused_by_system = ? OR (is_skipped = ? AND is_downloaded = ?) OR (is_pushed = ? AND downloader_task_id = '')) AND created_at < ?",
		true, true, true, false, true, cutoff,
	).Find(&torrentsToArchive).Error
	if err != nil {
		global.GetSlogger().Errorf("查询待归档种子失败: %v", err)
		return
	}

	if len(torrentsToArchive) == 0 {
		return
	}

	archived := 0
	for _, t := range torrentsToArchive {
		archive := models.TorrentInfoArchive{
			OriginalID:        t.ID,
			SiteName:          t.SiteName,
			TorrentID:         t.TorrentID,
			TorrentHash:       t.TorrentHash,
			IsFree:            t.IsFree,
			IsDownloaded:      t.IsDownloaded,
			IsPushed:          t.IsPushed,
			IsSkipped:         t.IsSkipped,
			FreeLevel:         t.FreeLevel,
			FreeEndTime:       t.FreeEndTime,
			PushTime:          t.PushTime,
			Title:             t.Title,
			Category:          t.Category,
			Tag:               t.Tag,
			OriginalCreatedAt: t.CreatedAt,
			OriginalUpdatedAt: t.UpdatedAt,
			IsExpired:         t.IsExpired,
			LastCheckTime:     t.LastCheckTime,
			RetryCount:        t.RetryCount,
			LastError:         t.LastError,
			DownloadSource:    t.DownloadSource,
			FilterRuleID:      t.FilterRuleID,
			DownloaderID:      t.DownloaderID,
			DownloaderName:    t.DownloaderName,
			CompletedAt:       t.CompletedAt,
			IsPausedBySystem:  t.IsPausedBySystem,
			PauseOnFreeEnd:    t.PauseOnFreeEnd,
			PausedAt:          t.PausedAt,
			PauseReason:       t.PauseReason,
			IsCompleted:       t.IsCompleted,
			Progress:          t.Progress,
			TorrentSize:       t.TorrentSize,
			DownloaderTaskID:  t.DownloaderTaskID,
			CheckCount:        t.CheckCount,
		}

		err := m.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&archive).Error; err != nil {
				return err
			}
			return tx.Delete(&t).Error
		})
		if err != nil {
			global.GetSlogger().Errorf("归档种子失败 (ID:%d, %s): %v", t.ID, t.Title, err)
			continue
		}
		archived++
	}

	if archived > 0 {
		global.GetSlogger().Infof("已归档 %d 个超过 %d 天的种子记录", archived, archiveRetentionDays)
	}
}

func (m *FreeEndMonitor) periodicAllTasksProgressUpdate() {
	defer m.wg.Done()

	m.updateAllPushedTasksProgress()

	ticker := time.NewTicker(allTasksProgressInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.updateAllPushedTasksProgress()
		}
	}
}

func (m *FreeEndMonitor) updateAllPushedTasksProgress() {
	var torrents []models.TorrentInfo
	pushed := true
	err := m.db.Where(
		"is_pushed = ? AND is_completed = ? AND is_paused_by_system = ? AND downloader_task_id != ''",
		&pushed, false, false,
	).Limit(progressUpdateBatchSize).Find(&torrents).Error
	if err != nil {
		global.GetSlogger().Errorf("查询待更新进度的任务失败: %v", err)
		return
	}

	if len(torrents) == 0 {
		global.GetSlogger().Debugf("没有待更新进度的已推送任务")
		return
	}

	global.GetSlogger().Infof("开始更新 %d 个已推送任务的下载进度", len(torrents))

	downloaderCache := make(map[string]downloader.Downloader)
	defer func() {
		for _, dl := range downloaderCache {
			dl.Close()
		}
	}()

	updated := 0
	skipped := 0
	for _, t := range torrents {
		if t.DownloaderName == "" {
			global.GetSlogger().Warnf("任务缺少下载器名称 (ID:%d, Title:%s, TaskID:%s)", t.ID, t.Title, t.DownloaderTaskID)
			skipped++
			continue
		}

		dl, ok := downloaderCache[t.DownloaderName]
		if !ok {
			var err error
			dl, err = m.getDownloader(t)
			if err != nil {
				global.GetSlogger().Warnf("获取下载器失败 (ID:%d, Title:%s, Downloader:%s): %v", t.ID, t.Title, t.DownloaderName, err)
				skipped++
				continue
			}
			downloaderCache[t.DownloaderName] = dl
		}

		info, err := dl.GetTorrent(t.DownloaderTaskID)
		if err != nil {
			if errors.Is(err, downloader.ErrTorrentNotFound) {
				// 种子已从下载器中删除，更新数据库状态
				global.GetSlogger().Warnf("种子已从下载器删除，标记任务状态 (ID:%d, Title:%s, TaskID:%s)", t.ID, t.Title, t.DownloaderTaskID)
				m.markRemovedFromDownloader(t)
				updated++
				continue
			}
			global.GetSlogger().Warnf("获取种子信息失败 (ID:%d, Title:%s, TaskID:%s): %v", t.ID, t.Title, t.DownloaderTaskID, err)
			skipped++
			continue
		}

		progress := info.Progress * 100
		updates := map[string]any{
			"progress":        progress,
			"torrent_size":    info.TotalSize,
			"last_check_time": time.Now(),
			"check_count":     gorm.Expr("check_count + 1"),
		}

		if info.Progress >= 1.0 {
			updates["is_completed"] = true
			updates["completed_at"] = time.Now()
			global.GetSlogger().Infof("任务已完成下载 (ID:%d, Title:%s)", t.ID, t.Title)
		}

		if err := m.db.Model(&models.TorrentInfo{}).Where("id = ?", t.ID).Updates(updates).Error; err != nil {
			global.GetSlogger().Errorf("更新任务进度失败 (ID:%d): %v", t.ID, err)
			continue
		}
		updated++
	}

	global.GetSlogger().Infof("已推送任务进度更新完成: 更新=%d, 跳过=%d, 总计=%d", updated, skipped, len(torrents))
}
