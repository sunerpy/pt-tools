package assistant

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStripToolMarkup(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"没有标记的回答。", "没有标记的回答。"},
		{"我来查一下。\n\n<｜DSML｜function_calls", "我来查一下。"},
		{"我来查一下。<｜DSML｜function_calls>\n<｜DSML｜invoke name=\"list_tasks\">\n</｜DSML｜function_calls>", "我来查一下。"},
		{"好的。<｜tool▁calls▁begin｜><｜tool▁call▁begin｜>function<｜tool▁sep｜>list_tasks\n{}<｜tool▁call▁end｜><｜tool▁calls▁end｜>", "好的。"},
		{"<｜tool▁call▁begin｜>function<｜tool▁sep｜>x", ""},
		{"比较符号 a < b 不算标记", "比较符号 a < b 不算标记"},
	} {
		assert.Equal(t, c.want, stripToolMarkup(c.in), c.in)
	}
}

// 流式增量：末尾可能是标记开头的一段先不发，已经出现的标记从头截掉。
func TestToolMarkupHold(t *testing.T) {
	for _, c := range []struct {
		in   string
		want int
	}{
		{"回答。", len("回答。")},
		{"回答。<", len("回答。")},
		{"回答。<｜", len("回答。")},
		{"回答。<｜DS", len("回答。")},
		{"回答。<｜tool▁", len("回答。")},
		{"a < b", len("a < b")},
		{"a <b", len("a <b")},
	} {
		assert.Equal(t, c.want, markupHold(c.in), c.in)
	}
	assert.Equal(t, "回答。\n\n", cutToolMarkup("回答。\n\n<｜DSML｜function_calls"))
}

// 用户给了站点与种子编号（或者贴了地址）时直接推送：2026-10-11 真实验收里 DeepSeek 照「用 search_torrents 结果里的编号」
// 先拿编号当关键词搜，搜不到就不推了。
func TestSystemPromptPushesGivenIDs(t *testing.T) {
	now := time.Date(2026, 10, 11, 22, 0, 0, 0, time.Local)
	p := systemPrompt(now, admin)
	assert.Contains(t, p, "直接推送")
	assert.Contains(t, p, "不要先搜索")
	assert.NotContains(t, systemPrompt(now, Actor{Owner: "token:1", Entry: "app"}), "直接推送", "只读的入口没有推送")
}
