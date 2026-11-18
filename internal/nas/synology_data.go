package nas

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// 群晖读不到的两类：pt-tools 现在不读 DSM 的通知与套件。
const (
	synologyNoAlerts = "pt-tools 还没接群晖的通知；存储空间坏了或降级时，存储池那里会写"
	synologyNoApps   = "pt-tools 还没接群晖的套件与 Container Manager"
)

// synoNum 是群晖回应里的数：有时写成数，有时写成字符串（"4000787030016"、"42"）；没有、null 或解不出时 ok 为假。
type synoNum struct {
	raw string
	ok  bool
}

func (n *synoNum) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if strings.HasPrefix(s, `"`) {
		var str string
		if json.Unmarshal(b, &str) != nil {
			return nil
		}
		s = strings.TrimSpace(str)
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) {
		n.raw, n.ok = s, true
	}
	return nil
}

func (n synoNum) float() (float64, bool) {
	if !n.ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(n.raw, 64)
	return v, err == nil
}

// uint 是非负的整数（字节数）：整数按原文解，免得大数在 float64 里丢精度；小数取整，负数记 0。
func (n synoNum) uint() (uint64, bool) {
	if !n.ok {
		return 0, false
	}
	if v, err := strconv.ParseUint(n.raw, 10, 64); err == nil {
		return v, true
	}
	f, _ := n.float()
	return toBytes(f), true
}

// synoSystemInfo 是 SYNO.Core.System 的 info：型号、系统版本、运行时长（「48:30:15」，小时可以超过 24）、系统温度
// 与 CPU 核数。回应里还有序列号，这里不读也不存。
type synoSystemInfo struct {
	Model       string  `json:"model"`
	FirmwareVer string  `json:"firmware_ver"`
	VersionStr  string  `json:"version_string"`
	UpTime      string  `json:"up_time"`
	SysTemp     synoNum `json:"sys_temp"`
	CPUCores    synoNum `json:"cpu_cores"`
}

// synoUptimeRe 是运行时长「时:分:秒」（小时不封顶）。
var synoUptimeRe = regexp.MustCompile(`^(\d+):(\d{1,2}):(\d{1,2})$`)

// parseSynologyUptime 是运行时长的秒数；看不懂时为 0（合并时沿用上一次的）。
func parseSynologyUptime(s string) int64 {
	m := synoUptimeRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	h, _ := strconv.ParseInt(m[1], 10, 64)
	mi, _ := strconv.ParseInt(m[2], 10, 64)
	sec, _ := strconv.ParseInt(m[3], 10, 64)
	return h*3600 + mi*60 + sec
}

// system 是主机信息：平台写 DSM，版本去掉 firmware_ver 前面的「DSM」（「7.2.2-72806 Update 3」），型号是整机型号
// （DS920+）。DSM 不在这里给主机名，主机名留空。
func (i synoSystemInfo) system() System {
	ver := strings.TrimSpace(i.FirmwareVer)
	if ver == "" {
		ver = strings.TrimSpace(i.VersionStr)
	}
	ver = strings.TrimSpace(strings.TrimPrefix(ver, "DSM"))
	out := System{Platform: "DSM", Version: ver, Model: strings.TrimSpace(i.Model), UptimeSeconds: parseSynologyUptime(i.UpTime)}
	if c, ok := i.CPUCores.float(); ok && c > 0 && c < math.MaxInt32 {
		out.Cores = int(c)
	}
	return out
}

// temp 是系统温度（DSM「信息中心」里那一项，在 CPU 卡里显示）；0 与离谱的值不算。
func (i synoSystemInfo) temp() (float64, bool) {
	t, ok := i.SysTemp.float()
	if !ok || t <= 0 || t >= 150 {
		return 0, false
	}
	return t, true
}

// fillSystem 读 SYNO.Core.System 的 info：主机信息与系统温度（轻量采集也读，温度与运行时长跟着更新）。
func (s *synology) fillSystem(ctx context.Context, snap *Snapshot) error {
	var info synoSystemInfo
	code, err := s.call(ctx, synoAPISystem, "info", nil, &info)
	if err != nil {
		return err
	}
	if code != 0 {
		snap.Facets[FacetSystem] = synoFacet(code, "主机信息")
		snap.Facets[FacetCPUTemp] = synoFacet(code, "系统温度")
		return nil
	}
	snap.System = info.system()
	snap.Facets[FacetSystem] = Facet{Status: FacetOK}
	if t, ok := info.temp(); ok {
		snap.CPU.TempC = &t
		snap.Facets[FacetCPUTemp] = Facet{Status: FacetOK}
	} else {
		snap.Facets[FacetCPUTemp] = Facet{Status: FacetUnsupported, Reason: "DSM 没给系统温度"}
	}
	return nil
}

