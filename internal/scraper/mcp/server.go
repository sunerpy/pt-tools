package mcp

import (
	"context"

	"gorm.io/gorm"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
	"github.com/sunerpy/pt-tools/internal/scraper/service"
)

type LLMProvider interface {
	Name() string
	Kind() string
	Extract(context.Context, LLMExtractRequest) (*MetadataResult, error)
	Close() error
}

type Deps struct {
	Scrape       *service.ScrapeService
	Library      *service.LibraryService
	DB           *gorm.DB
	SourceReg    *core.Registry[core.MediaScraper]
	WriterReg    *core.Registry[core.NfoWriter]
	ConnectorReg *core.Registry[core.MediaServerConnector]
	LLMProviders map[string]LLMProvider
}

func NewMCPServer(deps Deps) *mcpsdk.Server {
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "pt-scraper",
		Version: "0.1.0",
		Title:   "PT Scraper MCP Server",
	}, nil)

	registerTools(srv, deps)
	registerResources(srv, deps)
	registerPrompts(srv, deps)
	return srv
}
