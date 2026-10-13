package notify

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify/tmpl"
)

// 事件目录自身一致：类型不重复；组是认识的，只有通知有标题；同一个变量名在各事件里种类一样（零值才统一）；
// 不和公共变量重名；列表有字段说明，也有对应的 items_more；示例的种类和声明的一致。
func TestEventCatalogConsistent(t *testing.T) {
	common := map[string]bool{}
	for _, v := range CommonVars {
		common[v.Key] = true
	}
	groups := map[string]bool{}
	for _, g := range Groups {
		groups[g.Key] = true
	}
	kinds := map[string]VarType{}
	seen := map[string]bool{}
	for _, spec := range Events {
		require.False(t, seen[spec.Type], "重复的事件 %s", spec.Type)
		seen[spec.Type] = true
		require.NotEmpty(t, spec.Name, spec.Type)
		require.True(t, groups[spec.Group], "%s 的组 %q 不认识", spec.Type, spec.Group)
		require.Equal(t, spec.Group != GroupNotify, spec.NoTitle, "%s：只有通知有标题", spec.Type)
		if spec.NoTitle {
			require.Empty(t, spec.SampleTitle, spec.Type)
		} else {
			require.NotEmpty(t, spec.SampleTitle, spec.Type)
		}
		require.NotEmpty(t, spec.SampleText, spec.Type)
		keys := map[string]bool{}
		for _, v := range spec.Vars {
			require.False(t, keys[v.Key], "%s 里重复的变量 %s", spec.Type, v.Key)
			keys[v.Key] = true
			require.False(t, common[v.Key], "%s 的 %s 和公共变量重名", spec.Type, v.Key)
			if k, ok := kinds[v.Key]; ok {
				require.Equal(t, k, v.Type, "变量 %s 在各事件里的种类不一样", v.Key)
			}
			kinds[v.Key] = v.Type
			if v.Type == VarList {
				require.NotEmpty(t, v.Item, "%s 的列表 %s 没有字段说明", spec.Type, v.Key)
				require.True(t, hasVar(spec.Vars, v.Key+"_more"), "%s 的列表 %s 没有 %s_more", spec.Type, v.Key, v.Key)
			}
		}
		ev, _, _ := SampleEvent(spec.Type)
		for _, v := range spec.Vars {
			assert.IsType(t, zeroOf(v.Type), ev.Fields[v.Key], "%s 的 %s 示例规范化以后的类型", spec.Type, v.Key)
		}
	}
}

func hasVar(vars []Var, key string) bool {
	for _, v := range vars {
		if v.Key == key {
			return true
		}
	}
	return false
}

