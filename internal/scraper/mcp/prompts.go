package mcp

import (
	"context"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerPrompts(srv *mcpsdk.Server, _ Deps) {
	srv.AddPrompt(&mcpsdk.Prompt{
		Name:        "scrape-movie-workflow",
		Title:       "Scrape Movie Workflow",
		Description: "Guides an agent through searching, scraping, and validating a movie scrape.",
		Arguments: []*mcpsdk.PromptArgument{{
			Name:        "media_path",
			Description: "Movie file path to scrape",
			Required:    false,
		}, {
			Name:        "providers",
			Description: "Preferred providers, comma separated",
			Required:    false,
		}},
	}, func(_ context.Context, req *mcpsdk.GetPromptRequest) (*mcpsdk.GetPromptResult, error) {
		mediaPath := req.Params.Arguments["media_path"]
		providers := req.Params.Arguments["providers"]
		text := "1. Call scraper_parse_filename to extract title/year from the movie filename.\n" +
			"2. Call scraper_search_media with media_type=movie to compare candidates across providers.\n" +
			"3. Use scraper_get_metadata for the best provider match, then run scraper_scrape_file to write NFO/artwork.\n" +
			"4. If metadata is incomplete, call scraper_scrape_with_llm or scraper_enrich_partial, then scraper_validate_metadata."
		if mediaPath != "" {
			text += fmt.Sprintf("\n\nTarget media path: %s", mediaPath)
		}
		if providers != "" {
			text += fmt.Sprintf("\nPreferred providers: %s", providers)
		}
		return promptResult("Movie scrape workflow", text), nil
	})

	srv.AddPrompt(&mcpsdk.Prompt{
		Name:        "bulk-scrape-workflow",
		Title:       "Bulk Scrape Workflow",
		Description: "Steps for bulk scraping a library or directory with queue visibility.",
		Arguments: []*mcpsdk.PromptArgument{{
			Name:        "library_name",
			Description: "Library name to process",
			Required:    false,
		}, {
			Name:        "mode",
			Description: "movie, tv, or episode",
			Required:    false,
		}},
	}, func(_ context.Context, req *mcpsdk.GetPromptRequest) (*mcpsdk.GetPromptResult, error) {
		libraryName := req.Params.Arguments["library_name"]
		mode := req.Params.Arguments["mode"]
		text := "1. Start with scraper_list_libraries or resource scraper://libraries to identify target library settings.\n" +
			"2. Queue or run scraping with scraper_scrape_directory for each target path.\n" +
			"3. Monitor progress with scraper_list_tasks and scraper_get_task_status.\n" +
			"4. Trigger scraper_refresh_jellyfin after completion if the library is attached to a connector."
		if libraryName != "" {
			text += fmt.Sprintf("\n\nTarget library: %s", libraryName)
		}
		if mode != "" {
			text += fmt.Sprintf("\nScrape mode hint: %s", mode)
		}
		return promptResult("Bulk scrape workflow", text), nil
	})

	srv.AddPrompt(&mcpsdk.Prompt{
		Name:        "llm-scrape-with-context",
		Title:       "LLM Scrape With Context",
		Description: "How to use LLM-based scraper tools with partial metadata and provider context.",
		Arguments: []*mcpsdk.PromptArgument{{
			Name:        "raw_title",
			Description: "Raw release title or filename",
			Required:    false,
		}, {
			Name:        "hints",
			Description: "Extra context or site hints",
			Required:    false,
		}},
	}, func(_ context.Context, req *mcpsdk.GetPromptRequest) (*mcpsdk.GetPromptResult, error) {
		rawTitle := req.Params.Arguments["raw_title"]
		hints := strings.TrimSpace(req.Params.Arguments["hints"])
		text := "Use scraper_scrape_with_llm when you only have release text and need structured metadata.\n" +
			"Use scraper_generate_metadata_from_text for freeform descriptions or notes.\n" +
			"Use scraper_enrich_partial to fill only missing fields after a search or scrape.\n" +
			"Always run scraper_validate_metadata before scraper_write_nfo."
		if rawTitle != "" {
			text += fmt.Sprintf("\n\nRaw title: %s", rawTitle)
		}
		if hints != "" {
			text += fmt.Sprintf("\nExtra hints: %s", hints)
		}
		return promptResult("LLM scrape workflow", text), nil
	})
}

func promptResult(description, text string) *mcpsdk.GetPromptResult {
	return &mcpsdk.GetPromptResult{
		Description: description,
		Messages: []*mcpsdk.PromptMessage{{
			Role:    "user",
			Content: &mcpsdk.TextContent{Text: text},
		}},
	}
}
