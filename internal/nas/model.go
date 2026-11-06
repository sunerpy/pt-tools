// Package nas 是首页「NAS 概览」与设置页「NAS 监控」的数据层（首页 NAS 概览 H2）：来源类型的注册表、地址与传输规则、
// 证书钉住、本机来源与采集器。
//
// 只依赖标准库、github.com/coder/websocket 与 internal/redact：读写数据库、解密密钥在 cmd（nas_wiring.go），
// 接口在 web（api_nas.go）。采集到的数据只在内存里：最新一份快照与最近 60 拍的历史，重启清空。
package nas

import "time"

// 快照里各类数据的键（Snapshot.Facets）：界面据此写「这个来源读不到硬盘温度」，不把没有当成 0。
const (
	FacetSystem  = "system"
	FacetCPU     = "cpu"
	FacetCPUTemp = "cpu_temp"
	FacetMemory  = "memory"
	FacetNetwork = "network"
	FacetPools   = "pools"
	FacetDisks   = "disks"
	FacetAlerts  = "alerts"
	FacetApps    = "apps"
)

// FacetKeys 是全部的键，按界面上的顺序。
var FacetKeys = []string{FacetSystem, FacetCPU, FacetCPUTemp, FacetMemory, FacetNetwork, FacetPools, FacetDisks, FacetAlerts, FacetApps}

// 一类数据读没读到（Facet.Status）。
const (
	FacetOK = "ok"
	// FacetUnsupported 是这个来源没有这类数据（本机读不到存储池、虚拟机没有 CPU 温度）
	FacetUnsupported = "unsupported"
	// FacetDenied 是账号没有读这类数据的权限
	FacetDenied = "denied"
	// FacetError 是读的时候出错了（这一次）
	FacetError = "error"
)

// Facet 是一类数据的情况：Reason 是给人看的原因（读到了时为空）。
type Facet struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// Snapshot 是一个来源某一时刻的数据。轻量采集只更新 CPU、内存、网络与负载，存储池、硬盘、告警与应用沿用上一次完整采集的。
type Snapshot struct {
	At      time.Time        `json:"at"`
	System  System           `json:"system"`
	CPU     CPU              `json:"cpu"`
	Memory  Memory           `json:"memory"`
	Network Network          `json:"network"`
	Pools   []Pool           `json:"pools"`
	Disks   []Disk           `json:"disks"`
	Alerts  []Alert          `json:"alerts"`
	Apps    Apps             `json:"apps"`
	Facets  map[string]Facet `json:"facets"`
}

