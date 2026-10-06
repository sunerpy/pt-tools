package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/internal/chatops"
)

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "signin",
		Description: "立即签到，不带站点时签到所有开启了自动签到的站点 (Sign in now; all enabled sites when no site is given)",
		AdminOnly:   true,
		Handler:     signinHandler,
	})
}

func signinHandler(ctx context.Context, args []string, src chatops.Source) (chatops.Reply, error) {
	svc := getServices()
	if svc == nil || svc.Attendance == nil {
		return errReply(src.ReplyLang, "签到服务不可用", "sign-in service unavailable"), nil
	}
	if len(args) >= 1 {
		site := strings.TrimSpace(args[0])
		outcome, err := svc.Attendance.SignIn(ctx, site)
		if err != nil {
			return errReply(src.ReplyLang, "签到失败: %v", "sign-in failed: %v", err), nil
		}
		return okReply(signinLine(src.ReplyLang, outcome)), nil
	}
	outcomes, err := svc.Attendance.SignInAll(ctx)
	if err != nil {
		return errReply(src.ReplyLang, "签到失败: %v", "sign-in failed: %v", err), nil
	}
	if len(outcomes) == 0 {
		return okReply(tr(src.ReplyLang, "没有开启自动签到的站点", "no site has automatic sign-in enabled")), nil
	}
	var b strings.Builder
	for _, o := range outcomes {
		b.WriteString(signinLine(src.ReplyLang, o))
		b.WriteString("\n")
	}
	return okReply(strings.TrimRight(b.String(), "\n")), nil
}

func signinLine(lang string, o AttendanceOutcome) string {
	label := signinStatusLabel(lang, o.Status)
	detail := o.Message
	if o.Status != "signed" && o.Status != "already" && o.Error != "" {
		detail = o.Error
	}
	if detail == "" {
		return fmt.Sprintf("%s: %s", o.Site, label)
	}
	return fmt.Sprintf("%s: %s (%s)", o.Site, label, truncate(detail, 80))
}

func signinStatusLabel(lang, status string) string {
	switch status {
	case "signed":
		return tr(lang, "签到成功", "signed in")
	case "already":
		return tr(lang, "今天已签到", "already signed in today")
	case "failed":
		return tr(lang, "签到失败", "failed")
	case "unsupported":
		return tr(lang, "不支持自动签到", "not supported")
	default:
		return tr(lang, "未完成，稍后自动重试", "not done, will retry")
	}
}
