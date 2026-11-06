package nas

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSource 是测试用的来源：fn 决定每次采集的结果（为空时给一份带存储池的完整快照）。
type fakeSource struct {
	mu     sync.Mutex
	fn     func(ctx context.Context, full bool, n int) (*Snapshot, error)
	calls  []bool
	closed int
}

func (f *fakeSource) Collect(ctx context.Context, full bool) (*Snapshot, error) {
	f.mu.Lock()
	n := len(f.calls)
	f.calls = append(f.calls, full)
	fn := f.fn
	f.mu.Unlock()
	if fn == nil {
		return snapFor(full, float64(n)), nil
	}
	return fn(ctx, full, n)
}

func (f *fakeSource) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
	return nil
}

func (f *fakeSource) setFn(fn func(ctx context.Context, full bool, n int) (*Snapshot, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fn = fn
}

func (f *fakeSource) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeSource) fulls() []bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]bool(nil), f.calls...)
}

func (f *fakeSource) closes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// extraSource 在第一次采集以后交出一次刷新的凭据。
type extraSource struct {
	fakeSource
	once  sync.Once
	extra map[string]string
}

func (e *extraSource) TakeExtra() map[string]string {
	var out map[string]string
	e.once.Do(func() { out = e.extra })
	return out
}

// snapFor 是一次采集的结果：完整采集带一个存储池与一条告警，轻量采集只有 CPU、内存与网络。
func snapFor(full bool, cpu float64) *Snapshot {
	s := &Snapshot{
		CPU:     CPU{Percent: cpu},
		Memory:  Memory{Total: 100, Used: 40, Available: 60},
		Network: Network{RxBps: 10, TxBps: 5, Interfaces: []string{"eth0"}},
		Facets:  map[string]Facet{FacetCPU: {Status: FacetOK}, FacetMemory: {Status: FacetOK}, FacetNetwork: {Status: FacetOK}},
	}
	if full {
		s.System = System{Hostname: "nas", UptimeSeconds: 100}
		s.Pools = []Pool{{Name: "tank", Size: 10, Used: 5, Free: 5}}
		s.Alerts = []Alert{{Level: AlertCritical, Title: "坏了一块盘"}}
		s.Facets[FacetPools] = Facet{Status: FacetOK}
		s.Facets[FacetAlerts] = Facet{Status: FacetOK}
	} else {
		s.System = System{UptimeSeconds: 160}
	}
	return s
}

// testRig 是一套采集器：cfg 是 Load 读到的设置（测试里改它再 Reload），srcs 是各个来源（按编号，本机是 0）。
type testRig struct {
	t    *testing.T
	c    *Collector
	mu   sync.Mutex
	cfg  Config
	err  error
	made map[uint][]*fakeSource
	next map[uint]func() *fakeSource
	logs []string
	save []string
}

func newRig(t *testing.T, cfg Config) *testRig {
	t.Helper()
	r := &testRig{t: t, cfg: cfg, made: map[uint][]*fakeSource{}, next: map[uint]func() *fakeSource{}}
	r.c = &Collector{
		Load: func(context.Context) (Config, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			return r.cfg, r.err
		},
		SaveExtra: func(_ context.Context, id uint, rev int64, extra map[string]string) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.save = append(r.save, fmt.Sprintf("%d@%d=%v", id, rev, extra))
			if extra["fail"] != "" {
				return errors.New("写库失败 token=abc")
			}
			return nil
		},
		Factory: func(sc SourceConfig) (Source, error) {
			if sc.Kind == "bad" {
				return nil, errors.New("设置不对 https://u:p@nas")
			}
			return r.newSource(sc.ID), nil
		},
		NewLocal: func() Source { return r.newSource(0) },
		Logf: func(format string, args ...any) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.logs = append(r.logs, fmt.Sprintf(format, args...))
		},
		Light:   time.Millisecond,
		Full:    time.Hour,
		Timeout: time.Second,
		Backoff: []time.Duration{time.Millisecond},
	}
	t.Cleanup(func() { _ = r.c.Stop(context.Background()) })
	return r
}

