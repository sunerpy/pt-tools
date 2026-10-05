package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/chatops"
)

type fakeAttendance struct {
	one     AttendanceOutcome
	all     []AttendanceOutcome
	err     error
	gotSite string
	allCall int
}

func (f *fakeAttendance) SignIn(_ context.Context, site string) (AttendanceOutcome, error) {
	f.gotSite = site
	return f.one, f.err
}

func (f *fakeAttendance) SignInAll(context.Context) ([]AttendanceOutcome, error) {
	f.allCall++
	return f.all, f.err
}

// M1c：/signin [站点] 是写操作，只给管理员；经命令注册表进入 MessageChain 的权限与审计。
func TestSigninCommandRegistered(t *testing.T) {
	spec, ok := chatops.DefaultRegistry().Get("signin")
	require.True(t, ok)
	assert.True(t, spec.AdminOnly)
}

func TestSigninHandler(t *testing.T) {
	src := chatops.Source{ReplyLang: "zh", IsAdmin: true}

	setupServices(t, &Services{})
	reply, err := handler(t, "signin")(context.Background(), nil, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "签到服务不可用")

	att := &fakeAttendance{one: AttendanceOutcome{Site: "hdtime", Status: "signed", Message: "这是您的第 9 次签到"}}
	setupServices(t, &Services{Attendance: att})
	reply, err = handler(t, "signin")(context.Background(), []string{"hdtime"}, src)
	require.NoError(t, err)
	assert.Equal(t, "hdtime", att.gotSite)
	assert.Contains(t, reply.Text, "hdtime")
	assert.Contains(t, reply.Text, "签到成功")
	assert.Contains(t, reply.Text, "第 9 次签到")

	att.one = AttendanceOutcome{Site: "hdsky", Status: "unsupported", Error: "签到需要验证码，暂不支持自动签到"}
	reply, err = handler(t, "signin")(context.Background(), []string{"hdsky"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "不支持")
	assert.Contains(t, reply.Text, "验证码")

	att.err = errors.New("站点正在探测或签到，请稍后再试")
	reply, err = handler(t, "signin")(context.Background(), []string{"hdtime"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "签到失败")
	assert.Contains(t, reply.Text, "稍后再试")
}

func TestSigninHandler_All(t *testing.T) {
	src := chatops.Source{ReplyLang: "zh", IsAdmin: true}
	att := &fakeAttendance{}
	setupServices(t, &Services{Attendance: att})

	reply, err := handler(t, "signin")(context.Background(), nil, src)
	require.NoError(t, err)
	assert.Equal(t, 1, att.allCall)
	assert.Contains(t, reply.Text, "没有开启自动签到的站点")

	att.all = []AttendanceOutcome{
		{Site: "hdtime", Status: "signed", Message: "这是您的第 9 次签到"},
		{Site: "pthome", Status: "already", Message: "您今天已经签到过了"},
		{Site: "audiences", Status: "pending", Error: "HTTP 502: Bad Gateway"},
	}
	reply, err = handler(t, "signin")(context.Background(), nil, src)
	require.NoError(t, err)
	for _, want := range []string{"hdtime", "签到成功", "pthome", "今天已签到", "audiences", "502"} {
		assert.Contains(t, reply.Text, want)
	}

	reply, err = handler(t, "signin")(context.Background(), nil, chatops.Source{ReplyLang: "en", IsAdmin: true})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "signed in")

	att.err = errors.New("db down")
	reply, err = handler(t, "signin")(context.Background(), nil, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "签到失败")
}

func TestSigninLine(t *testing.T) {
	assert.Equal(t, "hdtime: 签到成功", signinLine("zh", AttendanceOutcome{Site: "hdtime", Status: "signed"}))
	assert.Equal(t, "hdtime: 签到失败 (Cookie 已失效)", signinLine("zh", AttendanceOutcome{Site: "hdtime", Status: "failed", Error: "Cookie 已失效"}))
	assert.Equal(t, "hdtime: failed (boom)", signinLine("en", AttendanceOutcome{Site: "hdtime", Status: "failed", Error: "boom"}))
}