// synoUtilization 是 SYNO.Core.System.Utilization 的 get（DSM 的资源监控）：CPU 的各项占用是百分比，平均负载是乘了
// 100 的整数（Home Assistant 的群晖集成除以 100 显示）；内存以 KB 计；网络是每块网卡与合计（device 为 total）的
// 每秒字节数。
type synoUtilization struct {
	CPU *struct {
		User   synoNum `json:"user_load"`
		System synoNum `json:"system_load"`
		Other  synoNum `json:"other_load"`
		Load1  synoNum `json:"1min_load"`
		Load5  synoNum `json:"5min_load"`
		Load15 synoNum `json:"15min_load"`
	} `json:"cpu"`
	Memory *struct {
		RealUsage  synoNum `json:"real_usage"`
		TotalReal  synoNum `json:"total_real"`
		MemorySize synoNum `json:"memory_size"`
		AvailReal  synoNum `json:"avail_real"`
		Buffer     synoNum `json:"buffer"`
		Cached     synoNum `json:"cached"`
	} `json:"memory"`
	Network []struct {
		Device string  `json:"device"`
		RX     synoNum `json:"rx"`
		TX     synoNum `json:"tx"`
	} `json:"network"`
}

// fillUtilization 读资源监控：CPU 占用、负载、内存与网络。账号没有权限时这三类都记 denied。
func (s *synology) fillUtilization(ctx context.Context, snap *Snapshot) error {
	var u synoUtilization
	code, err := s.call(ctx, synoAPIUtilization, "get", nil, &u)
	if err != nil {
		return err
	}
	if code != 0 {
		f := synoFacet(code, "资源监控")
		snap.Facets[FacetCPU], snap.Facets[FacetMemory], snap.Facets[FacetNetwork] = f, f, f
		return nil
	}
	snap.Facets[FacetCPU] = u.fillCPU(snap)
	snap.Facets[FacetMemory] = u.fillMemory(snap)
	snap.Facets[FacetNetwork] = u.fillNetwork(snap)
	return nil
}

// fillCPU：占用是用户、系统与其他三项之和（同 py-synologydsm-api 的 cpu_total_load），封顶 100。
func (u *synoUtilization) fillCPU(snap *Snapshot) Facet {
	if u.CPU == nil {
		return Facet{Status: FacetError, Reason: "DSM 没给 CPU 占用"}
	}
	total, seen := 0.0, false
	for _, n := range []synoNum{u.CPU.User, u.CPU.System, u.CPU.Other} {
		if v, ok := n.float(); ok {
			total += v
			seen = true
		}
	}
	if !seen {
		return Facet{Status: FacetError, Reason: "DSM 没给 CPU 占用"}
	}
	snap.CPU.Percent = clampPercent(total)
	load := func(n synoNum) float64 {
		v, _ := n.float()
		return math.Max(0, v) / 100
	}
	snap.CPU.Load1, snap.CPU.Load5, snap.CPU.Load15 = load(u.CPU.Load1), load(u.CPU.Load5), load(u.CPU.Load15)
	return Facet{Status: FacetOK}
}

// fillMemory：总量是 total_real（没有时 memory_size）乘 1024；已用按 DSM 自己的 real_usage 百分比算（DSM 资源监控
// 里显示的就是它），没有时按「总量 - 空闲 - 缓冲 - 缓存」算。
func (u *synoUtilization) fillMemory(snap *Snapshot) Facet {
	if u.Memory == nil {
		return Facet{Status: FacetError, Reason: "DSM 没给内存"}
	}
	kb, ok := u.Memory.TotalReal.uint()
	if !ok || kb == 0 {
		kb, ok = u.Memory.MemorySize.uint()
	}
	if !ok || kb == 0 || kb > math.MaxUint64/1024 {
		return Facet{Status: FacetError, Reason: "DSM 没给内存总量"}
	}
	total := kb * 1024
	var used uint64
	if pct, ok := u.Memory.RealUsage.float(); ok {
		used = uint64(math.Round(float64(total) * clampPercent(pct) / 100))
	} else {
		free, ok := u.Memory.AvailReal.uint()
		if !ok {
			return Facet{Status: FacetError, Reason: "DSM 没给内存用量"}
		}
		buf, _ := u.Memory.Buffer.uint()
		cache, _ := u.Memory.Cached.uint()
		availKB := min(min(free, kb)+min(buf, kb)+min(cache, kb), kb)
		used = (kb - availKB) * 1024
	}
	used = min(used, total)
	snap.Memory = Memory{Total: total, Used: used, Available: total - used}
	return Facet{Status: FacetOK}
}

