package nas

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Glances 读不到某一类时写给人看的原因（Facets）。
const (
	glancesNoCPUTemp = "Glances 没有给 CPU 温度（虚拟机里通常没有温度传感器）"
	glancesNoDisks   = "Glances 没有给硬盘温度（SATA 盘要在 NAS 上加载 drivetemp 模块才有）；Glances 也不给硬盘的型号与容量"
	glancesNoPools   = "Glances 只看得到挂进它的容器的目录：把存储目录只读挂进去（例如 /volume1:/volume1:ro）"
	glancesNoApps    = "Glances 没读到容器：要把 /var/run/docker.sock 挂进 Glances 的容器"
	glancesRaidOff   = "Glances 默认关着 raid 插件，读不到软 RAID 阵列的状态（在 glances.conf 的 [raid] 里写 disable=False 才读）"
	glancesNoRaid    = "Glances 没有发现软 RAID（mdadm）阵列；Glances 不给 NAS 自己的告警，只能看这种阵列有没有降级"
)

// cpuTempPriority 是 sensors 里代表整颗 CPU 的标签（有就用，按这个顺序）；同本机来源的 cpuTempLabels。
var cpuTempPriority = []string{"Package id 0", "Tdie", "Tctl"}

// cpuCoreRe 是各核的温度（「Core 0」，重名时 Glances 在后面加序号：「Core 0 1」）。
var cpuCoreRe = regexp.MustCompile(`^Core \d+( \d+)?$`)

// cpuChipPrefixes 是标签为空时 Glances 用「芯片名 序号」当标签的 CPU 温度芯片（树莓派的 cpu_thermal 等）。
var cpuChipPrefixes = []string{"cpu_thermal", "cpu-thermal", "coretemp", "k10temp", "zenpower", "k8temp", "soc_thermal", "soc-thermal", "x86_pkg_temp"}

// nvmeCompositeRe 是 NVMe 盘的温度：内核给的标签是 Composite，几块盘重名时 Glances 写成 Composite、Composite 1……
var nvmeCompositeRe = regexp.MustCompile(`^Composite( \d+)?$`)

// validTemp 是读得到的温度：没有、0 及以下（云主机的 NVMe 会给 -273）、高得不合理的都不算。
func validTemp(s glSensor) (float64, bool) {
	if s.Value == nil || !strings.EqualFold(s.Unit, "C") {
		return 0, false
	}
	v := *s.Value
	if math.IsNaN(v) || v <= 0 || v > 150 {
		return 0, false
	}
	return v, true
}

// fillSensors 从 sensors 里取 CPU 温度（每次采集）与硬盘温度（完整采集）。
func (g *glances) fillSensors(ctx context.Context, snap *Snapshot, full bool) error {
	var list []glSensor
	if err := g.get(ctx, "sensors", &list); err != nil {
		f, fatal := pluginFacet(err, "温度")
		if fatal != nil {
			return fatal
		}
		snap.Facets[FacetCPUTemp] = f
		if full {
			snap.Facets[FacetDisks] = f
		}
		return nil
	}
	if t, ok := glancesCPUTemp(list); ok {
		snap.CPU.TempC = &t
		snap.Facets[FacetCPUTemp] = Facet{Status: FacetOK}
	} else {
		snap.Facets[FacetCPUTemp] = Facet{Status: FacetUnsupported, Reason: glancesNoCPUTemp}
	}
	if !full {
		return nil
	}
	snap.Disks = glancesDisks(list)
	if len(snap.Disks) == 0 {
		snap.Facets[FacetDisks] = Facet{Status: FacetUnsupported, Reason: glancesNoDisks}
	} else {
		snap.Facets[FacetDisks] = Facet{Status: FacetOK}
	}
	return nil
}

// glancesCPUTemp 按「整颗 CPU 的标签 → 各核里最高的 → CPU 温度芯片里最高的 → 以 CPU 开头的标签里最高的」取 CPU 温度。
func glancesCPUTemp(list []glSensor) (float64, bool) {
	temps := map[string]float64{}
	for _, s := range list {
		if v, ok := validTemp(s); ok {
			if _, dup := temps[s.Label]; !dup {
				temps[s.Label] = v
			}
		}
	}
	for _, l := range cpuTempPriority {
		if v, ok := temps[l]; ok {
			return v, true
		}
	}
	matchers := []func(string) bool{
		cpuCoreRe.MatchString,
		func(l string) bool {
			for _, p := range cpuChipPrefixes {
				if strings.HasPrefix(l, p) {
					return true
				}
			}
			return false
		},
		func(l string) bool { return strings.HasPrefix(strings.ToUpper(l), "CPU") },
	}
	for _, match := range matchers {
		best, found := 0.0, false
		for l, v := range temps {
			if match(l) && (!found || v > best) {
				best, found = v, true
			}
		}
		if found {
			return best, true
		}
	}
	return 0, false
}

