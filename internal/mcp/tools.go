package mcp

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolNames 是全部工具的名字（文档与测试用）。
var ToolNames = []string{
	"list_tasks", "list_downloader_torrents", "get_downloader_stats", "search_torrents",
	"pause_torrent", "resume_torrent", "delete_torrent", "push_torrent",
	"get_site_userinfo", "check_updates",
	"explore_media", "list_subscriptions", "add_subscription",
}

// withQuery 拼查询串（空值不放）。
func withQuery(path string, kv ...string) string {
	q := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		if v := strings.TrimSpace(kv[i+1]); v != "" {
			q.Set(kv[i], v)
		}
	}
	if len(q) == 0 {
		return path
	}
	return path + "?" + q.Encode()
}

func itoa(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func utoa(n uint) string {
	if n == 0 {
		return ""
	}
	return strconv.FormatUint(uint64(n), 10)
}

// filterItems 只留 items 里满足 keep 的（get_site_userinfo、get_downloader_stats 按参数筛）。
func filterItems(out map[string]any, keep func(item map[string]any) bool) map[string]any {
	items, _ := out["items"].([]any)
	kept := make([]any, 0, len(items))
	for _, it := range items {
		if m, ok := it.(map[string]any); ok && keep(m) {
			kept = append(kept, m)
		}
	}
	out["items"] = kept
	return out
}

type listTasksIn struct {
	Site       string `json:"site,omitempty" jsonschema:"only this site (site ID such as hdsky)"`
	Keyword    string `json:"keyword,omitempty" jsonschema:"only titles containing this keyword"`
	PushedOnly bool   `json:"pushed_only,omitempty" jsonschema:"only torrents that were pushed to a downloader"`
	Page       int    `json:"page,omitempty" jsonschema:"page number starting at 1"`
	PageSize   int    `json:"page_size,omitempty" jsonschema:"items per page, 1 to 100 (default 20)"`
}

type listTorrentsIn struct {
	DownloaderID uint   `json:"downloader_id,omitempty" jsonschema:"only this downloader (ids from get_downloader_stats); every enabled downloader when omitted"`
	State        string `json:"state,omitempty" jsonschema:"only torrents in this state"`
	Keyword      string `json:"keyword,omitempty" jsonschema:"only titles containing this keyword"`
	Sort         string `json:"sort,omitempty" jsonschema:"sort field"`
	Order        string `json:"order,omitempty" jsonschema:"sort order"`
	Page         int    `json:"page,omitempty" jsonschema:"page number starting at 1"`
	PageSize     int    `json:"page_size,omitempty" jsonschema:"items per page, 1 to 200 (default 20)"`
}

type downloaderStatsIn struct {
	DownloaderID uint `json:"downloader_id,omitempty" jsonschema:"only this downloader; every enabled downloader when omitted"`
}

type searchIn struct {
	Keyword    string   `json:"keyword" jsonschema:"search keyword"`
	Sites      []string `json:"sites,omitempty" jsonschema:"only these sites (site IDs); every enabled site when omitted"`
	Category   string   `json:"category,omitempty" jsonschema:"site category to search in"`
	FreeOnly   bool     `json:"free_only,omitempty" jsonschema:"only free torrents"`
	MinSeeders int      `json:"min_seeders,omitempty" jsonschema:"only torrents with at least this many seeders"`
	Limit      int      `json:"limit,omitempty" jsonschema:"return at most this many results, 1 to 100 (default 20)"`
}

type torrentIn struct {
	DownloaderID uint   `json:"downloader_id" jsonschema:"downloader id from list_downloader_torrents"`
	TaskID       string `json:"task_id" jsonschema:"task_id from list_downloader_torrents"`
	Confirm      bool   `json:"confirm" jsonschema:"must be true; ask the user before calling"`
}

type deleteIn struct {
	DownloaderID uint   `json:"downloader_id" jsonschema:"downloader id from list_downloader_torrents"`
	TaskID       string `json:"task_id" jsonschema:"task_id from list_downloader_torrents"`
	RemoveData   bool   `json:"remove_data,omitempty" jsonschema:"also delete the downloaded files from disk"`
	Confirm      bool   `json:"confirm" jsonschema:"must be true; ask the user before calling"`
}

type pushIn struct {
	Site         string `json:"site,omitempty" jsonschema:"site ID from search_torrents (together with torrent_id)"`
	TorrentID    string `json:"torrent_id,omitempty" jsonschema:"torrent ID from search_torrents (together with site)"`
	TorrentURL   string `json:"torrent_url,omitempty" jsonschema:"instead of site and torrent_id: the torrent's download URL on a configured site"`
	DownloaderID uint   `json:"downloader_id,omitempty" jsonschema:"target downloader; the default downloader when omitted"`
	Category     string `json:"category,omitempty" jsonschema:"downloader category"`
	Tags         string `json:"tags,omitempty" jsonschema:"comma-separated downloader tags"`
	SavePath     string `json:"save_path,omitempty" jsonschema:"save directory in the downloader"`
	Confirm      bool   `json:"confirm" jsonschema:"must be true; ask the user before calling"`
}

type siteInfoIn struct {
	Site string `json:"site,omitempty" jsonschema:"only this site (site ID such as hdsky); every configured site when omitted"`
}

type updatesIn struct {
	IncludePrerelease bool `json:"include_prerelease,omitempty" jsonschema:"also report preview releases"`
}

type exploreIn struct {
	Kind  string `json:"kind" jsonschema:"movie or tv"`
	Query string `json:"query,omitempty" jsonschema:"search TMDB for this title; the list below is used when empty"`
	List  string `json:"list,omitempty" jsonschema:"trending (default) or popular, when query is empty"`
	Page  int    `json:"page,omitempty" jsonschema:"page number starting at 1 (lists only)"`
}

type listSubsIn struct {
	Status  string `json:"status,omitempty" jsonschema:"only subscriptions in this status"`
	Keyword string `json:"keyword,omitempty" jsonschema:"only titles containing this keyword"`
}

type addSubIn struct {
	MediaType string `json:"media_type" jsonschema:"movie or tv"`
	TMDBID    int    `json:"tmdb_id" jsonschema:"TMDB id from explore_media"`
	Season    int    `json:"season,omitempty" jsonschema:"season number for tv (default 1)"`
	Upgrade   bool   `json:"upgrade,omitempty" jsonschema:"keep looking for better releases after the first download"`
	Confirm   bool   `json:"confirm" jsonschema:"must be true; ask the user before calling"`
}

// errTarget 是暂停、继续、删除没给种子。
var errTarget = errors.New("要给 downloader_id 与 task_id（从 list_downloader_torrents 取）")

func torrentAction(action string, downloaderID uint, taskID string) (call, error) {
	if downloaderID == 0 || strings.TrimSpace(taskID) == "" {
		return call{}, errTarget
	}
	return call{
		req: Request{Method: http.MethodPost, Path: "/torrents/actions", Body: map[string]any{
			"action": action, "targets": []map[string]any{{"downloader_id": downloaderID, "task_id": taskID}},
		}},
		audit: map[string]any{"downloader_id": downloaderID, "task_id": taskID, "action": action},
	}, nil
}

func registerTools(s *sdk.Server, d *Deps) {
	register(s, d, spec{
		name: "list_tasks", title: "List fetched torrents",
		min: map[string]float64{"page": 1, "page_size": 1}, max: map[string]float64{"page_size": 100},
		description: "List torrents pt-tools fetched or pushed (RSS, subscriptions, app, MCP), newest first: site, size, free status, H&R, push state and download progress.",
	}, func(in listTasksIn) (call, error) {
		pushed := ""
		if in.PushedOnly {
			pushed = "1"
		}
		return call{req: Request{Method: http.MethodGet, Path: withQuery("/tasks",
			"site", in.Site, "q", in.Keyword, "pushed", pushed, "page", itoa(in.Page), "page_size", itoa(in.PageSize))}}, nil
	})

	register(s, d, spec{
		name: "list_downloader_torrents", title: "List downloader torrents",
		min: map[string]float64{"downloader_id": 1, "page": 1, "page_size": 1}, max: map[string]float64{"page_size": 200},
		description: "List torrents in the user's downloaders (qBittorrent/Transmission), paginated. Each item has downloader_id and task_id for pause_torrent, resume_torrent and delete_torrent.",
		enums: map[string][]any{
			"state": {"downloading", "seeding", "paused", "stopped", "queued", "checking", "error"},
			"sort":  {"added_at", "completed_at", "title", "progress", "size", "ratio", "state", "upload_speed", "download_speed", "eta", "seeds"},
			"order": {"asc", "desc"},
		},
	}, func(in listTorrentsIn) (call, error) {
		return call{req: Request{Method: http.MethodGet, Path: withQuery("/torrents",
			"downloader_id", utoa(in.DownloaderID), "state", in.State, "q", in.Keyword, "sort", in.Sort, "order", in.Order,
			"page", itoa(in.Page), "page_size", itoa(in.PageSize))}}, nil
	})

	register(s, d, spec{
		name: "get_downloader_stats", title: "Downloader statistics",
		min:         map[string]float64{"downloader_id": 1},
		description: "Show each enabled downloader: id, name, type, whether it is reachable, current upload and download speed, totals and free disk space.",
	}, func(in downloaderStatsIn) (call, error) {
		c := call{req: Request{Method: http.MethodGet, Path: "/downloaders"}}
		if in.DownloaderID != 0 {
			want := float64(in.DownloaderID)
			c.shape = func(out map[string]any) map[string]any {
				return filterItems(out, func(m map[string]any) bool { return m["id"] == want })
			}
		}
		return c, nil
	})

	register(s, d, spec{
		name: "search_torrents", title: "Search PT sites", openWorld: true,
		min: map[string]float64{"min_seeders": 0, "limit": 1}, max: map[string]float64{"limit": 100},
		description: "Search torrents on the user's enabled PT sites. Results have site and torrent_id (use them with push_torrent), size, seeders, free/discount status and H&R. No download links are returned.",
	}, func(in searchIn) (call, error) {
		kw := strings.TrimSpace(in.Keyword)
		if kw == "" {
			return call{}, errors.New("keyword 要填")
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		limit = min(limit, 100)
		return call{
			req: Request{Method: http.MethodPost, Path: "/search", Body: map[string]any{
				"keyword": kw, "sites": in.Sites, "category": in.Category, "free_only": in.FreeOnly, "min_seeders": in.MinSeeders,
			}},
			shape: func(out map[string]any) map[string]any {
				if items, ok := out["items"].([]any); ok && len(items) > limit {
					out["items"], out["truncated"] = items[:limit], true
				}
				return out
			},
		}, nil
	})

	register(s, d, spec{
		min:  map[string]float64{"downloader_id": 1},
		name: "pause_torrent", title: "Pause a torrent", write: true,
		description: "Pause one torrent in a downloader. Changes the user's downloader: ask first and pass confirm=true.",
	}, func(in torrentIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		return torrentAction("pause", in.DownloaderID, in.TaskID)
	})

	register(s, d, spec{
		min:  map[string]float64{"downloader_id": 1},
		name: "resume_torrent", title: "Resume a torrent", write: true,
		description: "Resume one paused torrent in a downloader. Changes the user's downloader: ask first and pass confirm=true.",
	}, func(in torrentIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		return torrentAction("resume", in.DownloaderID, in.TaskID)
	})

	register(s, d, spec{
		min:  map[string]float64{"downloader_id": 1},
		name: "delete_torrent", title: "Delete a torrent", write: true, destructive: true,
		description: "Remove one torrent from a downloader, optionally deleting its files (remove_data). Cannot be undone: ask first and pass confirm=true.",
	}, func(in deleteIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		action := "delete"
		if in.RemoveData {
			action = "delete_with_files"
		}
		return torrentAction(action, in.DownloaderID, in.TaskID)
	})

	register(s, d, spec{
		name: "push_torrent", title: "Push a torrent to a downloader", write: true, openWorld: true,
		description: "Download a torrent through pt-tools' own site configuration and add it to a downloader, applying the disk-space and site-capacity checks. " +
			"Give site and torrent_id from search_torrents, or the torrent's download URL on a configured site. Magnet links and other URLs are not accepted. " +
			"Changes the user's downloader: ask first and pass confirm=true.",
	}, func(in pushIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		site, id := strings.TrimSpace(in.Site), strings.TrimSpace(in.TorrentID)
		if raw := strings.TrimSpace(in.TorrentURL); raw != "" {
			if site != "" || id != "" {
				return call{}, errors.New("torrent_url 与 site、torrent_id 只给一种")
			}
			var ok bool
			if d.ResolveURL != nil {
				site, id, ok = d.ResolveURL(raw)
			}
			if !ok {
				return call{}, errors.New("认不出这个地址：只收已配置站点的种子下载地址（磁力链接不收）")
			}
		}
		if site == "" || id == "" {
			return call{}, errors.New("要给 site 与 torrent_id（从 search_torrents 取），或者 torrent_url")
		}
		return call{
			req: Request{Method: http.MethodPost, Path: "/push", Body: map[string]any{
				"site": site, "torrent_id": id, "downloader_id": in.DownloaderID, "category": in.Category, "tags": in.Tags, "save_path": in.SavePath,
			}},
			// 下载地址里可能有 passkey：审计只记解析出来的站点与编号
			audit: map[string]any{"site": site, "torrent_id": id, "downloader_id": in.DownloaderID},
		}, nil
	})

	register(s, d, spec{
		name: "get_site_userinfo", title: "Site account statistics",
		description: "Show the user's account on each configured PT site: upload, download, ratio, bonus, seeding count, unread messages, login status and check-in result (cached; updated_at tells when).",
	}, func(in siteInfoIn) (call, error) {
		c := call{req: Request{Method: http.MethodGet, Path: "/sites"}}
		if site := strings.ToLower(strings.TrimSpace(in.Site)); site != "" {
			c.shape = func(out map[string]any) map[string]any {
				return filterItems(out, func(m map[string]any) bool {
					name, _ := m["name"].(string)
					return strings.EqualFold(name, site)
				})
			}
		}
		return c, nil
	})

	register(s, d, spec{
		name: "check_updates", title: "Check for pt-tools updates", openWorld: true,
		description: "Check whether a newer pt-tools release exists. Read-only: never upgrades.",
	}, func(in updatesIn) (call, error) {
		pre := ""
		if in.IncludePrerelease {
			pre = "1"
		}
		return call{req: Request{Method: http.MethodGet, Path: withQuery("/updates", "include_prerelease", pre)}}, nil
	})

	register(s, d, spec{
		name: "explore_media", title: "Find movies and TV shows", openWorld: true,
		min: map[string]float64{"page": 1}, max: map[string]float64{"page": 20},
		description: "Search TMDB for a movie or TV show by title, or list trending/popular ones. Items have the TMDB id for add_subscription and say whether it is already in the library or subscribed.",
		enums:       map[string][]any{"kind": {"movie", "tv"}, "list": {"trending", "popular"}},
	}, func(in exploreIn) (call, error) {
		list := in.List
		if strings.TrimSpace(in.Query) != "" {
			list = "search"
		}
		return call{req: Request{Method: http.MethodGet, Path: withQuery("/explore",
			"kind", in.Kind, "list", list, "q", in.Query, "page", itoa(in.Page))}}, nil
	})

	register(s, d, spec{
		name: "list_subscriptions", title: "List media subscriptions",
		description: "List the user's movie and TV subscriptions with their progress (episodes aired, in the library, downloading, missing).",
		enums:       map[string][]any{"status": {"active", "paused", "pending", "done"}},
	}, func(in listSubsIn) (call, error) {
		return call{req: Request{Method: http.MethodGet, Path: withQuery("/subscriptions", "status", in.Status, "q", in.Keyword)}}, nil
	})

	register(s, d, spec{
		name: "add_subscription", title: "Subscribe to a movie or TV show", write: true, openWorld: true,
		min: map[string]float64{"tmdb_id": 1, "season": 0},
		description: "Subscribe to a movie or a TV season by TMDB id (from explore_media): pt-tools then searches the user's sites and downloads it. " +
			"Changes the user's setup: ask first and pass confirm=true.",
		enums: map[string][]any{"media_type": {"movie", "tv"}},
	}, func(in addSubIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		if in.TMDBID <= 0 {
			return call{}, errors.New("tmdb_id 要填（从 explore_media 取）")
		}
		return call{
			req:   Request{Method: http.MethodPost, Path: "/subscriptions", Body: map[string]any{"media_type": in.MediaType, "tmdb_id": in.TMDBID, "season": in.Season, "upgrade": in.Upgrade}},
			audit: map[string]any{"media_type": in.MediaType, "tmdb_id": in.TMDBID, "season": in.Season},
		}, nil
	})
}
