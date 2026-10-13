package tmpl

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var lists = map[string]bool{"items": true}

func data() map[string]any {
	return map[string]any{
		"title":      `他说 "好" <b>&`,
		"text":       "第一行\n第二行 ✓",
		"site":       "hdtime",
		"count":      int64(3),
		"bonus":      float64(1),
		"half":       1.5,
		"timestamp":  int64(1791800000),
		"size_bytes": int64(16106127360),
		"free":       true,
		"rule":       "",
		"items": []any{
			map[string]any{"site": "a", "size_bytes": int64(1024)},
			map[string]any{"site": "b", "size_bytes": int64(2048)},
		},
	}
}

func render(t *testing.T, src string, kind Kind) string {
	t.Helper()
	tp, err := Parse(src, kind, lists)
	require.NoError(t, err, src)
	out, err := tp.Check(data())
	require.NoError(t, err, src)
	return out
}

func TestFuncs(t *testing.T) {
	// json：引号、换行、Unicode、HTML 字符都写成合法 JSON，不转义 <>&
	out := render(t, `{"t":{{json .title}},"x":{{json .text}},"n":{{json .count}},"f":{{json .free}}}`, Body)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got), out)
	assert.Equal(t, `他说 "好" <b>&`, got["t"])
	assert.Equal(t, "第一行\n第二行 ✓", got["x"])
	assert.Contains(t, out, `<b>&`)
	assert.Equal(t, float64(3), got["n"])
	// 整份数据也能写成 JSON
	require.NoError(t, json.Unmarshal([]byte(render(t, `{{json .}}`, Body)), &got))
	assert.Equal(t, "hdtime", got["site"])

	cases := map[string]string{
		`{{.rule | default "无"}}`:                                       "无",
		`{{.site | default "无"}}`:                                       "hdtime",
		`{{.text | truncate 3}}`:                                        "第一行…",
		`{{.site | truncate 10}}`:                                       "hdtime",
		`{{.site | upper}}`:                                             "HDTIME",
		`{{.site | replace "hd" "HD"}}`:                                 "HDtime",
		`{{if contains "time" .site}}y{{end}}`:                          "y",
		`{{if hasPrefix "hd" .site}}y{{end}}`:                           "y",
		`{{split "-" "a-b" | join "+"}}`:                                "a+b",
		`{{bytes .size_bytes}}`:                                         "15.00 GB",
		`{{bytes 512}}`:                                                 "512 B",
		`{{printf "%05d|%.1f" .count .half}}`:                           "00003|1.5",
		`{{len .items}}`:                                                "2",
		`{{(index .items 1).site}}`:                                     "b",
		`{{range $i, $x := .items}}{{if $i}},{{end}}{{$x.site}}{{end}}`: "a,b",
		`{{range .items}}{{.site}}{{range $.items}}.{{end}}{{end}}`:     "a..b..",
	}
	for src, want := range cases {
		assert.Equal(t, want, render(t, src, Text), src)
	}

	// date：时间戳与两种写法的时间
	at := time.Unix(1791800000, 0).Local()
	assert.Equal(t, at.Format("01-02 15:04"), render(t, `{{date "01-02 15:04" .timestamp}}`, Text))
	assert.Equal(t, "2026-10-12", render(t, `{{date "2006-01-02" "2026-10-12 08:00:00"}}`, Text))
	assert.Equal(t, "2026-10-12", render(t, `{{date "2006-01-02" "2026-10-12T08:00:00+08:00"}}`, Text))
}

// 比较认数字：整数与浮点数可以直接比，变量取整数值还是带小数的值都一样能用
func TestNumberAwareComparisons(t *testing.T) {
	cases := map[string]string{
		`{{if eq .count 3}}y{{end}}`:           "y",
		`{{if eq .count 3.0}}y{{end}}`:         "y",
		`{{if gt .bonus 0}}y{{end}}`:           "y",
		`{{if eq .bonus 1}}y{{end}}`:           "y",
		`{{if lt .half 2}}y{{end}}`:            "y",
		`{{if ge .half 1.5}}y{{end}}`:          "y",
		`{{if gt .size_bytes 1e10}}y{{end}}`:   "y",
		`{{if eq .site "a" "hdtime"}}y{{end}}`: "y",
		`{{if ne .site "x"}}y{{end}}`:          "y",
		`{{if lt "a" "b"}}y{{end}}`:            "y",
		`{{if eq .free true}}y{{end}}`:         "y",
	}
	for src, want := range cases {
		assert.Equal(t, want, render(t, src, Text), src)
	}
	tp, err := Parse(`{{if eq .site 1}}y{{end}}`, Text, lists)
	require.NoError(t, err)
	_, err = tp.Check(data())
	assert.ErrorIs(t, err, ErrInvalid, "文字和数字不能比")
}