// glancesDisks 是 sensors 里认得出的硬盘温度：NVMe 的 Composite（按出现的顺序叫「NVMe 1」「NVMe 2」）、drivetemp 模块给的
// SATA 盘、hddtemp 给的（type 是 temperature_hdd）。Glances 不给型号与容量，这两项留空。
func glancesDisks(list []glSensor) []Disk {
	out := []Disk{}
	nvme := 0
	for _, s := range list {
		v, ok := validTemp(s)
		if !ok {
			continue
		}
		var d Disk
		switch {
		case nvmeCompositeRe.MatchString(s.Label):
			nvme++
			d = Disk{Name: fmt.Sprintf("NVMe %d", nvme), Kind: DiskNVMe}
		case strings.HasPrefix(s.Label, "drivetemp") || s.Type == "temperature_hdd":
			d = Disk{Name: s.Label, Kind: DiskUnknown}
		default:
			continue
		}
		t := v
		d.TempC = &t
		out = append(out, d)
	}
	return out
}

// glancesSkipFSTypes 是不算存储的文件系统（容器的层、内存盘与内核的虚拟文件系统）。
var glancesSkipFSTypes = map[string]bool{
	"overlay": true, "aufs": true, "tmpfs": true, "devtmpfs": true, "ramfs": true, "squashfs": true, "iso9660": true,
	"proc": true, "sysfs": true, "cgroup": true, "cgroup2": true, "nsfs": true, "autofs": true, "fuse.lxcfs": true,
	"efivarfs": true, "devpts": true, "mqueue": true, "debugfs": true, "tracefs": true, "securityfs": true, "pstore": true,
	"bpf": true, "configfs": true, "fusectl": true, "hugetlbfs": true, "binfmt_misc": true, "rpc_pipefs": true, "nfsd": true,
}

// glancesSkipMount 是不算存储的挂载点：Docker 挂进容器的单个文件（/etc/hosts、/etc/hostname、/etc/resolv.conf，
// 挂进来的 /etc/os-release 显示成 /usr/lib/os-release）与系统目录。
func glancesSkipMount(m string) bool {
	for _, p := range []string{"/etc", "/usr", "/proc", "/sys", "/dev", "/run", "/var/run", "/boot", "/snap"} {
		if m == p || strings.HasPrefix(m, p+"/") {
			return true
		}
	}
	return false
}

// fillPools 把文件系统当成存储池：同一个设备挂了几处（绑定挂载、btrfs 的子卷）只算一次，名字用最短的挂载点；
// ZFS 按池合计（池名是数据集名的第一段）：已用是挂进来的各数据集已用之和，可用取其中最大的。健康读不到（Healthy 为空）。
func (g *glances) fillPools(ctx context.Context, snap *Snapshot, _ bool) error {
	var list []glFS
	if err := g.get(ctx, "fs", &list); err != nil {
		f, fatal := pluginFacet(err, "文件系统")
		if fatal != nil {
			return fatal
		}
		snap.Facets[FacetPools] = f
		return nil
	}
	snap.Pools = glancesPools(list)
	if len(snap.Pools) == 0 {
		snap.Facets[FacetPools] = Facet{Status: FacetUnsupported, Reason: glancesNoPools}
	} else {
		snap.Facets[FacetPools] = Facet{Status: FacetOK}
	}
	return nil
}

