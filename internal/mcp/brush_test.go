package mcp

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/apitoken"
)

func TestBrushToolsPreviewAndConfirm(t *testing.T) {
	e := newEnv(t)
	e.backend.reply = func(req Request) Response {
		if req.Path == "/brush/tasks/preview" {
			return Response{Status: 200, Body: []byte(`{"summary":"暂停任务","changes":[],"impact":["停止自动挑种"],"applied":false}`)}
		}
		return Response{Status: 200, Body: []byte(`{"id":9,"enabled":false}`)}
	}
	args := map[string]any{"op": "update", "id": 9, "values": map[string]any{"enabled": false}}
	text, bad, _ := e.call("change_brush_task", args)
	require.False(t, bad, text)
	require.Len(t, e.backend.got(), 1)
	assert.Equal(t, "/brush/tasks/preview", e.backend.got()[0].Path)
	args["confirm"] = true
	text, bad, _ = e.call("change_brush_task", args)
	require.False(t, bad, text)
	req := e.backend.got()[1]
	assert.Equal(t, http.MethodPut, req.Method)
	assert.Equal(t, "/brush/tasks/9", req.Path)
	require.NotEmpty(t, e.audit.all())
	e.caller.Scopes = []string{apitoken.ScopeMCPRead}
	text, bad, _ = e.call("change_brush_task", args)
	assert.True(t, bad, text)
	assert.Len(t, e.backend.got(), 2, "read-only never executes writes")
}

func TestBrushToolsReadPaths(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		args       map[string]any
	}{
		{"list_brush_tasks", "/brush/tasks", nil},
		{"get_brush_task", "/brush/tasks/3", map[string]any{"id": 3}},
		{"list_brush_runs", "/brush/tasks/3/runs", map[string]any{"id": 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			text, bad, _ := e.call(tc.name, tc.args)
			require.False(t, bad, text)
			require.Len(t, e.backend.got(), 1)
			assert.Equal(t, tc.path, e.backend.got()[0].Path)
		})
	}
}

func TestBrushToolsAddDeleteAndRejects(t *testing.T) {
	e := newEnv(t)
	e.backend.reply = func(req Request) Response {
		return Response{Status: 200, Body: []byte(`{"id":9,"ok":true}`)}
	}
	values := map[string]any{"name": "phone", "site_name": "hdsky", "downloader_id": 1}
	text, bad, _ := e.call("change_brush_task", map[string]any{"op": "add", "values": values, "confirm": true})
	require.False(t, bad, text)
	add := e.backend.got()[0]
	assert.Equal(t, http.MethodPost, add.Method)
	assert.Equal(t, "/brush/tasks", add.Path)

	text, bad, _ = e.call("change_brush_task", map[string]any{"op": "delete", "id": 9, "confirm": true})
	require.False(t, bad, text)
	del := e.backend.got()[1]
	assert.Equal(t, http.MethodDelete, del.Method)
	assert.Equal(t, "/brush/tasks/9", del.Path)
	assert.Nil(t, del.Body, "delete sends no body")

	// update and delete need a task: neither the preview nor the write reaches the App API
	text, bad, _ = e.call("change_brush_task", map[string]any{"op": "update", "values": map[string]any{"enabled": true}})
	assert.True(t, bad)
	assert.Contains(t, text, "task id is required")
	text, bad, _ = e.call("change_brush_task", map[string]any{"op": "delete", "confirm": true})
	assert.True(t, bad)
	assert.Contains(t, text, "task id is required")
	assert.Len(t, e.backend.got(), 2)

	// a preview the App API refuses is shown as an error, and nothing is written
	e.backend.reply = func(Request) Response {
		return Response{Status: http.StatusBadRequest, Body: []byte(`{"error":"invalid_argument","message":"没有这个下载器"}`)}
	}
	text, bad, _ = e.call("change_brush_task", map[string]any{"op": "add", "values": values})
	assert.True(t, bad)
	assert.Contains(t, text, "没有这个下载器")
	require.Len(t, e.backend.got(), 3)
	assert.Equal(t, "/brush/tasks/preview", e.backend.got()[2].Path)
}

func TestAddSubscriptionSendsTitleKeywords(t *testing.T) {
	e := newEnv(t)
	e.backend.reply = func(Request) Response { return Response{Status: 200, Body: []byte(`{"id":1}`)} }
	text, bad, _ := e.call("add_subscription", map[string]any{
		"media_type": "tv", "tmdb_id": 1001, "include_keywords": "2160p", "exclude_keywords": "", "confirm": true,
	})
	require.False(t, bad, text)
	var create Request
	for _, r := range e.backend.got() {
		if r.Method == http.MethodPost && r.Path == "/subscriptions" {
			create = r
		}
	}
	body, ok := create.Body.(map[string]any)
	require.True(t, ok, "create request: %+v", e.backend.got())
	assert.Equal(t, "2160p", body["include_keywords"])
	ex, present := body["exclude_keywords"]
	assert.True(t, present, "an explicit empty exclude list is sent so it clears")
	assert.Equal(t, "", ex)
}
