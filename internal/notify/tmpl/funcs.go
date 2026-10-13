package tmpl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"text/template"
	"time"
	"unicode/utf8"
)

// funcs 是模板能用的函数。eq/ne/lt/le/gt/ge 换成认数字的版本（整数与浮点数可以直接比），
// printf 限制宽度与精度，call 不让用（数据里没有函数，留着只会让人困惑）。
// 内置的 print、println、html、js、urlquery 会把所有参数连成一串，换成限制了参数合计与结果长度的版本。
func funcs() template.FuncMap {
	return template.FuncMap{
		"json":      toJSON,
		"default":   dflt,
		"truncate":  truncate,
		"upper":     func(s any) (string, error) { return capped(strings.ToUpper(str(s))) },
		"lower":     func(s any) (string, error) { return capped(strings.ToLower(str(s))) },
		"trim":      func(s any) string { return strings.TrimSpace(str(s)) },
		"replace":   replace,
		"contains":  func(sub string, s any) bool { return strings.Contains(str(s), sub) },
		"hasPrefix": func(p string, s any) bool { return strings.HasPrefix(str(s), p) },
		"hasSuffix": func(p string, s any) bool { return strings.HasSuffix(str(s), p) },
		"join":      join,
		"split":     func(sep string, s any) []string { return strings.Split(str(s), sep) },
		"bytes":     humanBytes,
		"date":      date,
		"printf":    safePrintf,
		"print":     bounded(fmt.Sprint),
		"println":   bounded(fmt.Sprintln),
		"html":      bounded(template.HTMLEscaper),
		"js":        bounded(template.JSEscaper),
		"urlquery":  bounded(template.URLQueryEscaper),
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

// maxIntermediate 是会让结果变长的函数的结果上限，也是把参数连成一串的函数的参数合计上限。
// 函数的结果还能交给别的函数、同一个长字符串还能作为参数写很多遍，最终输出的上限管不到这些中间结果：
// 没有这一条，replace、printf、html 这类可以先造出几百 G 的字符串。和最大的输出上限（请求体）一样大。
const maxIntermediate = 64 << 10

var (
	errTooLongResult = fmt.Errorf("结果太长：一个函数的结果最多 %d 字节", maxIntermediate)
	errTooLongArgs   = fmt.Errorf("参数太长：一个函数的参数合计最多 %d 字节", maxIntermediate)
)

func capped(s string) (string, error) {
	if len(s) > maxIntermediate {
		return "", errTooLongResult
	}
	return s, nil
}

// bounded 给把参数连成一串的函数（print、html 这些）加上限：先查参数合计，再查结果。
func bounded(f func(...any) string) func(...any) (string, error) {
	return func(args ...any) (string, error) {
		if err := checkArgs(args); err != nil {
			return "", err
		}
		return capped(f(args...))
	}
}

func checkArgs(args []any) error {
	size := 0
	for _, a := range args {
		if size += argSize(a); size > maxIntermediate {
			return errTooLongArgs
		}
	}
	return nil
}

// argSize 估算值写成文字有多长：字符串按字节数，列表与对象按各项之和，数字、真假值按 24 字节。
func argSize(v any) int {
	if v == nil {
		return 0
	}
	if s, ok := v.(string); ok {
		return len(s)
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.String:
		return r.Len()
	case reflect.Slice, reflect.Array:
		n := 0
		for i := range r.Len() {
			n += 1 + argSize(r.Index(i).Interface())
		}
		return n
	case reflect.Map:
		n := 0
		for it := r.MapRange(); it.Next(); {
			n += 2 + argSize(it.Key().Interface()) + argSize(it.Value().Interface())
		}
		return n
	}
	return 24
}

// toJSON 是值的 JSON 写法（字符串带引号、转义好），用在请求体里：{"content": {{json .text}}}。
func toJSON(v any) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return capped(strings.TrimSuffix(b.String(), "\n"))
}

// replace 把 s 里的 old 全部换成 repl；先算出结果多长，超过上限就不做（old 为空时每个字符之间都插一份 repl）。
func replace(old, repl string, s any) (string, error) {
	text := str(s)
	n := strings.Count(text, old)
	if size := len(text) + n*(len(repl)-len(old)); size > maxIntermediate {
		return "", errTooLongResult
	}
	return strings.ReplaceAll(text, old, repl), nil
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
	size := 0
	for i := range parts {
		parts[i] = str(r.Index(i).Interface())
		if size += len(parts[i]); i > 0 {
			size += len(sep)
		}
		if size > maxIntermediate {
			return "", errTooLongResult
		}
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
		return capped(t.Local().Format(layout))
	case string:
		if t == "" {
			return "", nil
		}
		for _, l := range []string{time.RFC3339Nano, time.DateTime} {
			if at, err := time.ParseInLocation(l, t, time.Local); err == nil {
				return capped(at.Local().Format(layout))
			}
		}
		return "", fmt.Errorf("date 读不懂这个时间：%q", t)
	}
	sec, ok := toFloat(v)
	if !ok {
		return "", fmt.Errorf("date 要的是时间戳或时间，给的是 %v", v)
	}
	return capped(time.Unix(int64(sec), 0).Local().Format(layout))
}

// maxFmtWidth 是 printf 的宽度与精度上限：不然 %999999999d 一下就要分配上 G 的内存
const maxFmtWidth = 256

// checkFormat 逐个看格式串里的占位符：从 % 读到动词（字母）为止，中间每一段数字（宽度、精度）都不能超过上限；
// 不认 * 与 [n]（用参数指定宽度、指定第几个参数），它们能绕开写在格式串里的数字。
func checkFormat(format string) error {
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		j := i + 1
		if j < len(format) && format[j] == '%' {
			i = j
			continue
		}
		num := 0
	spec:
		for ; j < len(format); j++ {
			c := format[j]
			switch {
			case c >= '0' && c <= '9':
				num = num*10 + int(c-'0')
				if num > maxFmtWidth {
					return fmt.Errorf("printf 的宽度与精度最多 %d", maxFmtWidth)
				}
			case c == '*' || c == '[' || c == ']':
				return errors.New("printf 不支持 * 与 [n] 的写法")
			case c == '.' || c == '-' || c == '+' || c == '#' || c == ' ':
				num = 0
			default:
				break spec
			}
		}
		i = j
	}
	return nil
}

// safePrintf 是限制了宽度与精度、参数合计与结果长度的 printf。
func safePrintf(format string, args ...any) (string, error) {
	if err := checkFormat(format); err != nil {
		return "", err
	}
	if err := checkArgs(args); err != nil {
		return "", err
	}
	return capped(fmt.Sprintf(format, args...))
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
