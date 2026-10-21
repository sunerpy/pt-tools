package organize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
)

// job 是排队等着整理的一个种子。done 不为空时整理完把结果送回去（手动整理在等）。
type job struct {
	req     Request
	dlName  string
	trigger string
	// retryIDs 是到期重试排上队的记录，lease 是 retryDue 给它们写的重试时间（租约）：整理会改写它们；还没整理就出错时按退避改，
	// 整理完了却没改写的清掉；租约没被改写时说明这次重试没做完（例如中途重启），租约到期以后还会再试
	retryIDs []uint
	lease    time.Time
	// rescrape 不为 0 时这是重新刮削这条整理记录所在的电影或剧集（见 Rescrape），不是整理
	rescrape uint
	done     chan jobResult
}

type jobResult struct {
	res    *Result
	scrape *RescrapeResult
	err    error
}

// key 是排队去重用的键（见 jobKey）。
func (j job) key() string {
	if j.rescrape > 0 {
		return fmt.Sprintf("rescrape|%d", j.rescrape)
	}
	return jobKey(j.req.DownloaderID, j.dlName, j.req.queueHash())
}

// backoffState 是补查与扫描时整理不了（还没写记录）的种子的退避：下次什么时候再试。
type backoffState struct {
	fails int
	next  time.Time
}

const (
	jobQueueSize = 256
	// sweepInterval 是补查 pt-tools 推送、已下载完成但还没整理的种子的间隔（完成事件可能丢）
	sweepInterval = 10 * time.Minute
	// tickInterval 是后台循环的节拍：到期的重试、补查、扫描与清理按各自的间隔在节拍上做
	tickInterval = time.Minute
	// maxPerRound 是一轮补查或扫描最多排队几个种子
	maxPerRound = 50
	// waitManual 是手动整理等结果的时长：复制大文件时先回，整理在后台继续
	waitManual = 20 * time.Second
	// retryLease 是到期重试排上队时写进记录的重试时间：整理以前 pt-tools 重启了，过了这个时间还会再试
	// （不和 retryDelays 里的任何一档相同，整理写的重试时间不会恰好等于它）
	retryLease = 45 * time.Minute
	// retryOutOfScope 是到期重试时种子不在自动整理范围里：过这么久再看（范围可能改了）
	retryOutOfScope = 6 * time.Hour
)

// jobKey 是排队去重与退避用的键：知道下载器编号时只用编号（完成事件带了名字，补查与扫描只有编号，要对得上）。
func jobKey(dlID uint, dlName, hash string) string {
	if dlID != 0 {
		dlName = ""
	}
	return fmt.Sprintf("%d|%s|%s", dlID, dlName, strings.ToLower(strings.TrimSpace(hash)))
}

// enqueue 把种子放进队列；已经在队列里的不重复放。队列满时返回 false。
func (s *Service) enqueue(j job) bool {
	return s.put(j) != queueFull
}

const (
	queueAdded = iota
	queueDup
	queueFull
)

// put 把任务放进队列，报告是新放进去了、已经在队列里（这个任务没放），还是队列满了。
func (s *Service) put(j job) int {
	key := j.key()
	s.qmu.Lock()
	if s.queued[key] && j.done == nil {
		s.qmu.Unlock()
		return queueDup
	}
	s.queued[key] = true
	s.qmu.Unlock()
	select {
	case s.jobs <- j:
		return queueAdded
	default:
		s.qmu.Lock()
		delete(s.queued, key)
		s.qmu.Unlock()
		return queueFull
	}
}

// Start 开始后台循环：订阅下载完成事件、处理队列、定时重试、补查、扫描与清理入库链接。重复调用无副作用。
func (s *Service) Start() {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.running {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.running = cancel, true
	_, ch, unsubscribe := events.Subscribe(64)
	s.wg.Go(func() {
		defer unsubscribe()
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-ch:
				if !ok {
					return
				}
				s.onEvent(ctx, e)
			}
		}
	})
	s.wg.Go(func() {
		// 上次没做完的替换（覆盖策略）先恢复，再开始整理
		s.recoverReplacements(ctx)
		s.worker(ctx)
	})
	s.wg.Go(func() { s.loop(ctx) })
}