func TestNewEventNormalizes(t *testing.T) {
	ev := NewEvent(EventReportDaily, time.Now(), map[string]any{
		"day":            "2026-10-12",
		"uploaded_bytes": int32(10),
		"bonus":          1, // 整数也转成 float64
		"nope":           "没声明的丢掉",
		"items": []map[string]any{
			{"site": "a", "bonus": 2, "uploaded_bytes": uint64(5), "extra": "丢掉"},
		},
	})
	assert.Equal(t, int64(10), ev.Fields["uploaded_bytes"])
	assert.Equal(t, float64(1), ev.Fields["bonus"])
	assert.Equal(t, "", ev.Fields["uploaded"], "没给的按零值补上")
	assert.NotContains(t, ev.Fields, "nope")
	items := ev.Fields["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	assert.Equal(t, float64(2), item["bonus"])
	assert.Equal(t, int64(5), item["uploaded_bytes"])
	assert.NotContains(t, item, "extra")
	assert.Equal(t, int64(0), ev.Fields["items_more"])

	// 列表截到 100 项，截掉的条数（加上生产者自己报的）记在 items_more
	long := make([]map[string]any, 130)
	for i := range long {
		long[i] = map[string]any{"name": fmt.Sprint(i)}
	}
	ev = NewEvent(EventDeadTorrents, time.Now(), map[string]any{"items": long, "items_more": 5})
	assert.Len(t, ev.Fields["items"], tmpl.MaxList)
	assert.Equal(t, int64(35), ev.Fields["items_more"])
}

// 实时发送与 JSON 往返以后（outbox、RSS 与监控通知的重试）变量的类型完全一样，渲染结果也一样
func TestEventJSONRoundTripKeepsTypes(t *testing.T) {
	for _, spec := range Events {
		ev, _, _ := SampleEvent(spec.Type)
		buf, err := json.Marshal(ev)
		require.NoError(t, err)
		var back Event
		require.NoError(t, json.Unmarshal(buf, &back))
		assert.Equal(t, ev.Fields, back.Fields, spec.Type)
		assert.True(t, ev.Time.Equal(back.Time))
	}

	src := `{{if gt .bonus 1000}}多{{else}}少{{end}} {{if eq .bonus 1234.5 1}}中{{end}} {{printf "%d|%.1f" .uploaded_bytes .bonus}} {{json .bonus}} {{range .items}}{{if ge .bonus 1}}{{.site}}{{end}}{{end}}`
	p, err := tmpl.Parse(src, tmpl.Text, ListVars())
	require.NoError(t, err)
	for _, bonus := range []any{1.0, 1.5, 1234.5, 2000} {
		ev := NewEvent(EventReportDaily, time.Now(), map[string]any{
			"bonus": bonus, "uploaded_bytes": 1 << 40,
			"items": []map[string]any{{"site": "a", "bonus": bonus}},
		})
		live, err := p.Check(ev.Data("t", "x"))
		require.NoError(t, err, bonus)
		buf, _ := json.Marshal(ev)
		var back Event
		require.NoError(t, json.Unmarshal(buf, &back))
		retried, err := p.Check(back.Data("t", "x"))
		require.NoError(t, err, bonus)
		assert.Equal(t, live, retried, "bonus=%v", bonus)
	}
}

// 每种事件的示例都能把它的全部变量渲染出来（示例里有全部变量；模板能引用每一个）
func TestEverySampleRendersAllVars(t *testing.T) {
	for _, spec := range Events {
		var b strings.Builder
		for _, v := range append(append([]Var{}, CommonVars...), spec.Vars...) {
			if v.Type == VarList {
				fmt.Fprintf(&b, "{{range .%s}}", v.Key)
				for _, f := range v.Item {
					fmt.Fprintf(&b, "{{.%s}}|", f.Key)
				}
				b.WriteString("{{end}}")
				continue
			}
			fmt.Fprintf(&b, "{{.%s}}|", v.Key)
		}
		out, err := RenderSample(spec.Type, tmpl.Body, b.String())
		require.NoError(t, err, spec.Type)
		assert.NotEmpty(t, out)
	}
	_, err := RenderSample("nope", tmpl.Text, "{{.title}}")
	assert.ErrorIs(t, err, tmpl.ErrInvalid)
	// 空模板给的是内置文字的示例
	out, err := RenderSample(EventTest, tmpl.Title, "")
	require.NoError(t, err)
	assert.Equal(t, "pt-tools 测试通知", out)
	// 变量名写错、渲染结果为空都会被拒
	_, err = RenderSample(EventRSSMatched, tmpl.Text, "{{.torrent_titl}}")
	assert.ErrorContains(t, err, "没有这个变量：torrent_titl")
	_, err = RenderSample(EventRSSMatched, tmpl.Text, "{{.rule | trim}}{{if false}}x{{end}} ")
	assert.NoError(t, err, "有内容")
	_, err = RenderSample(EventTest, tmpl.Text, "{{if false}}x{{end}}")
	assert.ErrorContains(t, err, "渲染结果是空的")
}

// 别的事件才有的变量在这个事件里是零值，不会打出 <no value>（Webhook 收到各种事件时用同一份模板）
func TestDataHasZeroValuesForOtherEvents(t *testing.T) {
	ev, title, text := SampleEvent(EventTest)
	p, err := tmpl.Parse(`[{{.torrent_title}}][{{.size_bytes}}][{{.free}}][{{len .items}}][{{.channel}}]`, tmpl.Text, ListVars())
	require.NoError(t, err)
	out, err := p.Check(ev.Data(title, text))
	require.NoError(t, err)
	assert.Equal(t, "[][0][false][0][我的 Telegram]", out)
	d := ev.Data(title, text)
	assert.Equal(t, EventTest, d["event"])
	assert.Equal(t, "通道测试", d["event_name"])
	assert.Equal(t, "2026-10-12 08:00:00", d["time"])
	assert.Equal(t, title, d["title"])
}

// 零值按组分：命令回复拿不到通知才有的变量（变量名写错会被查出来），通知拿不到回复的变量；公共变量各组都有，
// Webhook 的变量只在通知组。
func TestDataZeroValuesPerGroup(t *testing.T) {
	ev, _, text := SampleEvent(EventReplyStatus)
	d := ev.Data("", text)
	assert.NotContains(t, d, "torrent_title")
	assert.NotContains(t, d, "channel")
	assert.Contains(t, d, "downloader", "同一组别的回复的变量是零值")
	assert.Equal(t, "", d["downloader"])
	assert.Equal(t, text, d["text"])
	assert.Equal(t, "/status 状态概览", d["event_name"])

	_, err := RenderSample(EventReplyStatus, tmpl.Reply, "{{.torrent_title}}")
	assert.ErrorContains(t, err, "没有这个变量：torrent_title")

	n, title, ntext := SampleEvent(EventRSSMatched)
	nd := n.Data(title, ntext)
	assert.NotContains(t, nd, "kind")
	assert.NotContains(t, nd, "command")
	assert.Contains(t, nd, "channel")

	a, _, atext := SampleEvent(EventAssistantReply)
	ad := a.Data("", atext)
	assert.Equal(t, "answer", ad["kind"])
	assert.NotContains(t, ad, "command")
}

func TestEventGroupHelpers(t *testing.T) {
	assert.True(t, IsNotifyEvent(EventRSSMatched))
	assert.False(t, IsNotifyEvent(EventReplyOther))
	assert.False(t, IsNotifyEvent(EventAssistantReply))
	assert.False(t, IsNotifyEvent("nope"))
	assert.Equal(t, tmpl.Text, TextKind(EventRSSMatched))
	assert.Equal(t, tmpl.Reply, TextKind(EventReplyStatus))
	assert.Equal(t, tmpl.Reply, TextKind(EventAssistantReply))
	assert.Equal(t, tmpl.Text, TextKind("nope"))
	for _, spec := range Events {
		if spec.Group == GroupNotify {
			continue
		}
		assert.True(t, spec.NoTitle, spec.Type)
	}
}
