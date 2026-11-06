package nas

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ HistorySource = (*trueNAS)(nil)

func collectOK(t *testing.T, src *trueNAS, full bool) *Snapshot {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	snap, err := src.Collect(ctx, full)
	require.NoError(t, err)
	require.NotNil(t, snap)
	return snap
}

func TestTrueNAS_KindRegistered(t *testing.T) {
	k, ok := LookupKind(KindTrueNAS)
	require.True(t, ok)
	assert.Equal(t, "TrueNAS", k.Label)
	assert.Equal(t, UsernameNone, k.Username)
	assert.Equal(t, PlainNever, k.Plain)
	assert.False(t, k.Experimental)
	assert.True(t, k.RequiresSecret(""))
	_, err := ParseURL(KindTrueNAS, "http://192.168.1.10")
	require.Error(t, err, "TrueNAS 只收 https://")
	u, err := ParseURL(KindTrueNAS, "https://192.168.1.10:8443/")
	require.NoError(t, err)
	assert.Equal(t, "https://192.168.1.10:8443", u.String())
}

func TestTrueNAS_NewRejectsHTTPAndEmptyKey(t *testing.T) {
	_, err := newTrueNAS(SourceConfig{URL: "http://nas", Secret: "k"}, nil)
	require.ErrorIs(t, err, errTrueNASHTTP)
	_, err = newTrueNAS(SourceConfig{URL: "https://nas", Secret: "  "}, nil)
	require.ErrorIs(t, err, errTrueNASNoKey)
	_, err = newTrueNAS(SourceConfig{URL: "https://nas:bad", Secret: "k"}, nil)
	require.Error(t, err)
	k, _ := LookupKind(KindTrueNAS)
	src, err := k.New(SourceConfig{URL: "http://nas", Secret: "k"})
	require.Error(t, err)
	assert.Nil(t, src, "出错时不能返回带类型的空指针")
	src, err = k.New(SourceConfig{URL: "https://nas", Secret: "k"})
	require.NoError(t, err)
	require.NoError(t, src.Close())
	_, err = dialRPC(context.Background(), &url.URL{Scheme: "http", Host: "nas"}, "", nil, "k")
	require.ErrorIs(t, err, errTrueNASHTTP)
}

