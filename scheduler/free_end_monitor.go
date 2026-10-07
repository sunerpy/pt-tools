package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	maxFreeEndAdvance        = 60 * time.Minute
)

type FreeEndMonitor struct {
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	db            *gorm.DB
	downloaderMgr *downloader.DownloaderManager
	checkInterval time.Duration
	pendingTasks  map[uint]*monitorTask
	// inFlight 记录正在处理的种子：独立定时器与周期巡检可能同时拿到同一个到期种子，
	// 只让先到的那个处理，避免重复暂停、删除和改写状态。
	inFlight map[uint]struct{}
	wg       sync.WaitGroup
	running  bool
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
		inFlight:      make(map[uint]struct{}),
	}
}

// beginProcessing 占用种子的处理权；已有协程在处理时返回 false。
func (m *FreeEndMonitor) beginProcessing(id uint) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inFlight == nil {
		m.inFlight = make(map[uint]struct{})
	}
	if _, busy := m.inFlight[id]; busy {
		return false
	}
	m.inFlight[id] = struct{}{}
	return true
}

func (m *FreeEndMonitor) endProcessing(id uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.inFlight, id)
}

func (m *FreeEndMonitor) advanceDuration() time.Duration {
	var cfg models.SettingsGlobal
	if err := m.db.First(&cfg).Error; err != nil {
		return 0
	}
	adv := time.Duration(cfg.FreeEndAdvanceMinutes) * time.Minute
	if adv < 0 {
		return 0
	}
	if adv > maxFreeEndAdvance {
		return maxFreeEndAdvance
	}
	return adv
}

