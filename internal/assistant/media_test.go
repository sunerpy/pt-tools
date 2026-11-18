package assistant

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mediaCands = []MediaCandidate{
	{Kind: "movie", Title: "奥本海默", OriginalTitle: "Oppenheimer", Year: 2023, Overview: "原子弹之父的故事。"},
	{Kind: "tv", Title: "最后生还者", OriginalTitle: "The Last of Us", Year: 2023, Overview: strings.Repeat("末日", 60)},
	{Kind: "tv", Title: "无名", OriginalTitle: "无名"},
}

// 让 AI 认条目：种子名、站点标题与带编号的候选发给助手的模型（不带工具），回答里的编号换成下标，理由与用量带回来
func TestChooseMedia(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{text: "2\n名字和年份都对得上", usage: [2]int64{321, 9}})
	got, err := e.svc.ChooseMedia(context.Background(), "The.Last.of.Us.S01E02.2160p", "最后生还者 第一季", mediaCands)
	require.NoError(t, err)
	assert.Equal(t, MediaChoice{Index: 1, Reason: "名字和年份都对得上", Usage: Usage{Model: "fake-model", PromptTokens: 321, CompletionTokens: 9}}, got)
	reqs := e.llm.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "fake-model", reqs[0].Model)
	assert.False(t, reqs[0].Stream)
	assert.Empty(t, reqs[0].Tools, "不带工具")
	prompt := reqs[0].Messages[0]["content"].(string)
	assert.Contains(t, prompt, "种子名：The.Last.of.Us.S01E02.2160p\n站点上的标题：最后生还者 第一季\n")
	assert.Contains(t, prompt, "1. 电影：奥本海默（Oppenheimer，2023）：原子弹之父的故事。\n")
	assert.Contains(t, prompt, "2. 剧集：最后生还者（The Last of Us，2023）："+strings.Repeat("末日", 40)+"…\n", "简介截到 80 个字")
	assert.Contains(t, prompt, "3. 剧集：无名\n", "原名一样、没有年份与简介时不写")

	for _, tc := range []struct {
		reply  string
		index  int
		reason string
	}{
		{"都不是\n候选里没有 2 这一季", -1, "候选里没有 2 这一季"},
		{"7", -1, ""},
		{"认不出来", -1, "认不出来"},
		{"<think>先比年份……</think>\n1，年份是 2023", 0, "年份是 2023"},
		{"第 3 个。原名一样", 2, "第 3 个。原名一样"},
	} {
		e.llm.push(reply{text: tc.reply})
		c, cerr := e.svc.ChooseMedia(context.Background(), "x", "", mediaCands)
		require.NoError(t, cerr, tc.reply)
		assert.Equal(t, tc.index, c.Index, tc.reply)
		assert.Equal(t, tc.reason, c.Reason, tc.reply)
	}
	assert.NotContains(t, e.llm.requests()[1].Messages[0]["content"], "站点上的标题", "没有站点标题时不写")

	_, err = e.svc.ChooseMedia(context.Background(), "x", "", nil)
	require.ErrorIs(t, err, ErrInvalid)
}

// 发给模型的种子名与站点标题先遮掉像密钥的长串
func TestChooseMediaRedacts(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{text: "都不是"})
	_, err := e.svc.ChooseMedia(context.Background(), "Show.S01 passkey=0123456789abcdef0123456789abcdef", "", mediaCands)
	require.NoError(t, err)
	assert.NotContains(t, e.llm.requests()[0].Raw, "0123456789abcdef0123456789abcdef")
}

// 助手没启用时不调模型；模型出错、超时时回给人看的错误，不带 Key
func TestChooseMediaErrors(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{status: http.StatusInternalServerError, body: "upstream down"})
	_, err := e.svc.ChooseMedia(context.Background(), "x", "", mediaCands)
	require.Error(t, err)
	var te *TestError
	require.ErrorAs(t, err, &te)
	assert.NotContains(t, err.Error(), testKey)

	// 不回答的模型服务；测试结束时放行（保存设置后后台还会去读模型列表，不放行的话 Close 一直等）
	stop := make(chan struct{})
	hang := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-stop:
		}
	}))
	t.Cleanup(func() {
		close(stop)
		hang.Close()
	})
	old := mediaChooseTimeout
	t.Cleanup(func() { mediaChooseTimeout = old })
	mediaChooseTimeout = 100 * time.Millisecond
	key := testKey
	_, err = e.svc.SaveSettings(context.Background(), SettingsInput{Enabled: true, Provider: "custom", BaseURL: hang.URL + "/v1", Model: "fake-model", APIKey: &key})
	require.NoError(t, err)
	start := time.Now()
	_, err = e.svc.ChooseMedia(context.Background(), "x", "", mediaCands)
	require.ErrorContains(t, err, "超时")
	assert.Less(t, time.Since(start), 10*time.Second)

	_, err = e.svc.SaveSettings(context.Background(), SettingsInput{Enabled: false, Provider: "custom", BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model"})
	require.NoError(t, err)
	before := len(e.llm.requests())
	_, err = e.svc.ChooseMedia(context.Background(), "x", "", mediaCands)
	require.ErrorIs(t, err, ErrDisabled)
	assert.Len(t, e.llm.requests(), before, "没启用时不调模型")
}
