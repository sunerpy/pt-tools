package organize

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

// 实时目录监控（整理设置里的「实时监控」，默认开）：监控目录里有改动时，把改动的那一项的扫描提前到「最后一次改动 + 稳定期」
// 以后，不用等下一次按间隔扫描。扫描照旧走 watchQueue（排队、退避、稳定期、整理记录与文件编号的规则都不变），按间隔扫描
// 照旧兜底。加不了监听的目录（inotify 的上限、网络文件系统、目录太多）退回只按间隔扫描，监控目录列表里写明原因。

// 实时监控的状态（WatchDirView.Realtime）。
const (
	// RealtimeOn 是在监听
	RealtimeOn = "on"
	// RealtimeOff 是设置里关了实时监控、监控目录关着，或者后台没在跑
	RealtimeOff = "off"
	// RealtimeUnavailable 是加不了监听，只按间隔扫描（WatchDirView.RealtimeNote 写明原因）
	RealtimeUnavailable = "unavailable"
)

const (
	// realtimeMargin 是稳定期以后再等的一会儿：文件的修改时间与收到事件的时间差一点
	realtimeMargin = 5 * time.Second
	// realtimeMaxDirs 是一个监控目录里最多给几个目录加监听
	realtimeMaxDirs = 4096
	// realtimeSyncWait 是改了设置、监控目录以后等实时监控重新核对的时长（页面马上要读状态）
	realtimeSyncWait = 3 * time.Second
	// rescanAll 是排着的「整个目录重新扫一遍」（事件太多、丢了一些时）：项的名字不会是空串
	rescanAll = ""
)

var (
	// realtimeTick 是看有没有到期的项的间隔（测试里换成短的）
	realtimeTick = 5 * time.Second
	// realtimeResync 是重新核对监控目录（开关、增删，加不了监听的再试一次）的间隔
	realtimeResync = time.Minute
	// newFSWatcher 建监听（测试里换成假的）
	newFSWatcher = newNotifyWatcher
	// isNetworkFS 报告目录在不在网络文件系统上（见 realtime_linux.go）
	isNetworkFS = networkFS
)

// errTooManyDirs 表示监控目录里的目录太多，不全加监听。
var errTooManyDirs = fmt.Errorf("目录太多（超过 %d 个）", realtimeMaxDirs)

// fsWatcher 是用到的 fsnotify.Watcher 的部分。
type fsWatcher interface {
	Add(name string) error
	Remove(name string) error
	Events() <-chan fsnotify.Event
	Errors() <-chan error
	Close() error
}

type notifyWatcher struct{ w *fsnotify.Watcher }

func (n notifyWatcher) Add(name string) error         { return n.w.Add(name) }
func (n notifyWatcher) Remove(name string) error      { return n.w.Remove(name) }
func (n notifyWatcher) Events() <-chan fsnotify.Event { return n.w.Events }
func (n notifyWatcher) Errors() <-chan error          { return n.w.Errors }
func (n notifyWatcher) Close() error                  { return n.w.Close() }

func newNotifyWatcher() (fsWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return notifyWatcher{w}, nil
}

// rtDir 是加了监听的一个监控目录。
type rtDir struct {
	path    string
	watched map[string]bool
}

// rtState 是一个监控目录的实时监控状态与原因。
type rtState struct {
	state, note string
}

// realtime 是实时监控的状态：监听、每个监控目录的状态、排着的扫描（监控目录 → 那一项 → 什么时候扫）。
// 监听只在 realtimeLoop 里建、换与关；别处改设置后经 sync 请它重新核对。
type realtime struct {
	mu      sync.Mutex
	w       fsWatcher
	dirs    map[uint]*rtDir
	states  map[uint]rtState
	pending map[uint]map[string]time.Time
	sync    chan chan struct{}
}

// WatchDirView 是监控目录与它的实时监控状态。
type WatchDirView struct {
	models.MediaWatchDir
	Realtime     string `json:"realtime"`
	RealtimeNote string `json:"realtime_note,omitempty"`
}

