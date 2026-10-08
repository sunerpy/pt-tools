package organize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
)

// job 是排队等着整理的一个种子。done 不为空时整理完把结果送回去（手动整理在等）。
type job struct {
	req     Request
	dlName  string
	trigger string
	// retryIDs 是到期重试时 retryDue 清掉了重试时间的记录：还没整理就出错时（例如下载器连不上）按退避恢复
	retryIDs []uint
	done     chan jobResult
}

type jobResult struct {
	res *Result
	err error
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
	key := jobKey(j.req.DownloaderID, j.dlName, j.req.Hash)
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
	s.wg.Go(func() { s.worker(ctx) })
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
			res, err := s.handle(ctx, j)
			if j.done != nil {
				j.done <- jobResult{res: res, err: err}
			}
		}
	}
}

// handle 整理从队列里取出的一个任务：先从去重表里拿掉，整理完清掉它带的重试标记。
func (s *Service) handle(ctx context.Context, j job) (*Result, error) {
	s.qmu.Lock()
	delete(s.queued, jobKey(j.req.DownloaderID, j.dlName, j.req.Hash))
	s.qmu.Unlock()
	defer s.markRetrying(j.retryIDs, false)
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

// Retrying 报告一条失败的记录是不是正在重试：到期重试排上队以后、整理完以前，记录里的重试时间是空的，
// 只看记录会以为它不再重试了。
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
	// 自动与扫描触发的（包括它们失败后的重试）按现在的设置：对应的开关关了、种子不在范围里的不整理
	if j.trigger == models.MediaTriggerAuto || j.trigger == models.MediaTriggerScan {
		ok, err := s.automated(ctx, j)
		if err != nil {
			s.fail(ctx, j)
			return nil, err
		}
		if !ok {
			return nil, nil
		}
	}
	s.mu.Lock()
	res, err := s.organize(ctx, j.req, j.trigger)
	s.mu.Unlock()
	if err != nil {
		s.fail(ctx, j)
		s.cfg.Logger.Warnf("[整理入库] 整理失败 (下载器 %d, %s): %v", j.req.DownloaderID, j.req.Hash, err)
		return nil, err
	}
	s.clearBackoff(j)
	if res.Created > 0 {
		s.cfg.Logger.Infof("[整理入库] %s：新入库 %d 个文件", res.Plan.Name, res.Created)
	}
	return res, nil
}

// automated 报告自动与扫描触发的整理现在还该不该做：自动整理或定期扫描（看触发方式）开着，种子在范围里。
func (s *Service) automated(ctx context.Context, j job) (bool, error) {
	set, err := s.Settings(ctx)
	if err != nil {
		return false, err
	}
	if (j.trigger == models.MediaTriggerAuto && !set.AutoEnabled) || (j.trigger == models.MediaTriggerScan && !set.ScanEnabled) {
		return false, nil
	}
	req := j.req
	dl, _, err := s.cfg.Downloaders.Get(ctx, req.DownloaderID)
	if err != nil {
		return false, err
	}
	t, err := dl.GetTorrent(req.Hash)
	if err != nil {
		return false, err
	}
	return scopeOf(set).match(req.DownloaderID, t), nil
}

// fail 记下退避：补查与扫描在退避期内不再排这个种子（手动整理不退避）。到期重试的任务还没整理就出错时，
// 把 retryDue 清掉的重试时间恢复成退避到期的时间（手动的过一个补查间隔），不然这些记录再也不会自动重试。
func (s *Service) fail(ctx context.Context, j job) {
	next := s.cfg.Now().Add(sweepInterval)
	if j.trigger != models.MediaTriggerManual {
		key := jobKey(j.req.DownloaderID, j.dlName, j.req.Hash)
		s.qmu.Lock()
		b := s.backoff[key]
		b.fails++
		b.next = s.cfg.Now().Add(min(time.Duration(1<<min(b.fails, 6))*sweepInterval, 12*time.Hour))
		s.backoff[key] = b
		next = b.next
		s.qmu.Unlock()
	}
	if len(j.retryIDs) == 0 {
		return
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).
		Where("id IN ? AND status = ? AND next_retry_at IS NULL", j.retryIDs, models.MediaTransferFailed).
		Update("next_retry_at", &next).Error; err != nil {
		s.cfg.Logger.Warnf("[整理入库] 恢复重试时间失败: %v", err)
	}
}

