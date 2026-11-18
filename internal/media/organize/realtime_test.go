package organize

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// fakeWatcher 是假的监听：记下加了哪些目录，测试往 events 里放事件。addErr 不为空时加到它认的目录会失败。
type fakeWatcher struct {
	mu      sync.Mutex
	added   map[string]bool
	adds    map[string]int
	removed []string
	events  chan fsnotify.Event
	errs    chan error
	closed  bool
	addErr  func(string) error
}

func newFakeWatcher() *fakeWatcher {
	return &fakeWatcher{added: map[string]bool{}, adds: map[string]int{}, events: make(chan fsnotify.Event, 16), errs: make(chan error, 4)}
}

func (f *fakeWatcher) Add(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.addErr != nil {
		if err := f.addErr(name); err != nil {
			return err
		}
	}
	f.added[name] = true
	f.adds[name]++
	return nil
}

func (f *fakeWatcher) addCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.adds[name]
}

func (f *fakeWatcher) Remove(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.added, name)
	f.removed = append(f.removed, name)
	return nil
}

func (f *fakeWatcher) Events() <-chan fsnotify.Event { return f.events }
func (f *fakeWatcher) Errors() <-chan error          { return f.errs }

func (f *fakeWatcher) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeWatcher) watching() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.added))
	for p := range f.added {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

// fakeWatchers 是建监听的假工厂：回 next（err 不为空时回错误），记下建了几次。后台在跑时也能换（带锁）。
type fakeWatchers struct {
	mu      sync.Mutex
	next    *fakeWatcher
	err     error
	created int
}

func (f *fakeWatchers) build() (fsWatcher, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created++
	if f.err != nil {
		return nil, f.err
	}
	return f.next, nil
}

func (f *fakeWatchers) set(w *fakeWatcher, err error) {
	f.mu.Lock()
	f.next, f.err = w, err
	f.mu.Unlock()
}

func (f *fakeWatchers) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.created
}

// useFakeWatcher 让建监听回 w（建几次都回它），网络文件系统一律认成不是；测试结束换回来。
func useFakeWatcher(t *testing.T, w *fakeWatcher) *fakeWatchers {
	t.Helper()
	oldNew, oldNet := newFSWatcher, isNetworkFS
	t.Cleanup(func() { newFSWatcher, isNetworkFS = oldNew, oldNet })
	f := &fakeWatchers{next: w}
	newFSWatcher = f.build
	isNetworkFS = func(string) (string, bool) { return "", false }
	return f
}

func (e *env) realtimeState(id uint) rtState {
	e.svc.rt.mu.Lock()
	defer e.svc.rt.mu.Unlock()
	return e.svc.rt.states[id]
}

func (e *env) pendingAt(id uint, entry string) (time.Time, bool) {
	e.svc.rt.mu.Lock()
	defer e.svc.rt.mu.Unlock()
	at, ok := e.svc.rt.pending[id][entry]
	return at, ok
}

func TestRealtimeSyncWatchesTree(t *testing.T) {
	if !realtimeSupported() {
		t.Skip("这个系统上不做实时监控")
	}
	e, d := watchEnv(t)
	w := newFakeWatcher()
	useFakeWatcher(t, w)
	for _, p := range []string{"Movie.2020/Subs", ".hidden/x", "@eaDir/y", "a/b/c/d/e/f/g"} {
		require.NoError(t, os.MkdirAll(filepath.Join(d.Path, filepath.FromSlash(p)), 0o755))
	}
	assert.True(t, e.mustSettings().WatchRealtime, "默认开着")

	e.svc.rtSync(e.ctx)
	want := []string{d.Path, filepath.Join(d.Path, "Movie.2020"), filepath.Join(d.Path, "Movie.2020", "Subs"), filepath.Join(d.Path, "a")}
	for _, p := range []string{"a/b", "a/b/c", "a/b/c/d", "a/b/c/d/e", "a/b/c/d/e/f"} {
		want = append(want, filepath.Join(d.Path, filepath.FromSlash(p)))
	}
	slices.Sort(want)
	assert.Equal(t, want, w.watching(), "跳过隐藏与系统目录；太深的（watchFiles 也不整理）不加")
	assert.Equal(t, rtState{state: RealtimeOn}, e.realtimeState(d.ID))

	// 关掉实时监控：监听关掉，状态清掉
	off := false
	e.settings(SettingsInput{MinVideoMB: 1, WatchRealtime: &off})
	assert.False(t, e.mustSettings().WatchRealtime)
	e.svc.rtSync(e.ctx)
	assert.True(t, w.closed)
	assert.Equal(t, rtState{}, e.realtimeState(d.ID))
	views, err := e.svc.WatchDirViews(e.ctx)
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, RealtimeOff, views[0].Realtime)

	// 不给开关时不改
	e.settings(SettingsInput{MinVideoMB: 1})
	assert.False(t, e.mustSettings().WatchRealtime)
	on := true
	e.settings(SettingsInput{MinVideoMB: 1, WatchRealtime: &on})
	assert.True(t, e.mustSettings().WatchRealtime)
}

