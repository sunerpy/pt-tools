package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/notify/tmpl"
)

// Event 是一条通知对应的事件：类型、发生时间与结构化变量，给消息模板与 Webhook 请求体用。
// 只能经 NewEvent 建：变量按事件目录里声明的种类规范化（int → int64，float → float64，列表截到 tmpl.MaxList 项），
// JSON 往返（outbox、RSS 与监控通知的重试）以后用同一个函数规范化，所以实时发送与重试拿到的类型完全一样。
type Event struct {
	Type   string         `json:"type"`
	Time   time.Time      `json:"time"`
	Fields map[string]any `json:"fields,omitempty"`
}

// NewEvent 建一个事件。fields 的键要和事件目录里声明的变量一致：没声明的丢掉，没给的按零值补上。
func NewEvent(typ string, at time.Time, fields map[string]any) *Event {
	return &Event{Type: typ, Time: at, Fields: normalizeFields(typ, roundTrip(fields))}
}

// UnmarshalJSON 解出持久化的事件，按目录重新规范化。
func (e *Event) UnmarshalJSON(b []byte) error {
	var w struct {
		Type   string          `json:"type"`
		Time   time.Time       `json:"time"`
		Fields json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	var fields map[string]any
	if len(w.Fields) > 0 && string(w.Fields) != "null" {
		dec := json.NewDecoder(bytes.NewReader(w.Fields))
		dec.UseNumber()
		if err := dec.Decode(&fields); err != nil {
			return fmt.Errorf("解析事件变量失败: %w", err)
		}
	}
	e.Type, e.Time, e.Fields = w.Type, w.Time, normalizeFields(w.Type, fields)
	return nil
}

// Data 是渲染模板用的数据：先放好同一组所有事件的全部变量的零值（Webhook 收到别的通知时不会打出 <no value>），
// 再放这个事件的变量与公共变量。title、text 是内置的标题与正文。目录里没有的事件类型按通知组算。
func (e *Event) Data(title, text string) map[string]any {
	group := GroupNotify
	spec := specByType[e.Type]
	if spec != nil {
		group = spec.Group
	}
	d := maps.Clone(zeroData[group])
	maps.Copy(d, e.Fields)
	d["event"] = e.Type
	d["event_name"] = e.Type
	if spec != nil {
		d["event_name"] = spec.Name
	}
	at := e.Time
	if at.IsZero() {
		at = time.Now()
	}
	d["time"] = at.Local().Format(time.DateTime)
	d["timestamp"] = at.Unix()
	d["title"] = title
	d["text"] = text
	return d
}

// roundTrip 把生产者给的变量经 JSON 编码、UseNumber 解码，换成 map、[]any、string、json.Number、bool 这几种。
func roundTrip(fields map[string]any) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	buf, err := json.Marshal(fields)
	if err != nil {
		return nil
	}
	var out map[string]any
	dec := json.NewDecoder(bytes.NewReader(buf))
	dec.UseNumber()
	if dec.Decode(&out) != nil {
		return nil
	}
	return out
}

// normalizeFields 按事件目录规范化变量；目录里没有的事件类型只把数字按字面换成 int64 或 float64。
func normalizeFields(typ string, raw map[string]any) map[string]any {
	spec := specByType[typ]
	if spec == nil {
		out := make(map[string]any, len(raw))
		for k, v := range raw {
			out[k] = generic(v)
		}
		return out
	}
	return convertVars(spec.Vars, raw)
}

func convertVars(vars []Var, raw map[string]any) map[string]any {
	out := make(map[string]any, len(vars))
	for _, v := range vars {
		if v.Type != VarList {
			out[v.Key] = convert(v.Type, raw[v.Key])
		}
	}
	// 列表截到 tmpl.MaxList 项，截掉的条数加进 <列表名>_more（生产者自己先截过的，它给的条数也算上）
	for _, v := range vars {
		if v.Type == VarList {
			list, more := convertList(v, raw[v.Key])
			prev, _ := toInt64(raw[v.Key+"_more"])
			out[v.Key] = list
			out[v.Key+"_more"] = more + max(prev, 0)
		}
	}
	return out
}

func convertList(v Var, x any) ([]any, int64) {
	items, _ := x.([]any)
	var more int64
	if len(items) > tmpl.MaxList {
		more = int64(len(items) - tmpl.MaxList)
		items = items[:tmpl.MaxList]
	}
	out := make([]any, 0, len(items))
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, convertVars(v.Item, m))
	}
	return out, more
}

func convert(typ VarType, x any) any {
	switch typ {
	case VarInt:
		n, _ := toInt64(x)
		return n
	case VarFloat:
		f, _ := toFloat64(x)
		return f
	case VarBool:
		switch b := x.(type) {
		case bool:
			return b
		case string:
			v, _ := strconv.ParseBool(b)
			return v
		}
		return false
	default:
		switch s := x.(type) {
		case nil:
			return ""
		case string:
			return s
		case json.Number:
			return s.String()
		}
		return fmt.Sprint(x)
	}
}

func toInt64(x any) (int64, bool) {
	switch n := x.(type) {
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i, true
		}
		if f, err := n.Float64(); err == nil {
			return int64(math.Trunc(f)), true
		}
	case string:
		if i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64); err == nil {
			return i, true
		}
	case int64:
		return n, true
	case float64:
		return int64(math.Trunc(n)), true
	}
	return 0, false
}

func toFloat64(x any) (float64, bool) {
	switch n := x.(type) {
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return f, true
		}
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
			return f, true
		}
	case float64:
		return n, true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// generic 只用于目录里没有的事件类型：数字按字面换成 int64 或 float64，列表与对象逐项换。
func generic(x any) any {
	switch v := x.(type) {
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		f, _ := v.Float64()
		return f
	case []any:
		out := make([]any, len(v))
		for i, it := range v {
			out[i] = generic(it)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, it := range v {
			out[k] = generic(it)
		}
		return out
	}
	return x
}
