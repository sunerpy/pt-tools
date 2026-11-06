package health

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sunerpy/pt-tools/internal/nas"
)

const (
	// nasStale 是 NAS 的数据多久没更新算过时、连不上多久算失败（采集每 30 秒一次）
	nasStale = 3 * time.Minute
	// nasHotHDD、nasHotSSD 是机械盘、固态盘（含 NVMe）到多少度要提醒（°C）；不知道类型的按机械盘算
	nasHotHDD = 50
	nasHotSSD = 70
)

// NAS 看「系统 → NAS 监控」里每个启用的来源一项：连不连得上、存储池健不健康、有没有严重告警、硬盘热不热。
// 只读采集器内存里的状态与最新快照，不连 NAS，重复体检不会多登录一次。本机来源不算（它读的是 pt-tools 所在的主机，
// 磁盘空间那一组已经看了）。list 为空表示 NAS 监控没有启动。
func NAS(list func() []nas.Status, now func() time.Time) Check {
	return Check{Group: GroupNAS, Title: "NAS", Run: func(context.Context) []Item {
		if list == nil {
			return []Item{{Key: "nas", Group: GroupNAS, Title: "NAS", Status: StatusSkip, Detail: "NAS 监控没有启动"}}
		}
		t := now()
		var items []Item
		added := 0
		for _, st := range list() {
			if st.ID == nas.LocalID {
				continue
			}
			added++
			if st.Enabled {
				items = append(items, nasItem(st, t))
			}
		}
		if len(items) > 0 {
			return items
		}
		it := Item{Key: "nas", Group: GroupNAS, Title: "NAS", Status: StatusSkip}
		if added > 0 {
			it.Detail, it.Hint = "NAS 来源都停用了", "在「系统 → NAS 监控」里编辑来源，打开「启用」"
		} else {
			it.Detail = "还没添加 NAS 来源"
			it.Hint = "在「系统 → NAS 监控」里添加来源" + nasKindLabels() + "，体检会看它的存储池、告警与硬盘温度"
		}
		return []Item{it}
	}}
}

// nasKindLabels 是能接入的来源类型（按名字排，例如「（能接入 Glances、TrueNAS）」）；一种都没有时为空。
func nasKindLabels() string {
	var labels []string
	for _, k := range nas.Kinds() {
		labels = append(labels, k.Label)
	}
	if len(labels) == 0 {
		return ""
	}
	return "（能接入 " + strings.Join(labels, "、") + "）"
}

// nasProblem 是一个来源的一个问题：一项里有几个问题时，状态取最坏的，说明依次写，怎么修写最坏的那个的。
type nasProblem struct {
	status, text, hint string
}

