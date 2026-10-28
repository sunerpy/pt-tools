package mcp

import (
	"context"
	"fmt"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type (
	brushReadIn struct {
		ID uint `json:"id" jsonschema:"brush task id from list_brush_tasks"`
	}
	brushChangeIn struct {
		Op      string         `json:"op" jsonschema:"add, update or delete; update enabled to enable or pause a task"`
		ID      uint           `json:"id,omitempty" jsonschema:"task id, required for update and delete"`
		Values  map[string]any `json:"values,omitempty" jsonschema:"brush configuration fields only; get_brush_task shows the current fields. Add needs name, site_name, downloader_id; update sends only changed fields"`
		Confirm bool           `json:"confirm,omitempty" jsonschema:"true only after user approval; otherwise return a preview without changing anything"`
	}
)

func brushToolPath(id uint, suffix string) string { return "/brush/tasks/" + utoa(id) + suffix }
func registerBrushTools(s *sdk.Server, d *Deps) {
	register(s, d, spec{name: "list_brush_tasks", title: "List brush tasks", description: "Read brush tasks, upload gains, enabled state and current counts. Use get_brush_task to read editable settings."}, func(_ struct{}) (call, error) {
		return call{req: Request{Method: http.MethodGet, Path: "/brush/tasks"}}, nil
	})
	register(s, d, spec{name: "get_brush_task", title: "Brush task configuration", min: map[string]float64{"id": 1}, description: "Read a brush task's editable configuration. Contains no downloader credentials or tracker URLs."}, func(in brushReadIn) (call, error) {
		return call{req: Request{Method: http.MethodGet, Path: brushToolPath(in.ID, "")}}, nil
	})
	register(s, d, spec{name: "list_brush_runs", title: "Brush execution history", min: map[string]float64{"id": 1}, description: "Read the most recent 20 brush executions, steps, counts and sanitized errors."}, func(in brushReadIn) (call, error) {
		return call{req: Request{Method: http.MethodGet, Path: brushToolPath(in.ID, "/runs")}}, nil
	})
	registerWrite(s, d, spec{name: "change_brush_task", title: "Change brush task", destructive: true, enums: map[string][]any{"op": {"add", "update", "delete"}}, description: "Create, edit, pause/enable or delete a brush task. Update values.enabled to pause/enable. Preview first; writes may add or remove torrents according to the configuration. Never run without the user's confirmation."}, func(ctx context.Context, d *Deps, c Caller, in brushChangeIn) (Preview, error) {
		if in.Op != "add" && in.ID == 0 {
			return Preview{}, fmt.Errorf("task id is required")
		}
		var res configResult
		body := map[string]any{"op": in.Op, "values": in.Values}
		if in.ID != 0 {
			body["id"] = utoa(in.ID)
		}
		if err := readJSON(ctx, d, c, http.MethodPost, "/brush/tasks/preview", body, &res); err != nil {
			return Preview{}, err
		}
		return res.preview(), nil
	}, func(in brushChangeIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		path, method := brushToolPath(in.ID, ""), http.MethodPut
		var body any = in.Values
		switch in.Op {
		case "add":
			path, method = "/brush/tasks", http.MethodPost
		case "delete":
			method = http.MethodDelete
			body = nil
		case "update":
		default:
			return call{}, fmt.Errorf("invalid operation")
		}
		if in.Op != "add" && in.ID == 0 {
			return call{}, fmt.Errorf("task id is required")
		}
		return call{req: Request{Method: method, Path: path, Body: body}, audit: map[string]any{"op": in.Op, "id": in.ID, "fields": sortedKeys(in.Values)}}, nil
	})
}
