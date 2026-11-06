package nas

import (
	"strings"
)

// 以下是 TrueNAS 各个方法回复里用到的字段（25.04 的 midclt 实测）。

// tnDate 是 TrueNAS 的时间：{"$date": 毫秒}。
type tnDate struct {
	Date int64 `json:"$date"`
}

type tnSystemInfo struct {
	Version       string    `json:"version"`
	Hostname      string    `json:"hostname"`
	Model         string    `json:"model"`
	Cores         int       `json:"cores"`
	Physmem       uint64    `json:"physmem"`
	UptimeSeconds float64   `json:"uptime_seconds"`
	Loadavg       []float64 `json:"loadavg"`
	Datetime      tnDate    `json:"datetime"`
}

// system 是主机行要的：版本去掉老版本带的「TrueNAS-SCALE-」前缀；型号是 CPU 型号（同本机来源）。
func (i tnSystemInfo) system() System {
	v := strings.TrimSpace(i.Version)
	for _, p := range []string{"TrueNAS-SCALE-", "TrueNAS-"} {
		v = strings.TrimPrefix(v, p)
	}
	return System{
		Hostname:      i.Hostname,
		Platform:      "TrueNAS",
		Version:       v,
		Model:         strings.TrimSpace(i.Model),
		Cores:         i.Cores,
		UptimeSeconds: int64(i.UptimeSeconds),
	}
}

type tnInterface struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	State struct {
		LinkState string `json:"link_state"`
	} `json:"state"`
}

type tnPool struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Healthy   *bool  `json:"healthy"`
	Size      uint64 `json:"size"`
	Allocated uint64 `json:"allocated"`
	Free      uint64 `json:"free"`
}

// tnProp 是数据集的一个属性：parsed 是换成数字的值。
type tnProp struct {
	Parsed *uint64 `json:"parsed"`
}

type tnDataset struct {
	ID        string `json:"id"`
	Used      tnProp `json:"used"`
	Available tnProp `json:"available"`
}

type tnDisk struct {
	Name  string `json:"name"`
	Model string `json:"model"`
	Type  string `json:"type"`
	Size  uint64 `json:"size"`
	Pool  string `json:"pool"`
}

// tnAlert 是一条告警：args 随告警类型不同，可能是对象、列表或 null。
type tnAlert struct {
	Klass     string `json:"klass"`
	Level     string `json:"level"`
	Formatted string `json:"formatted"`
	Args      any    `json:"args"`
	Dismissed bool   `json:"dismissed"`
	Datetime  tnDate `json:"datetime"`
}

type tnApp struct {
	Name                  string `json:"name"`
	State                 string `json:"state"`
	UpgradeAvailable      bool   `json:"upgrade_available"`
	ImageUpdatesAvailable bool   `json:"image_updates_available"`
}

// tnGraph 是 reporting.get_data 的一张图：legend 第一列是时间，data 每行同 legend；aggregations.mean 是这段时间各列的平均。
type tnGraph struct {
	Name         string       `json:"name"`
	Identifier   *string      `json:"identifier"`
	Legend       []string     `json:"legend"`
	Data         [][]*float64 `json:"data"`
	Aggregations struct {
		Mean map[string]*float64 `json:"mean"`
	} `json:"aggregations"`
}

// findGraph 是 list 里名字是 name 的第一张图（没有时是空图）。
func findGraph(list []tnGraph, name string) tnGraph {
	for _, g := range list {
		if g.Name == name {
			return g
		}
	}
	return tnGraph{}
}

func (g tnGraph) col(name string) int {
	for i, l := range g.Legend {
		if l == name {
			return i
		}
	}
	return -1
}

// mean 是一列这段时间的平均：先用 TrueNAS 算好的，没有时按各行算（跳过空值）；没有数据时 ok 为假。
func (g tnGraph) mean(name string) (v float64, ok bool) {
	if m, has := g.Aggregations.Mean[name]; has && m != nil {
		return *m, true
	}
	i := g.col(name)
	if i < 0 {
		return 0, false
	}
	var sum float64
	n := 0
	for _, row := range g.Data {
		if len(row) > i && row[i] != nil {
			sum += *row[i]
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}

// last 是一列最后一个不是空的值。
func (g tnGraph) last(name string) (float64, bool) {
	i := g.col(name)
	if i < 0 {
		return 0, false
	}
	for r := len(g.Data) - 1; r >= 0; r-- {
		if row := g.Data[r]; len(row) > i && row[i] != nil {
			return *row[i], true
		}
	}
	return 0, false
}