func (r *testRig) newSource(id uint) *fakeSource {
	r.mu.Lock()
	defer r.mu.Unlock()
	var f *fakeSource
	if mk := r.next[id]; mk != nil {
		f = mk()
	} else {
		f = &fakeSource{}
	}
	r.made[id] = append(r.made[id], f)
	return f
}

func (r *testRig) source(id uint) *fakeSource {
	r.t.Helper()
	var f *fakeSource
	require.Eventually(r.t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		if list := r.made[id]; len(list) > 0 {
			f = list[len(list)-1]
			return true
		}
		return false
	}, 5*time.Second, time.Millisecond)
	return f
}

func (r *testRig) setCfg(fn func(*Config)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(&r.cfg)
}

func (r *testRig) status(id string) Status {
	r.t.Helper()
	for _, s := range r.c.List() {
		if s.ID == id {
			return s
		}
	}
	r.t.Fatalf("没有来源 %s", id)
	return Status{}
}

func (r *testRig) waitState(id, state string) Status {
	r.t.Helper()
	var st Status
	require.Eventually(r.t, func() bool {
		st = r.status(id)
		return st.State == state
	}, 5*time.Second, time.Millisecond, "等 %s 变成 %s", id, state)
	return st
}

func TestCollector_StartListAndStates(t *testing.T) {
	testKind(t, Kind{Name: "fk", Label: "假的", Experimental: true})
	r := newRig(t, Config{LocalEnabled: true, Sources: []SourceConfig{
		{ID: 1, Name: "一号", Kind: "fk", Enabled: true, URL: "https://nas", Secret: "k", PinSHA256: "ab", InsecureAck: true, Revision: 3},
		{ID: 2, Name: "二号", Kind: "fk", Enabled: false},
		{ID: 3, Name: "三号", Kind: "fk", Enabled: true, Broken: "密钥解不开"},
		{ID: 4, Name: "四号", Kind: "gone", Enabled: true},
		{ID: 5, Name: "五号", Kind: "bad", Enabled: true},
	}})
	r.c.Factory = func(sc SourceConfig) (Source, error) {
		if sc.Kind == "gone" || sc.Kind == "bad" {
			return nil, fmt.Errorf("不认识的类型 %s", sc.Kind)
		}
		return r.newSource(sc.ID), nil
	}
	r.c.Start(context.Background())
	r.c.Start(context.Background()) // 第二次不再起一套

	one := r.waitState("1", StateOK)
	local := r.waitState(LocalID, StateOK)
	ids := []string{}
	for _, s := range r.c.List() {
		ids = append(ids, s.ID)
	}
	assert.Equal(t, []string{LocalID, "1", "2", "3", "4", "5"}, ids, "本机在最前，其余按读到的顺序")

	assert.Equal(t, "本机", local.Name)
	assert.Equal(t, LocalKind, local.Kind)
	assert.False(t, local.Experimental)

	assert.Equal(t, "一号", one.Name)
	assert.True(t, one.Experimental, "按注册表的类型")
	assert.True(t, one.Enabled)
	assert.Equal(t, "https://nas", one.URL)
	assert.True(t, one.HasSecret)
	assert.Equal(t, "ab", one.PinSHA256)
	assert.True(t, one.InsecureAck)
	require.NotNil(t, one.Snapshot)
	require.NotNil(t, one.LastOKAt)
	assert.Nil(t, one.NextTryAt)
	assert.NotEmpty(t, one.History)
	assert.Equal(t, "nas", one.Snapshot.System.Hostname)

	assert.Equal(t, StateDisabled, r.status("2").State)
	assert.Nil(t, r.status("2").Snapshot)
	three := r.status("3")
	assert.Equal(t, [2]string{StateError, "密钥解不开"}, [2]string{three.State, three.Message})
	assert.Equal(t, StateError, r.status("4").State)
	assert.Equal(t, "不认识的类型 gone", r.status("4").Message)

	// 给出去的快照是拷贝
	one.Snapshot.Pools[0].Name = "改了"
	assert.Equal(t, "tank", r.status("1").Snapshot.Pools[0].Name)

	require.NoError(t, r.c.Stop(context.Background()))
	assert.Equal(t, 1, r.source(1).closes(), "退出时关掉来源")
	assert.Equal(t, 1, r.source(0).closes())
	r.c.Start(context.Background())
	assert.NoError(t, r.c.Reload(context.Background(), "1"), "停了以后什么也不做")
}

