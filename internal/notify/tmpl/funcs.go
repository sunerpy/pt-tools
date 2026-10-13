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

// funcs 是模板能用的函数，b 是这一次渲染的工作量预算（解析时传 nil：那时只用到函数名，见 Template.exec）。
// eq/ne/lt/le/gt/ge 换成认数字的版本（整数与浮点数可以直接比），printf 限制宽度与精度，
// call 不让用（数据里没有函数，留着只会让人困惑）；内置的 print、println、html、js、urlquery 会把所有参数连成一串，
// 也换成有上限的版本。处理文字与列表的函数都按参数与结果的大小扣同一份预算。
func funcs(b *budget) template.FuncMap {
	return template.FuncMap{
		"json":      b.toJSON,
		"default":   dflt,
		"truncate":  b.truncate,
		"upper":     b.convert(strings.ToUpper),
		"lower":     b.convert(strings.ToLower),
		"trim":      b.convert(strings.TrimSpace),
		"replace":   b.replace,
		"contains":  b.test(strings.Contains),
		"hasPrefix": b.test(strings.HasPrefix),
		"hasSuffix": b.test(strings.HasSuffix),
		"join":      b.join,
		"split":     b.split,
		"bytes":     humanBytes,
		"date":      b.date,
		"printf":    b.printf,
		"print":     b.concat(fmt.Sprint),
		"println":   b.concat(fmt.Sprintln),
		"html":      b.concat(template.HTMLEscaper),
		"js":        b.concat(template.JSEscaper),
		"urlquery":  b.concat(template.URLQueryEscaper),
		"eq":        b.eq,
		"ne":        b.compare(ne),
		"lt":        b.compare(orderBy(func(c int) bool { return c < 0 })),
		"le":        b.compare(orderBy(func(c int) bool { return c <= 0 })),
		"gt":        b.compare(orderBy(func(c int) bool { return c > 0 })),
		"ge":        b.compare(orderBy(func(c int) bool { return c >= 0 })),
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

const (
	// maxIntermediate 是一个函数的结果上限，也是把参数连成一串的函数的参数合计上限：
	// 函数的结果还能交给别的函数、同一个长字符串还能作为参数写很多遍，最终输出的上限管不到这些中间结果。
	// 和最大的输出上限（请求体）一样大。
	maxIntermediate = 64 << 10
	// maxWork 是一次渲染里所有函数合计能处理的字节数（参数加结果）。单个函数的上限管不到循环：
	// 两层 range 里每次都对一个 64 KiB 的变量调函数，一次渲染就要处理几个 G。
	maxWork = 4 << 20
	// elemOverhead 是列表与对象的每一项另算的字节数（字符串头的大小）：拆出很多个短字符串也占内存。
	elemOverhead = 16
)

var (
	errTooLongResult = fmt.Errorf("结果太长：一个函数的结果最多 %d 字节", maxIntermediate)
	errTooLongArgs   = fmt.Errorf("参数太长：一个函数的参数合计最多 %d 字节", maxIntermediate)
	errTooMuchWork   = fmt.Errorf("函数处理的内容太多：一次渲染里所有函数的参数与结果合计最多 %d 字节", maxWork)
	errNoBudget      = errors.New("模板函数只能在渲染时调用")
)

// budget 是一次渲染的工作量预算，这次渲染里的所有函数共用；每次渲染都重新建一份。
type budget struct{ left int }

func newBudget() *budget { return &budget{left: maxWork} }

// spend 扣掉 n 字节，不够就报错。没有预算（解析时绑定的函数被调用了）也报错。
func (b *budget) spend(n int) error {
	if b == nil {
		return errNoBudget
	}
	if n > b.left {
		b.left = 0
		return errTooMuchWork
	}
	b.left -= n
	return nil
}

// args 在调用之前按参数的大小扣预算。
func (b *budget) args(vs ...any) error {
	n := 0
	for _, v := range vs {
		n += argSize(v)
	}
	return b.spend(n)
}

// result 在调用之后按结果的大小扣预算，并检查一个结果的上限。
func (b *budget) result(s string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	if len(s) > maxIntermediate {
		return "", errTooLongResult
	}
	if err := b.spend(len(s)); err != nil {
		return "", err
	}
	return s, nil
}

// argsSize 是参数合计的大小，超过一次调用的上限就报错。
func argsSize(args []any) (int, error) {
	size := 0
	for _, a := range args {
		if size += argSize(a); size > maxIntermediate {
			return 0, errTooLongArgs
		}
	}
	return size, nil
}

// argSize 估算值有多大：字符串按字节数，列表与对象按各项之和、每项另加 elemOverhead，数字、真假值按 24 字节。
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
			n += elemOverhead + argSize(r.Index(i).Interface())
		}
		return n
	case reflect.Map:
		n := 0
		for it := r.MapRange(); it.Next(); {
			n += elemOverhead + argSize(it.Key().Interface()) + argSize(it.Value().Interface())
		}
		return n
	}
	return 24
}

