package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/redact"
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
	// pauseConfirmWait 是暂停以后回读状态的间隔（测试里改短）
	pauseConfirmWait time.Duration
	notifier         *MonitorNotifier
}

// freeEndNotifySource 是免费结束监控告警在通知日志里的来源。
const freeEndNotifySource = "free_end"

// SetNotifier 接上告警用的通知投递器（暂停连续失败时发）。
func (m *FreeEndMonitor) SetNotifier(n *MonitorNotifier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifier = n
}

const (
	// freeEndAlertAfter：暂停（或删除）连续失败这么多次起告警（每个免费期一次）
	freeEndAlertAfter = 3
	// pauseConfirmReads 是暂停以后回读状态的次数，间隔 pauseConfirmWait
	pauseConfirmReads       = 3
	defaultPauseConfirmWait = time.Second
	// pendingPushGrace：推送标记写下不到这么久的不碰（推送可能还在进行）
	pendingPushGrace = 2 * time.Minute
	// pendingPushExpire：标记写下这么久下载器里还没有，就是加入没发生
	pendingPushExpire = 30 * time.Minute
)

// freeEndBackoff 是第 n 次失败以后的重试间隔：30 秒起翻倍，10 分钟封顶。
func freeEndBackoff(n int) time.Duration {
	if n < 1 {
		n = 1
	}
	d := baseRetryDelay
	for i := 1; i < n && d < maxRetryDelay; i++ {
		d *= 2
	}
	return min(d, maxRetryDelay)
}

// freeEndMonitored 报告这条记录现在还归免费结束监控管：开着到期暂停、没暂停也没完成、有任务编号，
// 而且有处理时刻（计划暂停时间，或者免费的结束时间）。
func freeEndMonitored(t models.TorrentInfo) bool {
	return t.PauseOnFreeEnd && !t.IsPausedBySystem && !t.IsCompleted && t.DownloaderTaskID != "" &&
		(t.PausePlannedAt != nil || (t.IsFree && t.FreeEndTime != nil))
}

// pauseDeadline 是这条记录该处理（暂停或删除）的时刻：计划暂停时间与「免费结束时间减提前量」里更早的那个，
// 都没有时为零。计划暂停时间在推送时按结束时间写下；RSS 读到已推送的种子免费缩短、取消时把它改早。
func pauseDeadline(t models.TorrentInfo, advance time.Duration) time.Time {
	var d time.Time
	if t.PausePlannedAt != nil {
		d = *t.PausePlannedAt
	}
	if t.IsFree && t.FreeEndTime != nil {
		if e := effectiveDeadline(t.FreeEndTime, advance); d.IsZero() || e.Before(d) {
			d = e
		}
	}
	return d
}

// dbTime 把写进库里、会在 SQL 里比较的时间换成进程本地时区，与 internal.DBTime 一致（见那里的说明）。
func dbTime(t time.Time) time.Time { return t.In(time.Local) }

// 归免费结束监控管的记录（与 freeEndMonitored 同一个条件）
const monitoredWhere = "pause_on_free_end = ? AND is_paused_by_system = ? AND is_completed = ? AND downloader_task_id != '' AND " +
	"(pause_planned_at IS NOT NULL OR (is_free = ? AND free_end_time IS NOT NULL))"

// 到了处理时刻的记录：计划暂停时间已到，或者免费结束时间减提前量已到
//
// 时间按时刻比较（julianday），不按文本：SQLite 按文本存时间，旧记录与新记录的时区可能不一样。
const dueWhere = "((pause_planned_at IS NOT NULL AND julianday(pause_planned_at) <= julianday(?)) OR " +
	"(is_free = ? AND free_end_time IS NOT NULL AND julianday(free_end_time) <= julianday(?)))"