func TestTrueNAS_CollectFull(t *testing.T) {
	f := newFakeTrueNAS(t)
	snap := collectOK(t, f.source(fakeTrueNASKey), true)

	assert.Equal(t, System{Hostname: "nas", Platform: "TrueNAS", Version: "25.04.1", Model: "Intel(R) Core(TM) i5-12400", Cores: 12, UptimeSeconds: 90061}, snap.System)
	assert.InDelta(t, 12.5, snap.CPU.Percent, 1e-9)
	assert.Equal(t, [3]float64{1.5, 1.25, 1}, [3]float64{snap.CPU.Load1, snap.CPU.Load5, snap.CPU.Load15})
	assert.Nil(t, snap.CPU.TempC, "温度全是 0 是读不到")
	assert.Nil(t, snap.CPU.IOWait)
	assert.Equal(t, Memory{Total: 64 << 30, Used: 48 << 30, Available: 16 << 30}, snap.Memory)
	// 两块连着的物理网卡（不算断开的 enp3s0 与网桥 br0），千比特/秒 × 125
	assert.Equal(t, uint64(1000*125), snap.Network.RxBps)
	assert.Equal(t, uint64(100*125), snap.Network.TxBps)
	assert.Equal(t, []string{"enp1s0", "enp2s0"}, snap.Network.Interfaces)

	require.Len(t, snap.Pools, 2)
	uni, cold := snap.Pools[0], snap.Pools[1]
	assert.Equal(t, "UniData", uni.Name)
	assert.Equal(t, "ONLINE", uni.Status)
	require.NotNil(t, uni.Healthy)
	assert.True(t, *uni.Healthy)
	assert.Equal(t, uint64(32_000_000_000_000), uni.Size, "容量按根数据集的 used + available")
	assert.Equal(t, uint64(27_000_000_000_000), uni.Used)
	assert.Equal(t, uint64(5_000_000_000_000), uni.Free)
	assert.Equal(t, 2, uni.Disks)
	assert.Equal(t, Pool{Name: "Cold", Status: "DEGRADED", Healthy: cold.Healthy, Size: 4_000_000_000_000, Used: 1_000_000_000_000, Free: 3_000_000_000_000, Disks: 1}, cold, "没有根数据集时按裸容量")
	require.NotNil(t, cold.Healthy)
	assert.False(t, *cold.Healthy)
	assert.Equal(t, Facet{Status: FacetOK, Reason: "存储池 Cold 读不到根数据集，容量按裸容量算（含校验盘）"}, snap.Facets[FacetPools])

	require.Len(t, snap.Disks, 4)
	assert.Equal(t, Disk{Name: "sda", Model: "Virtual_disk", Kind: DiskSSD, Size: 50_000_000_000, Pool: "boot-pool"}, snap.Disks[0], "温度 0 是读不到")
	assert.Equal(t, DiskHDD, snap.Disks[1].Kind)
	require.NotNil(t, snap.Disks[2].TempC)
	assert.InDelta(t, 38.5, *snap.Disks[2].TempC, 1e-9)
	assert.Equal(t, DiskNVMe, snap.Disks[3].Kind)
	require.NotNil(t, snap.Disks[3].TempC)
	assert.InDelta(t, 61, *snap.Disks[3].TempC, 1e-9)
	raw, err := json.Marshal(snap)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "SN-SHOULD-NOT-LEAK", "不要序列号")

	require.Len(t, snap.Alerts, 4, "被忽略的告警不要")
	assert.Equal(t, AlertWarning, snap.Alerts[0].Level)
	assert.Equal(t, "存储池 UniData 已用 85%", snap.Alerts[0].Title)
	require.NotNil(t, snap.Alerts[0].At)
	assert.Equal(t, int64(1_799_990_000), snap.Alerts[0].At.Unix())
	assert.Equal(t, Alert{Level: AlertCritical, Title: "Rsync 拉取任务失败：/mnt/UniData/backup", At: snap.Alerts[1].At}, snap.Alerts[1])
	assert.Equal(t, AlertCritical, snap.Alerts[2].Level, "ERROR 算严重")
	assert.Equal(t, "Something new happened.", snap.Alerts[2].Title, "不认得的告警用 TrueNAS 的说明：去掉标签，只要第一行")
	assert.Equal(t, Alert{Level: AlertInfo, Title: "TrueNAS 有新版本可以更新"}, snap.Alerts[3])

	assert.Equal(t, Apps{Running: 2, Total: 3, Upgradable: 2}, snap.Apps)

	for _, k := range FacetKeys {
		want := FacetOK
		if k == FacetCPUTemp {
			want = FacetUnsupported
		}
		assert.Equal(t, want, snap.Facets[k].Status, k)
	}
	assert.Contains(t, snap.Facets[FacetCPUTemp].Reason, "虚拟机")
	assert.True(t, snap.At.IsZero(), "时间由采集器按本机时钟填")
}

func TestTrueNAS_ReportingWindowFollowsServerClock(t *testing.T) {
	f := newFakeTrueNAS(t)
	src := f.source(fakeTrueNASKey)
	src.now = func() time.Time { return f.server.Add(7 * time.Minute) } // 本机比 TrueNAS 快 7 分钟
	collectOK(t, src, true)
	end := f.server.Unix() - trueNASLag
	f.mu.Lock()
	windows := append([][2]int64(nil), f.windows...)
	f.mu.Unlock()
	assert.Contains(t, windows, [2]int64{end - trueNASWindow, end}, "报表的时间按 TrueNAS 的时钟")
	assert.Contains(t, windows, [2]int64{end - trueNASHistory, end}, "第一次采集补 30 分钟")
}