// Stop 停止后台循环，等正在整理的种子做完。
func (s *Service) Stop() {
	s.runMu.Lock()
	if !s.running {
		s.runMu.Unlock()
		return
	}
	s.running = false
	cancel := s.cancel
	s.runMu.Unlock()
	cancel()
	s.wg.Wait()
}

func (s *Service) isRunning() bool {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	return s.running
}

// onEvent 处理 pt-tools 推送的种子下载完成的事件：打开了自动整理时排队。
func (s *Service) onEvent(ctx context.Context, e events.Event) {
	if e.Type != events.EvtTorrentCompleted {
		return
	}
	var pl events.TorrentCompletedPayload
	if err := json.Unmarshal(e.Payload, &pl); err != nil || (pl.DownloaderID == 0 && pl.DownloaderName == "") {
		return
	}
	set, err := s.Settings(ctx)
	if err != nil || !set.AutoEnabled {
		return
	}
	hash := pl.TaskID
	if hash == "" {
		hash = pl.InfoHash
	}
	if hash == "" {
		return
	}
	if !s.enqueue(job{req: Request{DownloaderID: pl.DownloaderID, Hash: hash}, dlName: pl.DownloaderName, trigger: models.MediaTriggerAuto}) {
		s.cfg.Logger.Warnf("[整理入库] 队列已满，%s 留给定时补查", pl.Title)
	}
}

// worker 一个个整理队列里的种子。
func (s *Service) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-s.jobs:
			if j.rescrape > 0 {
				res, err := s.handleRescrape(ctx, j)
				if j.done != nil {
					j.done <- jobResult{scrape: res, err: err}
				}
				continue
			}
			res, err := s.handle(ctx, j)
			if j.done != nil {
				j.done <- jobResult{res: res, err: err}
			}
		}
	}
}

// handleRescrape 做队列里的一次重新刮削：先从去重表里拿掉。
func (s *Service) handleRescrape(ctx context.Context, j job) (*RescrapeResult, error) {
	s.qmu.Lock()
	delete(s.queued, j.key())
	s.qmu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.rescrape(ctx, j.rescrape)
	if err != nil {
		s.cfg.Logger.Warnf("[整理入库] 重新刮削失败 (记录 %d): %v", j.rescrape, err)
	}
	return res, err
}

// handle 整理从队列里取出的一个任务：先从去重表里拿掉，整理完清掉它带的重试标记。
func (s *Service) handle(ctx context.Context, j job) (*Result, error) {
	s.qmu.Lock()
	delete(s.queued, j.key())
	s.qmu.Unlock()
	defer s.markRetrying(j.retryIDs, false)
	if j.req.WatchDirID > 0 {
		// 整理完（不管结果）就拿掉，下一轮扫描按记录重新看它
		defer s.setWatchBusy(j.req.queueHash(), false)
	}
	return s.runJob(ctx, j)
}

// markRetrying 标上或清掉到期重试排上队的记录。
func (s *Service) markRetrying(ids []uint, on bool) {
	if len(ids) == 0 {
		return
	}
	s.qmu.Lock()
	defer s.qmu.Unlock()
	for _, id := range ids {
		if on {
			s.retrying[id] = true
		} else {
			delete(s.retrying, id)
		}
	}
}

// Retrying 报告一条失败的记录是不是正在重试：到期重试排上队以后、整理完以前，记录里的重试时间是租约到期的时间，
// 只看记录会以为它要等到那时才重试。
func (s *Service) Retrying(id uint) bool {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	return s.retrying[id]
}

