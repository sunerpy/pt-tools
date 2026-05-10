package mcp

import (
	"context"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerResources(srv *mcpsdk.Server, deps Deps) {
	srv.AddResource(&mcpsdk.Resource{
		Name:        "scraper-libraries",
		Title:       "Scraper Libraries",
		URI:         "scraper://libraries",
		MIMEType:    "application/json",
		Description: "Configured media libraries in pt-scraper.",
	}, func(ctx context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		libraries, err := listLibraries(ctx, deps.Library)
		if err != nil {
			return nil, err
		}
		return writeJSONResource("scraper://libraries", libraries)
	})

	srv.AddResource(&mcpsdk.Resource{
		Name:        "scraper-recent-tasks",
		Title:       "Recent Scrape Tasks",
		URI:         "scraper://tasks/recent",
		MIMEType:    "application/json",
		Description: "Most recent scrape tasks from the task queue.",
	}, func(ctx context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		tasks, err := listTasks(ctx, deps.DB, "", 20)
		if err != nil {
			return nil, err
		}
		items := make([]TaskInfo, 0, len(tasks))
		for _, task := range tasks {
			items = append(items, toTaskInfo(task))
		}
		return writeJSONResource("scraper://tasks/recent", items)
	})

	srv.AddResource(&mcpsdk.Resource{
		Name:        "scraper-providers",
		Title:       "Scraper Provider Status",
		URI:         "scraper://providers",
		MIMEType:    "application/json",
		Description: "Available source and LLM providers exposed by the scraper MCP server.",
	}, func(ctx context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		providers, err := listProviderStatus(ctx, deps)
		if err != nil {
			return nil, err
		}
		return writeJSONResource("scraper://providers", providers)
	})
}