func TestTrueNAS_LightCollectOnlyLightCalls(t *testing.T) {
	f := newFakeTrueNAS(t)
	src := f.source(fakeTrueNASKey)
	collectOK(t, src, true)
	f.resetCalls()
	snap := collectOK(t, src, false)
	assert.ElementsMatch(t, []string{"system.info", "reporting.get_data", "reporting.get_data", "reporting.get_data"}, f.callList(),
		"轻量采集只读主机、CPU、内存、温度与网卡（网卡列表沿用上一次完整采集的）")
	assert.Equal(t, uint64(1000*125), snap.Network.RxBps)
	assert.Empty(t, snap.Pools)
	_, hasPools := snap.Facets[FacetPools]
	assert.False(t, hasPools, "轻量采集不写存储池（采集器沿用上一次完整采集的）")
	f.mu.Lock()
	windows := append([][2]int64(nil), f.windows...)
	f.mu.Unlock()
	for _, w := range windows {
		assert.Equal(t, int64(trueNASWindow), w[1]-w[0], "历史只补一次")
	}
}

func TestTrueNAS_LoginOncePerConnection(t *testing.T) {
	f := newFakeTrueNAS(t)
	src := f.source(fakeTrueNASKey)
	collectOK(t, src, true)
	collectOK(t, src, false)
	collectOK(t, src, true)
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 1, f.logins)
	assert.Equal(t, 1, f.upgrades)
}

func TestTrueNAS_WrongKeyIsAuthError(t *testing.T) {
	f := newFakeTrueNAS(t)
	wrong := "1-wrongKeyThatMustNotAppearInErrors"
	_, err := f.source(wrong).Collect(context.Background(), true)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAuth)
	var ae *AuthError
	require.ErrorAs(t, err, &ae)
	assert.Contains(t, ae.Reason, "吊销")
	assert.NotContains(t, err.Error(), wrong)
	assert.Equal(t, []string{"auth.login_with_api_key"}, f.callList(), "登录失败以后什么都不读")
}

func TestTrueNAS_UntrustedCertSendsNothing(t *testing.T) {
	f := newFakeTrueNAS(t)
	src, err := newTrueNAS(SourceConfig{URL: f.ts.URL, Secret: fakeTrueNASKey}, nil)
	require.NoError(t, err)
	_, err = src.Collect(context.Background(), true)
	require.ErrorIs(t, err, ErrCertUntrusted)
	var ce *CertError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, f.pin(), ce.Cert.SHA256)
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 0, f.requests, "证书不认时连 WebSocket 请求都没发，更不会发 Key")
	assert.Equal(t, 0, f.logins)
}

func TestTrueNAS_PinChangedAndTrustedRoots(t *testing.T) {
	f := newFakeTrueNAS(t)
	src, err := newTrueNAS(SourceConfig{URL: f.ts.URL, Secret: fakeTrueNASKey, PinSHA256: strings.Repeat("ab", 32)}, nil)
	require.NoError(t, err)
	_, err = src.Collect(context.Background(), true)
	require.ErrorIs(t, err, ErrCertChanged)

	roots := x509.NewCertPool()
	roots.AddCert(f.ts.Certificate())
	src, err = newTrueNAS(SourceConfig{URL: f.ts.URL, Secret: fakeTrueNASKey}, roots)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	collectOK(t, src, true)
}

func TestTrueNAS_DeniedAndMissingFacets(t *testing.T) {
	f := newFakeTrueNAS(t)
	f.fail("alert.list", errFakeDenied)
	f.fail("app.query", errFakeMissing)
	f.fail("disk.temperatures", errFakeDenied)
	f.fail("interface.query", errFakeDenied)
	f.fail("pool.dataset.query", errFakeDenied)
	src := f.source(fakeTrueNASKey)
	snap := collectOK(t, src, true)
	assert.Equal(t, Facet{Status: FacetDenied, Reason: "这个 API Key 的账号没有读告警的权限"}, snap.Facets[FacetAlerts])
	assert.Equal(t, Facet{Status: FacetUnsupported, Reason: "这个版本的 TrueNAS 读不到应用"}, snap.Facets[FacetApps])
	assert.Equal(t, FacetDenied, snap.Facets[FacetNetwork].Status)
	assert.Equal(t, FacetOK, snap.Facets[FacetDisks].Status, "读不到温度时硬盘照样列出")
	for _, d := range snap.Disks {
		assert.Nil(t, d.TempC)
	}
	require.Len(t, snap.Pools, 2)
	assert.Equal(t, uint64(48_000_000_000_000), snap.Pools[0].Size, "读不到根数据集时按裸容量")
	assert.Equal(t, FacetOK, snap.Facets[FacetCPU].Status)

	light := collectOK(t, src, false)
	assert.Equal(t, FacetDenied, light.Facets[FacetNetwork].Status, "轻量采集沿用读网卡列表的结果")
}