// runJob 整理一个种子：自动与扫描触发的要在范围里；出错的进退避，下一轮补查或扫描时再试。
func (s *Service) runJob(ctx context.Context, j job) (*Result, error) {
	if j.req.DownloaderID == 0 && j.dlName != "" {
		_, setting, err := s.cfg.Downloaders.ByName(ctx, j.dlName)
		if err != nil {
			s.fail(ctx, j)
			return nil, err
		}
		j.req.DownloaderID = setting.ID
	}
	// 目录监控发现的（包括失败后的重试）：目录关了就不整理，过一阵再看；删了就不再重试
	if j.trigger == models.MediaTriggerWatch {
		dir, err := s.watchDir(ctx, j.req.WatchDirID)
		if err != nil {
			s.fail(ctx, j)
			return nil, err
		}
		if dir.ID == 0 || !dir.Enabled {
			var next time.Time
			if dir.ID != 0 {
				next = s.cfg.Now().Add(retryOutOfScope)
			}
			s.releaseLease(ctx, j, next)
			return nil, nil
		}
	}
	// 自动与扫描触发的（包括它们失败后的重试）按现在的设置：对应的开关关了、种子不在范围里的不整理
	if j.trigger == models.MediaTriggerAuto || j.trigger == models.MediaTriggerScan {
		st, err := s.automated(ctx, j)
		if err != nil {
			s.fail(ctx, j)
			return nil, err
		}
		switch st {
		case automationOff:
			// 开关在排队以后关了：重试记录回到「到期」，retryDue 在开关关着时不排它们，打开以后接着试
			s.releaseLease(ctx, j, s.cfg.Now())
			return nil, nil
		case outOfScope:
			s.releaseLease(ctx, j, s.cfg.Now().Add(retryOutOfScope))
			return nil, nil
		}
	}
	s.mu.Lock()
	res, err := s.organize(ctx, j.req, j.trigger)
	s.mu.Unlock()
	if err != nil {
		s.fail(ctx, j)
		s.cfg.Logger.Warnf("[整理入库] 整理失败 (下载器 %d, %s): %v", j.req.DownloaderID, firstNonEmpty(j.req.Hash, j.req.Entry), err)
		return nil, err
	}
	// 这次整理没有改写的重试记录（例如它的文件已经不在种子里）不再重试
	s.releaseLease(ctx, j, time.Time{})
	s.clearBackoff(j)
	if res.Created > 0 {
		s.cfg.Logger.Infof("[整理入库] %s：新入库 %d 个文件", res.Plan.Name, res.Created)
	}
	return res, nil
}

// automation 是自动与扫描触发的整理现在该不该做。
type automation int

const (
	inScope       automation = iota
	automationOff            // 自动整理或定期扫描（看触发方式）关着
	outOfScope               // 开着，但种子不在范围里
)

// automated 报告自动与扫描触发的整理现在还该不该做：自动整理或定期扫描（看触发方式）开着，种子在范围里。
func (s *Service) automated(ctx context.Context, j job) (automation, error) {
	set, err := s.Settings(ctx)
	if err != nil {
		return automationOff, err
	}
	if switchedOff(set, j.trigger) {
		return automationOff, nil
	}
	req := j.req
	dl, _, err := s.cfg.Downloaders.Get(ctx, req.DownloaderID)
	if err != nil {
		return automationOff, err
	}
	t, err := dl.GetTorrent(req.Hash)
	if err != nil {
		return automationOff, err
	}
	if !scopeOf(set).match(req.DownloaderID, t) {
		return outOfScope, nil
	}
	return inScope, nil
}

// switchedOffTriggers 是现在对应开关关着的触发方式（记录里没写触发方式的算自动整理）。
func switchedOffTriggers(set Settings) []string {
	var off []string
	if !set.AutoEnabled {
		off = append(off, models.MediaTriggerAuto, "")
	}
	if !set.ScanEnabled {
		off = append(off, models.MediaTriggerScan)
	}
	return off
}

// switchedOff 报告这种触发方式对应的开关（自动整理、定期扫描）关着；手动的没有开关。
func switchedOff(set Settings, trigger string) bool {
	return (trigger == models.MediaTriggerAuto && !set.AutoEnabled) || (trigger == models.MediaTriggerScan && !set.ScanEnabled)
}