func nasItem(st nas.Status, now time.Time) Item {
	it := Item{Key: "nas:" + st.ID, Group: GroupNAS, Title: st.Name}
	label := nasKindLabel(st.Kind)
	if strings.TrimSpace(it.Title) == "" {
		it.Title = label
	}
	reason := func(def string) string {
		if m := short(st.Message); m != "" {
			return m
		}
		return def
	}
	switch st.State {
	case nas.StateAuthFailed:
		it.Status, it.Detail = StatusFail, reason("认证失败")
		it.Hint = "在「系统 → NAS 监控」里编辑这个来源，重新填 API Key 或密码后保存；改好以前 pt-tools 不会再登录"
		return it
	case nas.StateCertUntrusted:
		it.Status, it.Detail = StatusFail, reason("证书不受信任")
		it.Hint = "在「系统 → NAS 监控」里编辑这个来源、点「测试连接」，核对指纹是这台 NAS 的以后点「信任这张证书并再测一次」，再保存"
		return it
	case nas.StateCertChanged:
		it.Status, it.Detail = StatusFail, reason("证书和信任的那张不一样")
		it.Hint = "先确认是你自己换了 NAS 的证书：是的话在「系统 → NAS 监控」里编辑这个来源、测试连接并信任新证书；" +
			"不是的话先别连，查一下是不是连到了别的机器"
		return it
	case nas.StateUnsupported:
		it.Status, it.Detail = StatusSkip, reason("这个版本不支持这类来源")
		return it
	case nas.StateConnecting:
		it.Status, it.Detail = StatusSkip, "正在连接，还没读到数据"
		return it
	}

	var probs []nasProblem
	switch {
	case st.State == nas.StateError && st.FailingSince == nil:
		// 没有在重试：设置本身用不了（保存的密钥解不开、地址不对），改好以前不会好
		probs = append(probs, nasProblem{StatusFail, reason("用不了"), "在「系统 → NAS 监控」里编辑这个来源，改好地址或重新填 API Key、密码后保存"})
	case st.State == nas.StateError && now.Sub(*st.FailingSince) >= nasStale:
		text := fmt.Sprintf("%s 起连不上：%s", stamp(*st.FailingSince), reason("出错了"))
		if st.LastOKAt != nil {
			text += "（最后一次读到是 " + stamp(*st.LastOKAt) + "）"
		}
		probs = append(probs, nasProblem{StatusFail, text, "看 NAS 开着没有、地址与端口对不对；修好以后在「系统 → NAS 监控」里点「重新连接」，或等它自己重试"})
	case st.State == nas.StateError:
		text := fmt.Sprintf("%s 起连不上，正在重试：%s", stamp(*st.FailingSince), reason("出错了"))
		probs = append(probs, nasProblem{StatusWarn, text, "多半是 NAS 在重启或网络断了一下；连不上超过 3 分钟会记成失败"})
	case st.LastOKAt != nil && now.Sub(*st.LastOKAt) >= nasStale:
		probs = append(probs, nasProblem{StatusWarn, "数据过时了：最后一次读到是 " + stamp(*st.LastOKAt), "NAS 的采集像是停了：在「系统 → NAS 监控」里点「重新连接」，还不行就重启 pt-tools"})
	}
	// 存储池、告警与硬盘只看新的数据：过时的快照不当成现在的样子
	snap := st.Snapshot
	fresh := snap != nil && now.Sub(snap.At) < nasStale
	if fresh {
		probs = append(probs, nasSnapshotProblems(snap, label)...)
	}

	it.Status = StatusOK
	var texts []string
	for _, rank := range []string{StatusFail, StatusWarn} {
		for _, p := range probs {
			if p.status != rank {
				continue
			}
			if it.Status == StatusOK {
				it.Status, it.Hint = p.status, p.hint
			}
			texts = append(texts, p.text)
		}
	}
	switch {
	case len(texts) > 0:
		it.Detail = joinDetail(texts...)
	case fresh:
		it.Detail = joinDetail(append(nasSummary(snap, label), stamp(snap.At)+" 更新")...)
	default:
		// 连着、也没过时，但还没有快照（不该有这种情况）
		it.Status, it.Detail = StatusSkip, "还没读到数据"
	}
	return it
}

// nasKindLabel 是来源类型给人看的名字（TrueNAS、群晖…）；这个版本不认的类型写原样。
func nasKindLabel(kind string) string {
	if k, ok := nas.LookupKind(kind); ok {
		return k.Label
	}
	if kind == "" {
		return "NAS"
	}
	return kind
}

