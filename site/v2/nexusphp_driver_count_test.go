package v2

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildNexusPHPCountRow 构造一行标准 NexusPHP 搜索结果表格。
// 列顺序与 DefaultNexusPHPSelectors 一致：第 5 列为体积，第 6/7/8 列分别为
// 做种数 / 下载数 / 完成数。三个计数列接收原始 HTML 片段，用于模拟真实站点
// 里 <b>7,170</b> 这类带千分位分隔符的 markup。
func buildNexusPHPCountRow(seeders, leechers, snatched string) string {
	return fmt.Sprintf(`
	<html>
	<body>
	<table class="torrents">
		<tbody>
			<tr><td>Header</td></tr>
			<tr>
				<td><img alt="Movie" /></td>
				<td><a href="details.php?id=99001">Count Separator Repro 2026</a></td>
				<td></td>
				<td><span>2026-01-01</span></td>
				<td>1.5 GB</td>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
			</tr>
		</tbody>
	</table>
	</body>
	</html>
	`, seeders, leechers, snatched)
}

// TestNexusPHPDriver_ParseSearch_CountSeparators 覆盖共享 NexusPHP 驱动里
// 做种数 / 下载数 / 完成数的解析。
//
// 回归背景：这三个字段原先直接调用 strconv.Atoi 且丢弃 error，遇到站点渲染的
// 千分位分隔符（如 ptzone.xyz 的 <b>7,170</b>）会静默得到 0，导致依赖计数的
// RSS 过滤规则失效。
func TestNexusPHPDriver_ParseSearch_CountSeparators(t *testing.T) {
	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: "https://example.com",
		Cookie:  "test-cookie",
	})

	tests := []struct {
		name         string
		seeders      string
		leechers     string
		snatched     string
		wantSeeders  int
		wantLeechers int
		wantSnatched int
	}{
		{
			// THE BUG：取自 ptzone.xyz 真实抓包 markup，修复前 Snatched 为 0
			name:         "千分位分隔符（完成数）",
			seeders:      "12",
			leechers:     "3",
			snatched:     "<b>7,170</b>",
			wantSeeders:  12,
			wantLeechers: 3,
			wantSnatched: 7170,
		},
		{
			// 三列都带分隔符，证明修复覆盖全部三个字段而非只有一个
			name:         "千分位分隔符（三列同时）",
			seeders:      "1,024",
			leechers:     "2,048",
			snatched:     "3,072",
			wantSeeders:  1024,
			wantLeechers: 2048,
			wantSnatched: 3072,
		},
		{
			name:         "多组分隔符",
			seeders:      "1,234,567",
			leechers:     "12,345",
			snatched:     "9,876,543",
			wantSeeders:  1234567,
			wantLeechers: 12345,
			wantSnatched: 9876543,
		},
		{
			// 回归保护：普通整数必须与修复前完全一致
			name:         "普通整数",
			seeders:      "42",
			leechers:     "7",
			snatched:     "99",
			wantSeeders:  42,
			wantLeechers: 7,
			wantSnatched: 99,
		},
		{
			name:         "零值",
			seeders:      "0",
			leechers:     "0",
			snatched:     "0",
			wantSeeders:  0,
			wantLeechers: 0,
			wantSnatched: 0,
		},
		{
			// 回归保护：空单元格行为不变，仍为 0
			name:         "空单元格",
			seeders:      "",
			leechers:     "",
			snatched:     "",
			wantSeeders:  0,
			wantLeechers: 0,
			wantSnatched: 0,
		},
		{
			// 回归保护：无数字的占位文本行为不变，仍为 0
			name:         "无数字占位文本",
			seeders:      "N/A",
			leechers:     "--",
			snatched:     "暂无",
			wantSeeders:  0,
			wantLeechers: 0,
			wantSnatched: 0,
		},
		{
			// 负数必须保留符号。这条用例防止再次改回 [\d,]+ 这类正则提取：
			// 该字符类不含 '-'，会把 -3 吃成 3，静默反转语义。
			name:         "负数保留符号",
			seeders:      "-3",
			leechers:     "-12",
			snatched:     "-1",
			wantSeeders:  -3,
			wantLeechers: -12,
			wantSnatched: -1,
		},
		{
			// 选择器配错时才会命中日期列。期望 0（而非 2026）：宽松提取会把年份
			// 当成计数返回，看似合理却完全错误，掩盖配置错误。
			name:         "日期单元格（选择器配错）",
			seeders:      "2026-08-10 14:38:57",
			leechers:     "2026-08-10 14:38:57",
			snatched:     "2026-08-10 14:38:57",
			wantSeeders:  0,
			wantLeechers: 0,
			wantSnatched: 0,
		},
		{
			// 同上，误取到"剩余时间"列。期望 0（而非 2），保持失败可见。
			name:         "相对时间单元格（选择器配错）",
			seeders:      "2天22时",
			leechers:     "5时30分",
			snatched:     "3月1天",
			wantSeeders:  0,
			wantLeechers: 0,
			wantSnatched: 0,
		},
		{
			// 只处理逗号分隔符；空格分组不在支持范围内，期望 0（而非 1），
			// 避免把 1 234 截断成 1 这种量级错误。
			name:         "空格分组不支持",
			seeders:      "1 234",
			leechers:     "12 345",
			snatched:     "1 234 567",
			wantSeeders:  0,
			wantLeechers: 0,
			wantSnatched: 0,
		},
		{
			// 回归保护：strconv.Atoi 原生接受显式正号，行为不变
			name:         "显式正号",
			seeders:      "+5",
			leechers:     "+0",
			snatched:     "+1234",
			wantSeeders:  5,
			wantLeechers: 0,
			wantSnatched: 1234,
		},
		{
			// 回归保护：单元格内外多余空白由 TrimSpace 处理，行为不变
			name:         "首尾空白",
			seeders:      "  88  ",
			leechers:     "\n 7 \t",
			snatched:     "  1,024  ",
			wantSeeders:  88,
			wantLeechers: 7,
			wantSnatched: 1024,
		},
		{
			// 回归保护：小数与破折号占位仍为 0，与修复前一致
			name:         "小数与破折号占位",
			seeders:      "1.5",
			leechers:     "—",
			snatched:     "3.14",
			wantSeeders:  0,
			wantLeechers: 0,
			wantSnatched: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(
				strings.NewReader(buildNexusPHPCountRow(tt.seeders, tt.leechers, tt.snatched)),
			)
			require.NoError(t, err)

			items, err := driver.ParseSearch(NexusPHPResponse{Document: doc})
			require.NoError(t, err)
			require.Len(t, items, 1)

			assert.Equal(t, tt.wantSeeders, items[0].Seeders, "Seeders")
			assert.Equal(t, tt.wantLeechers, items[0].Leechers, "Leechers")
			assert.Equal(t, tt.wantSnatched, items[0].Snatched, "Snatched")

			// 体积解析走独立路径，此处仅作旁路回归保护
			assert.Positive(t, items[0].SizeBytes, "SizeBytes 不应受计数解析改动影响")
		})
	}
}