// releaseLease 把还停在租约上的重试记录（这次没被整理改写的）改成 next；next 为零值时清空，不再自动重试。
func (s *Service) releaseLease(ctx context.Context, j job, next time.Time) {
	if len(j.retryIDs) == 0 || j.lease.IsZero() {
		return
	}
	var v any
	if !next.IsZero() {
		v = &next
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).
		Where("id IN ? AND status = ? AND next_retry_at = ?", j.retryIDs, models.MediaTransferFailed, j.lease).
		Update("next_retry_at", v).Error; err != nil {
		s.cfg.Logger.Warnf("[整理入库] 更新重试时间失败: %v", err)
	}
}

// fail 记下退避：补查与扫描在退避期内不再排这个种子（手动整理不退避）。到期重试的任务还没整理就出错时，
// 还停在租约上的重试记录改成退避到期的时间（手动的过一个补查间隔）。
func (s *Service) fail(ctx context.Context, j job) {
	next := s.cfg.Now().Add(sweepInterval)
	if j.trigger != models.MediaTriggerManual {
		key := jobKey(j.req.DownloaderID, j.dlName, j.req.queueHash())
		s.qmu.Lock()
		b := s.backoff[key]
		b.fails++
		b.next = s.cfg.Now().Add(min(time.Duration(1<<min(b.fails, 6))*sweepInterval, 12*time.Hour))
		s.backoff[key] = b
		next = b.next
		s.qmu.Unlock()
	}
	s.releaseLease(ctx, j, next)
}

func (s *Service) clearBackoff(j job) {
	s.qmu.Lock()
	delete(s.backoff, jobKey(j.req.DownloaderID, j.dlName, j.req.queueHash()))
	s.qmu.Unlock()
}

func (s *Service) backedOff(dlID uint, hash string) bool {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	b, ok := s.backoff[jobKey(dlID, "", hash)]
	return ok && s.cfg.Now().Before(b.next)
}

// Organize 手动整理一个种子：排进队列，等最多 20 秒；没等到时 Result.Queued 为真，整理在后台继续。
// 后台循环没在跑时（例如测试）直接整理。
func (s *Service) Organize(ctx context.Context, req Request) (*Result, error) {
	if req.WatchDirID == 0 && (req.DownloaderID == 0 || strings.TrimSpace(req.Hash) == "") {
		return nil, fmt.Errorf("%w: 要指定下载器与种子", ErrInvalid)
	}
	if req.WatchDirID > 0 && strings.TrimSpace(req.Entry) == "" {
		return nil, fmt.Errorf("%w: 要指定监控目录里的哪一项", ErrInvalid)
	}
	if !s.isRunning() {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.organize(ctx, req, models.MediaTriggerManual)
	}
	done := make(chan jobResult, 1)
	if !s.enqueue(job{req: req, trigger: models.MediaTriggerManual, done: done}) {
		return nil, errors.New("整理队列已满，稍后再试")
	}
	timer := time.NewTimer(waitManual)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.res, r.err
	case <-timer.C:
		return &Result{Queued: true}, nil
	case <-ctx.Done():
		return &Result{Queued: true}, nil
	}
}