func TestCollector_RegistryFactory(t *testing.T) {
	made := 0
	testKind(t, Kind{Name: "reg", New: func(SourceConfig) (Source, error) {
		made++
		return &fakeSource{}, nil
	}})
	c := &Collector{
		Load: func(context.Context) (Config, error) {
			return Config{Sources: []SourceConfig{{ID: 1, Kind: "reg", Enabled: true}, {ID: 2, Kind: "gone", Enabled: true}}}, nil
		},
		NewLocal: func() Source { return &fakeSource{} },
		Light:    time.Hour,
	}
	c.Start(context.Background())
	defer func() { _ = c.Close(context.Background()) }()
	require.Eventually(t, func() bool { return c.List()[1].State == StateOK }, 5*time.Second, time.Millisecond)
	assert.Equal(t, 1, made)
	gone := c.List()[2]
	assert.Equal(t, StateUnsupported, gone.State)
	assert.Equal(t, "这个版本不支持「gone」来源", gone.Message)
	assert.Equal(t, StateDisabled, c.List()[0].State, "本机来源关着")
}

func TestCollector_LightMergeAndFullCadence(t *testing.T) {
	base := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	var clockMu sync.Mutex
	now := base
	r := newRig(t, Config{Sources: []SourceConfig{{ID: 1, Kind: "fk", Enabled: true}}})
	r.c.Full = 5 * time.Minute
	r.c.Now = func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		return now
	}
	r.next[1] = func() *fakeSource {
		return &fakeSource{fn: func(_ context.Context, full bool, n int) (*Snapshot, error) {
			s := snapFor(full, float64(n))
			clockMu.Lock()
			now = now.Add(2 * time.Minute) // 每次采集「过了」两分钟
			s.At = now
			clockMu.Unlock()
			return s, nil
		}}
	}
	r.c.Start(context.Background())
	src := r.source(1)
	require.Eventually(t, func() bool { return src.count() >= 6 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, []bool{true, false, false, false, true}, src.fulls()[:5], "第一次完整采集，之后每 5 分钟一次")

	st := r.status("1")
	require.NotNil(t, st.Snapshot)
	assert.Equal(t, "tank", st.Snapshot.Pools[0].Name, "轻量采集沿用上一次完整采集的存储池")
	assert.Len(t, st.Snapshot.Alerts, 1)
	assert.Equal(t, FacetOK, st.Snapshot.Facets[FacetPools].Status)
	assert.Equal(t, "nas", st.Snapshot.System.Hostname)
	assert.NotEmpty(t, st.History)
	for _, h := range st.History {
		assert.InDelta(t, 40.0, h.Mem, 0.01)
		assert.Equal(t, uint64(10), h.Rx)
	}
}

func TestMergeLight(t *testing.T) {
	full := snapFor(true, 1)
	full.At = time.Unix(100, 0)
	light := snapFor(false, 9)
	light.At = time.Unix(130, 0)
	light.Facets[FacetCPUTemp] = Facet{Status: FacetUnsupported, Reason: "虚拟机"}
	m := mergeLight(full, light)
	assert.Equal(t, light.At, m.At)
	assert.Equal(t, 9.0, m.CPU.Percent)
	assert.Equal(t, int64(160), m.System.UptimeSeconds)
	assert.Equal(t, "nas", m.System.Hostname)
	assert.Len(t, m.Pools, 1)
	assert.Equal(t, FacetUnsupported, m.Facets[FacetCPUTemp].Status)
	assert.Equal(t, FacetOK, m.Facets[FacetPools].Status)
	assert.Equal(t, 1.0, full.CPU.Percent, "不改原来的")

	noUptime := snapFor(false, 2)
	noUptime.System.UptimeSeconds = 0
	assert.Equal(t, int64(100), mergeLight(full, noUptime).System.UptimeSeconds)

	assert.Equal(t, light.CPU, mergeLight(nil, light).CPU)
	bare := &Snapshot{}
	assert.NotNil(t, mergeLight(bare, light).Facets)
	assert.Nil(t, (*Snapshot)(nil).clone())
	assert.Zero(t, Memory{}.MemPercent())
}

