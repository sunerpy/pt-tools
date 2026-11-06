package nas

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 本机来源读不到的几类数据的原因（Facets）。
const (
	localNetworkReason = "本机来源不读网卡：容器里的网卡计数只有容器自己的流量，首页这一格用下载器的速度"
	localStorageReason = "本机来源读不到存储池健康、硬盘与告警，接入 NAS 以后才有"
	localAppsReason    = "本机来源读不到 NAS 上的应用，接入 NAS 以后才有"
	localTempReason    = "没找到 CPU 温度传感器（/sys/class/hwmon 里没有 coretemp、k10temp 或 cpu_thermal）"
)

// localFirstGap 是第一次采集时两次读 /proc/stat 的间隔：CPU 占用要两拍之差才算得出。
const localFirstGap = 500 * time.Millisecond

// cpuHwmon 是认作 CPU 温度的 hwmon 驱动名（Intel、AMD、树莓派等 ARM 板）。
var cpuHwmon = map[string]bool{"coretemp": true, "k10temp": true, "zenpower": true, "cpu_thermal": true, "k8temp": true}

// cpuTempLabels 是 hwmon 里代表整颗 CPU 的那一路（有它就用它，没有时取各路的最高值）。
var cpuTempLabels = []string{"Package id 0", "Tdie", "Tctl"}

// Local 是本机来源：从 root 下的 /proc 与 /sys 读 pt-tools 所在主机的数据（Docker 里读到的是宿主机的 CPU、内存、
// 负载与运行时长）。root 在生产里是 /，测试里是放着假 /proc 的临时目录。
type Local struct {
	root string
	// Wait 是第一次采集时两次读 /proc/stat 之间的等待（测试里换掉，在里面改写假的 /proc/stat）
	Wait func(ctx context.Context, d time.Duration) error

	mu   sync.Mutex
	prev *cpuTimes
	sys  *System
}

// NewLocal 是读 root 下 /proc 与 /sys 的本机来源。
func NewLocal(root string) *Local { return &Local{root: root, Wait: sleepCtx} }

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (l *Local) path(p string) string { return filepath.Join(l.root, filepath.FromSlash(p)) }

// Close 没有要断开的连接。
func (l *Local) Close() error { return nil }

// Collect 读一次本机的数据。full 不影响本机来源：这几个文件都很便宜，每次都读。
func (l *Local) Collect(ctx context.Context, _ bool) (*Snapshot, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cur, err := l.readCPU()
	if err != nil {
		return nil, err
	}
	if l.prev == nil {
		if err = l.Wait(ctx, localFirstGap); err != nil {
			return nil, err
		}
		l.prev = cur
		if cur, err = l.readCPU(); err != nil {
			return nil, err
		}
	}
	percent, iowait := cpuDelta(*l.prev, *cur)
	l.prev = cur

	mem, err := l.readMemory()
	if err != nil {
		return nil, err
	}
	snap := &Snapshot{
		At:     now,
		System: l.system(),
		Memory: mem,
		CPU:    CPU{Percent: percent, IOWait: iowait},
		Pools:  []Pool{},
		Disks:  []Disk{},
		Alerts: []Alert{},
		Facets: map[string]Facet{
			FacetSystem:  {Status: FacetOK},
			FacetCPU:     {Status: FacetOK},
			FacetMemory:  {Status: FacetOK},
			FacetNetwork: {Status: FacetUnsupported, Reason: localNetworkReason},
			FacetPools:   {Status: FacetUnsupported, Reason: localStorageReason},
			FacetDisks:   {Status: FacetUnsupported, Reason: localStorageReason},
			FacetAlerts:  {Status: FacetUnsupported, Reason: localStorageReason},
			FacetApps:    {Status: FacetUnsupported, Reason: localAppsReason},
		},
	}
	snap.Network.Interfaces = []string{}
	snap.CPU.Load1, snap.CPU.Load5, snap.CPU.Load15 = l.readLoad()
	snap.System.UptimeSeconds = l.readUptime()
	if t, ok := l.readCPUTemp(); ok {
		snap.CPU.TempC = &t
		snap.Facets[FacetCPUTemp] = Facet{Status: FacetOK}
	} else {
		snap.Facets[FacetCPUTemp] = Facet{Status: FacetUnsupported, Reason: localTempReason}
	}
	return snap, nil
}

// cpuTimes 是 /proc/stat 第一行的累计时间（单位是 USER_HZ，只用差值）。
type cpuTimes struct {
	total, idle, iowait uint64
}

func (l *Local) readCPU() (*cpuTimes, error) {
	b, err := os.ReadFile(l.path("/proc/stat"))
	if err != nil {
		return nil, fmt.Errorf("读不到 /proc/stat: %w", err)
	}
	return parseCPUStat(b)
}

// parseCPUStat 解析 /proc/stat 的 cpu 一行：user nice system idle iowait irq softirq steal guest guest_nice。
// guest 与 guest_nice 已经算在 user、nice 里，不再加进总数。
func parseCPUStat(b []byte) (*cpuTimes, error) {
	line, _, _ := bytes.Cut(b, []byte("\n"))
	f := strings.Fields(string(line))
	if len(f) < 5 || f[0] != "cpu" {
		return nil, errors.New("/proc/stat 的格式不认识")
	}
	var v [8]uint64
	for i := 0; i < len(v) && i+1 < len(f); i++ {
		n, err := strconv.ParseUint(f[i+1], 10, 64)
		if err != nil {
			return nil, errors.New("/proc/stat 的格式不认识")
		}
		v[i] = n
	}
	var total uint64
	for _, n := range v {
		total += n
	}
	return &cpuTimes{total: total, idle: v[3], iowait: v[4]}, nil
}

