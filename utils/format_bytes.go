package utils

import "fmt"

// FormatBytes 把字节数格式化成 IEC 单位（B、KiB、MiB、GiB、TiB、PiB），1 KiB 以上保留两位小数。
func FormatBytes(n int64) string {
	const unit = 1024
	if n < 0 {
		return "-" + FormatBytes(-n)
	}
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	value := float64(n) / unit
	i := 0
	for value >= unit && i < len(units)-1 {
		value /= unit
		i++
	}
	return fmt.Sprintf("%.2f %s", value, units[i])
}