func TestTrueNAS_NoPhysicalInterface(t *testing.T) {
	f := newFakeTrueNAS(t)
	f.handle("interface.query", func([]json.RawMessage) (any, *fakeRPCError) {
		return []map[string]any{{"name": "br0", "type": "BRIDGE", "state": map[string]any{"link_state": "LINK_STATE_UP"}}}, nil
	})
	snap := collectOK(t, f.source(fakeTrueNASKey), true)
	assert.Equal(t, FacetUnsupported, snap.Facets[FacetNetwork].Status)
	assert.Equal(t, 3, f.count("reporting.get_data"), "不查网卡的报表：只有 CPU 与内存、温度、历史三次")
}

func TestTrueNAS_ReportingErrors(t *testing.T) {
	f := newFakeTrueNAS(t)
	f.series = func(graph, ident, col string, ts int64) *float64 { return nil }
	snap := collectOK(t, f.source(fakeTrueNASKey), true)
	assert.Equal(t, FacetError, snap.Facets[FacetCPU].Status)
	assert.Equal(t, FacetError, snap.Facets[FacetMemory].Status)
	assert.Equal(t, FacetError, snap.Facets[FacetNetwork].Status)
	assert.Equal(t, FacetUnsupported, snap.Facets[FacetCPUTemp].Status)
	assert.Equal(t, uint64(64<<30), snap.Memory.Total, "总内存来自 system.info")

	f2 := newFakeTrueNAS(t)
	f2.fail("reporting.get_data", errFakeDenied)
	snap = collectOK(t, f2.source(fakeTrueNASKey), true)
	assert.Equal(t, FacetDenied, snap.Facets[FacetCPU].Status)
	assert.Equal(t, FacetDenied, snap.Facets[FacetMemory].Status)
	assert.Equal(t, FacetDenied, snap.Facets[FacetCPUTemp].Status)
	assert.Equal(t, FacetDenied, snap.Facets[FacetNetwork].Status)
}

func TestTrueNAS_MeansWithoutAggregations(t *testing.T) {
	f := newFakeTrueNAS(t)
	f.noAggregations = true
	cpuAt := func(ts int64) *float64 {
		if ts%3 == 0 {
			return nil // 空值跳过
		}
		x := float64(ts % 2 * 20)
		return &x
	}
	end := f.server.Unix() - trueNASLag
	var sum float64
	n := 0
	for ts := end - trueNASWindow; ts <= end; ts++ {
		if v := cpuAt(ts); v != nil {
			sum += *v
			n++
		}
	}
	f.series = func(graph, ident, col string, ts int64) *float64 {
		v := f.graphValue(graph, ident, col, ts)
		if graph == "cpu" && col == "cpu" {
			return cpuAt(ts)
		}
		if graph == "cputemp" {
			x := 47.0
			if col == "cpu" {
				return nil
			}
			if col == "cpu1" {
				x = 52
			}
			return &x
		}
		return v
	}
	snap := collectOK(t, f.source(fakeTrueNASKey), true)
	assert.InDelta(t, sum/float64(n), snap.CPU.Percent, 1e-9, "没有 aggregations 时按各行算，跳过空值")
	require.NotNil(t, snap.CPU.TempC)
	assert.InDelta(t, 52, *snap.CPU.TempC, 1e-9, "没有总的一列时取各核里最高的")
	assert.Equal(t, FacetOK, snap.Facets[FacetCPUTemp].Status)
}

func TestTrueNAS_SystemInfoDeniedParks(t *testing.T) {
	f := newFakeTrueNAS(t)
	f.fail("system.info", errFakeDenied)
	_, err := f.source(fakeTrueNASKey).Collect(context.Background(), true)
	var ae *AuthError
	require.ErrorAs(t, err, &ae, "账号没有只读权限：停住，等改了权限点「重新连接」")
	assert.Contains(t, ae.Reason, "READONLY_ADMIN")
	state, _, parked := classify(err, time.Second)
	assert.Equal(t, StateAuthFailed, state)
	assert.True(t, parked)
}