func TestRealtimeEventsScheduleAndScan(t *testing.T) {
	if !realtimeSupported() {
		t.Skip("这个系统上不做实时监控")
	}
	e, d := watchEnv(t)
	w := newFakeWatcher()
	useFakeWatcher(t, w)
	e.svc.rtSync(e.ctx)
	movie := filepath.Join(d.Path, "Oppenheimer.2023.2160p.BluRay.x265-FRDS")
	file := e.put(d, "Oppenheimer.2023.2160p.BluRay.x265-FRDS/Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv", 5, 0)

	// 新建的文件夹：补监听，排上这一项
	e.svc.rtEvent(fsnotify.Event{Name: movie, Op: fsnotify.Create})
	assert.Contains(t, w.watching(), movie)
	at, ok := e.pendingAt(d.ID, filepath.Base(movie))
	require.True(t, ok)
	assert.Equal(t, e.Now().Add(watchSettle+realtimeMargin), at)

	// 后面的改动把扫描往后推（合并成一次）
	e.advance(time.Minute)
	e.svc.rtEvent(fsnotify.Event{Name: file, Op: fsnotify.Write})
	at, _ = e.pendingAt(d.ID, filepath.Base(movie))
	assert.Equal(t, e.Now().Add(watchSettle+realtimeMargin), at, "最后一次改动 + 稳定期")

	// 只改属性、隐藏的项、不在监控目录里的路径、监控目录本身：不排
	e.svc.rtEvent(fsnotify.Event{Name: filepath.Join(d.Path, "other.mkv"), Op: fsnotify.Chmod})
	e.svc.rtEvent(fsnotify.Event{Name: filepath.Join(d.Path, ".partial"), Op: fsnotify.Create})
	e.svc.rtEvent(fsnotify.Event{Name: filepath.Join(e.root, "elsewhere.mkv"), Op: fsnotify.Create})
	e.svc.rtEvent(fsnotify.Event{Name: d.Path, Op: fsnotify.Write})
	e.svc.rt.mu.Lock()
	assert.Len(t, e.svc.rt.pending[d.ID], 1)
	e.svc.rt.mu.Unlock()

	// 没到时间不扫
	e.svc.rtFire(e.ctx)
	assert.Zero(t, e.drain())
	// 到了时间，但文件之后又写过（一分钟以前，没收到事件），还没过稳定期：按最新的修改时间再排
	e.advance(watchSettle + realtimeMargin)
	require.NoError(t, os.Chtimes(file, e.Now().Add(-time.Minute), e.Now().Add(-time.Minute)))
	e.svc.rtFire(e.ctx)
	assert.Zero(t, e.drain(), "还在稳定期里")
	at, ok = e.pendingAt(d.ID, filepath.Base(movie))
	require.True(t, ok, "再排")
	assert.WithinDuration(t, e.Now().Add(-time.Minute).Add(watchSettle+realtimeMargin), at, time.Second, "按文件最新的修改时间再排")

	// 过了稳定期：排进整理队列，整理成
	e.advance(watchSettle)
	e.svc.rtFire(e.ctx)
	require.Equal(t, 1, e.drain())
	rows := e.history()
	require.Len(t, rows, 1)
	assert.Equal(t, models.MediaTransferDone, rows[0].Status)
	assert.Equal(t, models.MediaTriggerWatch, rows[0].Trigger)
	_, ok = e.pendingAt(d.ID, filepath.Base(movie))
	assert.False(t, ok)

	// 事件太多、丢了一些：整个目录重新扫一遍（整理过的不再排）
	e.svc.rtError(fsnotify.ErrEventOverflow)
	_, ok = e.pendingAt(d.ID, rescanAll)
	require.True(t, ok)
	e.put(d, "The.Last.of.Us.S01E01.2160p.WEB-DL.DV.H.265-HHWEB.mkv", 5, 10*time.Minute)
	e.advance(watchSettle + realtimeMargin)
	e.svc.rtFire(e.ctx)
	assert.Equal(t, 1, e.drain(), "只排新放进来的那一项")
	e.svc.rtError(errors.New("别的错误"))
	_, ok = e.pendingAt(d.ID, rescanAll)
	assert.False(t, ok, "别的错误只写日志")
}