// System 是主机信息：Platform 是系统（Linux、TrueNAS SCALE、DSM），Version 是它的版本（本机是内核版本），Model 是型号
// （本机与读不到整机型号的来源写 CPU 型号）。
type System struct {
	Hostname      string `json:"hostname"`
	Platform      string `json:"platform"`
	Version       string `json:"version"`
	Model         string `json:"model"`
	Cores         int    `json:"cores"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

// CPU 是处理器：Percent 是占用百分比（0–100）；IOWait 是 IO 等待的百分比、TempC 是温度，读不到时为空。
type CPU struct {
	Percent float64  `json:"percent"`
	IOWait  *float64 `json:"iowait,omitempty"`
	Load1   float64  `json:"load1"`
	Load5   float64  `json:"load5"`
	Load15  float64  `json:"load15"`
	TempC   *float64 `json:"temp_c,omitempty"`
}

// Memory 是内存（字节）：Used = Total - Available。
type Memory struct {
	Total     uint64 `json:"total"`
	Used      uint64 `json:"used"`
	Available uint64 `json:"available"`
}

// Network 是各网卡合计的速度（字节/秒）与参与合计的网卡。
type Network struct {
	RxBps      uint64   `json:"rx_bps"`
	TxBps      uint64   `json:"tx_bps"`
	Interfaces []string `json:"interfaces"`
}

// Pool 是一个存储池：Size、Used、Free 是可用容量（扣掉校验盘以后的，不是裸容量）；Healthy 读不到时为空。
type Pool struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Healthy *bool  `json:"healthy,omitempty"`
	Size    uint64 `json:"size"`
	Used    uint64 `json:"used"`
	Free    uint64 `json:"free"`
	Disks   int    `json:"disks"`
}

// 硬盘的类型（Disk.Kind）。
const (
	DiskHDD     = "hdd"
	DiskSSD     = "ssd"
	DiskNVMe    = "nvme"
	DiskUnknown = "unknown"
)

// Disk 是一块硬盘：TempC 读不到时为空，Pool 是它所在的存储池（不在池里时为空）。
type Disk struct {
	Name  string   `json:"name"`
	Model string   `json:"model"`
	Kind  string   `json:"kind"`
	Size  uint64   `json:"size"`
	TempC *float64 `json:"temp_c,omitempty"`
	Pool  string   `json:"pool,omitempty"`
}

// 告警的级别（Alert.Level）。
const (
	AlertCritical = "critical"
	AlertWarning  = "warning"
	AlertInfo     = "info"
)

// Alert 是 NAS 自己的一条告警：At 是它发生的时间（来源没给时为空）。
type Alert struct {
	Level string     `json:"level"`
	Title string     `json:"title"`
	At    *time.Time `json:"at,omitempty"`
}

// Apps 是 NAS 上的应用（容器）：运行中、共几个、可更新几个。
type Apps struct {
	Running    int `json:"running"`
	Total      int `json:"total"`
	Upgradable int `json:"upgradable"`
}

// Sample 是历史的一拍：CPU 与 Mem 是百分比，Rx、Tx 是字节/秒。存储不进历史（存储卡画各池的分段条）。
type Sample struct {
	At  time.Time `json:"at"`
	CPU float64   `json:"cpu"`
	Mem float64   `json:"mem"`
	Rx  uint64    `json:"rx"`
	Tx  uint64    `json:"tx"`
}

// MemPercent 是内存已用的百分比（不知道总量时为 0）。
func (m Memory) MemPercent() float64 {
	if m.Total == 0 {
		return 0
	}
	return float64(m.Used) * 100 / float64(m.Total)
}

// sample 是快照对应的一拍历史。
func (s *Snapshot) sample() Sample {
	return Sample{At: s.At, CPU: s.CPU.Percent, Mem: s.Memory.MemPercent(), Rx: s.Network.RxBps, Tx: s.Network.TxBps}
}

// clone 是快照的一份拷贝：采集器给出去的快照不和它自己手里的共用切片与 map。
func (s *Snapshot) clone() *Snapshot {
	if s == nil {
		return nil
	}
	c := *s
	c.Network.Interfaces = append([]string(nil), s.Network.Interfaces...)
	c.Pools = append([]Pool(nil), s.Pools...)
	c.Disks = append([]Disk(nil), s.Disks...)
	c.Alerts = append([]Alert(nil), s.Alerts...)
	if s.Facets != nil {
		c.Facets = make(map[string]Facet, len(s.Facets))
		for k, v := range s.Facets {
			c.Facets[k] = v
		}
	}
	return &c
}

// lightFacets 是轻量采集会更新的几类数据：合并时只用新快照里的这几类，别的沿用上一次完整采集的。
var lightFacets = []string{FacetCPU, FacetCPUTemp, FacetMemory, FacetNetwork}

// mergeLight 把一次轻量采集并进上一份快照：CPU、内存、网络与运行时长用新的，其余沿用 prev。prev 为空时就是 light。
func mergeLight(prev, light *Snapshot) *Snapshot {
	if prev == nil {
		return light.clone()
	}
	out := prev.clone()
	out.At = light.At
	out.CPU = light.CPU
	out.Memory = light.Memory
	out.Network = light.Network
	out.Network.Interfaces = append([]string(nil), light.Network.Interfaces...)
	if light.System.UptimeSeconds > 0 {
		out.System.UptimeSeconds = light.System.UptimeSeconds
	}
	if out.Facets == nil {
		out.Facets = map[string]Facet{}
	}
	for _, k := range lightFacets {
		if f, ok := light.Facets[k]; ok {
			out.Facets[k] = f
		}
	}
	return out
}