func TestTrueNAS_ReconnectAfterDrop(t *testing.T) {
	f := newFakeTrueNAS(t)
	src := f.source(fakeTrueNASKey)
	// 第一条连接回完第一次完整采集的全部回复以后断开
	f.dropAfter = 1 + 7 + 6
	collectOK(t, src, true)
	require.Eventually(t, func() bool {
		src.mu.Lock()
		defer src.mu.Unlock()
		return src.conn == nil || src.conn.broken()
	}, 5*time.Second, 10*time.Millisecond)
	collectOK(t, src, false)
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 2, f.upgrades, "断了以后重连")
	assert.Equal(t, 2, f.logins, "新连接重新登录")
}

func TestTrueNAS_DropMidCollectReturnsErrorThenRecovers(t *testing.T) {
	f := newFakeTrueNAS(t)
	src := f.source(fakeTrueNASKey)
	f.dropAfter = 2 // 登录与 system.info 之后就断
	_, err := src.Collect(context.Background(), true)
	require.Error(t, err)
	assert.True(t, transportError(err), err.Error())
	src.mu.Lock()
	assert.Nil(t, src.conn, "出错以后断开，下次重连")
	src.mu.Unlock()
	collectOK(t, src, true)
}

func TestTrueNAS_UnauthenticatedReconnects(t *testing.T) {
	f := newFakeTrueNAS(t)
	src := f.source(fakeTrueNASKey)
	collectOK(t, src, true)
	f.fail("system.info", errFakeNoAuth)
	_, err := src.Collect(context.Background(), false)
	require.ErrorIs(t, err, errTrueNASReauth)
	_, _, parked := classify(err, time.Second)
	assert.False(t, parked, "登录失效了按网络错误重试，下次重新登录")
	f.defaults()
	collectOK(t, src, false)
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 2, f.logins)
}

func TestTrueNAS_ContextTimeout(t *testing.T) {
	f := newFakeTrueNAS(t)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	f.handle("alert.list", func([]json.RawMessage) (any, *fakeRPCError) {
		<-release
		return []any{}, nil
	})
	src := f.source(fakeTrueNASKey)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := src.Collect(ctx, true)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	src.mu.Lock()
	defer src.mu.Unlock()
	assert.Nil(t, src.conn)
}

func TestTrueNAS_HandshakeStatus(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   string
	}{
		{http.StatusNotFound, "25.04"},
		{http.StatusOK, "25.04"},
		{http.StatusFound, "重定向"},
		{http.StatusBadGateway, "HTTP 502"},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			f := newFakeTrueNAS(t)
			f.status = tc.status
			_, err := f.source(fakeTrueNASKey).Collect(context.Background(), true)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Equal(t, 0, f.logins)
		})
	}
}

func TestTrueNAS_ConnectionRefused(t *testing.T) {
	f := newFakeTrueNAS(t)
	u := f.url()
	f.ts.Close()
	src, err := newTrueNAS(SourceConfig{URL: u.String(), Secret: fakeTrueNASKey, PinSHA256: f.pin()}, nil)
	require.NoError(t, err)
	_, err = src.Collect(context.Background(), true)
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "连不上 TrueNAS："), err.Error())
	assert.NotContains(t, err.Error(), "failed to WebSocket dial", "不带 coder/websocket 套的前缀")
}

func TestTrueNAS_ErrorsDropTheKey(t *testing.T) {
	f := newFakeTrueNAS(t)
	f.fail("system.info", &fakeRPCError{Code: -32001, Message: "Method call error", Data: map[string]any{"error": 22, "errname": "EINVAL", "reason": "bad key " + fakeTrueNASKey + "\nsecond line"}})
	_, err := f.source(fakeTrueNASKey).Collect(context.Background(), true)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), fakeTrueNASKey)
	assert.NotContains(t, err.Error(), "second line")
	assert.Contains(t, err.Error(), "system.info")
}