func TestRealtimeUnavailable(t *testing.T) {
	if !realtimeSupported() {
		t.Skip("这个系统上不做实时监控")
	}
	e, d := watchEnv(t)
	w := newFakeWatcher()
	fw := useFakeWatcher(t, w)
	require.NoError(t, os.MkdirAll(filepath.Join(d.Path, "Movie", "Subs"), 0o755))

	// 加到一半失败：加了的全部拿掉，标成不可用，只按间隔扫描
	fail := filepath.Join(d.Path, "Movie", "Subs")
	w.addErr = func(p string) error {
		if p == fail {
			return errors.New("加不了")
		}
		return nil
	}
	e.svc.rtSync(e.ctx)
	st := e.realtimeState(d.ID)
	assert.Equal(t, RealtimeUnavailable, st.state)
	assert.Contains(t, st.note, "加不了")
	assert.Empty(t, w.watching(), "加了的拿掉")
	views, err := e.svc.WatchDirViews(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, RealtimeOff, views[0].Realtime, "后台没在跑时列表里是 off")

	// 下一次核对再试：好了就开着
	w.addErr = nil
	e.svc.rtSync(e.ctx)
	assert.Equal(t, RealtimeOn, e.realtimeState(d.ID).state)

	// 网络文件系统上不加
	e.svc.rt.mu.Lock()
	e.svc.rtDropLocked(d.ID)
	e.svc.rt.mu.Unlock()
	isNetworkFS = func(string) (string, bool) { return "NFS", true }
	e.svc.rtSync(e.ctx)
	st = e.realtimeState(d.ID)
	assert.Equal(t, RealtimeUnavailable, st.state)
	assert.Contains(t, st.note, "NFS")
	e.svc.rtSync(e.ctx)
	assert.Equal(t, st, e.realtimeState(d.ID), "每次核对再试，原因不变")

	// 目录太多
	isNetworkFS = func(string) (string, bool) { return "", false }
	assert.Contains(t, realtimeReason(errTooManyDirs), "目录太多")

	// 建不了监听
	fw.set(nil, errors.New("没有 inotify"))
	e.svc.rt.mu.Lock()
	e.svc.rtCloseLocked()
	e.svc.rt.mu.Unlock()
	e.svc.rtSync(e.ctx)
	assert.Contains(t, e.realtimeState(d.ID).note, "没有 inotify")

	// 监听意外停了：开着的目录标成不可用
	fw.set(w, nil)
	e.svc.rtSync(e.ctx)
	require.Equal(t, RealtimeOn, e.realtimeState(d.ID).state)
	e.svc.rtLost("监听意外停止")
	assert.Equal(t, RealtimeUnavailable, e.realtimeState(d.ID).state)

	// 新建的子目录加不了监听：这个目录退回只按间隔扫描
	e.svc.rtSync(e.ctx)
	require.Equal(t, RealtimeOn, e.realtimeState(d.ID).state)
	sub := filepath.Join(d.Path, "New")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	w.addErr = func(p string) error {
		if p == sub {
			return errors.New("满了")
		}
		return nil
	}
	e.svc.rtEvent(fsnotify.Event{Name: sub, Op: fsnotify.Create})
	assert.Equal(t, RealtimeUnavailable, e.realtimeState(d.ID).state)
}

