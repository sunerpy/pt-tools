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