func TestTrueNAS_LargeReply(t *testing.T) {
	f := newFakeTrueNAS(t)
	f.handle("alert.list", func([]json.RawMessage) (any, *fakeRPCError) {
		list := make([]map[string]any, 3000)
		for i := range list {
			list[i] = map[string]any{"klass": "SomethingNew", "level": "INFO", "formatted": strings.Repeat("x", 40)}
		}
		return list, nil
	})
	snap := collectOK(t, f.source(fakeTrueNASKey), true)
	assert.Len(t, snap.Alerts, 3000, "回复比 32 KiB 大")
}

func TestTrueNAS_HistoryBackfill(t *testing.T) {
	f := newFakeTrueNAS(t)
	end := f.server.Unix() - trueNASLag
	start := end - trueNASHistory
	f.series = func(graph, ident, col string, ts int64) *float64 {
		k := (ts - start) / trueNASStep
		switch {
		case graph == "cpu" && col == "cpu":
			x := float64(k) // 第 k 拍的 CPU 是 k
			return &x
		case graph == "memory":
			if k == 3 {
				return nil // 第 3 拍没有内存数据
			}
			x := float64(48 << 30)
			return &x
		}
		return f.graphValue(graph, ident, col, ts)
	}
	src := f.source(fakeTrueNASKey)
	src.now = func() time.Time { return f.server.Add(time.Minute) } // 本机比 TrueNAS 快 1 分钟
	collectOK(t, src, true)
	h := src.TakeHistory()
	require.Len(t, h, HistoryLen-1, "没有内存数据的那一拍不要")
	assert.Equal(t, time.Unix(start+trueNASStep, 0).Add(time.Minute), h[0].At, "时间换成本机的时钟")
	assert.InDelta(t, 0, h[0].CPU, 1e-9)
	assert.InDelta(t, 4, h[3].CPU, 1e-9, "第 3 拍跳过了")
	last := h[len(h)-1]
	assert.Equal(t, time.Unix(end, 0).Add(time.Minute), last.At)
	assert.InDelta(t, float64(HistoryLen-1), last.CPU, 1e-9)
	assert.InDelta(t, 25, last.Mem, 1e-9, "64 GiB 里可用 48 GiB")
	assert.Equal(t, uint64(1000*125), last.Rx, "两块网卡加起来")
	assert.Equal(t, uint64(100*125), last.Tx)
	assert.Nil(t, src.TakeHistory(), "只交一次")

	f.resetCalls()
	collectOK(t, src, true)
	assert.Nil(t, src.TakeHistory(), "每个来源只补一次")
}

func TestTrueNAS_HistoryFailureIsQuiet(t *testing.T) {
	f := newFakeTrueNAS(t)
	f.handle("reporting.get_data", func(params []json.RawMessage) (any, *fakeRPCError) {
		var opts struct{ Start, End int64 }
		_ = json.Unmarshal(params[1], &opts)
		if opts.End-opts.Start == trueNASHistory {
			return nil, errFakeDenied
		}
		return f.reporting(params)
	})
	src := f.source(fakeTrueNASKey)
	snap := collectOK(t, src, true)
	assert.Equal(t, FacetOK, snap.Facets[FacetCPU].Status)
	assert.Nil(t, src.TakeHistory(), "补历史没成功不影响这一次采集")
}

func TestTrueNAS_Test(t *testing.T) {
	f := newFakeTrueNAS(t)
	ctx := context.Background()
	res, err := testTrueNAS(ctx, TestInput{URL: f.url(), Secret: fakeTrueNASKey, PinSHA256: f.pin()}, nil)
	require.NoError(t, err)
	assert.Equal(t, "nas", res.System.Hostname)
	assert.Equal(t, "25.04.1", res.System.Version)
	assert.Nil(t, res.Extra)

	_, err = testTrueNAS(ctx, TestInput{URL: f.url(), Secret: "wrong-key", PinSHA256: f.pin()}, nil)
	assert.ErrorIs(t, err, ErrAuth)
	_, err = testTrueNAS(ctx, TestInput{URL: f.url(), Secret: fakeTrueNASKey}, nil)
	assert.ErrorIs(t, err, ErrCertUntrusted)
	_, err = testTrueNAS(ctx, TestInput{URL: f.url(), Secret: " "}, nil)
	assert.ErrorIs(t, err, errTrueNASNoKey)
	_, err = testTrueNAS(ctx, TestInput{Secret: fakeTrueNASKey}, nil)
	assert.Error(t, err)
	f.fail("system.info", errFakeDenied)
	_, err = testTrueNAS(ctx, TestInput{URL: f.url(), Secret: fakeTrueNASKey, PinSHA256: f.pin()}, nil)
	assert.ErrorIs(t, err, ErrAuth)
}