func TestCollector_BackoffOnErrors(t *testing.T) {
	r := newRig(t, Config{Sources: []SourceConfig{{ID: 1, Kind: "fk", Enabled: true}}})
	r.c.Backoff = []time.Duration{time.Hour}
	r.next[1] = func() *fakeSource {
		return &fakeSource{fn: func(context.Context, bool, int) (*Snapshot, error) {
			return nil, errors.New(`Get "https://admin:pw@nas/api?key=abc": connection refused`)
		}}
	}
	r.c.Start(context.Background())
	st := r.waitState("1", StateError)
	assert.NotContains(t, st.Message, "pw")
	assert.NotContains(t, st.Message, "key=abc")
	assert.Contains(t, st.Message, "connection refused")
	require.NotNil(t, st.NextTryAt)
	assert.WithinDuration(t, time.Now().Add(time.Hour), *st.NextTryAt, time.Minute, "按退避的第一档")
	assert.Nil(t, st.Snapshot)
	assert.Equal(t, 1, r.source(1).count(), "退避期间不再试")

	// 「重新连接」立即试一次：成功以后清掉错误与下次重试时间
	r.source(1).setFn(nil)
	got, err := r.c.Retry(context.Background(), "1")
	require.NoError(t, err)
	assert.Equal(t, StateOK, got.State)
	assert.Nil(t, got.NextTryAt)
	assert.Empty(t, got.Message)
}

func TestCollector_BackoffSteps(t *testing.T) {
	c := &Collector{}
	got := []time.Duration{}
	for i := range 6 {
		got = append(got, c.backoff(i))
	}
	assert.Equal(t, []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute, 5 * time.Minute, 5 * time.Minute}, got)
	assert.Equal(t, defaultLight, c.light())
	assert.Equal(t, defaultFull, c.full())
	assert.Equal(t, defaultTimeout, c.timeout())
	assert.False(t, c.now().IsZero())
}

func TestCollector_ErrorsKeepLastSnapshot(t *testing.T) {
	r := newRig(t, Config{Sources: []SourceConfig{{ID: 1, Kind: "fk", Enabled: true}}})
	r.c.Backoff = []time.Duration{time.Hour}
	r.c.Start(context.Background())
	r.waitState("1", StateOK)
	r.source(1).setFn(func(context.Context, bool, int) (*Snapshot, error) { return nil, errors.New("断网了") })
	st := r.waitState("1", StateError)
	require.NotNil(t, st.Snapshot, "出错时还给上一份（界面写「这是 N 分钟前的数据」）")
	require.NotNil(t, st.LastOKAt)
	assert.Equal(t, "断网了", st.Message)
}

func TestCollector_BreakerAndRetry(t *testing.T) {
	cases := []struct {
		err   error
		state string
		msg   string
	}{
		{&AuthError{Reason: "API Key 不对 token=abc"}, StateAuthFailed, "认证失败：API Key 不对 token=***"},
		{&CertError{Cert: CertInfo{SHA256: "x"}}, StateCertUntrusted, ErrCertUntrusted.Error()},
		{fmt.Errorf("连不上: %w", &CertError{Changed: true}), StateCertChanged, ErrCertChanged.Error()},
		{ErrUnsupported, StateUnsupported, ErrUnsupported.Error()},
	}
	for _, tc := range cases {
		t.Run(tc.state, func(t *testing.T) {
			r := newRig(t, Config{Sources: []SourceConfig{{ID: 1, Kind: "fk", Enabled: true}}})
			r.next[1] = func() *fakeSource {
				return &fakeSource{fn: func(context.Context, bool, int) (*Snapshot, error) { return nil, tc.err }}
			}
			r.c.Start(context.Background())
			st := r.waitState("1", tc.state)
			assert.Equal(t, tc.msg, st.Message)
			assert.Nil(t, st.NextTryAt, "停住了，没有下次重试时间")
			src := r.source(1)
			before := src.count()
			for i := 1; i <= 3; i++ {
				got, err := r.c.Retry(context.Background(), "1")
				require.NoError(t, err)
				assert.Equal(t, tc.state, got.State, "失败了照旧停住")
				assert.Equal(t, before+i, src.count(), "停住以后只在点「重新连接」时试一次")
			}
			src.setFn(nil)
			got, err := r.c.Retry(context.Background(), "1")
			require.NoError(t, err)
			assert.Equal(t, StateOK, got.State)
			assert.True(t, src.fulls()[len(src.fulls())-1], "重新连接是完整采集")
			n := src.count()
			require.Eventually(t, func() bool { return src.count() > n+2 }, 5*time.Second, time.Millisecond, "成功以后恢复定时采集")
		})
	}
}

