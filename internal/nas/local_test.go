package nas

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeProc 在临时目录里放一套假的 /proc 与 /sys，返回目录。
func fakeProc(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, body := range files {
		writeFile(t, root, p, body)
	}
	return root
}

func writeFile(t *testing.T, root, p, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(p))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
}

const (
	stat1 = "cpu  100 0 100 700 100 0 0 0 0 0\ncpu0 1 2 3 4 5 6 7 8\nintr 1\n"
	// 与 stat1 相比：总数多 200，其中空闲 100、IO 等待 20，忙 80
	stat2 = "cpu  140 0 140 800 120 0 0 0 30 0\ncpu0 1 2 3 4 5 6 7 8\n"
	// 再过一拍：总数多 100，空闲 90、IO 等待 0
	stat3     = "cpu  145 0 145 890 120 0 0 0 30 0\n"
	meminfo   = "MemTotal:        8000000 kB\nMemFree:         1000000 kB\nMemAvailable:    6000000 kB\nBuffers:          100000 kB\nCached:          2000000 kB\n"
	cpuinfo86 = "processor\t: 0\nmodel name\t: Intel(R) Celeron(R) N5105 @ 2.00GHz\n\nprocessor\t: 1\nmodel name\t: Intel(R) Celeron(R) N5105 @ 2.00GHz\n\nprocessor\t: 2\n\nprocessor\t: 3\n"
)

func baseProc() map[string]string {
	return map[string]string{
		"/proc/stat":                          stat1,
		"/proc/meminfo":                       meminfo,
		"/proc/loadavg":                       "0.52 0.58 0.61 1/1234 5678\n",
		"/proc/uptime":                        "3725.67 1200.00\n",
		"/proc/cpuinfo":                       cpuinfo86,
		"/proc/sys/kernel/osrelease":          "6.6.44-production+truenas\n",
		"/proc/sys/kernel/hostname":           "truenas\n",
		"/sys/class/hwmon/hwmon0/name":        "acpitz\n",
		"/sys/class/hwmon/hwmon0/temp1_input": "27800\n",
		"/sys/class/hwmon/hwmon1/name":        "coretemp\n",
		"/sys/class/hwmon/hwmon1/temp1_input": "52000\n",
		"/sys/class/hwmon/hwmon1/temp1_label": "Package id 0\n",
		"/sys/class/hwmon/hwmon1/temp2_input": "55000\n",
		"/sys/class/hwmon/hwmon1/temp2_label": "Core 0\n",
	}
}

func TestLocal_FirstCollectTakesTwoReadings(t *testing.T) {
	root := fakeProc(t, baseProc())
	l := NewLocal(root)
	waited := time.Duration(0)
	l.Wait = func(_ context.Context, d time.Duration) error {
		waited = d
		writeFile(t, root, "/proc/stat", stat2)
		return nil
	}
	s, err := l.Collect(context.Background(), true)
	require.NoError(t, err)
	assert.Equal(t, localFirstGap, waited, "第一次采集隔一小会儿再读一次 /proc/stat")
	assert.InDelta(t, 40.0, s.CPU.Percent, 0.01, "忙 80 / 总数 200")
	require.NotNil(t, s.CPU.IOWait)
	assert.InDelta(t, 10.0, *s.CPU.IOWait, 0.01)
	assert.Equal(t, [3]float64{0.52, 0.58, 0.61}, [3]float64{s.CPU.Load1, s.CPU.Load5, s.CPU.Load15})
	require.NotNil(t, s.CPU.TempC)
	assert.InDelta(t, 52.0, *s.CPU.TempC, 0.01, "用 Package id 0 那一路，不取最高的 Core 0")

	assert.Equal(t, Memory{Total: 8000000 * 1024, Used: 2000000 * 1024, Available: 6000000 * 1024}, s.Memory)
	assert.InDelta(t, 25.0, s.Memory.MemPercent(), 0.01)
	assert.Equal(t, System{
		Hostname: "truenas", Platform: "Linux", Version: "6.6.44-production+truenas",
		Model: "Intel(R) Celeron(R) N5105 @ 2.00GHz", Cores: 4, UptimeSeconds: 3725,
	}, s.System)
	assert.False(t, s.At.IsZero())

	for _, k := range []string{FacetSystem, FacetCPU, FacetCPUTemp, FacetMemory} {
		assert.Equal(t, FacetOK, s.Facets[k].Status, k)
	}
	for _, k := range []string{FacetNetwork, FacetPools, FacetDisks, FacetAlerts, FacetApps} {
		assert.Equal(t, FacetUnsupported, s.Facets[k].Status, k)
		assert.NotEmpty(t, s.Facets[k].Reason, k)
	}
	assert.Contains(t, s.Facets[FacetNetwork].Reason, "容器里的网卡计数只有容器自己的流量")
	assert.Contains(t, s.Facets[FacetPools].Reason, "读不到存储池健康、硬盘与告警")
	assert.NotNil(t, s.Pools)
	assert.NotNil(t, s.Disks)
	assert.NotNil(t, s.Alerts)

	// 第二次用上一拍，不再等
	writeFile(t, root, "/proc/stat", stat3)
	l.Wait = func(context.Context, time.Duration) error {
		t.Fatal("第二次采集不该再等")
		return nil
	}
	s2, err := l.Collect(context.Background(), false)
	require.NoError(t, err)
	assert.InDelta(t, 10.0, s2.CPU.Percent, 0.01)
	assert.InDelta(t, 0.0, *s2.CPU.IOWait, 0.01)
	assert.NoError(t, l.Close())
}

