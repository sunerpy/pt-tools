package definitions

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// Rousi 签到：POST /api/points/attendance，API Key 放 api-token 头，体 {"mode":"fixed"}。
// 已签到时站点回 400 + code 1 +「今日已签到」（2026-10-19 用真实 Key 验证过）。
func TestRousiDriver_Attend(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		wantStatus v2.AttendStatus
		wantMsg    string
		wantErr    error
		errText    string
	}{
		{
			name: "签到成功带奖励", status: 200, body: `{"code":0,"message":"success","data":{"base_reward":"100","current_streak":2}}`,
			wantStatus: v2.AttendSigned, wantMsg: "签到成功，获得 100 魔力，已连续签到 2 天",
		},
		{name: "签到成功没有数据", status: 200, body: `{"code":0,"message":"success"}`, wantStatus: v2.AttendSigned, wantMsg: "签到成功"},
		{name: "今天已签到", status: 400, body: `{"code":1,"message":"今日已签到","data":null}`, wantStatus: v2.AttendAlready, wantMsg: "今日已签到"},
		{name: "Key 失效", status: 401, body: `{"code":401,"message":"unauthorized"}`, wantErr: v2.ErrInvalidCredentials, errText: "attendance:claim"},
		{name: "没有签到权限", status: 403, body: `{"code":403,"message":"forbidden"}`, wantErr: v2.ErrInvalidCredentials, errText: "签到权限"},
		{name: "其他业务错误", status: 400, body: `{"code":2,"message":"签到暂未开放"}`, errText: "签到暂未开放"},
		{name: "服务端错误", status: 502, body: `bad gateway`, errText: "Rousi 签到失败"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotMethod, gotPath, gotToken, gotBody string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath, gotToken = r.Method, r.URL.Path, r.Header.Get("api-token")
				b, _ := io.ReadAll(r.Body)
				gotBody = string(b)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			res, err := newTestRousiDriverWithURL(server.URL).Attend(context.Background())
			assert.Equal(t, http.MethodPost, gotMethod)
			assert.Equal(t, "/api/points/attendance", gotPath)
			assert.Equal(t, "FAKE_TEST_PASSKEY_1234", gotToken)
			assert.JSONEq(t, `{"mode":"fixed"}`, gotBody)
			if tc.errText != "" || tc.wantErr != nil {
				require.Error(t, err)
				if tc.wantErr != nil {
					assert.True(t, errors.Is(err, tc.wantErr), "%v", err)
				}
				assert.Contains(t, err.Error(), tc.errText)
				assert.NotContains(t, err.Error(), "FAKE_TEST_PASSKEY_1234", "错误里不带 Key")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantStatus, res.Status)
			assert.Equal(t, tc.wantMsg, res.Message)
		})
	}
}

// 最近活跃只认站点的 last_active_at：今天签到过也不算（签到不刷新站点的记录，保号要用浏览器登录），也不再去查签到统计。
func TestRousiDriver_GetUserInfo_LastAccessIsSiteValue(t *testing.T) {
	statsCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/profile":
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"id":1,"username":"t","last_active_at":"2026-08-11T15:59:59Z"}}`))
		case "/api/points/attendance/stats":
			statsCalls++
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"claimed_today":true,"today":"2026-10-24","current_streak":1}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	info, err := newTestRousiDriverWithURL(server.URL).GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 8, 11, 15, 59, 59, 0, time.UTC).Unix(), info.LastAccess)
	assert.Zero(t, statsCalls, "不查签到统计")
}

// Rousi 走自己的签到接口，站点定义里标出支持签到；基于 BaseSite 包出来的站点也能签到。
func TestRousiPro_SupportsAttendance(t *testing.T) {
	ok, reason := v2.AttendanceSupport(RousiProDefinition)
	assert.True(t, ok)
	assert.Empty(t, reason)

	site, err := createRousiDriver(v2.SiteConfig{ID: "rousipro", Name: "Rousi Pro", Options: json.RawMessage(`{"passkey":"FAKE_TEST_PASSKEY_1234"}`)}, zap.NewNop())
	require.NoError(t, err)
	capable, ok := site.(v2.AttendanceCapable)
	require.True(t, ok)
	assert.True(t, capable.SupportsAttendance())
}
