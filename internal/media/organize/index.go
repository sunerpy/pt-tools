package organize

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/serverindex"
	"github.com/sunerpy/pt-tools/models"
)

// 媒体服务器索引的后台节拍（见 indexLoop）。
const (
	// indexTick 是多久看一次哪台服务器该同步了
	indexTick = 30 * time.Second
	// indexFirstDelay 是启动以后多久做第一次同步（从没同步过的服务器）：不和启动时的别的事挤在一起
	indexFirstDelay = time.Minute
	// indexRetry 是同步失败以后多久再试（同步间隔更短时按间隔）
	indexRetry = 30 * time.Minute
	// webhookSyncDelay 是 Webhook 报了新入库或删除以后多久同步（期间再来的合进这一次）
	webhookSyncDelay = 2 * time.Minute
)

// indexState 是索引同步的排期：pending 是排上的同步（Webhook、保存服务器以后），failed 是上一次失败的时间；
// run 让同步一次只做一台（后台节拍与手动同步共用）。
type indexState struct {
	mu        sync.Mutex
	pending   map[uint]time.Time
	failed    map[uint]time.Time
	startedAt time.Time
	run       sync.Mutex
	wake      chan struct{}
}

func newIndexState() indexState {
	return indexState{pending: map[uint]time.Time{}, failed: map[uint]time.Time{}, wake: make(chan struct{}, 1)}
}

// schedule 排一次同步：at 时刻以后的那一拍做；已经排了更早的就不改（合进那一次）。
func (st *indexState) schedule(id uint, at time.Time) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if old, ok := st.pending[id]; !ok || at.Before(old) {
		st.pending[id] = at
	}
}

// due 报告这台服务器现在该不该同步：排上的到了时间；失败过的过了重试时间；从没同步过的在启动 1 分钟以后；
// 同步过的过了同步间隔。
func (st *indexState) due(srv models.MediaServer, now time.Time) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if at, ok := st.pending[srv.ID]; ok && !now.Before(at) {
		return true
	}
	interval := time.Duration(indexHours(srv)) * time.Hour
	if failed, ok := st.failed[srv.ID]; ok {
		return now.Sub(failed) >= min(indexRetry, interval)
	}
	if srv.IndexedAt == nil {
		return now.Sub(st.startedAt) >= indexFirstDelay
	}
	return now.Sub(*srv.IndexedAt) >= interval
}

// done 记下一次同步的结果（started 时开始、at 时做完）：开始时已经到了时间的那一次做掉了，同步的时候新排上的
// （比如这时来了 Webhook）留着；失败时记下时间。
func (st *indexState) done(id uint, started, at time.Time, err error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if p, ok := st.pending[id]; ok && !p.After(started) {
		delete(st.pending, id)
	}
	if err != nil {
		st.failed[id] = at
	} else {
		delete(st.failed, id)
	}
}

// poke 叫醒后台节拍（已经有一次没处理时不再排）。
func (st *indexState) poke() {
	select {
	case st.wake <- struct{}{}:
	default:
	}
}

// indexLoop 是索引同步的后台节拍：每 30 秒看一次（排上马上要做的会提前叫醒），一台台同步到期的服务器。
func (s *Service) indexLoop(ctx context.Context) {
	s.idx.mu.Lock()
	s.idx.startedAt = s.cfg.Now()
	s.idx.mu.Unlock()
	t := time.NewTicker(indexTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.idx.wake:
		}
		s.indexDue(ctx)
	}
}

// indexDue 同步到期的服务器（启用的、索引开着的）。
func (s *Service) indexDue(ctx context.Context) {
	var servers []models.MediaServer
	if err := s.cfg.DB.WithContext(ctx).Where("enabled = ? AND index_off = ?", true, false).Order("id").Find(&servers).Error; err != nil {
		s.cfg.Logger.Warnf("[媒体服务器] 读取服务器失败: %v", err)
		return
	}
	for _, srv := range servers {
		if ctx.Err() != nil {
			return
		}
		if !s.idx.due(srv, s.cfg.Now()) {
			continue
		}
		if _, err := s.syncIndex(ctx, srv.ID); err != nil && !errors.Is(err, context.Canceled) {
			s.cfg.Logger.Warnf("[媒体服务器] 同步「%s」的索引失败: %v", srv.Name, err)
		}
	}
}

// SyncServer 马上同步一台服务器的索引（页面上的「同步」），等同步做完。服务器没启用或关了索引时回 ErrInvalid。
func (s *Service) SyncServer(ctx context.Context, id uint) (serverindex.Result, error) {
	var srv models.MediaServer
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&srv).Error; err != nil {
		return serverindex.Result{}, fmt.Errorf("读取媒体服务器失败: %w", err)
	}
	if srv.ID == 0 {
		return serverindex.Result{}, ErrNotFound
	}
	if !srv.Enabled || srv.IndexOff {
		return serverindex.Result{}, fmt.Errorf("%w: 这台服务器没启用，或关掉了索引", ErrInvalid)
	}
	return s.syncIndex(ctx, id)
}

// syncIndex 同步一台服务器（一次只同步一台）。拿到锁以后重读服务器设置：等锁的时候可能改过。
func (s *Service) syncIndex(ctx context.Context, id uint) (serverindex.Result, error) {
	s.idx.run.Lock()
	defer s.idx.run.Unlock()
	started := s.cfg.Now()
	var srv models.MediaServer
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&srv).Error; err != nil {
		return serverindex.Result{}, fmt.Errorf("读取媒体服务器失败: %w", err)
	}
	if srv.ID == 0 {
		s.idx.done(id, started, s.cfg.Now(), nil)
		return serverindex.Result{}, ErrNotFound
	}
	c, err := s.serverClient(srv)
	if err != nil {
		s.idx.done(id, started, s.cfg.Now(), err)
		return serverindex.Result{}, err
	}
	res, err := serverindex.Sync(ctx, s.cfg.DB, srv, c, started)
	if errors.Is(err, context.Canceled) {
		return res, err
	}
	s.idx.done(id, started, s.cfg.Now(), err)
	if err == nil {
		s.cfg.Logger.Infof("[媒体服务器] 同步了「%s」的索引：电影 %d、剧 %d、集 %d（新增 %d、改了 %d、删了 %d、又有了 %d，新问题 %d）",
			srv.Name, res.Movies, res.Series, res.Episodes, res.Added, res.Updated, res.Removed, res.Restored, res.Issues)
	}
	return res, err
}

// scheduleIndex 排一次索引同步（delay 以后做）：到时间叫醒后台节拍，不等下一个 30 秒；delay 不大于 0 时马上叫醒。
func (s *Service) scheduleIndex(id uint, delay time.Duration) {
	s.idx.schedule(id, s.cfg.Now().Add(delay))
	if delay <= 0 {
		s.idx.poke()
		return
	}
	time.AfterFunc(delay, s.idx.poke)
}
