package notify

import (
	"slices"
	"strings"
)

// NormalizeIDs 是通道名单的规整规则：去掉首尾空白、跳过空项、去重，保持原来的顺序。
// 飞书应用机器人解析配置（adapter/feishuapp）与 app 层读名单（绑定页、通知收件人）共用它，两边认的名单一样。
func NormalizeIDs(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item)
		if id == "" || slices.Contains(out, id) {
			continue
		}
		out = append(out, id)
	}
	return out
}