// WatchDirViews 列出监控的目录与各自的实时监控状态。
func (s *Service) WatchDirViews(ctx context.Context) ([]WatchDirView, error) {
	dirs, err := s.WatchDirs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]WatchDirView, 0, len(dirs))
	for _, d := range dirs {
		v := WatchDirView{MediaWatchDir: d, Realtime: RealtimeOff}
		if d.Enabled && s.isRunning() {
			s.rt.mu.Lock()
			if st, ok := s.rt.states[d.ID]; ok {
				v.Realtime, v.RealtimeNote = st.state, st.note
			}
			s.rt.mu.Unlock()
		}
		out = append(out, v)
	}
	return out, nil
}

// resyncRealtime 请实时监控按现在的设置与监控目录重新核对，等它做完（最多 realtimeSyncWait）。后台没在跑时直接返回。
func (s *Service) resyncRealtime(ctx context.Context) {
	if !s.isRunning() {
		return
	}
	done := make(chan struct{})
	select {
	case s.rt.sync <- done:
	default:
		return // 已经有一次排着了
	}
	t := time.NewTimer(realtimeSyncWait)
	defer t.Stop()
	select {
	case <-done:
	case <-t.C:
	case <-ctx.Done():
	}
}

// realtimeLoop 是实时监控的后台循环：核对监控目录、收事件、到期时扫描。退出时关掉监听。
func (s *Service) realtimeLoop(ctx context.Context) {
	tick := time.NewTicker(realtimeTick)
	defer tick.Stop()
	resync := time.NewTicker(realtimeResync)
	defer resync.Stop()
	defer s.rtStop()
	s.rtSync(ctx)
	for {
		events, errs := s.rtChans()
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				s.rtLost("监听意外停止")
				continue
			}
			s.rtEvent(ev)
		case err, ok := <-errs:
			if !ok {
				s.rtLost("监听意外停止")
				continue
			}
			s.rtError(err)
		case <-tick.C:
			s.rtFire(ctx)
		case <-resync.C:
			s.rtSync(ctx)
		case done := <-s.rt.sync:
			s.rtSync(ctx)
			close(done)
		}
	}
}

func (s *Service) rtChans() (<-chan fsnotify.Event, <-chan error) {
	s.rt.mu.Lock()
	defer s.rt.mu.Unlock()
	if s.rt.w == nil {
		return nil, nil
	}
	return s.rt.w.Events(), s.rt.w.Errors()
}

// rtStop 关掉监听，状态都清掉（后台停了，列表里是 off）。
func (s *Service) rtStop() {
	s.rt.mu.Lock()
	defer s.rt.mu.Unlock()
	s.rtCloseLocked()
	s.rt.states = nil
}

func (s *Service) rtCloseLocked() {
	if s.rt.w != nil {
		_ = s.rt.w.Close()
	}
	s.rt.w, s.rt.dirs, s.rt.pending = nil, nil, nil
}

// rtLost 是监听意外停了：关掉，开着的目录都标成不可用，下一次核对时重建。
func (s *Service) rtLost(why string) {
	s.rt.mu.Lock()
	defer s.rt.mu.Unlock()
	ids := make([]uint, 0, len(s.rt.dirs))
	for id := range s.rt.dirs {
		ids = append(ids, id)
	}
	s.rtCloseLocked()
	for _, id := range ids {
		s.rtSetLocked(id, RealtimeUnavailable, why+"，稍后重试")
	}
}

// rtSetLocked 记下一个监控目录的状态；变了时写日志。
func (s *Service) rtSetLocked(id uint, state, note string) {
	if s.rt.states == nil {
		s.rt.states = map[uint]rtState{}
	}
	if old, ok := s.rt.states[id]; ok && old.state == state && old.note == note {
		return
	}
	s.rt.states[id] = rtState{state: state, note: note}
	if state == RealtimeUnavailable {
		s.cfg.Logger.Warnf("[整理入库] 监控目录 %d 的实时监控不可用，按间隔扫描：%s", id, note)
	}
}

