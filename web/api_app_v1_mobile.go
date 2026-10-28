package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// AppMobileOption is an editor choice. It deliberately contains no credentials or addresses.
type AppMobileOption struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}
type AppMobileSiteOption struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}
type AppMobileOptions struct {
	Sites       []AppMobileSiteOption `json:"sites"`
	Downloaders []AppMobileOption     `json:"downloaders"`
	Profiles    []AppMobileOption     `json:"profiles"`
	Libraries   []AppMobileOption     `json:"libraries"`
	Errors      map[string]string     `json:"errors,omitempty"`
}

func (s *Server) appMobileOptions(w http.ResponseWriter, r *http.Request) {
	out := AppMobileOptions{Sites: []AppMobileSiteOption{}, Downloaders: []AppMobileOption{}, Profiles: []AppMobileOption{}, Libraries: []AppMobileOption{}, Errors: map[string]string{}}
	for _, source := range []string{"sites", "downloaders", "quality_profiles", "libraries"} {
		state, err := s.ConfigRead(r.Context(), source, "")
		if err != nil {
			out.Errors[source] = appRedactAddr(err.Error())
			continue
		}
		for _, item := range state.Items {
			name, _ := item["name"].(string)
			if source == "sites" {
				if enabled, ok := item["enabled"].(bool); ok && !enabled {
					continue
				}
				display, _ := item["display_name"].(string)
				if display == "" {
					display = name
				}
				out.Sites = append(out.Sites, AppMobileSiteOption{Name: name, DisplayName: display})
				continue
			}
			id, _ := strconv.ParseUint(configIDText(item["id"]), 10, 64)
			option := AppMobileOption{ID: uint(id), Name: name}
			switch source {
			case "downloaders":
				out.Downloaders = append(out.Downloaders, option)
			case "quality_profiles":
				out.Profiles = append(out.Profiles, option)
			case "libraries":
				out.Libraries = append(out.Libraries, option)
			}
		}
	}
	appJSON(w, out)
}

// Only this named configuration resource is exposed; devices cannot select arbitrary resources.
func (s *Server) appBrushConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := appID(w, r)
	if !ok {
		return
	}
	got, err := s.ConfigRead(r.Context(), "brush_tasks", strconv.FormatUint(uint64(id), 10))
	if err != nil {
		configWriteError(w, err)
		return
	}
	if len(got.Items) != 1 {
		appError(w, 404, "not_found", "没有这个刷流任务")
		return
	}
	appJSON(w, appBrushConfigView(got.Items[0]))
}

func (s *Server) appBrushPreview(w http.ResponseWriter, r *http.Request) {
	var in ConfigChange
	if !appDecode(w, r, &in) {
		return
	}
	out, err := s.ConfigPreview(r.Context(), "brush_tasks", in)
	if err != nil {
		configWriteError(w, err)
		return
	}
	appJSON(w, out)
}

func (s *Server) appBrushChange(w http.ResponseWriter, r *http.Request) {
	in := ConfigChange{Op: configOpAdd}
	if r.Method != http.MethodPost || r.PathValue("id") != "" {
		id, ok := appID(w, r)
		if !ok {
			return
		}
		in.ID = strconv.FormatUint(uint64(id), 10)
		in.Op = configOpUpdate
	}
	switch {
	case r.Method == http.MethodDelete:
		in.Op = configOpDelete
		if !appNoBody(w, r) {
			return
		}
	case strings.HasSuffix(r.URL.Path, "/status"):
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if !appDecode(w, r, &body) {
			return
		}
		if body.Enabled == nil {
			appError(w, 400, "invalid_argument", "缺少 enabled")
			return
		}
		in.Values = map[string]any{"enabled": *body.Enabled}
	default:
		if !appDecode(w, r, &in.Values) {
			return
		}
	}
	for key, value := range in.Values {
		if value == nil {
			delete(in.Values, key)
		}
	}
	appSetAuditArgs(r, map[string]any{"resource": "brush_tasks", "op": in.Op, "id": in.ID})
	out, err := s.ConfigApply(r.Context(), "brush_tasks", in)
	if err != nil {
		configWriteError(w, err)
		return
	}
	if in.Op == configOpDelete {
		appJSON(w, map[string]bool{"ok": true})
		return
	}
	appJSON(w, appBrushConfigView(out.Item))
}

// appMobileJSON adapts a trusted existing read handler and scrubs free-text errors.
// It never mounts the session mux or accepts a caller-selected handler/path.
func appMobileJSON(w http.ResponseWriter, r *http.Request, h http.HandlerFunc) {
	rec := &bufferWriter{header: http.Header{}}
	h(rec, r)
	if rec.statusCode() >= 400 {
		appError(w, rec.statusCode(), "request_failed", appRedactAddr(strings.TrimSpace(rec.body.String())))
		return
	}
	var value any
	decoder := json.NewDecoder(&rec.body)
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		appError(w, 500, "internal", "读取数据失败")
		return
	}
	appJSON(w, appMobileScrub(value))
}

func appMobileScrub(v any) any {
	switch x := v.(type) {
	case string:
		return appRedactAddr(x)
	case []any:
		for i := range x {
			x[i] = appMobileScrub(x[i])
		}
	case map[string]any:
		for k, item := range x {
			x[k] = appMobileScrub(item)
		}
	}
	return v
}

func (s *Server) appBrushRuns(w http.ResponseWriter, r *http.Request) {
	id, ok := appID(w, r)
	if !ok {
		return
	}
	repo, ok := brushRepo(w)
	if !ok {
		return
	}
	appMobileJSON(w, r, func(w http.ResponseWriter, r *http.Request) { listBrushRuns(w, r, repo, id) })
}

func (s *Server) appStatsAggregated(w http.ResponseWriter, r *http.Request) {
	appMobileJSON(w, r, s.apiUserInfoAggregated)
}

func (s *Server) appStatsSummary(w http.ResponseWriter, r *http.Request) {
	appMobileJSON(w, r, s.apiUserInfoSummary)
}

func appBrushConfigView(item map[string]any) map[string]any {
	out := map[string]any{"id": item["id"]}
	for _, key := range jsonFields[BrushTaskRequest]() {
		if value, ok := item[key]; ok {
			out[key] = value
		}
	}
	return out
}