func effectiveDeadline(freeEnd *time.Time, advance time.Duration) time.Time {
	if freeEnd == nil {
		return time.Time{}
	}
	return freeEnd.Add(-advance)
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
	adv := m.advanceDuration()
	for _, t := range torrents {
		if t.FreeEndTime == nil || effectiveDeadline(t.FreeEndTime, adv).Before(now) {
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
	if !torrent.PauseOnFreeEnd {
		return
	}
	if torrent.FreeEndTime == nil {
		global.GetSlogger().Debugf("[FreeEndMonitor] 跳过调度 (种子:%s, ID:%d): FreeEndTime 为空", torrent.Title, torrent.ID)
		return
	}
	if torrent.DownloaderTaskID == "" {
		global.GetSlogger().Debugf("[FreeEndMonitor] 跳过调度 (种子:%s, ID:%d): DownloaderTaskID 为空，将由周期巡检兜底", torrent.Title, torrent.ID)
		return
	}

	adv := m.advanceDuration()

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.pendingTasks[torrent.ID]; exists {
		return
	}

	m.scheduleTaskLockedWithAdvance(torrent, adv)
	global.GetSlogger().Infof("[FreeEndMonitor] 已调度种子 %s (ID:%d) 的免费结束监控，将在 %v 后检查",
		torrent.Title, torrent.ID, time.Until(effectiveDeadline(torrent.FreeEndTime, adv)))
}

func (m *FreeEndMonitor) scheduleTask(torrent models.TorrentInfo) {
	adv := m.advanceDuration()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scheduleTaskLockedWithAdvance(torrent, adv)
}

func (m *FreeEndMonitor) scheduleTaskLockedWithAdvance(torrent models.TorrentInfo, advance time.Duration) {
	if torrent.FreeEndTime == nil {
		return
	}

	delay := time.Until(effectiveDeadline(torrent.FreeEndTime, advance))
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
	// 按上次检查时间轮转：每批处理最久没更新的，任务多于一批时后面的也能轮到。
	err := m.db.Where(
		"pause_on_free_end = ? AND is_paused_by_system = ? AND is_completed = ? AND downloader_task_id != ''",
		true, false, false,
	).Order("last_check_time ASC, id ASC").Limit(progressUpdateBatchSize).Find(&torrents).Error
	if err != nil {
		global.GetSlogger().Errorf("查询待更新进度的种子失败: %v", err)
		return
	}

	if len(torrents) == 0 {
		return
	}

	global.GetSlogger().Debugf("开始更新 %d 个种子的下载进度", len(torrents))

	// 缓存的是 DownloaderManager 持有的共享实例，用完不能 Close。
	downloaderCache := make(map[string]downloader.Downloader)

	for _, t := range torrents {
		dl, ok := downloaderCache[t.DownloaderName]
		if !ok {
			var err error
			dl, err = m.getDownloader(t)
			if err != nil {
				global.GetSlogger().Warnf("获取下载器失败 (种子:%s): %v", t.Title, err)
				m.touchCheckTime(t.ID)
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
			m.touchCheckTime(t.ID)
			continue
		}

		progress := info.Progress * 100
		updates := map[string]any{
			"progress":        progress,
			"torrent_size":    info.TotalSize,
			"last_check_time": time.Now(),
			"check_count":     gorm.Expr("check_count + 1"),
		}

		if isTorrentTrulyCompleted(info) {
			updates["is_completed"] = true
			updates["completed_at"] = time.Now()
			global.GetSlogger().Infof("种子已完成下载: %s (ID:%d, state=%s)", t.Title, t.ID, info.State)
		}

		if err := m.db.Model(&models.TorrentInfo{}).Where("id = ?", t.ID).Updates(updates).Error; err != nil {
			global.GetSlogger().Errorf("更新种子进度失败 (种子:%s): %v", t.Title, err)
		}
	}

	global.GetSlogger().Debugf("已更新 %d 个种子的下载进度", len(torrents))
}

// isTorrentTrulyCompleted returns true only when the torrent is *actually*
// finished from the user's perspective: progress == 1.0 AND the downloader
// reports a state that means the data is complete (seeding/queued/checking
// post-download). A 100% progress reading on its own is unreliable because:
//   - qBit may briefly report progress=1.0 in transient states like
//     `checkingResumeData` after add, then drop back as files mismatch
//     (`missingFiles`) — marking complete locks us out of the free-end timer.
//   - paused/error/missingFiles at 100% means user/downloader interrupted
//     and we cannot infer "done". Free-end timer must still fire.
//
// isTorrentTrulyCompleted returns true only when the torrent is *actually*
// finished from the user's perspective: progress == 1.0 AND the downloader
// reports a state that means the data is complete (seeding/queued).
//
// Why TorrentChecking is excluded: qBit's mapQbitState collapses
// checkingDL / checkingUP / checkingResumeData onto a single generic
// TorrentChecking. checkingResumeData runs *before* downloading on add,
// so progress=1.0 there is meaningless (it's verifying an unexpectedly
// pre-existing file, not real completion). Excluding the whole group
// costs at most one cycle of latency for a torrent in checkingUP, which
// flips to TorrentSeeding on the next pass anyway.
//
// Why TorrentPaused / TorrentError are excluded: a paused-at-100% or
// missingFiles-at-100% torrent is in a degraded state where we cannot
// claim completion; the free-end timer must still fire so we can pause
// it explicitly when the free window closes.
func isTorrentTrulyCompleted(info downloader.Torrent) bool {
	if info.Progress < 1.0 {
		return false
	}
	switch info.State {
	case downloader.TorrentSeeding, downloader.TorrentQueued:
		return true
	}
	return false
}

func (m *FreeEndMonitor) checkAndProcessExpiredTorrents() {
	var torrents []models.TorrentInfo
	now := time.Now()
	cutoff := now.Add(m.advanceDuration())

	err := m.db.Where(
		"pause_on_free_end = ? AND is_paused_by_system = ? AND is_completed = ? AND free_end_time IS NOT NULL AND free_end_time <= ? AND downloader_task_id != ''",
		true, false, false, cutoff,
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

	m.rescheduleMissingFutureTorrents(cutoff)
}

// rescheduleMissingFutureTorrents catches torrents that should have an
// in-process timer but don't — e.g. when the initial Schedule call was
// missed because DownloaderTaskID hadn't been written yet, or a process
// restart dropped the timer for a row whose loadPendingTasksFromDB
// excluded it (transient is_paused_by_system / is_completed flag, etc.).
//
// Without this safety net, a future-expiring torrent missed by the
// in-memory scheduler would only be picked up after free_end_time has
// already passed (worst case: 5 minutes of non-free download before
// pause). Calling ScheduleTorrent is idempotent (it returns early when
// the torrent is already in pendingTasks), so this is a cheap reconcile.
func (m *FreeEndMonitor) rescheduleMissingFutureTorrents(cutoff time.Time) {
	var torrents []models.TorrentInfo
	// Cap the scan like updateAllMonitoredProgress: rows beyond the batch are
	// reconciled by the next 5-min periodicCheck (idempotent), so capping here
	// bounds a restart-time mass load without dropping any torrent permanently.
	// 已在内存里预约过的种子在 SQL 里排除：否则前 50 条都已预约时，每一批都只拿到它们，后面的永远补不上。
	query := m.db.Where(
		"pause_on_free_end = ? AND is_paused_by_system = ? AND is_completed = ? AND free_end_time IS NOT NULL AND free_end_time > ? AND downloader_task_id != ''",
		true, false, false, cutoff,
	)
	if pending := m.pendingTaskIDs(); len(pending) > 0 {
		query = query.Where("id NOT IN ?", pending)
	}
	err := query.Order("free_end_time ASC, id ASC").Limit(progressUpdateBatchSize).Find(&torrents).Error
	if err != nil {
		global.GetSlogger().Errorf("查询待补预约种子失败: %v", err)
		return
	}

	// Read advance once outside the hot mutex to avoid a DB query per torrent
	// under the lock.
	adv := m.advanceDuration()
	rescheduled := 0
	m.mu.Lock()
	for _, t := range torrents {
		if _, exists := m.pendingTasks[t.ID]; exists {
			continue
		}
		m.scheduleTaskLockedWithAdvance(t, adv)
		rescheduled++
	}
	m.mu.Unlock()

	if rescheduled > 0 {
		global.GetSlogger().Infof("[FreeEndMonitor] 周期巡检：补预约了 %d 个未在内存中的未来过期种子", rescheduled)
	}
}

// touchCheckTime 只更新检查时间：这一轮没能更新进度的任务也排到队尾，
// 否则持续失败的任务会一直占着每批最前面的名额。
func (m *FreeEndMonitor) touchCheckTime(id uint) {
	if err := m.db.Model(&models.TorrentInfo{}).Where("id = ?", id).Update("last_check_time", time.Now()).Error; err != nil {
		global.GetSlogger().Warnf("更新检查时间失败 (ID:%d): %v", id, err)
	}
}

// pendingTaskIDs 返回当前已在内存里预约的种子 ID。
func (m *FreeEndMonitor) pendingTaskIDs() []uint {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]uint, 0, len(m.pendingTasks))
	for id := range m.pendingTasks {
		ids = append(ids, id)
	}
	return ids
}

func (m *FreeEndMonitor) handleFreeEndedTorrent(torrent models.TorrentInfo) {
	global.GetSlogger().Debugf("[FreeEndMonitor] 开始处理免费期结束的种子: ID=%d, Title=%s, TaskID=%s, Downloader=%s",
		torrent.ID, torrent.Title, torrent.DownloaderTaskID, torrent.DownloaderName)

	if !m.beginProcessing(torrent.ID) {
		global.GetSlogger().Debugf("[FreeEndMonitor] 种子正在处理中，跳过 (种子:%s, ID:%d)", torrent.Title, torrent.ID)
		return
	}
	defer m.endProcessing(torrent.ID)

	advanced := m.advanceDuration() > 0

	m.mu.Lock()
	delete(m.pendingTasks, torrent.ID)
	m.mu.Unlock()

	// 种子仍处于待处理状态才继续：已被前一次处理暂停或标记完成的直接跳过。
	// 同时处理同一个种子由上面的 inFlight 挡住，这个条件更新只更新 last_check_time，挡不住并发。
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
	// dl 是 DownloaderManager 持有的共享实例，不能 Close：关掉会断开其他调用方正在用的会话。

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

		// 数据还被别的种子用着（如辅种）时只删种子、保留数据；读不到种子列表就下次再试
		withData, err := dataNotShared(dl, torrent.DownloaderTaskID)
		if err != nil {
			global.GetSlogger().Errorf("[FreeEndMonitor] 自动删除前读取种子列表失败 (种子:%s): %v", torrent.Title, err)
			m.markRetry(torrent, fmt.Sprintf("自动删除失败: %v", err))
			return
		}
		if !withData {
			global.GetSlogger().Infof("[FreeEndMonitor] 种子 %s 的数据还被别的种子用着，只删种子、保留数据", torrent.Title)
		}
		if err := dl.RemoveTorrent(torrent.DownloaderTaskID, withData); err != nil {
			if !errors.Is(err, downloader.ErrTorrentNotFound) {
				global.GetSlogger().Errorf("[FreeEndMonitor] 自动删除种子失败 (种子:%s): %v", torrent.Title, err)
				m.markRetry(torrent, fmt.Sprintf("自动删除失败: %v", err))
				return
			}
		}

		m.markAutoDeleted(torrent, progress, totalSize, advanced)
		global.GetSlogger().Infof("[FreeEndMonitor] 种子 %s 已自动删除 (进度:%.1f%%, 原因:免费期结束)", torrent.Title, progress)
		return
	}

	global.GetSlogger().Infof("[FreeEndMonitor] 准备暂停种子: %s (进度:%.1f%%, TaskID:%s)", torrent.Title, progress, torrent.DownloaderTaskID)

	if err := m.pauseTorrentWithRetry(ctx, dl, torrent); err != nil {
		global.GetSlogger().Errorf("[FreeEndMonitor] 暂停种子失败 (种子:%s): %v", torrent.Title, err)
		m.markRetry(torrent, fmt.Sprintf("暂停失败: %v", err))
		return
	}

	m.markPaused(torrent, progress, totalSize, advanced)
	global.GetSlogger().Infof("[FreeEndMonitor] 种子 %s 已暂停 (进度:%.1f%%, 原因:免费期结束)", torrent.Title, progress)
}

// dataNotShared 报告删除下载器里的这个种子时能不能连数据一起删：没有别的种子用着它的数据。
// 种子已不在列表里时返回 true（删除会报种子不存在）。
func dataNotShared(dl downloader.Downloader, taskID string) (bool, error) {
	all, err := dl.GetAllTorrents()
	if err != nil {
		return false, fmt.Errorf("读取下载器种子失败: %w", err)
	}
	for _, t := range all {
		if t.ID == taskID || strings.EqualFold(t.InfoHash, taskID) {
			return !downloader.SharesData(all, t, nil), nil
		}
	}
	return true, nil
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
	return isTorrentTrulyCompleted(info), progress, info.TotalSize, nil
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

func (m *FreeEndMonitor) markPaused(torrent models.TorrentInfo, progress float64, totalSize int64, advanced bool) {
	now := time.Now()
	reason := "免费期结束，下载未完成"
	if advanced {
		reason = "免费期临近结束，已暂停（未完成）"
	}
	updates := map[string]any{
		"is_paused_by_system": true,
		"paused_at":           now,
		"pause_reason":        reason,
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
		// 预补偿 advance：scheduleTaskLocked 会用 effectiveDeadline 再减去 advance，
		// 这里先加回去，使净触发时刻保持 now+retryDelay（重试不受提前量影响）。
		retryTime := retryDeadline(now, updated.RetryCount, m.advanceDuration())
		updated.FreeEndTime = &retryTime
		m.scheduleTask(updated)
	}
}

func retryDeadline(now time.Time, retryCount int, advance time.Duration) time.Time {
	retryDelay := min(baseRetryDelay*time.Duration(1<<uint(retryCount))*2, maxRetryDelay)
	return now.Add(retryDelay).Add(advance)
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

func (m *FreeEndMonitor) markAutoDeleted(torrent models.TorrentInfo, progress float64, totalSize int64, advanced bool) {
	now := time.Now()
	reason := "免费期结束，自动删除（未完成）"
	if advanced {
		reason = "免费期临近结束，自动删除（未完成）"
	}
	updates := map[string]any{
		"is_paused_by_system": true,
		"paused_at":           now,
		"pause_reason":        reason,
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
	// 按上次检查时间轮转：每批处理最久没更新的，任务多于一批时后面的也能轮到。
	err := m.db.Where(
		"is_pushed = ? AND is_completed = ? AND is_paused_by_system = ? AND downloader_task_id != ''",
		&pushed, false, false,
	).Order("last_check_time ASC, id ASC").Limit(progressUpdateBatchSize).Find(&torrents).Error
	if err != nil {
		global.GetSlogger().Errorf("查询待更新进度的任务失败: %v", err)
		return
	}

	if len(torrents) == 0 {
		global.GetSlogger().Debugf("没有待更新进度的已推送任务")
		return
	}

	global.GetSlogger().Infof("开始更新 %d 个已推送任务的下载进度", len(torrents))

	// 缓存的是 DownloaderManager 持有的共享实例，用完不能 Close。
	downloaderCache := make(map[string]downloader.Downloader)

	updated := 0
	skipped := 0
	for _, t := range torrents {
		if t.DownloaderName == "" {
			global.GetSlogger().Warnf("任务缺少下载器名称 (ID:%d, Title:%s, TaskID:%s)", t.ID, t.Title, t.DownloaderTaskID)
			m.touchCheckTime(t.ID)
			skipped++
			continue
		}

		dl, ok := downloaderCache[t.DownloaderName]
		if !ok {
			var err error
			dl, err = m.getDownloader(t)
			if err != nil {
				global.GetSlogger().Warnf("获取下载器失败 (ID:%d, Title:%s, Downloader:%s): %v", t.ID, t.Title, t.DownloaderName, err)
				m.touchCheckTime(t.ID)
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
			m.touchCheckTime(t.ID)
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

		if isTorrentTrulyCompleted(info) {
			updates["is_completed"] = true
			updates["completed_at"] = time.Now()
			global.GetSlogger().Infof("任务已完成下载 (ID:%d, Title:%s, state=%s)", t.ID, t.Title, info.State)
		}

		if err := m.db.Model(&models.TorrentInfo{}).Where("id = ?", t.ID).Updates(updates).Error; err != nil {
			global.GetSlogger().Errorf("更新任务进度失败 (ID:%d): %v", t.ID, err)
			continue
		}
		updated++
	}

	global.GetSlogger().Infof("已推送任务进度更新完成: 更新=%d, 跳过=%d, 总计=%d", updated, skipped, len(torrents))
}