// cpuDelta 是两拍之间的 CPU 占用与 IO 等待（百分比）：占用 = 1 − (空闲 + IO 等待) / 总数。
func cpuDelta(prev, cur cpuTimes) (percent float64, iowait *float64) {
	if cur.total <= prev.total {
		zero := 0.0
		return 0, &zero
	}
	total := float64(cur.total - prev.total)
	idle := float64(sub(cur.idle, prev.idle))
	wait := float64(sub(cur.iowait, prev.iowait))
	busy := total - idle - wait
	if busy < 0 {
		busy = 0
	}
	w := round1(wait * 100 / total)
	return round1(busy * 100 / total), &w
}

func sub(a, b uint64) uint64 {
	if a < b {
		return 0
	}
	return a - b
}

func round1(v float64) float64 { return float64(int64(v*10+0.5)) / 10 }

// readMemory 读 /proc/meminfo：已用 = MemTotal − MemAvailable；老内核没有 MemAvailable 时用 MemFree + Buffers + Cached。
func (l *Local) readMemory() (Memory, error) {
	b, err := os.ReadFile(l.path("/proc/meminfo"))
	if err != nil {
		return Memory{}, fmt.Errorf("读不到 /proc/meminfo: %w", err)
	}
	return parseMeminfo(b)
}

func parseMeminfo(b []byte) (Memory, error) {
	kv := map[string]uint64{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			continue
		}
		n, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			continue
		}
		if len(f) > 1 && f[1] == "kB" {
			n *= 1024
		}
		kv[name] = n
	}
	total, ok := kv["MemTotal"]
	if !ok || total == 0 {
		return Memory{}, errors.New("/proc/meminfo 里没有 MemTotal")
	}
	avail, ok := kv["MemAvailable"]
	if !ok {
		avail = kv["MemFree"] + kv["Buffers"] + kv["Cached"]
	}
	if avail > total {
		avail = total
	}
	return Memory{Total: total, Used: total - avail, Available: avail}, nil
}

// readLoad 读 /proc/loadavg 的前三个数；读不到时都是 0。
func (l *Local) readLoad() (l1, l5, l15 float64) {
	b, err := os.ReadFile(l.path("/proc/loadavg"))
	if err != nil {
		return 0, 0, 0
	}
	f := strings.Fields(string(b))
	if len(f) < 3 {
		return 0, 0, 0
	}
	p := func(s string) float64 {
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}
	return p(f[0]), p(f[1]), p(f[2])
}

// readUptime 读 /proc/uptime 的第一个数（秒）；读不到时是 0。
func (l *Local) readUptime() int64 {
	b, err := os.ReadFile(l.path("/proc/uptime"))
	if err != nil {
		return 0
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0
	}
	v, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0
	}
	return int64(v)
}

// system 是主机信息（第一次读完记下来：型号、核数、内核版本与主机名不会变）。
func (l *Local) system() System {
	if l.sys != nil {
		return *l.sys
	}
	s := System{Platform: "Linux"}
	if b, err := os.ReadFile(l.path("/proc/cpuinfo")); err == nil {
		s.Model, s.Cores = parseCPUInfo(b)
	}
	s.Version = l.readTrim("/proc/sys/kernel/osrelease")
	s.Hostname = l.readTrim("/proc/sys/kernel/hostname")
	l.sys = &s
	return s
}

func (l *Local) readTrim(p string) string {
	b, err := os.ReadFile(l.path(p))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// parseCPUInfo 从 /proc/cpuinfo 取型号与逻辑核数：x86 是 model name；ARM 板多是 Model 或 Hardware。
func parseCPUInfo(b []byte) (model string, cores int) {
	var hardware, boardModel string
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "processor":
			cores++
		case "model name":
			if model == "" {
				model = v
			}
		case "Hardware":
			hardware = v
		case "Model":
			boardModel = v
		}
	}
	if model == "" {
		model = boardModel
	}
	if model == "" {
		model = hardware
	}
	return model, cores
}

// readCPUTemp 找 CPU 温度：hwmon 里驱动是 coretemp、k10temp、cpu_thermal 等的那一个，优先用代表整颗 CPU 的那一路
// （Package id 0、Tdie、Tctl），没有时取各路的最高值。值是千分之一摄氏度；不在 1–150 °C 之间的不要。
func (l *Local) readCPUTemp() (float64, bool) {
	dirs, _ := filepath.Glob(l.path("/sys/class/hwmon/hwmon*"))
	sort.Strings(dirs)
	for _, d := range dirs {
		name, err := os.ReadFile(filepath.Join(d, "name"))
		if err != nil || !cpuHwmon[strings.TrimSpace(string(name))] {
			continue
		}
		if t, ok := hwmonTemp(d); ok {
			return t, true
		}
	}
	return 0, false
}

func hwmonTemp(dir string) (float64, bool) {
	inputs, _ := filepath.Glob(filepath.Join(dir, "temp*_input"))
	sort.Strings(inputs)
	byLabel := map[string]float64{}
	best, found := 0.0, false
	for _, in := range inputs {
		b, err := os.ReadFile(in)
		if err != nil {
			continue
		}
		milli, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
		if err != nil {
			continue
		}
		t := milli / 1000
		if t < 1 || t > 150 {
			continue
		}
		if lb, err := os.ReadFile(strings.TrimSuffix(in, "_input") + "_label"); err == nil {
			byLabel[strings.TrimSpace(string(lb))] = t
		}
		if !found || t > best {
			best, found = t, true
		}
	}
	for _, lb := range cpuTempLabels {
		if t, ok := byLabel[lb]; ok {
			return round1(t), true
		}
	}
	return round1(best), found
}
