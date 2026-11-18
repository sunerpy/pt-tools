package nas

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/sunerpy/pt-tools/internal/redact"
)

// alertNames 是常见告警类型的中文名（类型与英文标题见 TrueNAS 25.04 的 alert.list_categories）：只写中文名就够的类型。
var alertNames = map[string]string{
	"HasUpdate":                       "TrueNAS 有新版本可以更新",
	"CatalogSyncFailed":               "应用目录同步失败",
	"CatalogNotHealthy":               "应用目录不正常",
	"ApplicationsConfigurationFailed": "应用服务配置失败",
	"ApplicationsStartFailed":         "应用服务启动失败",
	"Smartd":                          "smartd 没有运行",
	"NTPHealthCheck":                  "NTP 时间同步检查失败",
	"PoolUpgraded":                    "存储池有新的功能可以升级",
	"UPSOnBattery":                    "UPS 在用电池供电",
	"UPSBatteryLow":                   "UPS 电量低",
	"UPSReplbatt":                     "UPS 的电池要换了",
	"UPSCommbad":                      "和 UPS 的通信断了",
	"UPSCommok":                       "和 UPS 的通信恢复了",
	"UPSOnline":                       "UPS 恢复了市电供电",
}

// alertPrefixes 是要带上 TrueNAS 说明的告警类型：中文名后面接说明的第一行（哪个任务、哪块盘、哪张证书）。
var alertPrefixes = map[string]string{
	"SMART":                       "S.M.A.R.T. 报错",
	"BootPoolStatus":              "启动池状态不健康",
	"ScrubPaused":                 "存储池校验暂停了",
	"ScrubNotStarted":             "存储池校验没能开始",
	"ScrubStarted":                "存储池校验开始了",
	"ReplicationFailed":           "复制任务失败",
	"ReplicationSuccess":          "复制任务完成",
	"CloudSyncTaskFailed":         "云同步任务失败",
	"SnapshotFailed":              "快照任务失败",
	"SnapshotTotalCount":          "快照太多了",
	"SnapshotCount":               "快照太多了",
	"CertificateExpired":          "证书过期了",
	"CertificateIsExpiring":       "证书快过期了",
	"CertificateIsExpiringSoon":   "证书快过期了",
	"CertificateRevoked":          "证书被吊销了",
	"WebUiCertificateSetupFailed": "网页的 HTTPS 证书设置失败",
	"FailuresInAppMigration":      "应用迁移失败",
	"PoolUSBDisks":                "存储池用了 USB 硬盘",
}

// trueNASAlertTitle 是一条告警给人看的标题：常见的几类写成中文（知道 args 的带上池名、路径），其余用 TrueNAS 自己的说明
// （英文，只要第一行）。
func trueNASAlertTitle(a tnAlert) string {
	arg := func(k string) string { return alertArg(a.Args, k) }
	if n, ok := alertNames[a.Klass]; ok {
		return n
	}
	if p, ok := alertPrefixes[a.Klass]; ok {
		if strings.TrimSpace(a.Formatted) == "" {
			return p
		}
		return truncRunes(p+"："+alertFormatted(a), 120)
	}
	switch a.Klass {
	case "ZpoolCapacityNotice", "ZpoolCapacityWarning", "ZpoolCapacityCritical":
		if v, c := arg("volume"), arg("capacity"); v != "" && c != "" {
			return "存储池 " + v + " 已用 " + c + "%"
		}
	case "RsyncFailed", "RsyncSuccess":
		if p := arg("path"); p != "" {
			verb := "完成"
			if a.Klass == "RsyncFailed" {
				verb = "失败"
			}
			return "Rsync " + rsyncDirection(arg("direction")) + "任务" + verb + "：" + p
		}
	case "AppUpdate":
		if n := arg("name"); n != "" {
			return "应用 " + n + " 有新版本"
		}
	case "VolumeStatus":
		title := "存储池状态不健康"
		if v := arg("volume"); v != "" {
			title = "存储池 " + v + " 状态不健康"
		}
		if st := arg("state"); st != "" {
			title += "（" + st + "）"
		}
		return title
	}
	return alertFormatted(a)
}

// truncRunes 把 s 截到 n 个字（多出来时加省略号）。
func truncRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func rsyncDirection(d string) string {
	switch strings.ToUpper(d) {
	case "PULL":
		return "拉取"
	case "PUSH":
		return "推送"
	default:
		return ""
	}
}

var htmlTag = regexp.MustCompile(`<[^>]*>`)

// alertFormatted 是 TrueNAS 自己写的说明：去掉 HTML 标签与凭证，只要第一行，最长 120 个字；没有说明时写告警类型。
func alertFormatted(a tnAlert) string {
	s := htmlTag.ReplaceAllString(a.Formatted, "")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = truncRunes(strings.Join(strings.Fields(redact.Secrets(s)), " "), 120)
	if s == "" {
		s = a.Klass
	}
	return s
}

// alertArg 是告警 args 里的一项（args 是对象时）：字符串原样，数字去掉多余的小数位。
func alertArg(args any, key string) string {
	m, ok := args.(map[string]any)
	if !ok {
		return ""
	}
	switch v := m[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return ""
	}
}
