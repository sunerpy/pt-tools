package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/sunerpy/pt-tools/internal/chatops"
)

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "help",
		Description: "列出可用命令 (List available commands)",
		Aliases:     []string{"h"},
		Handler:     helpHandler,
	})
}

func helpHandler(_ context.Context, _ []string, src chatops.Source) (chatops.Reply, error) {
	specs := chatops.DefaultRegistry().List()
	visible := make([]chatops.CommandSpec, 0, len(specs))
	for _, s := range specs {
		if s.AdminOnly && !src.IsAdmin {
			continue
		}
		visible = append(visible, s)
	}
	sort.Slice(visible, func(i, j int) bool { return visible[i].Name < visible[j].Name })

	var b strings.Builder
	b.WriteString(tr(src.ReplyLang, "可用命令：\n", "Available commands:\n"))
	for _, s := range visible {
		flag := ""
		if s.AdminOnly {
			flag = " [admin]"
		}
		fmt.Fprintf(&b, "/%s%s — %s\n", s.Name, flag, s.Description)
	}
	return chatops.Reply{Text: strings.TrimRight(b.String(), "\n")}, nil
}