// loop 是后台节拍：每分钟重试到期的失败记录；按间隔补查、扫描与清理入库链接。
func (s *Service) loop(ctx context.Context) {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	// lastRecover 从开始时算起：worker 开始时已经恢复过一次
	var lastSweep, lastScan, lastReconcile, lastWatch time.Time
	lastRecover := s.cfg.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		now := s.cfg.Now()
		set, err := s.Settings(ctx)
		if err != nil {
			s.cfg.Logger.Warnf("[整理入库] %v", err)
			continue
		}
		s.retryDue(ctx, set)
		if set.AutoEnabled && now.Sub(lastSweep) >= sweepInterval {
			lastSweep = now
			s.sweep(ctx, set)
		}
		if set.ScanEnabled && now.Sub(lastScan) >= time.Duration(set.ScanIntervalMin)*time.Minute {
			lastScan = now
			s.markRun(&s.scanAt, now)
			s.scan(ctx, set)
		}
		if set.DeleteLinksOnRemove && now.Sub(lastReconcile) >= sweepInterval {
			lastReconcile = now
			s.Reconcile(ctx)
		}
		if now.Sub(lastWatch) >= time.Duration(set.WatchIntervalMin)*time.Minute {
			lastWatch = now
			s.markRun(&s.watchTick, now)
			s.watchScan(ctx, set)
		}
		if now.Sub(lastRecover) >= sweepInterval {
			lastRecover = now
			s.recoverReplacements(ctx)
		}
	}
}

// retryDue 把到了重试时间的失败记录所在的种子排进队列（沿用原来的触发方式）。对应的开关（自动整理、定期扫描）关着时
// 不排，记录保持到期，打开以后的下一拍就重试。排上的记录的重试时间改成租约到期的时间（不是清空）：整理会改写它，
// 整理以前 pt-tools 重启了，过了租约还会再试。
func (s *Service) retryDue(ctx context.Context, set Settings) {
	q := s.cfg.DB.WithContext(ctx).
		Where("status = ? AND next_retry_at IS NOT NULL AND next_retry_at <= ?", models.MediaTransferFailed, s.cfg.Now())
	// 开关关着的触发方式（没写触发方式的按自动整理）在查询里就去掉：它们保持到期，不占这一轮的名额，
	// 否则一轮取满的都是关着的，后面能做的重试一直轮不到
	if off := switchedOffTriggers(set); len(off) > 0 {
		q = q.Where("`trigger` NOT IN ?", off)
	}
	var rows []models.MediaTransferHistory
	if err := q.Order("next_retry_at").Limit(maxPerRound).Find(&rows).Error; err != nil {
		s.cfg.Logger.Warnf("[整理入库] 读取待重试的记录失败: %v", err)
		return
	}
	seen := map[string]bool{}
	for _, r := range rows {
		key := jobKey(r.DownloaderID, "", r.InfoHash)
		if seen[key] {
			continue
		}
		seen[key] = true
		trigger := r.Trigger
		if trigger == "" {
			trigger = models.MediaTriggerAuto
		}
		if switchedOff(set, trigger) {
			continue
		}
		var pend []models.MediaTransferHistory
		if err := s.cfg.DB.WithContext(ctx).Select("id", "next_retry_at").
			Where("downloader_id = ? AND info_hash = ? AND status = ? AND next_retry_at IS NOT NULL", r.DownloaderID, r.InfoHash, models.MediaTransferFailed).
			Find(&pend).Error; err != nil || len(pend) == 0 {
			continue
		}
		// 读到到期以后，触发这次重试的那条被别的整理改写了（成功了、或者写了新的退避）：不排。
		// 它还是读到时的样子，后面 claimLease 逐条核对的就是到期的状态
		if !unchanged(pend, r) {
			continue
		}
		ids := make([]uint, 0, len(pend))
		for _, p := range pend {
			ids = append(ids, p.ID)
		}
		// 这个进程里已经排上、还没整理完的不重复排（租约比整理慢的大文件复制短时会到期）
		if s.anyRetrying(ids) {
			continue
		}
		lease := s.cfg.Now().Add(retryLease).Truncate(time.Second)
		s.markRetrying(ids, true)
		if claimed, err := s.claimLease(ctx, pend, lease); err != nil || !claimed {
			if err != nil {
				s.cfg.Logger.Warnf("[整理入库] 写重试租约失败: %v", err)
			}
			s.markRetrying(ids, false)
			continue
		}
		req := requestOf(r)
		// 失败是在手动指定条目以后的（记录里有条目），重试沿用这个条目
		if trigger == models.MediaTriggerManual && r.TMDBID > 0 {
			req.MediaType, req.TMDBID, req.LibraryID = r.MediaType, r.TMDBID, r.LibraryID
		}
		j := job{req: req, trigger: trigger, retryIDs: ids, lease: lease}
		// 没排上（队列满了，或者这个种子已经在队列里、那个任务不带这些记录）时，还停在租约上的改成下一拍再看
		if s.put(j) != queueAdded {
			s.releaseLease(ctx, j, s.cfg.Now().Add(tickInterval))
			s.markRetrying(ids, false)
		}
	}
}

