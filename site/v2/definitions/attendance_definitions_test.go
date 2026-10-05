package definitions

import (
	"context"
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
	"hdarea":    "签到方式",
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
		want := "/attendance.php"
		if def.ID == "pterclub" {
			want = "/attendance-ajax.php"
		}
		assert.Equal(t, want, cfg.Path, def.ID)
	}
	for id := range attendanceUnsupported {
		assert.True(t, seen[id], "%s is listed as unsupported but has no definition", id)
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