func TestRejected(t *testing.T) {
	bad := map[string]string{
		"":                                       "模板是空的",
		strings.Repeat("x", MaxSource+1):         "模板太长",
		`{{define "a"}}x{{end}}{{template "a"}}`: "define",
		`{{block "a" .}}x{{end}}`:                "define",
		`{{range 1000000000}}{{end}}`:            "range 只能遍历列表变量",
		`{{range .timestamp}}{{end}}`:            "range 只能遍历列表变量",
		`{{range .size_bytes}}{{end}}`:           "range 只能遍历列表变量",
		`{{$n := .count}}{{range $n}}{{end}}`:    "range 只能遍历列表变量",
		`{{range len .items}}{{end}}`:            "range 只能遍历列表变量",
		`{{range .site}}{{end}}`:                 "range 只能遍历列表变量",
		`{{range .items | len}}{{end}}`:          "range 只能遍历列表变量",
		`{{range .items}}{{range $.items}}{{range $.items}}{{end}}{{end}}{{end}}`: "最多嵌套 2 层",
		// 两个并排的两层空 range：100×(1+100)×2 = 20200 > 20000
		`{{range .items}}{{range $.items}}{{end}}{{end}}{{range .items}}{{range $.items}}{{end}}{{end}}`:                            "循环太多",
		`{{if .free}}{{range .items}}{{range $.items}}{{end}}{{end}}{{else}}{{range .items}}{{range $.items}}{{end}}{{end}}{{end}}`: "循环太多",
		`{{.nope`:     "第 1 行",
		`{{nope .x}}`: "没有这个函数：nope",
	}
	for src, want := range bad {
		_, err := Parse(src, Text, lists)
		require.Error(t, err, src)
		assert.ErrorIs(t, err, ErrInvalid)
		assert.Contains(t, err.Error(), want, src)
	}
	// 一个两层嵌套（10100）可以
	_, err := Parse(`{{range .items}}{{range $.items}}{{end}}{{end}}`, Text, lists)
	assert.NoError(t, err)
}

func TestExecuteErrors(t *testing.T) {
	// 变量名写错：Check 报出来，Execute 按零值
	tp, err := Parse(`{{.sitee}}`, Text, lists)
	require.NoError(t, err)
	_, err = tp.Check(data())
	assert.ErrorContains(t, err, "没有这个变量：sitee")

	tp, err = Parse(`{{printf "%999999999d" 1}}`, Text, lists)
	require.NoError(t, err)
	_, err = tp.Check(data())
	assert.ErrorContains(t, err, "printf 的宽度与精度最多")

	tp, err = Parse(`{{call .site}}`, Text, lists)
	require.NoError(t, err)
	_, err = tp.Check(data())
	assert.ErrorContains(t, err, "不支持 call")

	// 输出超过上限
	tp, err = Parse(`{{range .items}}{{range $.items}}`+strings.Repeat("x", 300)+`{{end}}{{end}}`, Title, lists)
	require.NoError(t, err)
	_, err = tp.Execute(data())
	assert.True(t, errors.Is(err, ErrInvalid))
	assert.ErrorContains(t, err, "渲染结果太长")
}

// 函数的中间结果也有上限：printf 的宽度写不大，也不能用 * 与 [n] 绕过；会让结果变长的函数，结果最多 64 KiB；
// 把参数连成一串的函数，参数合计也最多 64 KiB。否则几个函数一组合，最终输出的上限管不到之前就能造出几百 G 的字符串。
func TestIntermediateLimits(t *testing.T) {
	// 64 KiB 的空格：256 个空格里的每个都换成 256 个空格，正好在上限上
	const b64k = `{{$b := replace " " (printf "%256s" "") (printf "%256s" "")}}`
	bad := map[string]string{
		`{{printf "%1000000[1]s" "x"}}`:                                          "printf 的宽度与精度最多 256",
		`{{replace "" (printf "%1000000[1]s" "x") (printf "%1000000[1]s" "x")}}`: "printf 的宽度与精度最多 256",
		`{{printf "%[1]s" "x"}}`:                                                 "不支持 * 与 [n]",
		`{{printf "%[2]*[1]d" 1 5}}`:                                             "不支持 * 与 [n]",
		`{{printf "%*d" 5 1}}`:                                                   "不支持 * 与 [n]",
		`{{printf "%.*f" 2 1.5}}`:                                                "不支持 * 与 [n]",
		`{{printf "%.300f" 1.5}}`:                                                "printf 的宽度与精度最多 256",
		`{{printf "%0000300d" 1}}`:                                               "printf 的宽度与精度最多 256",
		// 256 + 257×256 字节
		`{{replace "" (printf "%256s" "x") (printf "%256s" "x")}}`: "结果太长",
		// 257 + 256×256 字节
		`{{join (printf "%256s" "") (split "" (print (printf "%256s" "") " "))}}`: "结果太长",
		// 32768 个 ɐ（每个 2 字节）转成大写是每个 3 字节
		`{{upper (replace " " (printf "%128s" "" | replace " " "ɐ") (printf "%256s" ""))}}`: "结果太长",
		b64k + `{{json $b}}`:                   "结果太长",
		b64k + `{{printf "%s%s" $b $b}}`:       "参数太长",
		b64k + `{{print $b $b}}`:               "参数太长",
		b64k + `{{println $b "x"}}`:            "参数太长",
		b64k + `{{html $b $b}}`:                "参数太长",
		b64k + `{{js $b "x"}}`:                 "参数太长",
		b64k + `{{urlquery $b "x"}}`:           "参数太长",
		b64k + `{{printf "%v" (split "" $b)}}`: "参数太长",
	}
	for src, want := range bad {
		tp, err := Parse(src, Body, lists)
		require.NoError(t, err, src)
		_, err = tp.Check(data())
		require.Error(t, err, src)
		assert.ErrorIs(t, err, ErrInvalid, src)
		assert.Contains(t, err.Error(), want, src)
	}

	ok := map[string]string{
		`{{printf "%%300d"}}`:           "%300d",
		`{{len (printf "%256s|" "x")}}`: "257",
		b64k + `{{len $b}}`:             "65536",
		// 256 + 255×256 字节，正好在上限上
		`{{len (join (printf "%256s" "") (split "" (printf "%256s" "")))}}`: "65536",
		`{{print "a" 1}}`:      "a1",
		`{{println "x"}}`:      "x\n",
		`{{html "<a>"}}`:       "&lt;a&gt;",
		`{{js "'"}}`:           `\'`,
		`{{urlquery "a b"}}`:   "a+b",
		`{{.site | urlquery}}`: "hdtime",
	}
	for src, want := range ok {
		assert.Equal(t, want, render(t, src, Body), src)
	}
}