// unchanged 报告 rows（这个种子仍在失败、有重试时间的记录）里还有 due 这一条，重试时间也没变。
func unchanged(rows []models.MediaTransferHistory, due models.MediaTransferHistory) bool {
	for _, r := range rows {
		if r.ID == due.ID {
			return r.NextRetryAt != nil && due.NextRetryAt != nil && r.NextRetryAt.Equal(*due.NextRetryAt)
		}
	}
	return false
}

// errLeaseLost 表示读到待重试的记录以后，有记录被别的整理改写了（成功了、或者写了新的退避）
var errLeaseLost = errors.New("记录在领租约以前被改写了")

// claimLease 在一个事务里把读到的这些记录的重试时间换成租约：每条都还是读到时的状态（失败、同一个重试时间）才换，
// 有一条被同时进行的整理改写了就整个不领（返回 false），这一拍不排，下一拍按新的状态再看。
func (s *Service) claimLease(ctx context.Context, rows []models.MediaTransferHistory, lease time.Time) (bool, error) {
	err := s.cfg.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, r := range rows {
			res := tx.Model(&models.MediaTransferHistory{}).
				Where("id = ? AND status = ? AND next_retry_at = ?", r.ID, models.MediaTransferFailed, r.NextRetryAt).
				Update("next_retry_at", &lease)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return errLeaseLost
			}
		}
		return nil
	})
	if errors.Is(err, errLeaseLost) {
		return false, nil
	}
	return err == nil, err
}

// anyRetrying 报告这些记录里有没有已经排上队、还没整理完的。
func (s *Service) anyRetrying(ids []uint) bool {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	for _, id := range ids {
		if s.retrying[id] {
			return true
		}
	}
	return false
}

// sweep 补查：pt-tools 推送的、打开自动整理以后下载完成的、还没有整理记录的种子（完成事件可能丢了）。
func (s *Service) sweep(ctx context.Context, set Settings) {
	if set.AutoSince == nil {
		return
	}
	var infos []models.TorrentInfo
	pushed := true
	if err := s.cfg.DB.WithContext(ctx).
		Where("is_pushed = ? AND is_completed = ? AND completed_at >= ? AND downloader_task_id != '' AND downloader_id IS NOT NULL", &pushed, true, *set.AutoSince).
		Where("NOT EXISTS (SELECT 1 FROM media_transfer_histories h WHERE h.downloader_id = torrent_infos.downloader_id AND h.info_hash = LOWER(COALESCE(torrent_infos.torrent_hash, '')))").
		Order("completed_at").Limit(maxPerRound).Find(&infos).Error; err != nil {
		s.cfg.Logger.Warnf("[整理入库] 补查已完成的种子失败: %v", err)
		return
	}
	for _, ti := range infos {
		if s.backedOff(*ti.DownloaderID, ti.DownloaderTaskID) {
			continue
		}
		s.enqueue(job{req: Request{DownloaderID: *ti.DownloaderID, Hash: ti.DownloaderTaskID}, trigger: models.MediaTriggerAuto})
	}
}