// 监控目录关掉、删掉以后不再监听，排着的扫描丢掉
func TestRealtimeDropsDirs(t *testing.T) {
	if !realtimeSupported() {
		t.Skip("这个系统上不做实时监控")
	}
	e, d := watchEnv(t)
	w := newFakeWatcher()
	fw := useFakeWatcher(t, w)
	e.svc.rtSync(e.ctx)
	e.svc.rtEvent(fsnotify.Event{Name: filepath.Join(d.Path, "a.mkv"), Op: fsnotify.Create})
	_, err := e.svc.SaveWatchDir(e.ctx, d.ID, WatchDirInput{Path: d.Path, Enabled: false})
	require.NoError(t, err)
	e.svc.rtSync(e.ctx)
	assert.True(t, w.closed, "没有开着的目录时关掉监听")
	_, ok := e.pendingAt(d.ID, "a.mkv")
	assert.False(t, ok)

	// 两个目录，删掉一个
	other := filepath.Join(e.root, "incoming2")
	require.NoError(t, os.MkdirAll(other, 0o755))
	_, err = e.svc.SaveWatchDir(e.ctx, d.ID, WatchDirInput{Path: d.Path, Enabled: true})
	require.NoError(t, err)
	d2, err := e.svc.SaveWatchDir(e.ctx, 0, WatchDirInput{Path: other, Enabled: true})
	require.NoError(t, err)
	w2 := newFakeWatcher()
	fw.set(w2, nil)
	e.svc.rtSync(e.ctx)
	assert.Equal(t, []string{d.Path, other}, w2.watching())
	e.svc.rtEvent(fsnotify.Event{Name: filepath.Join(other, "b.mkv"), Op: fsnotify.Create})
	_, ok = e.pendingAt(d2.ID, "b.mkv")
	require.True(t, ok)
	require.NoError(t, e.svc.DeleteWatchDir(e.ctx, d2.ID))
	e.svc.rtSync(e.ctx)
	assert.Equal(t, []string{d.Path}, w2.watching())
	assert.Equal(t, rtState{}, e.realtimeState(d2.ID))
	_, ok = e.pendingAt(d2.ID, "b.mkv")
	assert.False(t, ok, "删掉的目录排着的扫描丢掉")
}

// 跑起来的后台：改了设置以后马上核对；收到事件、到了时间就扫
func TestRealtimeLoop(t *testing.T) {
	if !realtimeSupported() {
		t.Skip("这个系统上不做实时监控")
	}
	e, d := watchEnv(t)
	w := newFakeWatcher()
	fw := useFakeWatcher(t, w)
	oldTick := realtimeTick
	t.Cleanup(func() { realtimeTick = oldTick })
	realtimeTick = 10 * time.Millisecond
	e.svc.Start()
	t.Cleanup(e.svc.Stop)
	require.Eventually(t, func() bool { return e.realtimeState(d.ID).state == RealtimeOn }, 5*time.Second, 10*time.Millisecond)
	views, err := e.svc.WatchDirViews(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, RealtimeOn, views[0].Realtime)

	// 新加的目录：保存时就核对过了
	other := filepath.Join(e.root, "incoming2")
	require.NoError(t, os.MkdirAll(other, 0o755))
	d2, err := e.svc.SaveWatchDir(e.ctx, 0, WatchDirInput{Path: other, Enabled: true})
	require.NoError(t, err)
	views, err = e.svc.WatchDirViews(e.ctx)
	require.NoError(t, err)
	require.Len(t, views, 2)
	assert.Equal(t, RealtimeOn, views[1].Realtime, "保存以后马上能读到")
	assert.Equal(t, d2.ID, views[1].ID)

	// 事件到了，过了稳定期就排进队列（后台在跑，整理由 worker 做）
	e.put(d, "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv", 5, 10*time.Minute)
	w.events <- fsnotify.Event{Name: filepath.Join(d.Path, "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv"), Op: fsnotify.Create}
	require.Eventually(t, func() bool { _, ok := e.pendingAt(d.ID, "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv"); return ok }, 5*time.Second, 10*time.Millisecond)
	e.advance(watchSettle + realtimeMargin)
	require.Eventually(t, func() bool { return len(e.history()) == 1 }, 10*time.Second, 20*time.Millisecond, "实时监控排上、worker 整理成")

	// 关掉实时监控：保存时就关
	off := false
	e.settings(SettingsInput{MinVideoMB: 1, WatchRealtime: &off})
	views, err = e.svc.WatchDirViews(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, RealtimeOff, views[0].Realtime)
	assert.Equal(t, 1, fw.count())

	// 再打开：保存时建新的监听；监听的通道关了：标成不可用，下一次核对时重建
	w3 := newFakeWatcher()
	fw.set(w3, nil)
	on := true
	e.settings(SettingsInput{MinVideoMB: 1, WatchRealtime: &on})
	require.Equal(t, RealtimeOn, e.realtimeState(d.ID).state)
	assert.Equal(t, 2, fw.count())
	close(w3.events)
	require.Eventually(t, func() bool { return e.realtimeState(d.ID).state == RealtimeUnavailable }, 5*time.Second, 10*time.Millisecond)

	// 保存设置时重建；事件太多、丢了一些：排上整个目录重扫；错误的通道关了：标成不可用
	w4 := newFakeWatcher()
	fw.set(w4, nil)
	e.settings(SettingsInput{MinVideoMB: 1, WatchRealtime: &on})
	require.Equal(t, RealtimeOn, e.realtimeState(d.ID).state)
	w4.errs <- fsnotify.ErrEventOverflow
	require.Eventually(t, func() bool { _, ok := e.pendingAt(d.ID, rescanAll); return ok }, 5*time.Second, 10*time.Millisecond)
	w4.errs <- errors.New("别的错误")
	close(w4.errs)
	require.Eventually(t, func() bool { return e.realtimeState(d.ID).state == RealtimeUnavailable }, 5*time.Second, 10*time.Millisecond)
}