// nasSnapshotProblems 是快照里的问题：存储池不健康是失败；严重告警、硬盘偏热是提醒。读不到的那几类不算问题（总结里写）。
func nasSnapshotProblems(s *nas.Snapshot, label string) []nasProblem {
	var out []nasProblem
	if s.Facets[nas.FacetPools].Status == nas.FacetOK {
		var bad []string
		for _, p := range s.Pools {
			if p.Healthy == nil || *p.Healthy {
				continue
			}
			name := p.Name
			if p.Status != "" {
				name += "（" + p.Status + "）"
			}
			bad = append(bad, name)
		}
		if len(bad) > 0 {
			out = append(out, nasProblem{
				StatusFail, "存储池不健康：" + strings.Join(bad, "、"),
				"在" + nasInText(label) + "的存储页里看是哪块盘出了问题；换盘或修复以后这里会恢复",
			})
		}
	}
	if s.Facets[nas.FacetAlerts].Status == nas.FacetOK {
		var n int
		var latest *nas.Alert
		for i, a := range s.Alerts {
			if a.Level != nas.AlertCritical {
				continue
			}
			n++
			if latest == nil || (a.At != nil && (latest.At == nil || a.At.After(*latest.At))) {
				latest = &s.Alerts[i]
			}
		}
		if n > 0 {
			out = append(out, nasProblem{
				StatusWarn, fmt.Sprintf("%d 条严重告警，最新一条：%s", n, latest.Title),
				"在" + nasInText(label) + "的告警里看详情并处理；处理好或在那里关掉以后这里不再提醒",
			})
		}
	}
	if s.Facets[nas.FacetDisks].Status == nas.FacetOK {
		var hot []string
		for _, d := range s.Disks {
			if d.TempC != nil && *d.TempC >= nasHotLimit(d.Kind) {
				hot = append(hot, fmt.Sprintf("%s %.0f °C", d.Name, *d.TempC))
			}
		}
		if len(hot) > 0 {
			out = append(out, nasProblem{
				StatusWarn, "硬盘偏热：" + strings.Join(hot, "、"),
				"看机箱的散热与风扇；机械盘长期在 50 °C 以上、固态盘在 70 °C 以上寿命会变短",
			})
		}
	}
	return out
}

// nasInText 是嵌进中文句子里的来源类型名：拉丁字母开头或结尾的（TrueNAS、Glances）在那一边留空格，中文的（群晖、飞牛）不留。
func nasInText(label string) string {
	if label == "" {
		return ""
	}
	first, _ := utf8.DecodeRuneInString(label)
	last, _ := utf8.DecodeLastRuneInString(label)
	if first < utf8.RuneSelf {
		label = " " + label
	}
	if last < utf8.RuneSelf {
		label += " "
	}
	return label
}

// nasHotLimit 是这类硬盘到多少度要提醒。
func nasHotLimit(kind string) float64 {
	if kind == nas.DiskSSD || kind == nas.DiskNVMe {
		return nasHotSSD
	}
	return nasHotHDD
}

// nasSummary 是没有问题时说明里写的：系统与版本、存储池、硬盘、告警，读不到的那几类也写上。
func nasSummary(s *nas.Snapshot, label string) []string {
	sys := s.System.Platform
	if sys == "" {
		sys = label
	}
	if s.System.Version != "" {
		sys += " " + s.System.Version
	}
	if s.System.Hostname != "" {
		sys += "（" + s.System.Hostname + "）"
	}
	parts := []string{sys}
	var unread []string
	facet := func(key, name string) bool {
		switch s.Facets[key].Status {
		case nas.FacetOK:
			return true
		case nas.FacetDenied, nas.FacetError:
			unread = append(unread, name)
		}
		return false
	}
	if facet(nas.FacetPools, "存储池") && len(s.Pools) > 0 {
		known := 0
		for _, p := range s.Pools {
			if p.Healthy != nil {
				known++
			}
		}
		if known == len(s.Pools) {
			parts = append(parts, fmt.Sprintf("%d 个存储池都健康", len(s.Pools)))
		} else {
			// 有的来源（Glances 的文件系统）读不到健康状态：不说「都健康」
			parts = append(parts, fmt.Sprintf("%d 个存储池", len(s.Pools)))
		}
	}
	if facet(nas.FacetDisks, "硬盘") && len(s.Disks) > 0 {
		hottest := -1.0
		for _, d := range s.Disks {
			if d.TempC != nil && *d.TempC > hottest {
				hottest = *d.TempC
			}
		}
		if hottest >= 0 {
			parts = append(parts, fmt.Sprintf("%d 块硬盘，最高 %.0f °C", len(s.Disks), hottest))
		} else {
			parts = append(parts, fmt.Sprintf("%d 块硬盘", len(s.Disks)))
		}
	}
	if facet(nas.FacetAlerts, "告警") {
		if len(s.Alerts) == 0 {
			parts = append(parts, "没有告警")
		} else {
			parts = append(parts, fmt.Sprintf("%d 条告警，都不严重", len(s.Alerts)))
		}
	}
	if len(unread) > 0 {
		parts = append(parts, "读不到"+strings.Join(unread, "、"))
	}
	return parts
}