type monitorTask struct {
	torrentID uint
	timer     *time.Timer
	cancel    context.CancelFunc
	// deadline 是这次预约的处理时刻（ScheduleTorrent 据此判断要不要换掉旧的预约）
	deadline time.Time
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
	err := m.db.Where(monitoredWhere, true, false, false, true).Find(&torrents).Error
	if err != nil {
		return fmt.Errorf("查询待监控种子失败: %w", err)
	}

	now := time.Now()
	adv := m.advanceDuration()
	for _, t := range torrents {
		if pauseDeadline(t, adv).Before(now) {
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

// ScheduleTorrent 按记录预约到期处理。已经预约过、处理时刻没变的不重复预约；时刻变了（RSS 读到免费缩短、取消或延长）就换掉旧的。
func (m *FreeEndMonitor) ScheduleTorrent(torrent models.TorrentInfo) {
	// 只管免费的种子：半价、2X 这类优惠结束时不暂停也不删（以前 RSS 把它们的结束时间也记成了免费到期）
	if !torrent.PauseOnFreeEnd {
		return
	}
	if torrent.PausePlannedAt == nil && (!torrent.IsFree || torrent.FreeEndTime == nil) {
		if torrent.IsFree {
			global.GetSlogger().Debugf("[FreeEndMonitor] 跳过调度 (种子:%s, ID:%d): FreeEndTime 为空", torrent.Title, torrent.ID)
		}
		return
	}
	if torrent.DownloaderTaskID == "" {
		global.GetSlogger().Debugf("[FreeEndMonitor] 跳过调度 (种子:%s, ID:%d): DownloaderTaskID 为空，将由周期巡检兜底", torrent.Title, torrent.ID)
		return
	}

	adv := m.advanceDuration()
	deadline := pauseDeadline(torrent, adv)

	m.mu.Lock()
	defer m.mu.Unlock()

	if old, exists := m.pendingTasks[torrent.ID]; exists {
		if old.deadline.Equal(deadline) {
			return
		}
		stopTask(old)
		delete(m.pendingTasks, torrent.ID)
	}

	m.scheduleAtLocked(torrent, deadline)
	global.GetSlogger().Infof("[FreeEndMonitor] 已调度种子 %s (ID:%d) 的免费结束监控，将在 %v 后检查",
		torrent.Title, torrent.ID, time.Until(deadline))
}

func stopTask(task *monitorTask) {
	if task.timer != nil {
		task.timer.Stop()
	}
	if task.cancel != nil {
		task.cancel()
	}
}

func (m *FreeEndMonitor) scheduleTask(torrent models.TorrentInfo) {
	adv := m.advanceDuration()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scheduleTaskLockedWithAdvance(torrent, adv)
}

func (m *FreeEndMonitor) scheduleTaskLockedWithAdvance(torrent models.TorrentInfo, advance time.Duration) {
	deadline := pauseDeadline(torrent, advance)
	if deadline.IsZero() {
		return
	}
	m.scheduleAtLocked(torrent, deadline)
}

// scheduleAtLocked 在 deadline 处理这条记录（调用方持有 mu）。到时处理函数会重读记录，不用这里的结构体判断。
func (m *FreeEndMonitor) scheduleAtLocked(torrent models.TorrentInfo, deadline time.Time) {
	delay := max(time.Until(deadline), 0)

	ctx, cancel := context.WithCancel(m.ctx)
	task := &monitorTask{
		torrentID: torrent.ID,
		cancel:    cancel,
		deadline:  deadline,
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
		stopTask(task)
		delete(m.pendingTasks, torrentID)
	}
}

func (m *FreeEndMonitor) periodicCheck() {
	defer m.wg.Done()

	m.checkAndProcessExpiredTorrents()
	m.reconcilePendingPushes()

	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkAndProcessExpiredTorrents()
			m.reconcilePendingPushes()
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

		completed := isTorrentTrulyCompleted(info)
		if completed {
			updates["is_completed"] = true
			updates["completed_at"] = time.Now()
			global.GetSlogger().Infof("种子已完成下载: %s (ID:%d, state=%s)", t.Title, t.ID, info.State)
		}

		if res := m.progressUpdate(t.ID, completed).Updates(updates); res.Error != nil {
			global.GetSlogger().Errorf("更新种子进度失败 (种子:%s): %v", t.Title, res.Error)
		} else if completed && res.RowsAffected > 0 {
			publishCompleted(t, info.InfoHash)
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

	err := m.db.Where(monitoredWhere, true, false, false, true).
		Where(dueWhere, now, true, cutoff).Find(&torrents).Error
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
	now := time.Now()
	var torrents []models.TorrentInfo
	// Cap the scan like updateAllMonitoredProgress: rows beyond the batch are
	// reconciled by the next 5-min periodicCheck (idempotent), so capping here
	// bounds a restart-time mass load without dropping any torrent permanently.
	// 已在内存里预约过的种子在 SQL 里排除：否则前 50 条都已预约时，每一批都只拿到它们，后面的永远补不上。
	query := m.db.Where(monitoredWhere, true, false, false, true).Where("NOT "+dueWhere, now, true, cutoff)
	if pending := m.pendingTaskIDs(); len(pending) > 0 {
		query = query.Where("id NOT IN ?", pending)
	}
	err := query.Order("COALESCE(pause_planned_at, free_end_time) ASC, id ASC").Limit(progressUpdateBatchSize).Find(&torrents).Error
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

// handleFreeEndedTorrent 处理到了暂停时间的种子。登记时的结构体只用来找记录：处理前重读，
// 不再归监控管（关了到期暂停、已暂停、已完成）就不动，免费延长了就按新的结束时间重新预约。
func (m *FreeEndMonitor) handleFreeEndedTorrent(scheduled models.TorrentInfo) {
	if !m.beginProcessing(scheduled.ID) {
		global.GetSlogger().Debugf("[FreeEndMonitor] 种子正在处理中，跳过 (种子:%s, ID:%d)", scheduled.Title, scheduled.ID)
		return
	}
	defer m.endProcessing(scheduled.ID)

	m.mu.Lock()
	delete(m.pendingTasks, scheduled.ID)
	m.mu.Unlock()

	var torrent models.TorrentInfo
	if err := m.db.First(&torrent, scheduled.ID).Error; err != nil {
		global.GetSlogger().Warnf("[FreeEndMonitor] 读取种子记录失败，周期巡检会再处理 (ID:%d): %v", scheduled.ID, err)
		return
	}
	if !freeEndMonitored(torrent) {
		global.GetSlogger().Debugf("[FreeEndMonitor] 种子已不需要到期处理，跳过 (种子:%s, ID:%d)", torrent.Title, torrent.ID)
		return
	}
	adv := m.advanceDuration()
	if deadline := pauseDeadline(torrent, adv); deadline.After(time.Now()) {
		// 免费延长了（或提前量改小了）：按记录里现在的处理时刻重新预约
		m.scheduleTask(torrent)
		global.GetSlogger().Infof("[FreeEndMonitor] 种子 %s 的处理时刻变晚了，改在 %s 处理", torrent.Title, deadline.Format(time.DateTime))
		return
	}
	advanced := adv > 0
	global.GetSlogger().Debugf("[FreeEndMonitor] 开始处理免费期结束的种子: ID=%d, Title=%s, TaskID=%s, Downloader=%s",
		torrent.ID, torrent.Title, torrent.DownloaderTaskID, torrent.DownloaderName)

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
		if errors.Is(err, downloader.ErrTorrentNotFound) {
			// 任务已经不在下载器里：不会再下载，不用暂停
			global.GetSlogger().Infof("[FreeEndMonitor] 种子 %s 已不在下载器里，不再监控", torrent.Title)
			m.markRemovedFromDownloader(torrent)
			return
		}
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

	if err := m.pauseAndConfirm(ctx, dl, torrent); err != nil {
		global.GetSlogger().Errorf("[FreeEndMonitor] 暂停种子失败 (种子:%s): %v", torrent.Title, err)
		m.markRetry(torrent, err.Error())
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
			return !downloader.NewDataSharing(all, downloader.FilesOf(dl)).Shares(t, nil), nil
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

// pauseAndConfirm 暂停种子并回读下载器确认：状态是暂停或停止才算成功（接口返回成功不够）。
// 种子已经不在下载器里也算：不会再下载。
func (m *FreeEndMonitor) pauseAndConfirm(ctx context.Context, dl downloader.Downloader, torrent models.TorrentInfo) error {
	if err := m.db.Model(&models.TorrentInfo{}).Where("id = ?", torrent.ID).Update("pause_requested_at", time.Now()).Error; err != nil {
		global.GetSlogger().Warnf("[FreeEndMonitor] 记录暂停请求时间失败 (种子:%s): %v", torrent.Title, err)
	}
	if err := dl.PauseTorrent(torrent.DownloaderTaskID); err != nil {
		return fmt.Errorf("暂停失败: %w", err)
	}
	wait := m.pauseConfirmWait
	if wait <= 0 {
		wait = defaultPauseConfirmWait
	}
	state := downloader.TorrentUnknown
	var readErr error
	for i := range pauseConfirmReads {
		if i > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("确认暂停被取消: %w", ctx.Err())
			case <-time.After(wait):
			}
		}
		info, err := dl.GetTorrent(torrent.DownloaderTaskID)
		if errors.Is(err, downloader.ErrTorrentNotFound) {
			return nil
		}
		if err != nil {
			readErr = err
			continue
		}
		state = info.State
		if state == downloader.TorrentPaused || state == downloader.TorrentStopped {
			return nil
		}
	}
	if readErr != nil && state == downloader.TorrentUnknown {
		return fmt.Errorf("暂停后读不到下载器里的状态: %w", readErr)
	}
	return fmt.Errorf("暂停接口返回成功，但下载器里的状态仍是 %s", state)
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
	res := m.progressUpdate(torrent.ID, true).Updates(updates)
	if res.Error != nil {
		global.GetSlogger().Errorf("更新种子完成状态失败 (种子:%s): %v", torrent.Title, res.Error)
		return
	}
	if res.RowsAffected > 0 {
		publishCompleted(torrent, "")
	}
}

// progressUpdate 是更新一个种子进度的语句。标记完成时只更新还没完成的记录：两个进度循环与免费到期处理
// 可能同时看到同一个种子下载完，只有真正把它改成完成的那一次发布完成事件。
func (m *FreeEndMonitor) progressUpdate(id uint, completing bool) *gorm.DB {
	q := m.db.Model(&models.TorrentInfo{}).Where("id = ?", id)
	if completing {
		q = q.Where("is_completed = ?", false)
	}
	return q
}

// publishCompleted 发布下载完成事件（整理入库订阅）。hash 为空时用记录里的。
func publishCompleted(t models.TorrentInfo, hash string) {
	if hash == "" && t.TorrentHash != nil {
		hash = *t.TorrentHash
	}
	payload := events.TorrentCompletedPayload{
		TorrentID: t.TorrentID, SiteName: t.SiteName, Title: t.Title,
		DownloaderName: t.DownloaderName, TaskID: t.DownloaderTaskID, InfoHash: strings.ToLower(hash),
	}
	if t.DownloaderID != nil {
		payload.DownloaderID = *t.DownloaderID
	}
	if err := events.PublishWithPayload(events.EvtTorrentCompleted, payload); err != nil {
		global.GetSlogger().Warnf("发布下载完成事件失败 (种子:%s): %v", t.Title, err)
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
		"free_end_failures":   0,
		"free_end_alerted_at": nil,
	}
	if err := m.db.Model(&models.TorrentInfo{}).Where("id = ?", torrent.ID).Updates(updates).Error; err != nil {
		global.GetSlogger().Errorf("更新种子暂停状态失败 (种子:%s): %v", torrent.Title, err)
	}
}

// markRetry 记一次到期处理失败（读不到下载器、暂停没生效、删除失败）：用单独的失败计数（不占推送的重试次数），
// 按退避一直重试，不因次数放弃；连续失败 freeEndAlertAfter 次起向所有启用的通知通道告警一次。
func (m *FreeEndMonitor) markRetry(torrent models.TorrentInfo, errMsg string) {
	now := time.Now()
	failures := torrent.FreeEndFailures + 1
	updates := map[string]any{
		"free_end_failures": failures,
		"last_error":        errMsg,
		"last_check_time":   now,
	}
	if err := m.db.Model(&models.TorrentInfo{}).Where("id = ?", torrent.ID).Updates(updates).Error; err != nil {
		global.GetSlogger().Errorf("更新种子重试状态失败 (种子:%s): %v", torrent.Title, err)
	}
	if failures >= freeEndAlertAfter && torrent.FreeEndAlertedAt == nil {
		m.alertPauseFailing(torrent, failures, errMsg)
	}
	// 重试在 now+退避 触发，不受提前量影响；到时处理函数重读记录
	retry := torrent
	retry.FreeEndFailures = failures
	m.mu.Lock()
	if old, exists := m.pendingTasks[torrent.ID]; exists {
		stopTask(old)
	}
	m.scheduleAtLocked(retry, now.Add(freeEndBackoff(failures)))
	m.mu.Unlock()
	global.GetSlogger().Warnf("[FreeEndMonitor] 种子 %s 到期处理失败（第 %d 次），%v 后重试: %s",
		torrent.Title, failures, freeEndBackoff(failures), errMsg)
}

// alertPauseFailing 发一次「到期暂停一直没成功」的告警，同一个免费期只发一次。
func (m *FreeEndMonitor) alertPauseFailing(t models.TorrentInfo, failures int, errMsg string) {
	m.mu.Lock()
	n := m.notifier
	m.mu.Unlock()
	if !n.Enabled() {
		global.GetSlogger().Warnf("[FreeEndMonitor] 种子 %s 已连续 %d 次没能暂停，没有可用的通知服务，没有发出告警", t.Title, failures)
		return
	}
	end := "（免费已结束或已取消）"
	if t.FreeEndTime != nil {
		end = t.FreeEndTime.Local().Format("2006-01-02 15:04")
	}
	deadline := pauseDeadline(t, m.advanceDuration())
	safeErr := redact.Text(errMsg)
	text := fmt.Sprintf("「%s」（%s）的免费期在 %s 结束，已经连续 %d 次没能在下载器 %s 里暂停：%s。pt-tools 会继续重试；也请到下载器里手动暂停，避免产生计费下载量。",
		t.Title, t.SiteName, end, failures, t.DownloaderName, safeErr)
	subject := strconv.FormatUint(uint64(t.ID), 10)
	// 同一次计划暂停只告警一次（免费结束时间可能已被清空，按处理时刻算）
	key := strconv.FormatInt(deadline.Unix(), 10)
	inserted, err := n.Enqueue(m.ctx, MonitorNotifyEntry{
		Source: freeEndNotifySource, Subject: subject, Kind: "pause_failed", EventKey: key,
		Title: "免费结束暂停失败", Text: text,
		Event: notify.NewEvent(notify.EventFreeEndPauseFailed, time.Now(), map[string]any{
			"site": t.SiteName, "torrent_title": t.Title, "downloader": t.DownloaderName,
			"free_end": end, "failures": failures, "error": safeErr,
		}),
	})
	if err != nil {
		global.GetSlogger().Warnf("[FreeEndMonitor] 写入暂停失败告警失败，下次失败时再试 (种子:%s): %v", t.Title, err)
		return
	}
	if inserted == 0 && !n.Logged(m.ctx, freeEndNotifySource, subject, "pause_failed", key) {
		global.GetSlogger().Warnf("[FreeEndMonitor] 没有启用的通知通道，暂停失败告警没有发出 (种子:%s)", t.Title)
		return
	}
	if err := m.db.Model(&models.TorrentInfo{}).Where("id = ?", t.ID).Update("free_end_alerted_at", time.Now()).Error; err != nil {
		global.GetSlogger().Warnf("[FreeEndMonitor] 记录告警时间失败 (种子:%s): %v", t.Title, err)
	}
}

// reconcilePendingPushes 补上承诺到期暂停的推送加入下载器以后没写成的记账：按哈希在下载器里找回任务，
// 写上任务编号、登记监控，任务保持暂停（不自动开始）。下载器里一直没有的，过了 pendingPushExpire 清掉标记。
func (m *FreeEndMonitor) reconcilePendingPushes() {
	now := time.Now()
	var rows []models.TorrentInfo
	if err := m.db.Where("push_pending_at IS NOT NULL AND julianday(push_pending_at) <= julianday(?)", now.Add(-pendingPushGrace)).
		Order("push_pending_at ASC, id ASC").Limit(progressUpdateBatchSize).Find(&rows).Error; err != nil {
		global.GetSlogger().Errorf("[FreeEndMonitor] 查询待补记账的推送失败: %v", err)
		return
	}
	for _, row := range rows {
		if row.TorrentHash == nil || *row.TorrentHash == "" {
			m.clearPushPending(row)
			continue
		}
		dl, err := m.getDownloader(row)
		if err != nil {
			continue // 下载器暂时不可用：下一轮再对
		}
		info, err := dl.GetTorrent(*row.TorrentHash)
		switch {
		case err == nil:
			m.finishPendingPush(row, info)
		case errors.Is(err, downloader.ErrTorrentNotFound):
			if row.PushPendingAt.Before(now.Add(-pendingPushExpire)) {
				m.clearPushPending(row)
			}
		default:
			global.GetSlogger().Warnf("[FreeEndMonitor] 对账时读取下载器失败，下一轮再试 (种子:%s): %v", row.Title, err)
		}
	}
}

func (m *FreeEndMonitor) finishPendingPush(row models.TorrentInfo, info downloader.Torrent) {
	taskID := info.ID
	if taskID == "" {
		taskID = *row.TorrentHash
	}
	pushed := true
	cols := map[string]any{
		"is_pushed":          &pushed,
		"push_time":          time.Now(),
		"downloader_task_id": taskID,
		"push_pending_at":    nil,
		"last_error":         "加入下载器以后的记账由对账补上：任务保持暂停，需要时请手动开始",
	}
	if row.PauseOnFreeEnd && row.IsFree && row.FreeEndTime != nil {
		cols["pause_planned_at"] = dbTime(row.FreeEndTime.Add(-m.advanceDuration()))
	}
	res := m.db.Model(&models.TorrentInfo{}).Where("id = ? AND push_pending_at IS NOT NULL", row.ID).Updates(cols)
	if res.Error != nil || res.RowsAffected == 0 {
		return
	}
	var updated models.TorrentInfo
	if err := m.db.First(&updated, row.ID).Error; err == nil {
		m.ScheduleTorrent(updated)
	}
	global.GetSlogger().Infof("[FreeEndMonitor] 补上了种子 %s 的加入记账（任务 %s），已登记到期监控", row.Title, taskID)
}

func (m *FreeEndMonitor) clearPushPending(row models.TorrentInfo) {
	if err := m.db.Model(&models.TorrentInfo{}).Where("id = ? AND push_pending_at IS NOT NULL", row.ID).
		Update("push_pending_at", nil).Error; err != nil {
		global.GetSlogger().Warnf("[FreeEndMonitor] 清除推送标记失败 (种子:%s): %v", row.Title, err)
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
			IMDbID:            t.IMDbID,
			DoubanID:          t.DoubanID,
			FreeValidity:      t.FreeValidity,
			FreeVerifiedAt:    t.FreeVerifiedAt,
			FreeStatusSource:  t.FreeStatusSource,
			FreeStatusError:   t.FreeStatusError,
			SourceRSSID:       t.SourceRSSID,
			RequireFree:       t.RequireFree,
			PausePlannedAt:    t.PausePlannedAt,
			PauseRequestedAt:  t.PauseRequestedAt,
			FreeEndFailures:   t.FreeEndFailures,
			FreeEndAlertedAt:  t.FreeEndAlertedAt,
			DetailID:          t.DetailID,
			PushPendingAt:     t.PushPendingAt,
			FreeEndedAt:       t.FreeEndedAt,
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

		completed := isTorrentTrulyCompleted(info)
		if completed {
			updates["is_completed"] = true
			updates["completed_at"] = time.Now()
			global.GetSlogger().Infof("任务已完成下载 (ID:%d, Title:%s, state=%s)", t.ID, t.Title, info.State)
		}

		res := m.progressUpdate(t.ID, completed).Updates(updates)
		if res.Error != nil {
			global.GetSlogger().Errorf("更新任务进度失败 (ID:%d): %v", t.ID, res.Error)
			continue
		}
		if completed && res.RowsAffected > 0 {
			publishCompleted(t, info.InfoHash)
		}
		updated++
	}

	global.GetSlogger().Infof("已推送任务进度更新完成: 更新=%d, 跳过=%d, 总计=%d", updated, skipped, len(torrents))
}