// 真的 inotify / ReadDirectoryChangesW：放进一个文件就排上那一项
func TestRealtimeRealWatcher(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("只在 Linux 与 Windows 上做实时监控")
	}
	e, d := watchEnv(t)
	oldTick, oldResync := realtimeTick, realtimeResync
	t.Cleanup(func() { realtimeTick, realtimeResync = oldTick, oldResync })
	realtimeTick, realtimeResync = 10*time.Millisecond, 20*time.Millisecond
	ctx, cancel := context.WithCancel(e.ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.svc.realtimeLoop(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	require.Eventually(t, func() bool { return e.realtimeState(d.ID).state != "" }, 5*time.Second, 10*time.Millisecond)
	if st := e.realtimeState(d.ID); st.state != RealtimeOn {
		t.Skipf("这台机器上加不了监听：%s", st.note)
	}
	require.NoError(t, os.MkdirAll(filepath.Join(d.Path, "Show"), 0o755))
	require.Eventually(t, func() bool { _, ok := e.pendingAt(d.ID, "Show"); return ok }, 5*time.Second, 10*time.Millisecond, "新建的文件夹")
	require.NoError(t, os.WriteFile(filepath.Join(d.Path, "Show", "a.mkv"), []byte("x"), 0o644))
	e.svc.rt.mu.Lock()
	watched := e.svc.rt.dirs[d.ID].watched[filepath.Join(d.Path, "Show")]
	e.svc.rt.mu.Unlock()
	assert.True(t, watched, "新建的文件夹补了监听")

	// 再加一个监控目录、又删掉：核对时加上、拿掉它的监听
	other := filepath.Join(e.root, "incoming2")
	require.NoError(t, os.MkdirAll(other, 0o755))
	d2, err := e.svc.SaveWatchDir(e.ctx, 0, WatchDirInput{Path: other, Enabled: true})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return e.realtimeState(d2.ID).state == RealtimeOn }, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, e.svc.DeleteWatchDir(e.ctx, d2.ID))
	require.Eventually(t, func() bool {
		e.svc.rt.mu.Lock()
		defer e.svc.rt.mu.Unlock()
		return e.svc.rt.dirs[d2.ID] == nil && e.svc.rt.dirs[d.ID] != nil
	}, 5*time.Second, 10*time.Millisecond)
}

// 到期扫描时的边角：隐藏的项与已经没了的项不排；监控目录在库里关了时不扫；监控目录读不了时整个目录重扫只写日志
func TestRealtimeFireEdgeCases(t *testing.T) {
	if !realtimeSupported() {
		t.Skip("这个系统上不做实时监控")
	}
	e, d := watchEnv(t)
	w := newFakeWatcher()
	useFakeWatcher(t, w)
	e.svc.rtSync(e.ctx)
	require.Equal(t, RealtimeOn, e.realtimeState(d.ID).state)
	schedule := func(names ...string) {
		e.svc.rt.mu.Lock()
		for _, n := range names {
			e.svc.rtScheduleLocked(d.ID, n, e.Now())
		}
		e.svc.rt.mu.Unlock()
	}

	e.put(d, ".partial.mkv", 5, 10*time.Minute)
	schedule(rescanAll, "gone.mkv")
	e.svc.rtFire(e.ctx)
	assert.Zero(t, e.drain(), "隐藏的项、已经没了的项都不排")

	// 监控目录在库里关了（还没核对）：不扫；打开以后照常
	e.put(d, "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv", 5, 10*time.Minute)
	require.NoError(t, e.db.Model(&models.MediaWatchDir{}).Where("id = ?", d.ID).Update("enabled", false).Error)
	schedule("Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv")
	e.svc.rtFire(e.ctx)
	assert.Zero(t, e.drain())
	require.NoError(t, e.db.Model(&models.MediaWatchDir{}).Where("id = ?", d.ID).Update("enabled", true).Error)
	schedule("Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv")
	e.svc.rtFire(e.ctx)
	assert.Equal(t, 1, e.drain())

	// 监控目录被删了：整个目录重扫时读不了，只写日志
	require.NoError(t, os.RemoveAll(d.Path))
	schedule(rescanAll)
	e.svc.rtFire(e.ctx)
	assert.Zero(t, e.drain())
}

