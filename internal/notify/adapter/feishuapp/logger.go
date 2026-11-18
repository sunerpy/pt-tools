package feishuapp

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
)

func sLogger() *zap.SugaredLogger {
	if l := global.GetSloggerSafe(); l != nil {
		return l
	}
	return zap.NewNop().Sugar()
}

// sdkLogger 把飞书 SDK 的日志接到 zap（实现 larkcore.Logger）。Debug 一律丢掉：SDK 在 Debug 级打印
// 每条入站消息的全文（ws/client_message.go），/addrss 带着 RSS 地址里的 passkey，名单外的人发的内容也不该进日志。
// 其余级别脱敏以后转给 zap。
type sdkLogger struct {
	log func() *zap.SugaredLogger
	red redactor
}

func (l sdkLogger) Debug(context.Context, ...any) {}

func (l sdkLogger) Info(_ context.Context, args ...any) { l.log().Info(l.text(args)) }

func (l sdkLogger) Warn(_ context.Context, args ...any) { l.log().Warn(l.text(args)) }

func (l sdkLogger) Error(_ context.Context, args ...any) { l.log().Error(l.text(args)) }

// text 拼出一行日志。SDK 有时把 fmtLog 的结果（一个切片）当成单个参数传进来，先展开。
func (l sdkLogger) text(args []any) string {
	if len(args) == 1 {
		if inner, ok := args[0].([]any); ok {
			args = inner
		}
	}
	parts := make([]string, 0, len(args))
	for _, a := range args {
		parts = append(parts, fmt.Sprint(a))
	}
	return "飞书 SDK: " + l.red.text(strings.Join(parts, " "))
}

var (
	// tenantTokenRe 是 tenant_access_token（t- 开头）。
	tenantTokenRe = regexp.MustCompile(`\bt-[A-Za-z0-9]{16,}`)
	// userinfoRe 是地址里的用户名与密码（代理地址可能带着）。
	userinfoRe = regexp.MustCompile(`(?i)\b((?:https?|socks5)://)[^/\s@"'<>]+@`)
)

// redactor 遮住要写进日志、返回出去的文字里的 App Secret、tenant_access_token 与地址里的用户名密码。
type redactor struct {
	secret string
}

func (r redactor) text(s string) string {
	if len(r.secret) >= 3 {
		s = strings.ReplaceAll(s, r.secret, "***")
	}
	s = tenantTokenRe.ReplaceAllString(s, "t-***")
	return userinfoRe.ReplaceAllString(s, "${1}***@")
}

// wrap 返回脱敏后的错误，Unwrap 仍是原错误（errors.Is / As 照常可用）。
func (r redactor) wrap(err error) error {
	if err == nil {
		return nil
	}
	return redactedError{err: err, red: r}
}

type redactedError struct {
	err error
	red redactor
}

func (e redactedError) Error() string { return e.red.text(e.err.Error()) }

func (e redactedError) Unwrap() error { return e.err }