// rtSync 按现在的设置与开着的监控目录核对监听：关了实时监控时全部关掉；新开的目录加监听，关掉、删掉的拿掉；
// 加不了监听的目录每次核对再试一次。
func (s *Service) rtSync(ctx context.Context) {
	set, err := s.Settings(ctx)
	if err != nil {
		s.cfg.Logger.Warnf("[整理入库] 实时监控读取设置失败: %v", err)
		return
	}
	var dirs []models.MediaWatchDir
	if err := s.cfg.DB.WithContext(ctx).Where("enabled = ?", true).Order("id").Find(&dirs).Error; err != nil {
		s.cfg.Logger.Warnf("[整理入库] 实时监控读取监控目录失败: %v", err)
		return
	}
	s.rt.mu.Lock()
	defer s.rt.mu.Unlock()
	if !set.WatchRealtime || len(dirs) == 0 {
		s.rtCloseLocked()
		s.rt.states = nil
		return
	}
	if !realtimeSupported() {
		s.rt.states = nil
		for _, d := range dirs {
			s.rtSetLocked(d.ID, RealtimeUnavailable, realtimeReason(errRealtimeUnsupported))
		}
		return
	}
	if s.rt.w == nil {
		w, err := newFSWatcher()
		if err != nil {
			s.rt.states = nil
			for _, d := range dirs {
				s.rtSetLocked(d.ID, RealtimeUnavailable, realtimeReason(err))
			}
			return
		}
		s.rt.w, s.rt.dirs = w, map[uint]*rtDir{}
	}
	keep := map[uint]bool{}
	for _, d := range dirs {
		keep[d.ID] = true
		if cur := s.rt.dirs[d.ID]; cur != nil {
			if samePath(cur.path, d.Path) {
				continue
			}
			s.rtDropLocked(d.ID)
		}
		s.rtAddDirLocked(d)
	}
	for id := range s.rt.dirs {
		if !keep[id] {
			s.rtDropLocked(id)
		}
	}
	for id := range s.rt.states {
		if !keep[id] {
			delete(s.rt.states, id)
		}
	}
	for id := range s.rt.pending {
		if !keep[id] {
			delete(s.rt.pending, id)
		}
	}
}

// rtDropLocked 拿掉一个监控目录的监听。
func (s *Service) rtDropLocked(id uint) {
	if d := s.rt.dirs[id]; d != nil && s.rt.w != nil {
		for p := range d.watched {
			_ = s.rt.w.Remove(p)
		}
	}
	delete(s.rt.dirs, id)
}

// rtAddDirLocked 给一个监控目录加监听：网络文件系统上不加；整棵树加不全时全部拿掉，标成不可用。
func (s *Service) rtAddDirLocked(d models.MediaWatchDir) {
	if kind, ok := isNetworkFS(d.Path); ok {
		s.rtSetLocked(d.ID, RealtimeUnavailable, fmt.Sprintf("在网络文件系统（%s）上，收不到别的机器写进来的通知", kind))
		return
	}
	dir := &rtDir{path: d.Path, watched: map[string]bool{}}
	if err := s.rtWatchTreeLocked(dir, d.Path); err != nil {
		for p := range dir.watched {
			_ = s.rt.w.Remove(p)
		}
		s.rtSetLocked(d.ID, RealtimeUnavailable, realtimeReason(err))
		return
	}
	s.rt.dirs[d.ID] = dir
	s.rtSetLocked(d.ID, RealtimeOn, "")
}

// rtWatchTreeLocked 给 start 与它下面的目录加监听：和 watchFiles 一样跳过隐藏与系统目录，太深的不加（watchFiles
// 认为那一项太大，本来就整理不了）；一个监控目录最多 realtimeMaxDirs 个。
func (s *Service) rtWatchTreeLocked(dir *rtDir, start string) error {
	return filepath.WalkDir(start, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			if p != start && errors.Is(err, os.ErrNotExist) {
				return nil // 刚删掉了
			}
			return err
		}
		if !e.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(dir.path, p)
		if rerr != nil {
			return rerr
		}
		if rel != "." && (skipWatchEntry(e.Name()) || strings.Count(filepath.ToSlash(rel), "/") >= watchMaxDepth) {
			return filepath.SkipDir
		}
		if dir.watched[p] {
			return nil
		}
		if len(dir.watched) >= realtimeMaxDirs {
			return errTooManyDirs
		}
		if err := s.rt.w.Add(p); err != nil {
			if p != start && errors.Is(err, os.ErrNotExist) {
				return filepath.SkipDir
			}
			return err
		}
		dir.watched[p] = true
		return nil
	})
}

// realtimeReason 是加不了监听的原因（给人看的）。
func realtimeReason(err error) string {
	switch {
	case errors.Is(err, errTooManyDirs):
		return err.Error()
	case watchLimit(err):
		return "inotify 的监听数到了上限（宿主机的 fs.inotify.max_user_watches）"
	case instanceLimit(err):
		return "inotify 的实例数到了上限（宿主机的 fs.inotify.max_user_instances）"
	case errors.Is(err, errRealtimeUnsupported):
		return err.Error()
	}
	return "加不了监听：" + err.Error()
}