func TestCollector_RetryErrors(t *testing.T) {
	r := newRig(t, Config{Sources: []SourceConfig{{ID: 1, Kind: "fk", Enabled: true}, {ID: 2, Kind: "fk", Enabled: false}}})
	block := make(chan struct{})
	r.next[1] = func() *fakeSource {
		return &fakeSource{fn: func(ctx context.Context, full bool, n int) (*Snapshot, error) {
			if n == 0 {
				return snapFor(full, 1), nil
			}
			<-block
			return snapFor(full, 1), nil
		}}
	}
	r.c.Light = time.Millisecond
	r.c.Start(context.Background())
	_, err := r.c.Retry(context.Background(), "9")
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = r.c.Retry(context.Background(), "2")
	assert.ErrorIs(t, err, ErrNoLoop)

	src := r.source(1)
	require.Eventually(t, func() bool { return src.count() >= 2 }, 5*time.Second, time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = r.c.Retry(ctx, "1")
	assert.ErrorIs(t, err, context.DeadlineExceeded, "循环在采集，等不到")
	close(block)
}

func TestCollector_RetryWaitsForAttempt(t *testing.T) {
	r := newRig(t, Config{Sources: []SourceConfig{{ID: 1, Kind: "fk", Enabled: true}}})
	release := make(chan struct{})
	started := make(chan struct{}, 4)
	r.next[1] = func() *fakeSource {
		return &fakeSource{fn: func(ctx context.Context, full bool, n int) (*Snapshot, error) {
			if n == 0 {
				return nil, &AuthError{}
			}
			started <- struct{}{}
			<-release
			return snapFor(full, 1), nil
		}}
	}
	r.c.Start(context.Background())
	r.waitState("1", StateAuthFailed)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := r.c.Retry(ctx, "1")
		done <- err
	}()
	<-started
	cancel()
	assert.ErrorIs(t, <-done, context.Canceled, "等这一次试完时取消了")
	close(release)
	r.waitState("1", StateOK)
}

func TestCollector_TimeoutAndEmpty(t *testing.T) {
	r := newRig(t, Config{Sources: []SourceConfig{{ID: 1, Kind: "fk", Enabled: true}, {ID: 2, Kind: "fk", Enabled: true}}})
	r.c.Timeout = 20 * time.Millisecond
	r.c.Backoff = []time.Duration{time.Hour}
	r.next[1] = func() *fakeSource {
		return &fakeSource{fn: func(ctx context.Context, _ bool, _ int) (*Snapshot, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}}
	}
	r.next[2] = func() *fakeSource {
		return &fakeSource{fn: func(context.Context, bool, int) (*Snapshot, error) { return nil, nil }}
	}
	r.c.Start(context.Background())
	assert.Equal(t, "20 毫秒内没有回应", r.waitState("1", StateError).Message)
	assert.Equal(t, "来源没有返回数据", r.waitState("2", StateError).Message)
	assert.Equal(t, "10 秒", humanDuration(10*time.Second))
}

