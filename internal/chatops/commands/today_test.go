package commands

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/internal/notify"
)

func TestTodayRegistered(t *testing.T) {
	spec, ok := chatops.DefaultRegistry().Get("today")
	require.True(t, ok)
	assert.False(t, spec.AdminOnly, "只读")
	assert.Equal(t, []string{"今日"}, spec.Aliases)
}

func TestTodayReply(t *testing.T) {
	zh := chatops.Source{ReplyLang: "zh"}
	en := chatops.Source{ReplyLang: "en"}
	setupSubscribe(t, nil)
	reply, err := handler(t, "today")(context.Background(), nil, zh)
	require.NoError(t, err)
	assert.True(t, reply.Failed)
	assert.Contains(t, reply.Text, "订阅服务不可用")

	fs := &fakeSubscribe{}
	setupSubscribe(t, fs)
	reply, err = handler(t, "today")(context.Background(), nil, zh)
	require.NoError(t, err)
	assert.Equal(t, "2026-11-03 订阅里没有播出的集", reply.Text)
	require.NotNil(t, reply.Event)
	assert.Equal(t, notify.EventReplyToday, reply.Event.Type)
	assert.Equal(t, int64(0), reply.Event.Fields["count"])

	fs.today = []TodayLine{
		{Title: "最后生还者 (2023) 第 2 季 E03「路」", MediaType: "tv", Season: 2, Episode: 3, Name: "路", AirTime: "21:30", State: "upcoming"},
		{Title: "现在就出发 (2023) 第 4 季 E05", MediaType: "tv", Season: 4, Episode: 5, State: "library"},
		{Title: "沙丘2 (2024) 上映", MediaType: "movie", State: "missing"},
	}
	reply, err = handler(t, "today")(context.Background(), nil, zh)
	require.NoError(t, err)
	assert.Equal(t, "2026-11-03 播出 2 集、上映 1 部电影：\n· 最后生还者 (2023) 第 2 季 E03「路」 21:30 · 还没播\n· 现在就出发 (2023) 第 4 季 E05 · 已入库\n· 沙丘2 (2024) 上映 · 缺", reply.Text)
	f := reply.Event.Fields
	assert.Equal(t, "2026-11-03", f["day"])
	assert.Equal(t, int64(3), f["count"])
	assert.Equal(t, int64(1), f["in_library"])
	items := f["items"].([]any)
	require.Len(t, items, 3)
	first := items[0].(map[string]any)
	assert.Equal(t, "21:30", first["air_time"])
	assert.Equal(t, int64(3), first["episode"])
	assert.Equal(t, "还没播", first["state_label"])

	reply, err = handler(t, "today")(context.Background(), nil, en)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "2026-11-03: 2 episode(s), 1 movie(s):")
	assert.Contains(t, reply.Text, "· 沙丘2 (2024) 上映 · missing")

	// 只有电影、只有集；状态全认得
	fs.today = []TodayLine{{Title: "沙丘2 (2024) 上映", MediaType: "movie", State: "downloading"}}
	reply, err = handler(t, "today")(context.Background(), nil, zh)
	require.NoError(t, err)
	assert.Equal(t, "2026-11-03 上映 1 部电影：\n· 沙丘2 (2024) 上映 · 下载中", reply.Text)
	for state, label := range map[string]string{"waiting_version": "等版本", "skipped": "不追", "odd": "odd"} {
		assert.Equal(t, label, todayStateLabel("zh", state))
	}
	assert.Equal(t, "in library", todayStateLabel("en", "library"))

	// 很多项时只列 30 项
	fs.today = nil
	for i := range 35 {
		fs.today = append(fs.today, TodayLine{Title: fmt.Sprintf("剧 E%02d", i+1), MediaType: "tv", Episode: i + 1, State: "missing"})
	}
	reply, err = handler(t, "today")(context.Background(), nil, zh)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "2026-11-03 播出 35 集：")
	assert.Contains(t, reply.Text, "· 剧 E30 · 缺\n……还有 5 项，到网页的订阅日历看")
	assert.NotContains(t, reply.Text, "E31")

	fs.err = errors.New("TMDB 超时")
	reply, err = handler(t, "today")(context.Background(), nil, zh)
	require.NoError(t, err)
	assert.True(t, reply.Failed)
	assert.Contains(t, reply.Text, "读取今日更新失败: TMDB 超时")
}