func TestTrueNASAlertTitle(t *testing.T) {
	for _, tc := range []struct {
		alert tnAlert
		want  string
	}{
		{tnAlert{Klass: "ZpoolCapacityCritical", Args: map[string]any{"volume": "tank", "capacity": 95.0}}, "存储池 tank 已用 95%"},
		{tnAlert{Klass: "ZpoolCapacityNotice", Args: map[string]any{"volume": "tank"}, Formatted: "Space usage"}, "Space usage"},
		{tnAlert{Klass: "RsyncSuccess", Args: map[string]any{"direction": "PUSH", "path": "/mnt/a"}}, "Rsync 推送任务完成：/mnt/a"},
		{tnAlert{Klass: "RsyncFailed", Args: map[string]any{"path": "/mnt/a"}}, "Rsync 任务失败：/mnt/a"},
		{tnAlert{Klass: "AppUpdate", Args: map[string]any{"name": "plex"}}, "应用 plex 有新版本"},
		{tnAlert{Klass: "AppUpdate", Args: []any{"plex"}, Formatted: "An update is available"}, "An update is available"},
		{tnAlert{Klass: "CatalogSyncFailed", Formatted: "Failed to sync TRUENAS catalog"}, "应用目录同步失败"},
		{tnAlert{Klass: "VolumeStatus", Args: map[string]any{"volume": "tank", "state": "DEGRADED"}}, "存储池 tank 状态不健康（DEGRADED）"},
		{tnAlert{Klass: "VolumeStatus"}, "存储池状态不健康"},
		{tnAlert{Klass: "SMART", Formatted: "Device: /dev/sdb [SAT], 8 Currently unreadable (pending) sectors.\nmore"}, "S.M.A.R.T. 报错：Device: /dev/sdb [SAT], 8 Currently unreadable (pending) sectors."},
		{tnAlert{Klass: "ScrubPaused"}, "存储池校验暂停了"},
		{tnAlert{Klass: "Weird", Formatted: "  "}, "Weird"},
		{tnAlert{Klass: "Long", Formatted: strings.Repeat("长", 130)}, strings.Repeat("长", 120) + "…"},
		{tnAlert{Klass: "Token", Formatted: "token=ghp_abcdefghijklmnopqrstuvwxyz0123456789"}, ""},
	} {
		got := trueNASAlertTitle(tc.alert)
		if tc.alert.Klass == "Token" {
			assert.NotContains(t, got, "ghp_abcdefghijklmnopqrstuvwxyz0123456789", "说明里的凭证要遮掉")
			continue
		}
		assert.Equal(t, tc.want, got, tc.alert.Klass)
	}
	long := trueNASAlertTitle(tnAlert{Klass: "ReplicationFailed", Formatted: strings.Repeat("长", 130)})
	assert.True(t, strings.HasPrefix(long, "复制任务失败："), long)
	assert.Len(t, []rune(long), 121, "连同中文名一起截到 120 个字")
	assert.Equal(t, AlertCritical, alertLevel("EMERGENCY"))
	assert.Equal(t, AlertCritical, alertLevel("ALERT"))
	assert.Equal(t, AlertInfo, alertLevel("NOTICE"))
	assert.Equal(t, DiskUnknown, diskKind("xvda", ""))
}

