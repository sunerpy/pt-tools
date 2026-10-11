package assistant

import (
	"testing"

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