func TestCollector_ReloadSemantics(t *testing.T) {
	r := newRig(t, Config{LocalEnabled: false, Sources: []SourceConfig{{ID: 1, Name: "一号", Kind: "fk", Enabled: true, Revision: 1}}})
	r.c.Light = time.Hour
	r.c.Start(context.Background())
	first := r.waitState("1", StateOK)
	ctx := context.Background()

	require.NoError(t, r.c.Reload(ctx, "1"))
	same := r.status("1")
	assert.Equal(t, first.LastOKAt, same.LastOKAt, "设置没变时不重启")
	assert.Len(t, r.made[1], 1)

	r.setCfg(func(c *Config) { c.Sources[0].Revision, c.Sources[0].Name = 2, "改名了" })
	require.NoError(t, r.c.Reload(ctx, "1"))
	r.waitState("1", StateOK)
	require.Eventually(t, func() bool { return r.made[1][0].closes() == 1 }, 5*time.Second, time.Millisecond, "旧的来源关掉了")
	assert.Len(t, r.made[1], 2, "新 revision 重建来源")
	assert.Equal(t, "改名了", r.status("1").Name)
	assert.Len(t, r.status("1").History, 1, "换了设置，历史从头记")

	// 只重启指定的那一个：本机的开关改了但 Reload 的是 1，本机不动
	r.setCfg(func(c *Config) { c.LocalEnabled = true })
	require.NoError(t, r.c.Reload(ctx, "1"))
	assert.Equal(t, StateDisabled, r.status(LocalID).State)
	require.NoError(t, r.c.Reload(ctx, LocalID))
	r.waitState(LocalID, StateOK)

	r.setCfg(func(c *Config) { c.Sources[0].Enabled = false })
	require.NoError(t, r.c.Reload(ctx, "1"))
	off := r.status("1")
	assert.Equal(t, StateDisabled, off.State)
	assert.Nil(t, off.Snapshot)

	// 新加的来源
	r.setCfg(func(c *Config) {
		c.Sources = append(c.Sources, SourceConfig{ID: 7, Kind: "fk", Enabled: true, Revision: 1})
	})
	require.NoError(t, r.c.Reload(ctx, "7"))
	r.waitState("7", StateOK)

	// 删掉的来源
	r.setCfg(func(c *Config) { c.Sources = c.Sources[1:] })
	require.NoError(t, r.c.Reload(ctx, "1"))
	ids := []string{}
	for _, s := range r.c.List() {
		ids = append(ids, s.ID)
	}
	assert.Equal(t, []string{LocalID, "7"}, ids)

	r.mu.Lock()
	r.err = errors.New("库锁住了")
	r.mu.Unlock()
	assert.EqualError(t, r.c.Reload(ctx, "7"), "库锁住了")
}

func TestCollector_ReloadBeforeStart(t *testing.T) {
	r := newRig(t, Config{})
	assert.NoError(t, r.c.Reload(context.Background(), "1"), "没启动时什么也不做")
	assert.Empty(t, r.c.List())
}

func TestCollector_StartLoadFails(t *testing.T) {
	r := newRig(t, Config{})
	r.err = errors.New("没有库")
	r.c.Start(context.Background())
	assert.Empty(t, r.c.List())
	assert.Contains(t, r.logs[0], "没有库")

	c := &Collector{}
	c.Start(context.Background())
	assert.Empty(t, c.List(), "没有读取函数")
	require.NoError(t, c.Stop(context.Background()))
}

func TestCollector_SaveExtra(t *testing.T) {
	r := newRig(t, Config{LocalEnabled: true, Sources: []SourceConfig{{ID: 1, Kind: "fk", Enabled: true, Revision: 4}, {ID: 2, Kind: "fk", Enabled: true, Revision: 1}}})
	r.c.Light = time.Hour
	r.c.NewLocal = func() Source { return &extraSource{extra: map[string]string{"did": "local"}} }
	r.c.Factory = func(sc SourceConfig) (Source, error) {
		if sc.ID == 1 {
			return &extraSource{extra: map[string]string{"did": "dev"}}, nil
		}
		return &extraSource{extra: map[string]string{"fail": "1"}}, nil
	}
	r.c.Start(context.Background())
	r.waitState(LocalID, StateOK)
	// 状态先变成 ok，凭据在那之后才写：等两条写回都到了
	require.Eventually(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return len(r.save) == 2 && len(r.logs) == 1
	}, 5*time.Second, time.Millisecond)
	_, err := r.c.Retry(context.Background(), "1") // Retry 等这一次（含写回）做完才返回
	require.NoError(t, err)
	r.mu.Lock()
	defer r.mu.Unlock()
	assert.ElementsMatch(t, []string{"1@4=map[did:dev]", "2@1=map[fail:1]"}, r.save, "只写一次；本机来源不写")
	require.Len(t, r.logs, 1)
	assert.Contains(t, r.logs[0], "保存刷新的凭据失败")
	assert.NotContains(t, r.logs[0], "abc")
}

