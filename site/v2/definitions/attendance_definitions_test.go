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

// 签到不走 attendance.php 通用流程、流程也没写的站点：标为不支持并写明原因，界面直接显示这句原因。
var attendanceUnsupported = map[string]string{
	"u2dmhy":       "动态图片验证码",
	"btschool":     "签到方式",
	"mteam":        "没有签到",
	"springsunday": "没有签到",
	"byrpt":        "没有签到",
	"tjupt":        "禁止用脚本或程序签到",
	"audiences":    "已取消签到",
}

// 不是 NexusPHP、但驱动自己实现了签到的站点：Rousi 走站点的签到接口，HDDolby 用 Cookie 打开 attendance.php。
var attendanceAPISites = map[string]bool{
	"rousipro": true,
	"hddolby":  true,
}

// 签到要过一关的站点：识别验证码或答题，要用 AI（流程见 site/v2/attendance_challenge.go）。
var attendanceChallenges = map[string]v2.AttendanceChallenge{
	"hdsky":     v2.ChallengeHDSkyCaptcha,
	"opencd":    v2.ChallengeOpenCDCaptcha,
	"ptchdbits": v2.ChallengeBakaQuiz,
	"52pt":      v2.ChallengeBakaQuiz,
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
		if want, listed := attendanceChallenges[def.ID]; listed {
			seen[def.ID] = true
			assert.True(t, ok, def.ID)
			assert.Empty(t, reason, def.ID)
			assert.Equal(t, want, v2.AttendanceChallengeOf(def), def.ID)
			continue
		}
		assert.Empty(t, v2.AttendanceChallengeOf(def), "%s 有签到关卡但不在名单里", def.ID)
		if attendanceAPISites[def.ID] {
			seen[def.ID] = true
			assert.True(t, ok, def.ID)
			assert.Empty(t, reason, def.ID)
			if def.Schema == v2.SchemaHDDolby {
				cfg := v2.ResolveAttendanceConfig(def)
				require.NotNil(t, cfg, def.ID)
				assert.Equal(t, "/attendance.php", cfg.Path, def.ID)
				assert.Equal(t, http.MethodGet, cfg.Method, def.ID)
			}
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
	for id := range attendanceChallenges {
		assert.True(t, seen[id], "%s is listed with a challenge but has no definition", id)
	}
	for id := range attendanceAPISites {
		assert.True(t, seen[id], "%s is listed as signing through its API but has no definition", id)
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
// 站点实际发出的是 PHP json_encode 的默认写法：中文写成 \uXXXX、斜杠写成 \/。2026-11-21 在已签到的账号上
// GET /attendance-ajax.php 拿到的回包就是 already (wire) 那条，原样照抄；signed (wire) 是把 MoviePilot 记下的
// 签到成功回包（plugins.v2/autosignin/sites/pterclub.py 的注释）按同样的写法转义。
func TestPTerClubAttendanceResponses(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		want    v2.AttendStatus
		message string
	}{
		{"signed", `{"status":"1","data":" (签到已成功300)","message":"<p>这是您的第<b>237</b>次签到，已连续签到<b>1</b>天，本次签到获得<b>300</b>克猫粮。</p>"}`, v2.AttendSigned, "这是您的第 237 次签到"},
		{"already", `{"status":"0","data":"抱歉","message":"您今天已经签到过了，请勿重复刷新。"}`, v2.AttendAlready, "您今天已经签到过了"},
		{
			"signed (wire)",
			`{"status":"1","data":" (\u7b7e\u5230\u5df2\u6210\u529f300)","message":"<p>\u8fd9\u662f\u60a8\u7684\u7b2c<b>237<\/b>\u6b21\u7b7e\u5230\uff0c\u5df2\u8fde\u7eed\u7b7e\u5230<b>237<\/b>\u5929\u3002<\/p><p>\u672c\u6b21\u7b7e\u5230\u83b7\u5f97<b>300<\/b>\u514b\u732b\u7cae\u3002<\/p>"}`,
			v2.AttendSigned,
			"这是您的第 237 次签到，已连续签到 237 天。",
		},
		{
			"already (wire)",
			`{"status":"0","data":"\u62b1\u6b49","message":"\u60a8\u4eca\u5929\u5df2\u7ecf\u7b7e\u5230\u8fc7\u4e86\uff0c\u8bf7\u52ff\u91cd\u590d\u5237\u65b0\u3002"}`,
			v2.AttendAlready,
			"您今天已经签到过了，请勿重复刷新。",
		},
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
			assert.Contains(t, res.Message, tc.message)
			assert.Equal(t, "/attendance-ajax.php", path)
		})
	}
}
