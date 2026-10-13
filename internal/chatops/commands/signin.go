package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/internal/notify"
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
		return eventReply(signinLine(src.ReplyLang, outcome), notify.EventReplySignin, signinFields(src.ReplyLang, []AttendanceOutcome{outcome})), nil
	}
	outcomes, err := svc.Attendance.SignInAll(ctx)
	if err != nil {
		return errReply(src.ReplyLang, "签到失败: %v", "sign-in failed: %v", err), nil
	}
	fields := signinFields(src.ReplyLang, outcomes)
	if len(outcomes) == 0 {
		return eventReply(tr(src.ReplyLang, "没有开启自动签到的站点", "no site has automatic sign-in enabled"), notify.EventReplySignin, fields), nil
	}
	var b strings.Builder
	for _, o := range outcomes {
		b.WriteString(signinLine(src.ReplyLang, o))
		b.WriteString("\n")
	}
	return eventReply(strings.TrimRight(b.String(), "\n"), notify.EventReplySignin, fields), nil
}

// signinFields 是 reply.signin 的变量：各站点的结果，与成功（含今天已签到）、失败的站点数。
func signinFields(lang string, outcomes []AttendanceOutcome) map[string]any {
	items := make([]map[string]any, 0, len(outcomes))
	signed, failed := 0, 0
	for _, o := range outcomes {
		switch o.Status {
		case "signed", "already":
			signed++
		case "failed":
			failed++
		}
		items = append(items, map[string]any{
			"site": o.Site, "status": o.Status, "status_label": signinStatusLabel(lang, o.Status), "message": signinDetail(o),
		})
	}
	return map[string]any{"signed": signed, "failed": failed, "items": items}
}

// signinDetail 是一个站点签到结果的说明：成功时是站点的说明，没成功时是失败原因（有的话）。
func signinDetail(o AttendanceOutcome) string {
	if o.Status != "signed" && o.Status != "already" && o.Error != "" {
		return o.Error
	}
	return o.Message
}

func signinLine(lang string, o AttendanceOutcome) string {
	label := signinStatusLabel(lang, o.Status)
	detail := signinDetail(o)
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
