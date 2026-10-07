package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/chatops"
)

type fakeSubscribe struct {
	found   []SubscribeCandidate
	lines   []SubscribeLine
	err     error
	subbed  []string
	keyword string
}

func (f *fakeSubscribe) Search(_ context.Context, kw string) ([]SubscribeCandidate, error) {
	f.keyword = kw
	return f.found, f.err
}

func (f *fakeSubscribe) Subscribe(_ context.Context, kind string, id, season int) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.subbed = append(f.subbed, kind+":"+itoa(id)+":"+itoa(season))
	return "名字", nil
}

func (f *fakeSubscribe) List(context.Context) ([]SubscribeLine, error) { return f.lines, f.err }

func itoa(n int) string {
	return string(rune('0'+n/100%10)) + string(rune('0'+n/10%10)) + string(rune('0'+n%10))
}

func setupSubscribe(t *testing.T, s SubscribeService) {
	t.Helper()
	prev := getSubscribeService()
	SetSubscribeService(s)
	t.Cleanup(func() { SetSubscribeService(prev) })
}

func TestSubCommandsRegistered(t *testing.T) {
	spec, ok := chatops.DefaultRegistry().Get("sub")
	require.True(t, ok)
	assert.True(t, spec.AdminOnly, "订阅是写操作")
	spec, ok = chatops.DefaultRegistry().Get("subs")
	require.True(t, ok)
	assert.False(t, spec.AdminOnly)
}

// /sub 关键词：列出条目，回复编号（剧集可加季号）订阅；回复别的取消
func TestSubPickFlow(t *testing.T) {
	src := chatops.Source{ReplyLang: "zh", IsAdmin: true, ChannelType: "telegram", ChannelConfID: 1, ChannelUserID: "u"}
	store := chatops.NewSessionStore()
	setupServices(t, &Services{Sessions: store})
	setupSubscribe(t, nil)
	reply, err := handler(t, "sub")(context.Background(), []string{"沙丘"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "订阅服务不可用")

	fs := &fakeSubscribe{found: []SubscribeCandidate{
		{Kind: "movie", TMDBID: 1, Title: "沙丘2", Year: 2024},
		{Kind: "tv", TMDBID: 2, Title: "沙丘：预言", Year: 2024, Subscribed: true},
	}}
	setupSubscribe(t, fs)
	reply, err = handler(t, "sub")(context.Background(), nil, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "用法")

	reply, err = handler(t, "sub")(context.Background(), []string{"沙丘", "2"}, src)
	require.NoError(t, err)
	assert.Equal(t, "沙丘 2", fs.keyword)
	assert.Contains(t, reply.Text, "1. 沙丘2 (2024) 电影")
	assert.Contains(t, reply.Text, "2. 沙丘：预言 (2024) 剧集（已订阅）")
	st, ok := store.Pending("telegram", 1, "u")
	require.True(t, ok)
	reply, err = st.Handler(context.Background(), []string{"2 3"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "已订阅：名字")
	assert.Equal(t, []string{"tv:002:003"}, fs.subbed)

	_, err = handler(t, "sub")(context.Background(), []string{"沙丘"}, src)
	require.NoError(t, err)
	st, _ = store.Pending("telegram", 1, "u")
	reply, err = st.Handler(context.Background(), []string{"1 2"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "季号不对", "电影不能写季号")
	reply, err = st.Handler(context.Background(), []string{"hello"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "已取消")
	reply, err = st.Handler(context.Background(), []string{"1"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "已订阅")
	assert.Equal(t, "movie:001:000", fs.subbed[1])

	fs.err = errors.New("已经订阅过了")
	reply, err = st.Handler(context.Background(), []string{"1"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "订阅失败")
	reply, err = handler(t, "sub")(context.Background(), []string{"x"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "搜索失败")
	fs.err, fs.found = nil, nil
	reply, err = handler(t, "sub")(context.Background(), []string{"x"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "没有找到")
}

func TestSubsList(t *testing.T) {
	src := chatops.Source{ReplyLang: "zh"}
	fs := &fakeSubscribe{}
	setupSubscribe(t, fs)
	reply, err := handler(t, "subs")(context.Background(), nil, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "还没有订阅")
	for range 25 {
		fs.lines = append(fs.lines, SubscribeLine{Title: "最后生还者 第 2 季", Status: "在找", Progress: "已入库 1/3"})
	}
	reply, err = handler(t, "subs")(context.Background(), nil, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "1. 最后生还者 第 2 季 · 在找 · 已入库 1/3")
	assert.Contains(t, reply.Text, "还有 5 个")
}
