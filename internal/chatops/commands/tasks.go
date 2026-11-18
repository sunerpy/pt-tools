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
	items := make([]map[string]any, 0, len(jobs))
	running := 0
	for _, j := range jobs {
		state := "stopped"
		if j.Running {
			state = "running"
			running++
		}
		items = append(items, map[string]any{"site": j.SiteName, "rss": j.RSSName, "state": state})
	}
	fields := map[string]any{"count": len(jobs), "running": running, "items": items}
	if len(jobs) == 0 {
		return eventReply(tr(src.ReplyLang, "无 RSS 任务", "no RSS jobs"), notify.EventReplyTasks, fields), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, tr(src.ReplyLang, "RSS 任务（%d）\n", "RSS Jobs (%d)\n"), len(jobs))
	for _, it := range items {
		fmt.Fprintf(&b, "[%s] %s/%s\n", it["state"], it["site"], it["rss"])
	}
	return eventReply(strings.TrimRight(b.String(), "\n"), notify.EventReplyTasks, fields), nil
}