// 一个监控目录里的目录超过上限：不全加监听，加了的拿掉，只按间隔扫描
func TestRealtimeTooManyDirs(t *testing.T) {
	if !realtimeSupported() {
		t.Skip("这个系统上不做实时监控")
	}
	e, d := watchEnv(t)
	w := newFakeWatcher()
	useFakeWatcher(t, w)
	for i := range realtimeMaxDirs {
		require.NoError(t, os.Mkdir(filepath.Join(d.Path, fmt.Sprintf("d%04d", i)), 0o755))
	}
	e.svc.rtSync(e.ctx)
	st := e.realtimeState(d.ID)
	assert.Equal(t, RealtimeUnavailable, st.state)
	assert.Equal(t, "目录太多（超过 4096 个）", st.note)
	assert.Empty(t, w.watching(), "加了的拿掉")
}

// 加监听时子目录刚被删掉：跳过它，目录照样开着；同一个新目录的事件来了两次：只加一次
func TestRealtimeWalkSkips(t *testing.T) {
	if !realtimeSupported() {
		t.Skip("这个系统上不做实时监控")
	}
	e, d := watchEnv(t)
	w := newFakeWatcher()
	useFakeWatcher(t, w)
	gone := filepath.Join(d.Path, "Movie", "Gone")
	require.NoError(t, os.MkdirAll(gone, 0o755))
	w.addErr = func(p string) error {
		if p == gone {
			return fmt.Errorf("inotify_add_watch: %w", os.ErrNotExist)
		}
		return nil
	}
	e.svc.rtSync(e.ctx)
	assert.Equal(t, RealtimeOn, e.realtimeState(d.ID).state)
	assert.Equal(t, []string{d.Path, filepath.Join(d.Path, "Movie")}, w.watching())

	w.addErr = nil
	sub := filepath.Join(d.Path, "New")
	require.NoError(t, os.Mkdir(sub, 0o755))
	e.svc.rtEvent(fsnotify.Event{Name: sub, Op: fsnotify.Create})
	e.svc.rtEvent(fsnotify.Event{Name: sub, Op: fsnotify.Create})
	assert.Equal(t, 1, w.addCount(sub))
	assert.Equal(t, RealtimeOn, e.realtimeState(d.ID).state)
}

// 读不了整理设置或监控目录：这次核对什么也不做
func TestRealtimeSyncReadErrors(t *testing.T) {
	if !realtimeSupported() {
		t.Skip("这个系统上不做实时监控")
	}
	e, d := watchEnv(t)
	w := newFakeWatcher()
	fw := useFakeWatcher(t, w)
	boom := errors.New("disk I/O error")
	for _, table := range []string{"media_organize_settings", "media_watch_dirs"} {
		name := "test:fail_" + table
		require.NoError(t, e.db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Schema != nil && tx.Statement.Schema.Table == table {
				_ = tx.AddError(boom)
			}
		}))
		e.svc.rtSync(e.ctx)
		require.NoError(t, e.db.Callback().Query().Remove(name))
		assert.Zero(t, fw.count(), table)
		assert.Equal(t, rtState{}, e.realtimeState(d.ID), table)
	}
	e.svc.rtSync(e.ctx)
	assert.Equal(t, RealtimeOn, e.realtimeState(d.ID).state)
}

// 监控目录的路径在库里变了（接口不让改，防万一）：拿掉旧的监听，加新的
func TestRealtimeDirPathChanged(t *testing.T) {
	if !realtimeSupported() {
		t.Skip("这个系统上不做实时监控")
	}
	e, d := watchEnv(t)
	w := newFakeWatcher()
	useFakeWatcher(t, w)
	e.svc.rtSync(e.ctx)
	other := filepath.Join(e.root, "moved")
	require.NoError(t, os.MkdirAll(other, 0o755))
	require.NoError(t, e.db.Model(&models.MediaWatchDir{}).Where("id = ?", d.ID).Update("path", other).Error)
	e.svc.rtSync(e.ctx)
	assert.Equal(t, []string{other}, w.watching())
	assert.Equal(t, RealtimeOn, e.realtimeState(d.ID).state)
}
