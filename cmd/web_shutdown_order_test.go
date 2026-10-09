package cmd

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/notify"
)

// shutdownRecorder 记下各关闭步骤的先后顺序。
type shutdownRecorder struct {
	mu    sync.Mutex
	steps []string
}

func (r *shutdownRecorder) add(step string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps = append(r.steps, step)
}

func (r *shutdownRecorder) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.steps...)
}

type recordingStopper struct {
	rec   *shutdownRecorder
	name  string
	block chan struct{} // 非 nil 时一直阻塞到被关闭，模拟停不下来的步骤
}

func (s recordingStopper) StopAll() { s.run() }

func (s recordingStopper) CloseAll() { s.run() }

func (s recordingStopper) Close() { s.run() }

func (s recordingStopper) Shutdown(context.Context) error {
	s.run()
	return nil
}

func (s recordingStopper) run() {
	s.rec.add(s.name)
	if s.block != nil {
		<-s.block
	}
}

type orderChannel struct {
	stubNotifyChannel
	rec *shutdownRecorder
}

func (c *orderChannel) Close(context.Context) error {
	c.rec.add("channels")
	return nil
}

// 收到信号后先停后台生产者（热重载、RSS 重试），再停调度器与下载器，然后关通知通道，最后关 HTTP。
// 原来只关通知通道和 HTTP：RSS 任务、监控器、下载器都没停，热重载还可能在关闭途中把通道建回来。
func TestRunShutdown_StopsEverythingInOrder(t *testing.T) {
	global.InitLogger(zap.NewNop())
	rec := &shutdownRecorder{}
	bs := &chatopsBootstrap{
		manager:  newLiveNotifyManager(nil),
		channels: map[uint]notify.Channel{1: &orderChannel{rec: rec}},
	}

	runShutdown(context.Background(), shutdownPlan{
		qbitCompat:     recordingStopper{rec: rec, name: "qbit-compat"},
		remote:         recordingStopper{rec: rec, name: "remote"},
		stopBackground: func() { rec.add("background") },
		scheduler:      recordingStopper{rec: rec, name: "scheduler"},
		downloaders:    recordingStopper{rec: rec, name: "downloaders"},
		bs:             bs,
		srv:            recordingStopper{rec: rec, name: "http"},
	})

	assert.Equal(t, []string{"qbit-compat", "remote", "background", "scheduler", "downloaders", "channels", "http"}, rec.list(),
		"qB 兼容入口与远程访问最先关：后面关下载器时不再有推送进来")
}

// 某一步停不下来（比如 RSS 任务卡在下载器请求上）时按步骤时限放弃等待，后面的步骤照常执行。
func TestRunShutdown_StuckStepIsBounded(t *testing.T) {
	global.InitLogger(zap.NewNop())
	rec := &shutdownRecorder{}
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })

	start := time.Now()
	runShutdown(context.Background(), shutdownPlan{
		scheduler:   recordingStopper{rec: rec, name: "scheduler", block: block},
		srv:         recordingStopper{rec: rec, name: "http"},
		stepTimeout: 50 * time.Millisecond,
	})

	assert.Less(t, time.Since(start), 2*time.Second)
	assert.Equal(t, []string{"scheduler", "http"}, rec.list())
}