// fillNetwork：速度用合计（device 为 total）那一项；参与合计的网卡是其余各项的名字。
func (u *synoUtilization) fillNetwork(snap *Snapshot) Facet {
	found := false
	for _, n := range u.Network {
		if n.Device == "total" {
			snap.Network.RxBps, _ = n.RX.uint()
			snap.Network.TxBps, _ = n.TX.uint()
			found = true
			continue
		}
		if n.Device != "" {
			snap.Network.Interfaces = append(snap.Network.Interfaces, n.Device)
		}
	}
	if !found {
		snap.Network = Network{Interfaces: []string{}}
		return Facet{Status: FacetError, Reason: "DSM 没给网络的合计（total）"}
	}
	return Facet{Status: FacetOK}
}

// synoStorage 是 SYNO.Storage.CGI.Storage 的 load_info（存储管理器的数据）：硬盘、存储池与存储空间（卷）。
// 字段照 py-synologydsm-api 的 storage.py；大小是写成字符串的字节数。
type synoStorage struct {
	Disks   []synoDisk   `json:"disks"`
	Pools   []synoPool   `json:"storagePools"`
	Volumes []synoVolume `json:"volumes"`
}

type synoDisk struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	LongName  string  `json:"longName"`
	Model     string  `json:"model"`
	DiskType  string  `json:"diskType"`
	IsSSD     *bool   `json:"isSsd"`
	SizeTotal synoNum `json:"size_total"`
	Temp      synoNum `json:"temp"`
}

// synoPool 是一个存储池：disks 是它的硬盘；RAID 的池用 deploy_path 指向建在上面的存储空间，SHR 的池用 pool_child。
type synoPool struct {
	ID         string   `json:"id"`
	Disks      []string `json:"disks"`
	DeployPath string   `json:"deploy_path"`
	Children   []struct {
		ID string `json:"id"`
	} `json:"pool_child"`
}

type synoVolume struct {
	ID       string `json:"id"`
	VolPath  string `json:"vol_path"`
	PoolPath string `json:"pool_path"`
	Status   string `json:"status"`
	Size     struct {
		Total synoNum `json:"total"`
		Used  synoNum `json:"used"`
	} `json:"size"`
}

// synoLegacyStorage 是 SYNO.Core.System 的 info 带 type="storage"（Homepage 的群晖小组件用它）：只有存储空间的容量与状态。
type synoLegacyStorage struct {
	Volumes []struct {
		Name   string  `json:"name"`
		Used   synoNum `json:"used_size"`
		Total  synoNum `json:"total_size"`
		Status string  `json:"status"`
	} `json:"vol_info"`
}

// fillStorage 读存储（完整采集）：先读 load_info（存储空间、硬盘与温度）；读不成时存储空间改读 type="storage" 的容量。
func (s *synology) fillStorage(ctx context.Context, snap *Snapshot) error {
	var st synoStorage
	code, err := s.call(ctx, synoAPIStorage, "load_info", url.Values{"action": {"load_info"}}, &st)
	if err != nil {
		return err
	}
	if code == 0 {
		snap.Pools, snap.Disks = st.pools(), st.disks()
		snap.Facets[FacetPools] = Facet{Status: FacetOK}
		snap.Facets[FacetDisks] = Facet{Status: FacetOK}
		return nil
	}
	snap.Facets[FacetDisks] = synoFacet(code, "硬盘")
	var legacy synoLegacyStorage
	code, err = s.call(ctx, synoAPISystem, "info", url.Values{"type": {`"storage"`}}, &legacy)
	if err != nil {
		return err
	}
	if code != 0 {
		snap.Facets[FacetPools] = synoFacet(code, "存储空间")
		return nil
	}
	for _, v := range legacy.Volumes {
		size, _ := v.Total.uint()
		used, _ := v.Used.uint()
		snap.Pools = append(snap.Pools, synoPoolOf(synoVolumeName("", v.Name), v.Status, size, used, 0))
	}
	snap.Facets[FacetPools] = Facet{Status: FacetOK}
	return nil
}

