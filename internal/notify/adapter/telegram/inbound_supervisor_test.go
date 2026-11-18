package telegram

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mymmrac/telego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// restartingSource 每次启动都给一个新的 updates channel，记下启动次数；
// failFirst 次之前的启动直接返回错误。
type restartingSource struct {
	mu        sync.Mutex
	starts    int
	failFirst int
	chans     []chan telego.Update
}

func (s *restartingSource) source() updateSource {
	return func(context.Context) (<-chan telego.Update, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.starts++
		if s.starts <= s.failFirst {
			return nil, errors.New("poll boom")
		}
		ch := make(chan telego.Update, 8)
		s.chans = append(s.chans, ch)
		return ch, nil
	}
}

func (s *restartingSource) startCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.starts
}

func (s *restartingSource) current() chan telego.Update {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.chans) == 0 {
		return nil
	}
	return s.chans[len(s.chans)-1]
}

func newSupervisedChannel() *TelegramChannel {
	c := New()
	c.logger = sLogger()
	c.cfg = &Config{}
	c.healthy = true
	c.pollDone = make(chan struct{})
	c.restartBase = time.Millisecond
	c.restartMax = 5 * time.Millisecond
	return c
}

// updates channel 在 poll context 还活着时被关掉：原来 runInbound 直接返回，入站命令从此停掉，
// Healthy() 仍为真。现在标记不健康并退避重启长轮询，重启成功后恢复健康。
func TestTelegram_RunInbound_RestartsAfterUnexpectedClose(t *testing.T) {
	c := newSupervisedChannel()
	src := &restartingSource{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.runInbound(ctx, src.source())

	require.Eventually(t, func() bool { return src.current() != nil }, time.Second, time.Millisecond)
	close(src.current())

	require.Eventually(t, func() bool { return src.startCount() >= 2 }, 2*time.Second, time.Millisecond,
		"关掉之后要重启长轮询")
	require.Eventually(t, c.Healthy, time.Second, time.Millisecond, "重启成功后恢复健康")

	cancel()
	select {
	case <-c.pollDone:
	case <-time.After(2 * time.Second):
		t.Fatal("ctx 取消后 runInbound 没有退出")
	}
}

// 长轮询启动失败时退避重试，而不是就此放弃。
func TestTelegram_RunInbound_StartErrorRetries(t *testing.T) {
	c := newSupervisedChannel()
	src := &restartingSource{failFirst: 2}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.runInbound(ctx, src.source())

	require.Eventually(t, func() bool { return src.startCount() >= 3 && src.current() != nil },
		2*time.Second, time.Millisecond, "失败两次之后第三次启动成功")
	require.Eventually(t, c.Healthy, time.Second, time.Millisecond)

	cancel()
	<-c.pollDone
}

// getUpdates 的结果决定健康状态：telego 自己重试失败的长轮询，不关 channel、不报错，
// 原来令牌失效或网络断开时通道一直显示「运行中」。
func TestTelegram_ReportPoll_HealthFollowsGetUpdates(t *testing.T) {
	c := newSupervisedChannel()
	c.pollDone = nil // 这里不跑 runInbound，Close 不必等它
	ctx, cancel := context.WithCancel(context.Background())
	c.pollCtx, c.pollCancel = ctx, cancel

	c.reportPoll(errors.New("getUpdates 返回 HTTP 401"))
	assert.False(t, c.Healthy())
	c.reportPoll(nil)
	assert.True(t, c.Healthy())

	require.NoError(t, c.Close(context.Background()))
	c.reportPoll(nil)
	assert.False(t, c.Healthy(), "关闭之后迟到的结果不能把通道标回健康")
}

// pollHealthTransport 只看 getUpdates：失败（含非 200）报错误，成功报 nil；其他接口不报。
func TestPollHealthTransport_ReportsGetUpdatesOnly(t *testing.T) {
	status := http.StatusUnauthorized
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"ok":false}`))
	}))
	defer srv.Close()

	var mu sync.Mutex
	var reports []error
	client := &http.Client{Transport: pollHealthTransport{
		next: http.DefaultTransport,
		report: func(err error) {
			mu.Lock()
			defer mu.Unlock()
			reports = append(reports, err)
		},
	}}
	get := func(path string) {
		resp, err := client.Get(srv.URL + path)
		require.NoError(t, err)
		_ = resp.Body.Close()
	}

	get("/bot123:SECRET/getUpdates")
	status = http.StatusOK
	get("/bot123:SECRET/getUpdates")
	get("/bot123:SECRET/sendMessage")

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, reports, 2, "sendMessage 不算长轮询")
	require.Error(t, reports[0])
	assert.Contains(t, reports[0].Error(), "401")
	assert.False(t, strings.Contains(reports[0].Error(), "SECRET"), "错误里不带令牌")
	assert.NoError(t, reports[1])
}
