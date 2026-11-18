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
		Name:        "status",
		Description: "查看任务与下载器状态 (Show task and downloader status)",
		Handler:     statusHandler,
	})
}

func statusHandler(ctx context.Context, _ []string, src chatops.Source) (chatops.Reply, error) {
	svc := getServices()
	if svc == nil {
		return errReply(src.ReplyLang, "服务尚未就绪", "service not ready"), nil
	}
	var b strings.Builder
	b.WriteString(tr(src.ReplyLang, "状态概览\n", "Status Overview\n"))
	fields := map[string]any{"tasks": 0, "running": 0}

	if svc.Task != nil {
		jobs, err := svc.Task.ListJobs(ctx)
		if err == nil {
			running := 0
			for _, j := range jobs {
				if j.Running {
					running++
				}
			}
			fmt.Fprintf(&b, tr(src.ReplyLang, "任务: %d 总计 / %d 运行中\n", "Tasks: %d total / %d running\n"), len(jobs), running)
			fields["tasks"], fields["running"] = len(jobs), running
		}
	}

	items := []map[string]any{}
	if svc.Downloader != nil {
		statuses := svc.Downloader.GetAllDownloaderStatus()
		fmt.Fprintf(&b, tr(src.ReplyLang, "下载器: %d\n", "Downloaders: %d\n"), len(statuses))
		for _, s := range statuses {
			health := "✗"
			if s.IsHealthy {
				health = "✓"
			}
			fmt.Fprintf(&b, "  %s %s (%s)\n", health, s.Name, s.Type)
			items = append(items, map[string]any{"name": s.Name, "type": s.Type, "healthy": s.IsHealthy})
		}
	}
	fields["items"] = items
	return eventReply(strings.TrimRight(b.String(), "\n"), notify.EventReplyStatus, fields), nil
}