// synoVolumeIDRe 是存储空间的编号（volume_1）：没给路径时按它写成 /volume1（DSM 上存储空间的挂载点）。
var synoVolumeIDRe = regexp.MustCompile(`^volume_(\d+)$`)

func synoVolumeName(path, id string) string {
	if p := strings.TrimSpace(path); p != "" {
		return p
	}
	if m := synoVolumeIDRe.FindStringSubmatch(id); m != nil {
		return "/volume" + m[1]
	}
	return id
}

// synoStatusText 是存储空间常见的几种状态写成中文；别的照原样。
var synoStatusText = map[string]string{"normal": "正常", "degraded": "降级", "crashed": "已损毁"}

// synoPoolOf 是一个存储空间：normal 是健康，degraded、crashed 是不健康；别的状态（修复中、扩充中…）不知道健不健康，
// 照原样写状态。容量是 DSM 报的已用与总量（存储空间的大小，已经扣掉了校验盘）。
func synoPoolOf(name, status string, size, used uint64, disks int) Pool {
	st := strings.ToLower(strings.TrimSpace(status))
	p := Pool{Name: name, Status: status, Size: size, Used: min(used, size), Disks: disks}
	p.Free = p.Size - p.Used
	if t, ok := synoStatusText[st]; ok {
		p.Status = t
		healthy := st == "normal"
		p.Healthy = &healthy
	}
	return p
}

// pool 是存储空间 v 所在的存储池（按 pool_path、deploy_path 或 pool_child 认）；找不到时为空。
func (st *synoStorage) pool(v synoVolume) *synoPool {
	for i := range st.Pools {
		p := &st.Pools[i]
		if (v.PoolPath != "" && p.ID == v.PoolPath) || (p.DeployPath != "" && p.DeployPath == v.ID) {
			return p
		}
		for _, c := range p.Children {
			if c.ID != "" && c.ID == v.ID {
				return p
			}
		}
	}
	return nil
}

func (st *synoStorage) pools() []Pool {
	out := make([]Pool, 0, len(st.Volumes))
	for _, v := range st.Volumes {
		size, _ := v.Size.Total.uint()
		used, _ := v.Size.Used.uint()
		disks := 0
		if p := st.pool(v); p != nil {
			disks = len(p.Disks)
		}
		out = append(out, synoPoolOf(synoVolumeName(v.VolPath, v.ID), v.Status, size, used, disks))
	}
	return out
}

// disks 是各块硬盘：所在的存储池写建在那个池上的第一个存储空间（首页的存储池列的是存储空间）。
func (st *synoStorage) disks() []Disk {
	poolOf := map[string]string{}
	for _, v := range st.Volumes {
		p := st.pool(v)
		if p == nil {
			continue
		}
		for _, id := range p.Disks {
			if _, seen := poolOf[id]; !seen {
				poolOf[id] = synoVolumeName(v.VolPath, v.ID)
			}
		}
	}
	out := make([]Disk, 0, len(st.Disks))
	for _, d := range st.Disks {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			name = strings.TrimSpace(d.LongName)
		}
		if name == "" {
			name = d.ID
		}
		disk := Disk{Name: name, Model: strings.TrimSpace(d.Model), Kind: synoDiskKind(d), Pool: poolOf[d.ID]}
		disk.Size, _ = d.SizeTotal.uint()
		if t, ok := d.Temp.float(); ok && t > 0 && t < 150 {
			disk.TempC = &t
		}
		out = append(out, disk)
	}
	return out
}

// synoDiskKind 是硬盘的类型：diskType 或编号里有 NVMe 的是 NVMe；isSsd 为真是固态盘、为假是机械盘；都没有时不知道。
func synoDiskKind(d synoDisk) string {
	switch {
	case strings.Contains(strings.ToLower(d.DiskType), "nvme") || strings.HasPrefix(strings.ToLower(d.ID), "nvme"):
		return DiskNVMe
	case d.IsSSD != nil && *d.IsSSD:
		return DiskSSD
	case d.IsSSD != nil:
		return DiskHDD
	}
	return DiskUnknown
}