func TestCollector_StopTimeout(t *testing.T) {
	r := newRig(t, Config{Sources: []SourceConfig{{ID: 1, Kind: "fk", Enabled: true}}})
	release := make(chan struct{})
	entered := make(chan struct{})
	r.next[1] = func() *fakeSource {
		return &fakeSource{fn: func(context.Context, bool, int) (*Snapshot, error) {
			close(entered)
			<-release // 不理 ctx 的来源
			return nil, errors.New("x")
		}}
	}
	r.c.Start(context.Background())
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	assert.ErrorContains(t, r.c.Stop(ctx), "等 NAS 监控的采集退出超时")
	close(release)
	assert.NoError(t, r.c.Stop(context.Background()))
}

func TestCollector_HistoryCap(t *testing.T) {
	c := &Collector{}
	e := &entry{id: "1"}
	src := &fakeSource{}
	for range HistoryLen + 10 {
		require.Equal(t, outcomeOK, c.attempt(context.Background(), e, src, false))
	}
	st := e.status()
	assert.Len(t, st.History, HistoryLen)
	assert.Equal(t, 10.0, st.History[0].CPU, "丢掉最早的")
	assert.Equal(t, float64(HistoryLen+9), st.History[HistoryLen-1].CPU)
}

func TestCollector_AttemptStopped(t *testing.T) {
	c := &Collector{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Equal(t, outcomeStopped, c.attempt(ctx, &entry{id: "1"}, &fakeSource{}, true))
}

func TestClassify(t *testing.T) {
	state, msg, parked := classify(context.DeadlineExceeded, 10*time.Second)
	assert.Equal(t, [3]any{StateError, "10 秒内没有回应", false}, [3]any{state, msg, parked})
	state, _, parked = classify(errors.New("x"), time.Second)
	assert.Equal(t, StateError, state)
	assert.False(t, parked)
}

// 来源自己的密钥与长期凭据不出现在原因里：来源的错误里本不该有它们，采集器再挡一道。
func TestCollector_ScrubsOwnSecrets(t *testing.T) {
	secret, device := "hunter22", "device-token-1"
	cfg := Config{Sources: []SourceConfig{
		{ID: 1, Kind: "fk", Enabled: true, Secret: secret, Extra: map[string]string{"did": device}},
		{ID: 2, Kind: "fk", Enabled: true, Secret: secret},
		{ID: 3, Kind: "bad", Enabled: true, Secret: "abc"},
	}}
	r := newRig(t, cfg)
	r.c.Backoff = []time.Duration{time.Hour}
	r.next[1] = func() *fakeSource {
		return &fakeSource{fn: func(context.Context, bool, int) (*Snapshot, error) {
			return nil, fmt.Errorf("登录 %s 用 %s 失败", device, secret)
		}}
	}
	r.next[2] = func() *fakeSource {
		return &fakeSource{fn: func(context.Context, bool, int) (*Snapshot, error) {
			return nil, &AuthError{Reason: "密码 " + secret + " 不对"}
		}}
	}
	r.c.Factory = func(sc SourceConfig) (Source, error) {
		if sc.Kind == "bad" {
			return nil, errors.New("设置不对：" + sc.Secret)
		}
		return r.newSource(sc.ID), nil
	}
	r.c.Start(context.Background())
	assert.Equal(t, "登录 *** 用 *** 失败", r.waitState("1", StateError).Message)
	assert.Equal(t, "认证失败：密码 *** 不对", r.waitState("2", StateAuthFailed).Message)
	assert.Equal(t, "设置不对：abc", r.status("3").Message, "短于 4 个字的不换，免得误伤普通文字")
	assert.Equal(t, "a *** b", scrubSecrets("a token b", SourceConfig{Extra: map[string]string{"k": "token"}}))
}

// historySource 是第一次采集以后交出一次过去历史的来源（TrueNAS 的报表）。
type historySource struct {
	fakeSource
	once sync.Once
	past []Sample
}

func (h *historySource) TakeHistory() []Sample {
	var out []Sample
	h.once.Do(func() { out = h.past })
	return out
}

func TestCollector_FailingSince(t *testing.T) {
	now := time.Date(2026, 11, 6, 20, 0, 0, 0, time.UTC)
	c := &Collector{Now: func() time.Time { return now }}
	e := &entry{id: "1"}
	src := &fakeSource{}
	require.Equal(t, outcomeOK, c.attempt(context.Background(), e, src, true))
	assert.Nil(t, e.status().FailingSince, "连着的时候为空")

	src.setFn(func(context.Context, bool, int) (*Snapshot, error) { return nil, errors.New("connection refused") })
	require.Equal(t, outcomeRetryLater, c.attempt(context.Background(), e, src, false))
	first := now
	st := e.status()
	require.NotNil(t, st.FailingSince)
	assert.Equal(t, first, *st.FailingSince)

	now = now.Add(2 * time.Minute)
	src.setFn(func(context.Context, bool, int) (*Snapshot, error) { return nil, &AuthError{Reason: "Key 不对"} })
	require.Equal(t, outcomeParked, c.attempt(context.Background(), e, src, false))
	st = e.status()
	require.NotNil(t, st.FailingSince)
	assert.Equal(t, first, *st.FailingSince, "一直出错时记的是第一次出错的时刻")

	now = now.Add(time.Minute)
	src.setFn(nil)
	require.Equal(t, outcomeOK, c.attempt(context.Background(), e, src, true))
	assert.Nil(t, e.status().FailingSince, "连上以后清掉")
}

func TestCollector_HistoryBackfill(t *testing.T) {
	now := time.Date(2026, 11, 6, 20, 0, 0, 0, time.UTC)
	c := &Collector{Now: func() time.Time { return now }}
	e := &entry{id: "1"}
	src := &historySource{past: []Sample{
		{At: now.Add(-60 * time.Second), CPU: 2},
		{At: now.Add(-90 * time.Second), CPU: 1},
		{At: now.Add(-30 * time.Second), CPU: 3},
		{At: now.Add(-5 * time.Second), CPU: 9}, // 和这一拍挨得太近，不要
		{At: now.Add(time.Minute), CPU: 9},      // 比这一拍还新，不要
	}}
	src.setFn(func(_ context.Context, full bool, _ int) (*Snapshot, error) {
		s := snapFor(full, 4)
		s.At = now
		return s, nil
	})
	require.Equal(t, outcomeOK, c.attempt(context.Background(), e, src, true))
	cpus := func() []float64 {
		out := []float64{}
		for _, h := range e.status().History {
			out = append(out, h.CPU)
		}
		return out
	}
	assert.Equal(t, []float64{1, 2, 3, 4}, cpus(), "过去的几拍按时间排在这一拍前面")

	now = now.Add(30 * time.Second)
	require.Equal(t, outcomeOK, c.attempt(context.Background(), e, src, false))
	assert.Equal(t, []float64{1, 2, 3, 4, 4}, cpus(), "只补一次")
}

func TestCollector_HistoryBackfillCapped(t *testing.T) {
	now := time.Date(2026, 11, 6, 20, 0, 0, 0, time.UTC)
	c := &Collector{Now: func() time.Time { return now }}
	e := &entry{id: "1"}
	past := make([]Sample, 0, HistoryLen+5)
	for i := HistoryLen + 5; i >= 1; i-- {
		past = append(past, Sample{At: now.Add(-time.Duration(i) * 30 * time.Second), CPU: float64(i)})
	}
	src := &historySource{past: past}
	src.setFn(func(_ context.Context, full bool, _ int) (*Snapshot, error) {
		s := snapFor(full, 0)
		s.At = now
		return s, nil
	})
	require.Equal(t, outcomeOK, c.attempt(context.Background(), e, src, true))
	h := e.status().History
	require.Len(t, h, HistoryLen)
	assert.Equal(t, float64(HistoryLen-1), h[0].CPU, "多的丢掉最早的")
	assert.Equal(t, 0.0, h[HistoryLen-1].CPU, "最后一拍是这一次采集的")
}