func TestLocal_OldKernelAndNoSensors(t *testing.T) {
	files := baseProc()
	files["/proc/meminfo"] = "MemTotal: 4000 kB\nMemFree: 1000 kB\nBuffers: 500 kB\nCached: 500 kB\n"
	files["/proc/cpuinfo"] = "processor\t: 0\nprocessor\t: 1\nHardware\t: BCM2835\nModel\t\t: Raspberry Pi 4 Model B Rev 1.4\n"
	for p := range files {
		if filepath.Dir(p) == "/sys/class/hwmon/hwmon1" {
			delete(files, p)
		}
	}
	root := fakeProc(t, files)
	l := NewLocal(root)
	l.Wait = func(context.Context, time.Duration) error { return nil }
	s, err := l.Collect(context.Background(), true)
	require.NoError(t, err)
	assert.Equal(t, Memory{Total: 4000 * 1024, Used: 2000 * 1024, Available: 2000 * 1024}, s.Memory, "没有 MemAvailable 时用 MemFree + Buffers + Cached")
	assert.Equal(t, "Raspberry Pi 4 Model B Rev 1.4", s.System.Model)
	assert.Equal(t, 2, s.System.Cores)
	assert.Nil(t, s.CPU.TempC, "acpitz 不算 CPU 温度")
	assert.Equal(t, Facet{Status: FacetUnsupported, Reason: localTempReason}, s.Facets[FacetCPUTemp])
	assert.Equal(t, 0.0, s.CPU.Percent, "两次读到一样的数（总数没变）")
}

func TestLocal_HwmonWithoutLabelUsesMax(t *testing.T) {
	files := baseProc()
	delete(files, "/sys/class/hwmon/hwmon1/temp1_label")
	delete(files, "/sys/class/hwmon/hwmon1/temp2_label")
	files["/sys/class/hwmon/hwmon1/name"] = "k10temp\n"
	files["/sys/class/hwmon/hwmon1/temp3_input"] = "200000\n" // 200 °C：读数坏了，不要
	files["/sys/class/hwmon/hwmon1/temp4_input"] = "abc\n"
	root := fakeProc(t, files)
	l := NewLocal(root)
	l.Wait = func(context.Context, time.Duration) error { return nil }
	s, err := l.Collect(context.Background(), true)
	require.NoError(t, err)
	require.NotNil(t, s.CPU.TempC)
	assert.InDelta(t, 55.0, *s.CPU.TempC, 0.01)
}