func TestRPCErrorText(t *testing.T) {
	e := &rpcError{Code: -32001, Message: "Method call error"}
	assert.Equal(t, "TrueNAS 返回错误：Method call error", e.Error())
	e.method = "pool.query"
	assert.Equal(t, "TrueNAS 的 pool.query 返回错误：Method call error", e.Error())
	assert.Equal(t, "TrueNAS 返回错误：错误码 -32000", (&rpcError{Code: -32000}).Error())
	long := &rpcError{Code: 1, Message: strings.Repeat("出错 ", 100)}
	assert.True(t, strings.HasSuffix(long.Error(), "…"))
	assert.False(t, (&rpcError{Code: -32001}).denied())
	assert.True(t, (&rpcError{Code: -32601}).missing())
	assert.False(t, transportError(nil))
	assert.False(t, transportError(&decodeError{method: "m", err: errors.New("x")}))
	assert.Contains(t, (&decodeError{method: "m", err: errors.New("x")}).Error(), "看不懂")
	assert.True(t, transportError(errors.New("boom")))
	assert.Nil(t, firstTransportError(nil, &rpcCall{err: &rpcError{}}))
	timeout := &rpcCall{err: fmt.Errorf("x: %w", context.DeadlineExceeded)}
	assert.Equal(t, timeout.err, firstTransportError(&rpcCall{err: errors.New("closed")}, timeout), "超时排在前面")
}

func TestRPCConn_IgnoresNotificationsAndUnknownIDs(t *testing.T) {
	f := newFakeTrueNAS(t)
	// 回复之前先推一条通知与一条对不上的回复
	f.handle("system.info", func([]json.RawMessage) (any, *fakeRPCError) {
		f.mu.Lock()
		ws := f.conns[len(f.conns)-1]
		f.mu.Unlock()
		ctx := context.Background()
		_ = ws.Write(ctx, websocket.MessageText, []byte(`{"jsonrpc":"2.0","method":"collection_update","params":{}}`))
		_ = ws.Write(ctx, websocket.MessageText, []byte(`{"jsonrpc":"2.0","id":999999,"result":true}`))
		_ = ws.Write(ctx, websocket.MessageText, []byte(`not json`))
		return map[string]any{"hostname": "nas", "physmem": 1 << 30, "datetime": map[string]any{"$date": f.server.UnixMilli()}}, nil
	})
	c, err := dialRPC(context.Background(), f.url(), f.pin(), nil, fakeTrueNASKey)
	require.NoError(t, err)
	t.Cleanup(c.close)
	require.NoError(t, c.login(context.Background(), fakeTrueNASKey))
	var info tnSystemInfo
	require.NoError(t, c.call(context.Background(), "system.info", nil, &info))
	assert.Equal(t, "nas", info.Hostname)
	var bad []string
	err = c.call(context.Background(), "system.info", nil, &bad)
	var de *decodeError
	require.ErrorAs(t, err, &de, "结果的样子不对")
	assert.False(t, c.broken(), "看不懂的回复不影响连接")
	c.close()
	assert.True(t, c.broken())
	assert.Error(t, c.call(context.Background(), "system.info", nil, nil), "关了以后不能再用")
}

func TestTnGraph(t *testing.T) {
	v := func(x float64) *float64 { return &x }
	g := tnGraph{Legend: []string{"time", "a"}, Data: [][]*float64{{v(1), v(2)}, {v(2), nil}, {v(3), v(4)}, {v(4)}}}
	m, ok := g.mean("a")
	require.True(t, ok)
	assert.InDelta(t, 3, m, 1e-9)
	l, ok := g.last("a")
	require.True(t, ok)
	assert.InDelta(t, 4, l, 1e-9)
	_, ok = g.mean("b")
	assert.False(t, ok)
	_, ok = g.last("b")
	assert.False(t, ok)
	_, ok = tnGraph{Legend: []string{"time", "a"}}.mean("a")
	assert.False(t, ok)
	_, ok = tnGraph{Legend: []string{"time", "a"}}.last("a")
	assert.False(t, ok)
	assert.Equal(t, tnGraph{}, findGraph(nil, "cpu"))
	assert.Equal(t, "24.10.2", tnSystemInfo{Version: "TrueNAS-24.10.2"}.system().Version)
	assert.Nil(t, historySamples(nil, nil, 0, 100, 0), "不知道总内存时不补")
	assert.Equal(t, []float64{}, bucketMeans(tnGraph{Legend: []string{"time", "x"}, Data: [][]*float64{{v(-50), v(1)}}}, "x", 0, 0))
}
