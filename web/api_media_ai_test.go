package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/assistant"
	"github.com/sunerpy/pt-tools/models"
)

// withAssistant 接上一个用假模型服务的 AI 助手（enabled 为假时只建服务、不启用）。
func (e *organizeTestEnv) withAssistant(t *testing.T, enabled bool) *fakeChatLLM {
	t.Helper()
	llm := &fakeChatLLM{}
	ts := httptest.NewServer(llm)
	t.Cleanup(ts.Close)
	svc := assistant.New(assistant.Config{DB: global.GlobalDB.DB, Cipher: apiTestCipher{}, Tools: e.srv.AssistantToolServer, HTTPClient: ts.Client()})
	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = svc.Close(ctx)
	})
	key := "sk-web-test-0000000000"
	_, err := svc.SaveSettings(context.Background(), assistant.SettingsInput{Enabled: enabled, Provider: "custom", BaseURL: ts.URL + "/v1", Model: "fake", APIKey: &key})
	require.NoError(t, err)
	e.srv.SetAssistantService(svc)
	e.srv.SetMediaService(e.rec)
	return llm
}

func (f *fakeChatLLM) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bodies)
}

// 让 AI 认：搜候选、模型挑一个，回按它算出的整理预览（不整理）；模型说都不是时没有预览；没有候选时不调模型
func TestOrganizeAPI_AIRecognize(t *testing.T) {
	e := newOrganizeServer(t)
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPost, "/api/media/libraries", `{"name":"电影","kind":"movie","path":`+jsonPath(e.library)+`,"enabled":true}`).Code)
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPost, "/api/media/path-maps", `{"downloader_id":1,"downloader_prefix":"/downloads","local_prefix":`+jsonPath(e.dlDir)+`}`).Code)

	// 还没接 AI 助手：409，状态写明原因
	e.srv.SetMediaService(e.rec)
	w := serveAuthed(e.mux, http.MethodPost, "/api/media/organize/ai-recognize", `{"downloader_id":1,"hash":"hash1"}`)
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	w = serveAuthed(e.mux, http.MethodGet, "/api/media/organize/ai-status", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"ready":false,"reason":"AI 助手没有启用：先在「AI 助手」里配好接口地址、Key 与模型"}`, w.Body.String())

	llm := e.withAssistant(t, true)
	w = serveAuthed(e.mux, http.MethodGet, "/api/media/organize/ai-status", "")
	assert.JSONEq(t, `{"ready":true}`, w.Body.String())
	// 认得出的种子：不用 AI
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/organize/ai-recognize", `{"downloader_id":1,"hash":"hash1"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "已经认出了条目")
	assert.Zero(t, llm.count())

	// 年份写错的种子：自动识别不认，AI 从候选里挑出来
	e.dl.torrents[0].Name = "The.Shawshank.Redemption.1971.1080p.BluRay.x264-WiKi"
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/organize", `{"downloader_id":1,"hash":"hash1"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"fail_kind":"recognize"`)
	var row models.MediaTransferHistory
	require.NoError(t, global.GlobalDB.DB.Where("info_hash = ?", "hash1").First(&row).Error)
	require.Equal(t, models.MediaFailRecognize, row.FailKind)

	llm.push(fakeChatReply{text: "1\n名字对得上，种子名里的年份写错了"})
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/history/"+strconv.Itoa(int(row.ID))+"/ai-recognize", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got aiRecognizeResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.NotEmpty(t, got.Candidates)
	assert.Equal(t, 278, got.Candidates[0].ID)
	assert.Equal(t, 0, got.Chosen)
	assert.Equal(t, "名字对得上，种子名里的年份写错了", got.Reason)
	assert.Equal(t, "fake", got.Model)
	assert.Equal(t, "The.Shawshank.Redemption.1971.1080p.BluRay.x264-WiKi", got.Input.Title, "「记成纠正」用的标题")
	require.NotNil(t, got.Request)
	assert.Equal(t, 278, got.Request.TMDBID)
	assert.Equal(t, "movie", got.Request.MediaType)
	require.NotNil(t, got.Preview)
	assert.Empty(t, got.Preview.Problem)
	require.Len(t, got.Preview.Items, 1)
	assert.Contains(t, got.Preview.Items[0].Target, "肖申克的救赎 (1994)")
	var after models.MediaTransferHistory
	require.NoError(t, global.GlobalDB.DB.First(&after, row.ID).Error)
	assert.Equal(t, models.MediaTransferFailed, after.Status, "只认、不整理")

	llm.push(fakeChatReply{text: "都不是\n候选的年份都不对"})
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/organize/ai-recognize", `{"downloader_id":1,"hash":"hash1"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got = aiRecognizeResult{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, -1, got.Chosen)
	assert.Nil(t, got.Preview)
	assert.Equal(t, "候选的年份都不对", got.Reason)

	// 没有候选：不调模型
	e.dl.torrents[0].Name = "Nothing.Here.2020.1080p"
	before := llm.count()
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/organize/ai-recognize", `{"downloader_id":1,"hash":"hash1"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "TMDB 上没有搜到候选")
	assert.Equal(t, before, llm.count())

	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodPost, "/api/media/history/9999/ai-recognize", "").Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/media/organize/ai-recognize", `{"downloader_id":1,"hash":"hash1","x":1}`).Code)

	// 模型服务连不上：502，写明 AI 没认成
	e.dl.torrents[0].Name = "The.Shawshank.Redemption.1971.1080p.BluRay.x264-WiKi"
	key := "sk-web-test-0000000000"
	_, err := e.srv.assistant.SaveSettings(context.Background(), assistant.SettingsInput{Enabled: true, Provider: "custom", BaseURL: "http://127.0.0.1:1/v1", Model: "fake", APIKey: &key})
	require.NoError(t, err)
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/organize/ai-recognize", `{"downloader_id":1,"hash":"hash1"}`)
	assert.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "AI 没认成")
	assert.NotContains(t, w.Body.String(), key, "错误里不带 Key")
}

// AI 助手没启用：409，写明去哪里配
func TestOrganizeAPI_AIRecognizeDisabled(t *testing.T) {
	e := newOrganizeServer(t)
	llm := e.withAssistant(t, false)
	w := serveAuthed(e.mux, http.MethodPost, "/api/media/organize/ai-recognize", `{"downloader_id":1,"hash":"hash1"}`)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "AI 助手没有启用")
	assert.Contains(t, serveAuthed(e.mux, http.MethodGet, "/api/media/organize/ai-status", "").Body.String(), `"ready":false`)
	assert.Zero(t, llm.count())
}
