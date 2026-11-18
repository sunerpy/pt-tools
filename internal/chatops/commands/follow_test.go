package commands

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/chatops"
)

func TestFollowCommandsRegistered(t *testing.T) {
	for name, admin := range map[string]bool{"follow": true, "追剧": true, "runs": false, "unlock": true} {
		spec, ok := chatops.DefaultRegistry().Get(name)
		require.True(t, ok, name)
		assert.Equal(t, admin, spec.AdminOnly, name)
	}
	spec, _ := chatops.DefaultRegistry().Get("追剧")
	assert.Equal(t, "follow", spec.Name, "「追剧」是 /follow 的别名")
}

// /follow 剧名：只列剧集，回复编号（可加季号）开始追
func TestFollowPickFlow(t *testing.T) {
	src := chatops.Source{ReplyLang: "zh", IsAdmin: true, ChannelType: "telegram", ChannelConfID: 1, ChannelUserID: "u"}
	store := chatops.NewSessionStore()
	setupServices(t, &Services{Sessions: store})
	setupSubscribe(t, nil)
	reply, err := handler(t, "follow")(context.Background(), []string{"现在就出发"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "订阅服务不可用")

	fs := &fakeSubscribe{found: []SubscribeCandidate{
		{Kind: "movie", TMDBID: 1, Title: "出发", Year: 2020},
		{Kind: "tv", TMDBID: 2, Title: "现在就出发", Year: 2023},
		{Kind: "tv", TMDBID: 3, Title: "出发吧", Year: 2024, Subscribed: true},
	}}
	setupSubscribe(t, fs)
	reply, err = handler(t, "follow")(context.Background(), nil, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "用法：/follow")

	reply, err = handler(t, "follow")(context.Background(), []string{"现在", "就出发"}, src)
	require.NoError(t, err)
	assert.Equal(t, "现在 就出发", fs.keyword)
	assert.NotContains(t, reply.Text, "电影", "只列剧集")
	assert.Contains(t, reply.Text, "1. 现在就出发 (2023) 剧集")
	assert.Contains(t, reply.Text, "2. 出发吧 (2024) 剧集（已订阅）")
	st, ok := store.Pending("telegram", 1, "u")
	require.True(t, ok)
	assert.Equal(t, followPickStep, st.Step)
	reply, err = st.Handler(context.Background(), []string{"1 4"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "开始追剧：名字，按播出时间搜索")
	assert.Equal(t, []string{"002:004"}, fs.followed)
	assert.Empty(t, fs.subbed, "不经 /sub 的订阅")

	reply, err = st.Handler(context.Background(), []string{"x"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "已取消")
	fs.err = errors.New("已经订阅过了")
	reply, err = st.Handler(context.Background(), []string{"1"}, src)
	require.NoError(t, err)
	assert.True(t, reply.Failed)
	assert.Contains(t, reply.Text, "追剧失败")

	fs.err, fs.found = nil, []SubscribeCandidate{{Kind: "movie", TMDBID: 1, Title: "出发"}}
	reply, err = handler(t, "follow")(context.Background(), []string{"出发"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "没有找到剧集")
}

// /runs 编号：最近几次运行，最近一次列出经过
func TestRunsCommand(t *testing.T) {
	src := chatops.Source{ReplyLang: "zh"}
	at := time.Date(2026, 10, 21, 20, 5, 0, 0, time.Local)
	fs := &fakeSubscribe{}
	setupSubscribe(t, fs)
	for _, args := range [][]string{nil, {"1", "2"}} {
		reply, err := handler(t, "runs")(context.Background(), args, src)
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "用法：/runs")
	}
	reply, err := handler(t, "runs")(context.Background(), []string{"abc"}, src)
	require.NoError(t, err)
	assert.True(t, reply.Failed)
	reply, err = handler(t, "runs")(context.Background(), []string{"#7"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "名字#007 还没有运行记录")

	steps := make([]string, 0, 10)
	for range 10 {
		steps = append(steps, "不要：分辨率不合")
	}
	fs.runs = []RunLine{
		{At: at, Trigger: "schedule", Status: "ok", Summary: "推送了第 3 集", Steps: steps},
		{At: at.Add(-24 * time.Hour), Trigger: "rss", Status: "nothing"},
		{At: at.Add(-48 * time.Hour), Trigger: "manual", Status: "changed", Summary: "跳过第 4 集"},
	}
	reply, err = handler(t, "runs")(context.Background(), []string{"7"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "名字#007 最近的运行：")
	assert.Contains(t, reply.Text, "1. 10-21 20:05 定时搜索 · 成功：推送了第 3 集")
	assert.Contains(t, reply.Text, "   - 不要：分辨率不合")
	assert.Contains(t, reply.Text, "还有 2 步")
	assert.Contains(t, reply.Text, "2. 10-20 20:05 RSS · 没有新的")
	assert.Contains(t, reply.Text, "3. 10-19 20:05 手动 · 改动：跳过第 4 集")

	// 和说明一样的那步不重复列
	fs.runs = []RunLine{{At: at, Trigger: "manual", Status: "changed", Summary: "跳过第 4 集", Steps: []string{"跳过第 4 集"}}}
	reply, err = handler(t, "runs")(context.Background(), []string{"7"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "1. 10-21 20:05 手动 · 改动：跳过第 4 集")
	assert.NotContains(t, reply.Text, "   - 跳过第 4 集")

	fs.err = errors.New("订阅不存在")
	reply, err = handler(t, "runs")(context.Background(), []string{"9"}, src)
	require.NoError(t, err)
	assert.True(t, reply.Failed)
	assert.Contains(t, reply.Text, "订阅不存在")
	setupSubscribe(t, nil)
	reply, err = handler(t, "runs")(context.Background(), []string{"9"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "订阅服务不可用")
}

// /unlock 编号 集号：这一集换版本
func TestUnlockCommand(t *testing.T) {
	src := chatops.Source{ReplyLang: "zh", IsAdmin: true}
	fs := &fakeSubscribe{}
	setupSubscribe(t, fs)
	reply, err := handler(t, "unlock")(context.Background(), []string{"7"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "用法：/unlock")
	for _, args := range [][]string{{"x", "1"}, {"7", "0"}, {"7", "y"}} {
		reply, err = handler(t, "unlock")(context.Background(), args, src)
		require.NoError(t, err)
		assert.True(t, reply.Failed, args)
	}
	reply, err = handler(t, "unlock")(context.Background(), []string{"#7", "3"}, src)
	require.NoError(t, err)
	assert.False(t, reply.Failed)
	assert.Contains(t, reply.Text, "名字 第 3 集不再受版本锁限制")
	assert.Equal(t, []string{"007:003"}, fs.unlocked)
	fs.err = errors.New("这一季没有第 9 集")
	reply, err = handler(t, "unlock")(context.Background(), []string{"7", "9"}, src)
	require.NoError(t, err)
	assert.True(t, reply.Failed)
	assert.Contains(t, reply.Text, "这一季没有第 9 集")
}
