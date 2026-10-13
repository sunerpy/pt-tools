package web

import "encoding/json"

// jsonPath 把路径写成 JSON 字符串（带引号）：Windows 的路径里有反斜杠，直接拼进 JSON 会变成不合法的转义（#542）。
func jsonPath(p string) string {
	b, _ := json.Marshal(p)
	return string(b)
}