// concat 是把参数连成一串的函数（print、html 这些）：参数合计与结果都有上限。
func (b *budget) concat(f func(...any) string) func(...any) (string, error) {
	return func(args ...any) (string, error) {
		size, err := argsSize(args)
		if err != nil {
			return "", err
		}
		if err := b.spend(size); err != nil {
			return "", err
		}
		return b.result(f(args...), nil)
	}
}

// convert 是 upper、lower、trim。
func (b *budget) convert(f func(string) string) func(any) (string, error) {
	return func(s any) (string, error) {
		if err := b.args(s); err != nil {
			return "", err
		}
		return b.result(f(str(s)), nil)
	}
}

// test 是 contains、hasPrefix、hasSuffix：模板里先写要找的，再写在哪里找（{{if contains "2160p" .torrent_title}}）。
func (b *budget) test(f func(s, sub string) bool) func(string, any) (bool, error) {
	return func(sub string, s any) (bool, error) {
		if err := b.args(sub, s); err != nil {
			return false, err
		}
		return f(str(s), sub), nil
	}
}

// compare 给比较记账：比较两个长字符串也要逐字节看。
func (b *budget) compare(f func(x, y any) (bool, error)) func(x, y any) (bool, error) {
	return func(x, y any) (bool, error) {
		if err := b.args(x, y); err != nil {
			return false, err
		}
		return f(x, y)
	}
}

func (b *budget) eq(x any, ys ...any) (bool, error) {
	if err := b.args(x); err != nil {
		return false, err
	}
	if err := b.args(ys...); err != nil {
		return false, err
	}
	return eq(x, ys...)
}

// toJSON 是值的 JSON 写法（字符串带引号、转义好），用在请求体里：{"content": {{json .text}}}。
func (b *budget) toJSON(v any) (string, error) {
	if err := b.args(v); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return b.result(strings.TrimSuffix(buf.String(), "\n"), nil)
}

// replace 把 s 里的 old 全部换成 repl；先算出结果多长，超过上限就不做（old 为空时每个字符之间都插一份 repl）。
func (b *budget) replace(old, repl string, s any) (string, error) {
	if err := b.args(old, repl, s); err != nil {
		return "", err
	}
	text := str(s)
	n := strings.Count(text, old)
	if size := len(text) + n*(len(repl)-len(old)); size > maxIntermediate {
		return "", errTooLongResult
	}
	return b.result(strings.ReplaceAll(text, old, repl), nil)
}

// split 按 sep 拆成列表；列表的每一项也按 elemOverhead 记账（按空串拆，每个字符都是一项）。
func (b *budget) split(sep string, s any) ([]string, error) {
	if err := b.args(sep, s); err != nil {
		return nil, err
	}
	text := str(s)
	if err := b.spend(elemOverhead * (strings.Count(text, sep) + 1)); err != nil {
		return nil, err
	}
	return strings.Split(text, sep), nil
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
func (b *budget) truncate(n, s any) (string, error) {
	limit, ok := toInt(n)
	if !ok || limit < 0 {
		return "", fmt.Errorf("truncate 的长度要是非负整数，给的是 %v", n)
	}
	if err := b.args(s); err != nil {
		return "", err
	}
	text := str(s)
	if utf8.RuneCountInString(text) <= int(limit) {
		return text, nil
	}
	runes := []rune(text)
	return b.result(string(runes[:limit])+"…", nil)
}

// join 把列表的各项用 sep 连起来。
func (b *budget) join(sep string, list any) (string, error) {
	if list == nil {
		return "", nil
	}
	r := reflect.ValueOf(list)
	if r.Kind() != reflect.Slice && r.Kind() != reflect.Array {
		return "", fmt.Errorf("join 要的是列表，给的是 %T", list)
	}
	if err := b.args(sep, list); err != nil {
		return "", err
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
	return b.result(strings.Join(parts, sep), nil)
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
func (b *budget) date(layout string, v any) (string, error) {
	if err := b.args(layout); err != nil {
		return "", err
	}
	return b.result(formatDate(layout, v))
}

func formatDate(layout string, v any) (string, error) {
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

// printf 限制了宽度与精度、参数合计与结果长度。
func (b *budget) printf(format string, args ...any) (string, error) {
	if err := checkFormat(format); err != nil {
		return "", err
	}
	size, err := argsSize(args)
	if err != nil {
		return "", err
	}
	if err := b.spend(len(format) + size); err != nil {
		return "", err
	}
	return b.result(fmt.Sprintf(format, args...), nil)
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

func orderBy(ok func(int) bool) func(a, b any) (bool, error) {
	return func(a, b any) (bool, error) { return order(a, b, ok) }
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