// scan 按范围扫描下载器里已经下载完成、还没有整理记录的种子（不是 pt-tools 推送的也整理）。
func (s *Service) scan(ctx context.Context, set Settings) {
	settings, err := s.cfg.Downloaders.List(ctx)
	if err != nil {
		s.cfg.Logger.Warnf("[整理入库] 读取下载器失败: %v", err)
		return
	}
	sc := scopeOf(set)
	queued := 0
	for _, ds := range settings {
		if len(sc.downloaders) > 0 && !containsID(sc.downloaders, ds.ID) {
			continue
		}
		dl, _, err := s.cfg.Downloaders.Get(ctx, ds.ID)
		if err != nil {
			s.cfg.Logger.Warnf("[整理入库] 扫描下载器「%s」失败: %v", ds.Name, err)
			continue
		}
		all, err := dl.GetAllTorrents()
		if err != nil {
			s.cfg.Logger.Warnf("[整理入库] 读取下载器「%s」的种子失败: %v", ds.Name, err)
			continue
		}
		var hashes []string
		for _, t := range all {
			if t.Progress >= 1 && sc.match(ds.ID, t) {
				hashes = append(hashes, strings.ToLower(t.InfoHash))
			}
		}
		if len(hashes) == 0 {
			continue
		}
		done := map[string]bool{}
		for i := 0; i < len(hashes); i += 500 {
			var have []string
			batch := hashes[i:min(i+500, len(hashes))]
			if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).
				Where("downloader_id = ? AND info_hash IN ?", ds.ID, batch).Distinct().Pluck("info_hash", &have).Error; err != nil {
				s.cfg.Logger.Warnf("[整理入库] 读取整理记录失败: %v", err)
				return
			}
			for _, h := range have {
				done[h] = true
			}
		}
		for _, t := range all {
			h := strings.ToLower(t.InfoHash)
			if t.Progress < 1 || done[h] || !sc.match(ds.ID, t) || s.backedOff(ds.ID, t.ID) {
				continue
			}
			if queued >= maxPerRound {
				return
			}
			if s.enqueue(job{req: Request{DownloaderID: ds.ID, Hash: t.ID}, trigger: models.MediaTriggerScan}) {
				queued++
			}
		}
	}
}

func containsID(ids []uint, id uint) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func (s *Service) markRun(at *time.Time, now time.Time) {
	s.timesMu.Lock()
	*at = now
	s.timesMu.Unlock()
}

// RunTimes 是定期扫描与目录监控扫描上一次与下一次大约什么时候（在内存里，pt-tools 重启以后从头算）。
// 后台没在跑、或者对应的开关关着时没有「下一次」；这次启动以后还没扫过时下一次是现在（下一拍就扫）。
type RunTimes struct {
	ScanLast  *time.Time `json:"scan_last,omitempty"`
	ScanNext  *time.Time `json:"scan_next,omitempty"`
	WatchLast *time.Time `json:"watch_last,omitempty"`
	WatchNext *time.Time `json:"watch_next,omitempty"`
}

// Runs 是按设置 set 算的上一次与下一次扫描的时间（见 RunTimes）。目录监控没有开着的目录时没有下一次（扫描只看开着的）。
func (s *Service) Runs(ctx context.Context, set Settings) RunTimes {
	var watching int64
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaWatchDir{}).Where("enabled = ?", true).Count(&watching).Error; err != nil {
		watching = 0
	}
	s.timesMu.Lock()
	scanAt, watchTick, watchAt := s.scanAt, s.watchTick, s.watchAt
	s.timesMu.Unlock()
	running, now := s.isRunning(), s.cfg.Now()
	// next 按后台节拍上一次做这件事的时间 tick 排（与 loop 一致）；last 是页面上的「上次」
	at := func(last, tick time.Time, on bool, intervalMin int) (*time.Time, *time.Time) {
		var l, n *time.Time
		if !last.IsZero() {
			l = &last
		}
		if running && on {
			next := now
			if !tick.IsZero() {
				if t := tick.Add(time.Duration(intervalMin) * time.Minute); t.After(now) {
					next = t
				}
			}
			n = &next
		}
		return l, n
	}
	var rt RunTimes
	rt.ScanLast, rt.ScanNext = at(scanAt, scanAt, set.ScanEnabled, set.ScanIntervalMin)
	rt.WatchLast, rt.WatchNext = at(watchAt, watchTick, watching > 0, set.WatchIntervalMin)
	return rt
}