func (s *Service) clearBackoff(j job) {
	s.qmu.Lock()
	delete(s.backoff, jobKey(j.req.DownloaderID, j.dlName, j.req.Hash))
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
	if req.DownloaderID == 0 || strings.TrimSpace(req.Hash) == "" {
		return nil, fmt.Errorf("%w: 要指定下载器与种子", ErrInvalid)
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
	var lastSweep, lastScan, lastReconcile time.Time
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
		s.retryDue(ctx)
		if set.AutoEnabled && now.Sub(lastSweep) >= sweepInterval {
			lastSweep = now
			s.sweep(ctx, set)
		}
		if set.ScanEnabled && now.Sub(lastScan) >= time.Duration(set.ScanIntervalMin)*time.Minute {
			lastScan = now
			s.scan(ctx, set)
		}
		if set.DeleteLinksOnRemove && now.Sub(lastReconcile) >= sweepInterval {
			lastReconcile = now
			s.Reconcile(ctx)
		}
	}
}

// retryDue 把到了重试时间的失败记录所在的种子排进队列（沿用原来的触发方式）。
func (s *Service) retryDue(ctx context.Context) {
	var rows []models.MediaTransferHistory
	if err := s.cfg.DB.WithContext(ctx).
		Where("status = ? AND next_retry_at IS NOT NULL AND next_retry_at <= ?", models.MediaTransferFailed, s.cfg.Now()).
		Order("next_retry_at").Limit(maxPerRound).Find(&rows).Error; err != nil {
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
		// 先把重试时间清掉再排队：排上以后失败时 record 会按次数重新算；队列满了排不上时恢复成下一拍再试
		pending := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).
			Where("downloader_id = ? AND info_hash = ? AND status = ? AND next_retry_at IS NOT NULL", r.DownloaderID, r.InfoHash, models.MediaTransferFailed)
		var ids []uint
		if err := pending.Pluck("id", &ids).Error; err != nil || len(ids) == 0 {
			continue
		}
		// 清掉重试时间以前先标上：清掉到整理完之间，别人（订阅）看记录也知道它还在重试
		s.markRetrying(ids, true)
		if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).Where("id IN ?", ids).Update("next_retry_at", nil).Error; err != nil {
			s.cfg.Logger.Warnf("[整理入库] 清除重试时间失败: %v", err)
			s.markRetrying(ids, false)
			continue
		}
		trigger := r.Trigger
		if trigger == "" {
			trigger = models.MediaTriggerAuto
		}
		req := Request{DownloaderID: r.DownloaderID, Hash: firstNonEmpty(r.TaskID, r.InfoHash)}
		// 失败是在手动指定条目以后的（记录里有条目），重试沿用这个条目
		if trigger == models.MediaTriggerManual && r.TMDBID > 0 {
			req.MediaType, req.TMDBID, req.LibraryID = r.MediaType, r.TMDBID, r.LibraryID
		}
		// 没排上（队列满了，或者这个种子已经在队列里、那个任务不带这些记录）时，恢复成下一拍再看；
		// 只恢复还空着的：那个任务已经写了记录的不覆盖
		if s.put(job{req: req, trigger: trigger, retryIDs: ids}) != queueAdded {
			next := s.cfg.Now().Add(tickInterval)
			s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).
				Where("id IN ? AND status = ? AND next_retry_at IS NULL", ids, models.MediaTransferFailed).Update("next_retry_at", &next)
			s.markRetrying(ids, false)
		}
	}
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
