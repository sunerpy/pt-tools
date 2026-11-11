package organize

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// hookCalls 记下整理完成钩子收到的 info hash。
type hookCalls struct {
	mu  sync.Mutex
	got []string
}

func (h *hookCalls) add(hash string) {
	h.mu.Lock()
	h.got = append(h.got, hash)
	h.mu.Unlock()
}

func (h *hookCalls) list() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.got...)
}

// startWithHook 接上钩子、开始后台循环；测试结束时停掉（Stop 等 worker 退出，之后不会再有回调）。
func startWithHook(t *testing.T, e *env) *hookCalls {
	t.Helper()
	h := &hookCalls{}
	e.svc.SetAfterOrganize(h.add)
	e.svc.Start()
	t.Cleanup(e.svc.Stop)
	return h
}

// 没人等结果的自动整理（下载完成事件）：入了库就回调一次，参数是 info hash。
func TestAfterOrganize_AutoJob(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{AutoEnabled: true, MinVideoMB: 1})
	e.oppenheimer()
	h := startWithHook(t, e)
	require.NoError(t, events.PublishWithPayload(events.EvtTorrentCompleted, events.TorrentCompletedPayload{
		Title: oppName, DownloaderName: "qb", TaskID: oppHash, InfoHash: oppHash,
	}))
	require.Eventually(t, func() bool { return len(h.list()) > 0 }, 5*time.Second, 20*time.Millisecond)
	e.svc.Stop()
	assert.Equal(t, []string{oppHash}, h.list())
	assert.Equal(t, int64(1), e.historyCount(oppHash))
}

// 手动整理在 20 秒内做完、调用方拿到了结果：钩子照样回调一次（订阅那边的刷新不靠调用方）。
func TestAfterOrganize_ManualWaitingCaller(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	h := startWithHook(t, e)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.False(t, res.Queued)
	assert.Equal(t, 1, res.Created)
	e.svc.Stop()
	assert.Equal(t, []string{oppHash}, h.list())
}

// 请求先取消了（调用方拿到 Queued），整理随后做完：钩子恰好回调一次。
func TestAfterOrganize_CancelledRequest(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	h := startWithHook(t, e)
	e.svc.mu.Lock() // worker 整理时要拿这把锁：先占着，调用方等不到结果
	ctx, cancel := context.WithCancel(e.ctx)
	cancel()
	res, err := e.svc.Organize(ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.True(t, res.Queued, "请求取消了，先回还在后台整理")
	assert.Empty(t, h.list())
	e.svc.mu.Unlock()
	require.Eventually(t, func() bool { return len(h.list()) > 0 }, 5*time.Second, 20*time.Millisecond)
	e.svc.Stop()
	assert.Equal(t, []string{oppHash}, h.list())
}

// 下载器的任务编号和 info hash 不一样（Transmission 是十进制编号）：钩子收到的是 info hash。
func TestAfterOrganize_PassesInfoHashNotTaskID(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.addTorrent(oppHash, oppName, map[string]int{oppName + "/" + oppName + ".mkv": 5}, func(t *downloader.Torrent) { t.ID = "17" })
	h := startWithHook(t, e)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "17"})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	e.svc.Stop()
	assert.Equal(t, []string{oppHash}, h.list())
}

// 没有文件入库（种子还没下完、找不到种子）时不回调。
func TestAfterOrganize_NotCalledWithoutLibraryFiles(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.addTorrent(oppHash, oppName, map[string]int{oppName + "/" + oppName + ".mkv": 5}, func(t *downloader.Torrent) { t.Progress = 0.5 })
	h := startWithHook(t, e)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Zero(t, res.Created)
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.ErrorIs(t, err, ErrNotFound)
	e.svc.Stop()
	assert.Empty(t, h.list())
}

// 后台循环没在跑时（直接整理）不回调：没有 worker。
func TestAfterOrganize_NotCalledWhenNotRunning(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	h := &hookCalls{}
	e.svc.SetAfterOrganize(h.add)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.Empty(t, h.list())
}
