// Package mcp documents the future MCP (Model Context Protocol) Server interface
// contract for pt-tools.
//
// This file is INTERFACE-ONLY for Phase 4. Implementation (transport, server lifecycle,
// tool dispatch) is deferred until after Phase 1+2 ship. The contract intentionally has
// ZERO external dependencies; the future runtime will introduce
// modelcontextprotocol/go-sdk separately.
//
// See docs/design/phase4-mcp.md for the full design rationale, transport selection,
// and authentication model.
package mcp

// Tool represents a future MCP tool exposed by pt-tools.
//
// Handler signature contract (future, NOT implemented in Phase 4):
//
//	func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)
//
// InputSchema follows the JSON Schema Draft 7 dialect (object with "type",
// "properties", "required", "description").
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// ContractTools enumerates the 10 MCP tools planned for Phase 4. The list is the
// authoritative inventory consumed by docs and future server registration. Order
// reflects pt-tools/docs/guide/chatops-mcp-agent-design.md §8.2.
//
// Mapping to internal/chatops/commands (existing ChatOps surface):
//
//	list_tasks               -> tasks.go
//	list_downloader_torrents -> torrents.go
//	get_downloader_stats     -> status.go
//	search_torrents          -> (web API; no chatops command)
//	pause_torrent            -> pause.go      (high-risk, requires confirm)
//	resume_torrent           -> resume.go
//	delete_torrent           -> delete.go     (high-risk, requires confirm + remove_data)
//	push_torrent             -> (web API; no chatops command)
//	get_site_userinfo        -> sites.go
//	check_updates            -> version.go
var ContractTools = []Tool{
	{
		Name:        "list_tasks",
		Description: "List all RSS subscription tasks managed by pt-tools, including their site, schedule, and last-run status.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"site_id": map[string]any{
					"type":        "string",
					"description": "Optional site identifier filter (e.g. HDSKY, MTEAM). When omitted returns tasks across all sites.",
				},
				"enabled_only": map[string]any{
					"type":        "boolean",
					"description": "When true, only return tasks with enabled=true. Defaults to false.",
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
	},
	{
		Name:        "list_downloader_torrents",
		Description: "List torrents currently managed by a downloader, paginated. Mirrors the GET /api/downloaders/{id}/torrents web endpoint.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"downloader_id": map[string]any{
					"type":        "integer",
					"description": "Downloader instance numeric ID (see list_downloaders).",
				},
				"page": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"default":     1,
					"description": "1-based page number.",
				},
				"page_size": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     200,
					"default":     50,
					"description": "Page size (max 200).",
				},
				"state_filter": map[string]any{
					"type":        "string",
					"enum":        []string{"all", "downloading", "seeding", "paused", "completed", "error"},
					"default":     "all",
					"description": "Optional torrent state filter.",
				},
			},
			"required":             []string{"downloader_id"},
			"additionalProperties": false,
		},
	},
	{
		Name:        "get_downloader_stats",
		Description: "Return aggregate statistics for one downloader: total torrents, active count, free disk space, upload/download speed.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"downloader_id": map[string]any{
					"type":        "integer",
					"description": "Downloader instance numeric ID.",
				},
			},
			"required":             []string{"downloader_id"},
			"additionalProperties": false,
		},
	},
	{
		Name:        "search_torrents",
		Description: "Search torrents across one or more PT sites. Returns matching torrent metadata (title, size, free state, download URL).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"keyword": map[string]any{
					"type":        "string",
					"minLength":   1,
					"description": "Search keyword (will be URL-encoded by the server).",
				},
				"sites": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional site identifier list. When omitted, searches all configured sites.",
				},
				"free_only": map[string]any{
					"type":        "boolean",
					"default":     false,
					"description": "When true, only return free torrents.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     100,
					"default":     20,
					"description": "Max results per site.",
				},
			},
			"required":             []string{"keyword"},
			"additionalProperties": false,
		},
	},
	{
		Name:        "pause_torrent",
		Description: "Pause a torrent in the downloader. HIGH-RISK: caller must supply confirm=true. Maps to chatops `pause` command.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"downloader_id": map[string]any{
					"type":        "integer",
					"description": "Downloader instance numeric ID.",
				},
				"task_id": map[string]any{
					"type":        "string",
					"description": "Downloader-specific torrent identifier (qBittorrent hash or Transmission ID).",
				},
				"confirm": map[string]any{
					"type":        "boolean",
					"const":       true,
					"description": "Must be literal true. Guard against accidental destructive calls.",
				},
			},
			"required":             []string{"downloader_id", "task_id", "confirm"},
			"additionalProperties": false,
		},
	},
	{
		Name:        "resume_torrent",
		Description: "Resume a previously paused torrent. Lower risk than pause/delete but still mutates downloader state.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"downloader_id": map[string]any{
					"type":        "integer",
					"description": "Downloader instance numeric ID.",
				},
				"task_id": map[string]any{
					"type":        "string",
					"description": "Downloader-specific torrent identifier.",
				},
			},
			"required":             []string{"downloader_id", "task_id"},
			"additionalProperties": false,
		},
	},
	{
		Name:        "delete_torrent",
		Description: "Delete a torrent from the downloader. HIGH-RISK: requires confirm=true; data deletion controlled by remove_data. Maps to chatops `delete` command.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"downloader_id": map[string]any{
					"type":        "integer",
					"description": "Downloader instance numeric ID.",
				},
				"task_id": map[string]any{
					"type":        "string",
					"description": "Downloader-specific torrent identifier.",
				},
				"remove_data": map[string]any{
					"type":        "boolean",
					"default":     false,
					"description": "When true, delete on-disk data files in addition to removing from downloader.",
				},
				"confirm": map[string]any{
					"type":        "boolean",
					"const":       true,
					"description": "Must be literal true. Guard for destructive calls.",
				},
			},
			"required":             []string{"downloader_id", "task_id", "confirm"},
			"additionalProperties": false,
		},
	},
	{
		Name:        "push_torrent",
		Description: "Push a torrent (by site URL or .torrent payload) to a target downloader. Mutates downloader state by adding a new task.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"downloader_id": map[string]any{
					"type":        "integer",
					"description": "Target downloader instance numeric ID.",
				},
				"torrent_url": map[string]any{
					"type":        "string",
					"format":      "uri",
					"description": "Site download URL (must include passkey). Mutually exclusive with torrent_b64.",
				},
				"torrent_b64": map[string]any{
					"type":        "string",
					"description": "Base64-encoded .torrent file content. Mutually exclusive with torrent_url.",
				},
				"category": map[string]any{
					"type":        "string",
					"description": "Optional downloader category/label.",
				},
				"save_path": map[string]any{
					"type":        "string",
					"description": "Optional save directory override.",
				},
				"paused": map[string]any{
					"type":        "boolean",
					"default":     false,
					"description": "When true, add the torrent in paused state.",
				},
			},
			"required":             []string{"downloader_id"},
			"additionalProperties": false,
		},
	},
	{
		Name:        "get_site_userinfo",
		Description: "Fetch aggregated user statistics (upload, download, ratio, bonus, level) for a configured PT site.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"site_id": map[string]any{
					"type":        "string",
					"description": "Site identifier (e.g. HDSKY, MTEAM, NOVAHD).",
				},
				"refresh": map[string]any{
					"type":        "boolean",
					"default":     false,
					"description": "When true, bypass cache and re-scrape the site.",
				},
			},
			"required":             []string{"site_id"},
			"additionalProperties": false,
		},
	},
	{
		Name:        "check_updates",
		Description: "Query the version subsystem for newer pt-tools releases. Read-only; never triggers self-upgrade.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"include_prerelease": map[string]any{
					"type":        "boolean",
					"default":     false,
					"description": "When true, also report prerelease versions.",
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
	},
}
