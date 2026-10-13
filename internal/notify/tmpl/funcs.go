package tmpl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode/utf8"
)

// funcs 是模板能用的函数。eq/ne/lt/le/gt/ge 换成认数字的版本（整数与浮点数可以直接比），
// printf 限制宽度与精度，call 不让用（数据里没有函数，留着只会让人困惑）。
func funcs() template.FuncMap {
	return template.FuncMap{
		"json":      toJSON,
		"default":   dflt,
		"truncate":  truncate,
		"upper":     func(s any) string { return strings.ToUpper(str(s)) },
		"lower":     func(s any) string { return strings.ToLower(str(s)) },
		"trim":      func(s any) string { return strings.TrimSpace(str(s)) },
		"replace":   func(old, repl string, s any) string { return strings.ReplaceAll(str(s), old, repl) },
		"contains":  func(sub string, s any) bool { return strings.Contains(str(s), sub) },
		"hasPrefix": func(p string, s any) bool { return strings.HasPrefix(str(s), p) },
		"hasSuffix": func(p string, s any) bool { return strings.HasSuffix(str(s), p) },
		"join":      join,
		"split":     func(sep string, s any) []string { return strings.Split(str(s), sep) },
		"bytes":     humanBytes,
		"date":      date,
		"printf":    safePrintf,
		"eq":        eq,
		"ne":        ne,
		"lt":        func(a, b any) (bool, error) { return order(a, b, func(c int) bool { return c < 0 }) },
		"le":        func(a, b any) (bool, error) { return order(a, b, func(c int) bool { return c <= 0 }) },
		"gt":        func(a, b any) (bool, error) { return order(a, b, func(c int) bool { return c > 0 }) },
		"ge":        func(a, b any) (bool, error) { return order(a, b, func(c int) bool { return c >= 0 }) },
		"call":      func(...any) (string, error) { return "", errors.New("不支持 call") },
	}
}

// str 把值写成文字：字符串原样，其他按 fmt 的默认写法，nil 是空串。
func str(v any) string {
	switch s := v.(type) {
	case nil:
		return ""
	case string:
		return s
	}
	return fmt.Sprint(v)
}

// toJSON 是值的 JSON 写法（字符串带引号、转义好），用在请求体里：{"content": {{json .text}}}。
func toJSON(v any) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// dflt 是 default：值为空（nil、空串、0、false、空列表）时用 def。写法：{{.rule | default "无"}}。
func dflt(def, v any) any {
	if empty(v) {
		return def
	}
	return v
}

func empty(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return r.Len() == 0
	case reflect.Bool:
		return !r.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return r.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return r.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return r.Float() == 0
	}
	return false
}

// truncate 按字符截断，截掉了就补「…」。写法：{{.text | truncate 100}}。
func truncate(n, s any) (string, error) {
	limit, ok := toInt(n)
	if !ok || limit < 0 {
		return "", fmt.Errorf("truncate 的长度要是非负整数，给的是 %v", n)
	}
	text := str(s)
	if utf8.RuneCountInString(text) <= int(limit) {
		return text, nil
	}
	runes := []rune(text)
	return string(runes[:limit]) + "…", nil
}

// join 把列表的各项用 sep 连起来。
func join(sep string, list any) (string, error) {
	if list == nil {
		return "", nil
	}
	r := reflect.ValueOf(list)
	if r.Kind() != reflect.Slice && r.Kind() != reflect.Array {
		return "", fmt.Errorf("join 要的是列表，给的是 %T", list)
	}
	parts := make([]string, r.Len())
	for i := range parts {
		parts[i] = str(r.Index(i).Interface())
	}
	return strings.Join(parts, sep), nil
}

// humanBytes 把字节数写成「15.00 GB」（和 RSS 通知里的写法一样）。
func humanBytes(v any) (string, error) {
	n, ok := toFloat(v)
	if !ok {
		return "", fmt.Errorf("bytes 要的是数字，给的是 %v", v)
	}
	const unit = 1024
	if math.Abs(n) < unit {
		return fmt.Sprintf("%d B", int64(n)), nil
	}
	div, exp := float64(unit), 0
	for x := math.Abs(n) / unit; x >= unit && exp < 5; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", n/div, "KMGTPE"[exp]), nil
}

// date 按 Go 的版式写时间：v 是秒级时间戳，或者 RFC3339 / 「2006-01-02 15:04:05」写法的时间。写法：{{date "01-02 15:04" .timestamp}}。
func date(layout string, v any) (string, error) {
	switch t := v.(type) {
	case time.Time:
		return t.Local().Format(layout), nil
	case string:
		if t == "" {
			return "", nil
		}
		for _, l := range []string{time.RFC3339Nano, time.DateTime} {
			if at, err := time.ParseInLocation(l, t, time.Local); err == nil {
				return at.Local().Format(layout), nil
			}
		}
		return "", fmt.Errorf("date 读不懂这个时间：%q", t)
	}
	sec, ok := toFloat(v)
	if !ok {
		return "", fmt.Errorf("date 要的是时间戳或时间，给的是 %v", v)
	}
	return time.Unix(int64(sec), 0).Local().Format(layout), nil
}