// rtOwnerLocked 找事件路径所在的监控目录，回监控目录的编号、监听与相对路径。
func (s *Service) rtOwnerLocked(name string) (uint, *rtDir, string) {
	for id, d := range s.rt.dirs {
		if samePath(d.path, name) || transfer.Within(d.path, name) {
			rel, err := filepath.Rel(d.path, name)
			if err != nil {
				return 0, nil, ""
			}
			return id, d, rel
		}
	}
	return 0, nil, ""
}

// rtEvent 处理一个事件：排上改动的那一项（第一级）的扫描，新建的目录补监听。只改了属性（例如硬链接数）的不算。
func (s *Service) rtEvent(ev fsnotify.Event) {
	if ev.Op == fsnotify.Chmod {
		return
	}
	s.rt.mu.Lock()
	defer s.rt.mu.Unlock()
	id, dir, rel := s.rtOwnerLocked(ev.Name)
	if dir == nil || rel == "." {
		return
	}
	entry, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
	if skipWatchEntry(entry) {
		return
	}
	switch {
	case ev.Has(fsnotify.Create):
		if info, err := os.Lstat(ev.Name); err == nil && info.IsDir() {
			if err := s.rtWatchTreeLocked(dir, ev.Name); err != nil {
				// 加不全了：这个目录退回只按间隔扫描
				s.rtDropLocked(id)
				s.rtSetLocked(id, RealtimeUnavailable, realtimeReason(err))
			}
		}
	case ev.Has(fsnotify.Remove), ev.Has(fsnotify.Rename):
		delete(dir.watched, ev.Name)
	}
	s.rtScheduleLocked(id, entry, s.cfg.Now().Add(watchSettle+realtimeMargin))
}

// rtError 处理监听的错误：事件太多、丢了一些时，各个目录整个重新扫一遍。
func (s *Service) rtError(err error) {
	if !errors.Is(err, fsnotify.ErrEventOverflow) {
		s.cfg.Logger.Warnf("[整理入库] 实时监控出错: %v", err)
		return
	}
	s.rt.mu.Lock()
	defer s.rt.mu.Unlock()
	due := s.cfg.Now().Add(watchSettle + realtimeMargin)
	for id := range s.rt.dirs {
		s.rtScheduleLocked(id, rescanAll, due)
	}
}

func (s *Service) rtScheduleLocked(id uint, entry string, at time.Time) {
	if s.rt.pending == nil {
		s.rt.pending = map[uint]map[string]time.Time{}
	}
	m := s.rt.pending[id]
	if m == nil {
		m = map[string]time.Time{}
		s.rt.pending[id] = m
	}
	m[entry] = at
}

// rtSchedule 排一项的扫描（还在改动、要再等一个稳定期时）。
func (s *Service) rtSchedule(id uint, entry string, at time.Time) {
	s.rt.mu.Lock()
	defer s.rt.mu.Unlock()
	if s.rt.dirs[id] == nil {
		return
	}
	s.rtScheduleLocked(id, entry, at)
}

// rtFire 扫描到期的项：还在改动的按它最新的修改时间再排。
func (s *Service) rtFire(ctx context.Context) {
	now := s.cfg.Now()
	due := map[uint][]string{}
	s.rt.mu.Lock()
	for id, m := range s.rt.pending {
		for name, at := range m {
			if !at.After(now) {
				due[id] = append(due[id], name)
				delete(m, name)
			}
		}
		if len(m) == 0 {
			delete(s.rt.pending, id)
		}
	}
	s.rt.mu.Unlock()
	if len(due) == 0 {
		return
	}
	set, err := s.Settings(ctx)
	if err != nil {
		s.cfg.Logger.Warnf("[整理入库] %v", err)
		return
	}
	ids := make([]uint, 0, len(due))
	for id := range due {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		names := due[id]
		sort.Strings(names)
		s.watchScanEntries(ctx, set, id, names, func(name string, newest time.Time) {
			s.rtSchedule(id, name, newest.Add(watchSettle+realtimeMargin))
		})
	}
}