func glancesPools(list []glFS) []Pool {
	type agg struct {
		name             string
		used, free, size uint64
		zfs              bool
	}
	byKey := map[string]*agg{}
	for _, f := range list {
		mount := strings.TrimSpace(f.Mount)
		if mount == "" || glancesSkipFSTypes[f.Type] || glancesSkipMount(mount) || f.Size <= 0 {
			continue
		}
		used, free, size := toBytes(f.Used), toBytes(f.Free), toBytes(f.Size)
		if f.Type == "zfs" {
			pool, _, _ := strings.Cut(f.Device, "/")
			if pool == "" {
				continue
			}
			key := "zfs:" + pool
			a := byKey[key]
			if a == nil {
				a = &agg{name: pool, zfs: true}
				byKey[key] = a
			}
			a.used += used
			a.free = max(a.free, free)
			continue
		}
		key := "dev:" + f.Device
		if f.Device == "" {
			key = "mnt:" + mount
		}
		if a := byKey[key]; a != nil {
			if len(mount) < len(a.name) {
				a.name = mount
			}
			continue
		}
		byKey[key] = &agg{name: mount, used: used, free: free, size: size}
	}
	out := make([]Pool, 0, len(byKey))
	for _, a := range byKey {
		p := Pool{Name: a.name, Used: a.used, Free: a.free, Size: a.size}
		if a.zfs {
			p.Size = a.used + a.free
		}
		if p.Size == 0 {
			p.Size = p.Used + p.Free
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// fillApps 把容器当成应用：运行中、共几个。Glances 不知道哪些能更新（UpdatesUnknown）。
func (g *glances) fillApps(ctx context.Context, snap *Snapshot, _ bool) error {
	var list []glContainer
	if err := g.get(ctx, "containers", &list); err != nil {
		f, fatal := pluginFacet(err, "容器")
		if fatal != nil {
			return fatal
		}
		snap.Facets[FacetApps] = f
		return nil
	}
	if len(list) == 0 {
		snap.Facets[FacetApps] = Facet{Status: FacetUnsupported, Reason: glancesNoApps}
		return nil
	}
	snap.Apps = Apps{Total: len(list), UpdatesUnknown: true}
	for _, c := range list {
		if strings.EqualFold(c.Status, "running") {
			snap.Apps.Running++
		}
	}
	snap.Facets[FacetApps] = Facet{Status: FacetOK}
	return nil
}

// fillRaid 把软 RAID（mdadm）阵列的问题写成告警：没在运行的、降级的（在用的盘比应有的少）。RAID 0 没有冗余，不算降级。
// Glances 默认关着 raid 插件；没有阵列时告警这一类记作读不到（Glances 不给 NAS 自己的告警）。
func (g *glances) fillRaid(ctx context.Context, snap *Snapshot, _ bool) error {
	var arrays map[string]glRaid
	if err := g.get(ctx, "raid", &arrays); err != nil {
		if he, ok := asGlancesHTTPError(err); ok && he.off {
			snap.Facets[FacetAlerts] = Facet{Status: FacetUnsupported, Reason: glancesRaidOff}
			return nil
		}
		f, fatal := pluginFacet(err, "RAID 阵列")
		if fatal != nil {
			return fatal
		}
		snap.Facets[FacetAlerts] = f
		return nil
	}
	if len(arrays) == 0 {
		snap.Facets[FacetAlerts] = Facet{Status: FacetUnsupported, Reason: glancesNoRaid}
		return nil
	}
	names := make([]string, 0, len(arrays))
	for n := range arrays {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if title := raidProblem(n, arrays[n]); title != "" {
			snap.Alerts = append(snap.Alerts, Alert{Level: AlertCritical, Title: title})
		}
	}
	snap.Facets[FacetAlerts] = Facet{Status: FacetOK}
	return nil
}

// raidProblem 是一个阵列的问题（没有问题时为空）。
func raidProblem(name string, a glRaid) string {
	kind := a.Type
	if kind == "" {
		kind = "RAID"
	}
	if a.Status != "" && !strings.EqualFold(a.Status, "active") {
		return fmt.Sprintf("RAID 阵列 %s（%s）没有在运行：%s", name, kind, a.Status)
	}
	if strings.EqualFold(a.Type, "raid0") {
		return ""
	}
	if a.Used.ok && a.Available.ok && a.Used.n < a.Available.n {
		return fmt.Sprintf("RAID 阵列 %s（%s）降级：%d 块盘里 %d 块在用", name, kind, a.Available.n, a.Used.n)
	}
	if strings.Contains(a.Config, "_") {
		return fmt.Sprintf("RAID 阵列 %s（%s）降级：有盘掉线（%s）", name, kind, a.Config)
	}
	return ""
}
