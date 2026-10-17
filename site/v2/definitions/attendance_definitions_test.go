package definitions

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 签到不走 attendance.php 通用流程的站点：标为不支持并写明原因，界面直接显示这句原因。
var attendanceUnsupported = map[string]string{
	"52pt":      "答题",
	"ptchdbits": "答题",
	"u2dmhy":    "答题",
	"hdsky":     "验证码",
	"opencd":    "验证码",
	"btschool":  "签到方式",
}

// M1c：每个内置站点的签到配置都能解析——NexusPHP 默认 GET /attendance.php，例外站点写明原因，其他架构不支持。
func TestAttendanceDefinitions(t *testing.T) {
	defs := v2.GetDefinitionRegistry().GetAll()
	require.NotEmpty(t, defs)
	seen := map[string]bool{}
	for _, def := range defs {
		ok, reason := v2.AttendanceSupport(def)
		if want, listed := attendanceUnsupported[def.ID]; listed {
			seen[def.ID] = true
			assert.False(t, ok, def.ID)
			assert.Contains(t, reason, want, def.ID)
			continue
		}
		if def.Schema != v2.SchemaNexusPHP {
			assert.False(t, ok, "%s (%s) has no attendance driver", def.ID, def.Schema)
			assert.NotEmpty(t, reason, def.ID)
			continue
		}
		assert.True(t, ok, def.ID)
		cfg := v2.ResolveAttendanceConfig(def)
		require.NotNil(t, cfg, def.ID)
		// 例外站点：PTerClub 走 ajax 接口；HDArea 的签到由首页 JS 触发，用 POST，参数在表单体里。
		wantPath, wantMethod := "/attendance.php", http.MethodGet
		switch def.ID {
		case "pterclub":
			wantPath = "/attendance-ajax.php"
		case "hdarea":
			wantPath, wantMethod = "/sign_in.php?action=sign_in", http.MethodPost
		}
		assert.Equal(t, wantPath, cfg.Path, def.ID)
		assert.Equal(t, wantMethod, cfg.Method, def.ID)
	}
	for id := range attendanceUnsupported {
		assert.True(t, seen[id], "%s is listed as unsupported but has no definition", id)
	}
}

// HDArea 的签到由首页 JS（common.js 的 sign_in）触发：POST sign_in.php，表单体 action=sign_in，
// 返回纯文本。下面两个 body 是站点真实回执：成功那条来自当天首次签到，
// 重复那条来自已签到后再次 POST（真机验证，2026-10-17）。
func TestHDAreaAttendanceResponses(t *testing.T) {
	cases := []struct {
		name string
		body string
		want v2.AttendStatus
	}{
		{"signed", "已连续签到2天，此次签到您获得了12魔力值奖励!", v2.AttendSigned},
		{"already", "请不要重复签到哦！", v2.AttendAlready},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var method, path, query, contentType, body string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				method, path, query = r.Method, r.URL.Path, r.URL.RawQuery
				contentType = r.Header.Get("Content-Type")
				raw, _ := io.ReadAll(r.Body)
				body = string(raw)
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			d := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: srv.URL, Cookie: "uid=1; pass=2"})
			d.SetSiteDefinition(HdareaDefinition)
			res, err := d.Attend(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tc.want, res.Status)
			assert.NotEmpty(t, res.Message)
			assert.Equal(t, http.MethodPost, method)
			assert.Equal(t, "/sign_in.php", path)
			// 参数只在表单体里：URL 上不能再带一份 action=sign_in
			assert.Empty(t, query, "POST 签到不带查询串")
			assert.Equal(t, "application/x-www-form-urlencoded", contentType)
			assert.Equal(t, "action=sign_in", body)
		})
	}
}

// PTerClub 的签到接口返回 JSON，message 里是 NexusPHP 的提示语（格式取自公开的签到返回）。
func TestPTerClubAttendanceResponses(t *testing.T) {
	cases := []struct {
		name string
		body string
		want v2.AttendStatus
	}{
		{"signed", `{"status":"1","data":" (签到已成功300)","message":"<p>这是您的第<b>237</b>次签到，已连续签到<b>1</b>天，本次签到获得<b>300</b>克猫粮。</p>"}`, v2.AttendSigned},
		{"already", `{"status":"0","data":"抱歉","message":"您今天已经签到过了，请勿重复刷新。"}`, v2.AttendAlready},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var path string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			d := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: srv.URL, Cookie: "uid=1; pass=2"})
			d.SetSiteDefinition(PTerClubDefinition)
			res, err := d.Attend(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tc.want, res.Status)
			assert.Equal(t, "/attendance-ajax.php", path)
		})
	}
}