func TestLocal_Errors(t *testing.T) {
	l := NewLocal(t.TempDir())
	_, err := l.Collect(context.Background(), true)
	assert.ErrorContains(t, err, "读不到 /proc/stat")

	root := fakeProc(t, map[string]string{"/proc/stat": "intr 1 2 3\n"})
	_, err = NewLocal(root).Collect(context.Background(), true)
	assert.ErrorContains(t, err, "格式不认识")

	root = fakeProc(t, map[string]string{"/proc/stat": "cpu a b c d e\n"})
	_, err = NewLocal(root).Collect(context.Background(), true)
	assert.ErrorContains(t, err, "格式不认识")

	root = fakeProc(t, map[string]string{"/proc/stat": stat1})
	l = NewLocal(root)
	l.Wait = func(context.Context, time.Duration) error { return nil }
	_, err = l.Collect(context.Background(), true)
	assert.ErrorContains(t, err, "读不到 /proc/meminfo")

	root = fakeProc(t, map[string]string{"/proc/stat": stat1, "/proc/meminfo": "MemFree: 1 kB\nbad line\nX:\nY: z kB\n"})
	l = NewLocal(root)
	l.Wait = func(context.Context, time.Duration) error { return nil }
	_, err = l.Collect(context.Background(), true)
	assert.ErrorContains(t, err, "没有 MemTotal")

	// 等第二次读数时取消了
	root = fakeProc(t, baseProc())
	l = NewLocal(root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = l.Collect(ctx, true)
	assert.True(t, errors.Is(err, context.Canceled))

	// 第二次读 /proc/stat 时它没了
	root = fakeProc(t, baseProc())
	l = NewLocal(root)
	l.Wait = func(context.Context, time.Duration) error { return os.Remove(filepath.Join(root, "proc", "stat")) }
	_, err = l.Collect(context.Background(), true)
	assert.ErrorContains(t, err, "读不到 /proc/stat")
}

func TestLocal_OptionalFilesMissing(t *testing.T) {
	root := fakeProc(t, map[string]string{
		"/proc/stat": stat1, "/proc/meminfo": meminfo,
		"/proc/loadavg": "x\n", "/proc/uptime": "nope\n",
	})
	l := NewLocal(root)
	l.Wait = func(context.Context, time.Duration) error { return nil }
	s, err := l.Collect(context.Background(), true)
	require.NoError(t, err)
	assert.Zero(t, s.CPU.Load1)
	assert.Zero(t, s.System.UptimeSeconds)
	assert.Equal(t, System{Platform: "Linux"}, s.System)

	root = fakeProc(t, map[string]string{"/proc/stat": stat1, "/proc/meminfo": meminfo, "/proc/uptime": "\n"})
	l = NewLocal(root)
	l.Wait = func(context.Context, time.Duration) error { return nil }
	s, err = l.Collect(context.Background(), true)
	require.NoError(t, err)
	assert.Zero(t, s.System.UptimeSeconds)
}

func TestSleepCtx(t *testing.T) {
	assert.NoError(t, sleepCtx(context.Background(), time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, sleepCtx(ctx, time.Hour), context.Canceled)
}

func TestCPUDelta(t *testing.T) {
	p, w := cpuDelta(cpuTimes{total: 100, idle: 50, iowait: 10}, cpuTimes{total: 100, idle: 50, iowait: 10})
	assert.Zero(t, p)
	assert.Zero(t, *w)
	// 计数器回绕或换了机器：空闲比上一拍小
	p, _ = cpuDelta(cpuTimes{total: 100, idle: 90, iowait: 10}, cpuTimes{total: 200, idle: 10, iowait: 5})
	assert.InDelta(t, 100.0, p, 0.01)
	// 空闲与 IO 等待加起来比总数的差还多（读数不一致）：占用按 0
	p, _ = cpuDelta(cpuTimes{total: 100}, cpuTimes{total: 110, idle: 50, iowait: 50})
	assert.Zero(t, p)
}

func TestNewLocalSource(t *testing.T) {
	src := NewLocalSource()
	require.NotNil(t, src)
	assert.NoError(t, src.Close())
	if !LocalSupported {
		_, err := src.Collect(context.Background(), true)
		assert.ErrorIs(t, err, ErrUnsupported)
	}
}
