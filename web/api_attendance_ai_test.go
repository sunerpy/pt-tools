package web

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/assistant"
)

// 「测试看图」：现画验证码交给看图模型（经 AI 助手的接口地址与 Key），回认出的字和对不对；设置页能看到 AI 助手是否可用。
func TestAttendanceAITest(t *testing.T) {
	e := newAssistantEnv(t)
	e.llm.push(fakeChatReply{text: "ABCDEF"})
	res := e.do(http.MethodPost, "/api/sites/attendance/ai-test", `{"vision_model":"vis-1"}`)
	require.Equal(t, http.StatusOK, res.StatusCode)
	var got assistant.VisionTestResult
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	assert.Equal(t, "vis-1", got.Model)
	assert.Len(t, got.Expected, 6)
	assert.Equal(t, "ABCDEF", got.Got)
	e.llm.mu.Lock()
	last := e.llm.bodies[len(e.llm.bodies)-1]
	e.llm.mu.Unlock()
	assert.Contains(t, last, `"image_url"`)
	assert.Contains(t, last, `"model":"vis-1"`)

	res = e.do(http.MethodGet, "/api/sites/attendance/settings", "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	var st AttendanceSettingsDTO
	require.NoError(t, json.NewDecoder(res.Body).Decode(&st))
	assert.True(t, st.AssistantReady)
	assert.Equal(t, "fake", st.AssistantModel)

	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, "/api/sites/attendance/ai-test", `{"model":"x"}`).StatusCode)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, "/api/sites/attendance/ai-test", `{"vision_model":"a b"}`).StatusCode)
	assert.Equal(t, http.StatusMethodNotAllowed, e.do(http.MethodGet, "/api/sites/attendance/ai-test", "").StatusCode)

	// AI 助手关掉以后试不了，说明原因
	res = e.do(http.MethodPut, "/api/assistant/settings", `{"enabled":false,"provider":"custom","base_url":"http://127.0.0.1:9/v1","model":"fake"}`)
	require.Equal(t, http.StatusOK, res.StatusCode)
	res = e.do(http.MethodPost, "/api/sites/attendance/ai-test", `{"vision_model":""}`)
	assert.Equal(t, http.StatusConflict, res.StatusCode)
}
