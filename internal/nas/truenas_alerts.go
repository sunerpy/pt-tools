package nas

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/sunerpy/pt-tools/internal/redact"
)

// trueNASAlertTitle 是一条告警给人看的标题：常见的几类按 args 写成中文，其余用 TrueNAS 自己的说明（英文，只要第一行）。
func trueNASAlertTitle(a tnAlert) string {
	arg := func(k string) string { return alertArg(a.Args, k) }
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
	case "HasUpdate":
		return "TrueNAS 有新版本可以更新"
	case "CatalogSyncFailed":
		return "应用目录同步失败"
	}
	return alertFormatted(a)
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
	s = strings.Join(strings.Fields(redact.Secrets(s)), " ")
	if r := []rune(s); len(r) > 120 {
		s = string(r[:120]) + "…"
	}
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
