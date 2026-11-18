//go:build !qa

package outbound

// qaDivert 在正式构建里恒为空：不改连任何请求。
func qaDivert() string { return "" }
