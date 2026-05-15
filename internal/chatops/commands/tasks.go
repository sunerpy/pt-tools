package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/internal/chatops"
)

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "tasks",
		Description: "列出 RSS 订阅任务 (List RSS subscription jobs)",
		Handler:     tasksHandler,
	})
}

func tasksHandler(ctx context.Context, _ []string, src chatops.Source) (chatops.Reply, error) {
	svc := getServices()
	if svc == nil || svc.Task == nil {
		return errReply(src.ReplyLang, "任务服务不可用", "task service unavailable"), nil
	}
	jobs, err := svc.Task.ListJobs(ctx)
	if err != nil {
		return errReply(src.ReplyLang, "查询任务失败: %v", "list jobs failed: %v", err), nil
	}
	if len(jobs) == 0 {
		return okReply(tr(src.ReplyLang, "无 RSS 任务", "no RSS jobs")), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, tr(src.ReplyLang, "RSS 任务（%d）\n", "RSS Jobs (%d)\n"), len(jobs))
	for _, j := range jobs {
		state := "stopped"
		if j.Running {
			state = "running"
		}
		fmt.Fprintf(&b, "[%s] %s/%s\n", state, j.SiteName, j.RSSName)
	}
	return okReply(strings.TrimRight(b.String(), "\n")), nil
}