// fmtWidthRe 找出格式串里每个占位符的宽度与精度
var fmtWidthRe = regexp.MustCompile(`%[-+# 0]*(\*|\d+)?(?:\.(\*|\d+))?[a-zA-Z%]`)

// maxFmtWidth 是 printf 的宽度与精度上限：不然 %999999999d 一下就要分配上 G 的内存
const maxFmtWidth = 256

// safePrintf 是限制了宽度与精度的 printf。
func safePrintf(format string, args ...any) (string, error) {
	for _, m := range fmtWidthRe.FindAllStringSubmatch(format, -1) {
		for _, w := range m[1:] {
			if w == "*" {
				return "", errors.New("printf 不支持用 * 指定宽度")
			}
			if w != "" {
				if n, err := strconv.Atoi(w); err != nil || n > maxFmtWidth {
					return "", fmt.Errorf("printf 的宽度与精度最多 %d", maxFmtWidth)
				}
			}
		}
	}
	return fmt.Sprintf(format, args...), nil
}

// eq 认数字：两边都是数字时按数值比（整数与浮点数可以比）；其他按同类比较。和内置的一样，可以给多个候选：eq .x 1 2 3。
func eq(a any, bs ...any) (bool, error) {
	if len(bs) == 0 {
		return false, errors.New("eq 至少要两个参数")
	}
	for _, b := range bs {
		ok, err := equal(a, b)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func ne(a, b any) (bool, error) {
	ok, err := equal(a, b)
	return !ok, err
}

func equal(a, b any) (bool, error) {
	if a == nil || b == nil {
		return a == nil && b == nil, nil
	}
	if c, ok := compareNumbers(a, b); ok {
		return c == 0, nil
	}
	ra, rb := reflect.ValueOf(a), reflect.ValueOf(b)
	switch {
	case ra.Kind() == reflect.String && rb.Kind() == reflect.String:
		return ra.String() == rb.String(), nil
	case ra.Kind() == reflect.Bool && rb.Kind() == reflect.Bool:
		return ra.Bool() == rb.Bool(), nil
	}
	return false, fmt.Errorf("不能比较 %s 和 %s", kindName(a), kindName(b))
}

// order 比大小：数字按数值，字符串按字典序；其他报错。
func order(a, b any, ok func(int) bool) (bool, error) {
	if c, isNum := compareNumbers(a, b); isNum {
		return ok(c), nil
	}
	if a != nil && b != nil {
		ra, rb := reflect.ValueOf(a), reflect.ValueOf(b)
		if ra.Kind() == reflect.String && rb.Kind() == reflect.String {
			return ok(strings.Compare(ra.String(), rb.String())), nil
		}
	}
	return false, fmt.Errorf("不能比较 %s 和 %s 的大小", kindName(a), kindName(b))
}

// compareNumbers 两边都是数字时返回比较结果：有一边是浮点数就按浮点比，否则按整数比。
func compareNumbers(a, b any) (int, bool) {
	ia, aInt := toIntExact(a)
	ib, bInt := toIntExact(b)
	if aInt && bInt {
		switch {
		case ia < ib:
			return -1, true
		case ia > ib:
			return 1, true
		}
		return 0, true
	}
	fa, aNum := toFloat(a)
	fb, bNum := toFloat(b)
	if !aNum || !bNum {
		return 0, false
	}
	switch {
	case fa < fb:
		return -1, true
	case fa > fb:
		return 1, true
	}
	return 0, true
}

// toIntExact 只认整数类型（不把浮点数当整数）。
func toIntExact(v any) (int64, bool) {
	if v == nil {
		return 0, false
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return r.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if u := r.Uint(); u <= math.MaxInt64 {
			return int64(u), true
		}
	}
	return 0, false
}

// toInt 认整数，也认没有小数部分的浮点数（函数参数用）。
func toInt(v any) (int64, bool) {
	if n, ok := toIntExact(v); ok {
		return n, true
	}
	if f, ok := toFloat(v); ok && f == math.Trunc(f) {
		return int64(f), true
	}
	return 0, false
}

func toFloat(v any) (float64, bool) {
	if v == nil {
		return 0, false
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(r.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(r.Uint()), true
	case reflect.Float32, reflect.Float64:
		return r.Float(), true
	}
	return 0, false
}

func kindName(v any) string {
	if v == nil {
		return "空值"
	}
	switch reflect.ValueOf(v).Kind() {
	case reflect.String:
		return "文字"
	case reflect.Bool:
		return "真假值"
	case reflect.Slice, reflect.Array:
		return "列表"
	case reflect.Map:
		return "对象"
	}
	if _, ok := toFloat(v); ok {
		return "数字"
	}
	return fmt.Sprintf("%T", v)
}
