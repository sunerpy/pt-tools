package mcp

import (
	"context"
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
	"get_subscription", "set_subscription_status", "search_subscription", "delete_subscription",
	"update_subscription", "list_subscription_runs", "set_episode_skip", "unlock_episode",
	"get_episode_rules", "save_episode_rule", "delete_episode_rule", "preview_schedule",
	"reorganize_subscription_torrent", "rescrape_subscription_torrent",
	"recognize_media", "list_media_libraries", "list_media_history", "preview_media_organize", "organize_media",
	"retry_media_organize", "rescrape_media", "get_config", "change_config", "get_skill",
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
	Confirm      bool   `json:"confirm,omitempty" jsonschema:"true to run it; without it nothing changes and you get a preview (changes and impact) to show the user first"`
}

type deleteIn struct {
	DownloaderID uint   `json:"downloader_id" jsonschema:"downloader id from list_downloader_torrents"`
	TaskID       string `json:"task_id" jsonschema:"task_id from list_downloader_torrents"`
	RemoveData   bool   `json:"remove_data,omitempty" jsonschema:"also delete the downloaded files from disk"`
	Confirm      bool   `json:"confirm,omitempty" jsonschema:"true to run it; without it nothing changes and you get a preview (changes and impact) to show the user first"`
}

type pushIn struct {
	Site         string `json:"site,omitempty" jsonschema:"site ID from search_torrents (together with torrent_id)"`
	TorrentID    string `json:"torrent_id,omitempty" jsonschema:"torrent ID from search_torrents (together with site)"`
	TorrentURL   string `json:"torrent_url,omitempty" jsonschema:"instead of site and torrent_id: the torrent's download URL on a configured site"`
	DownloaderID uint   `json:"downloader_id,omitempty" jsonschema:"target downloader; the default downloader when omitted"`
	Category     string `json:"category,omitempty" jsonschema:"downloader category"`
	Tags         string `json:"tags,omitempty" jsonschema:"comma-separated downloader tags"`
	SavePath     string `json:"save_path,omitempty" jsonschema:"save directory in the downloader"`
	Confirm      bool   `json:"confirm,omitempty" jsonschema:"true to run it; without it nothing changes and you get a preview (changes and impact) to show the user first"`
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
	MediaType    string   `json:"media_type" jsonschema:"movie or tv"`
	TMDBID       int      `json:"tmdb_id" jsonschema:"TMDB id from explore_media"`
	Season       int      `json:"season,omitempty" jsonschema:"season number for tv; the latest season when omitted"`
	Upgrade      bool     `json:"upgrade,omitempty" jsonschema:"keep looking for better releases after the first download"`
	ProfileID    uint     `json:"profile_id,omitempty" jsonschema:"quality profile id (get_config resource quality_profiles); the default profile when omitted"`
	Sites        []string `json:"sites,omitempty" jsonschema:"only search these sites (site IDs); every enabled site when omitted"`
	DownloaderID uint     `json:"downloader_id,omitempty" jsonschema:"downloader to push to (ids from get_downloader_stats); the subscription settings' default when omitted"`
	Category     string   `json:"category,omitempty" jsonschema:"downloader category for pushes"`
	Tags         string   `json:"tags,omitempty" jsonschema:"comma-separated downloader tags for pushes"`
	SavePath     string   `json:"save_path,omitempty" jsonschema:"save directory in the downloader; the downloader's default when omitted"`
	// 追剧（剧集）的设置
	Schedule         *scheduleIn    `json:"schedule,omitempty" jsonschema:"TV: when to search actively (check it with preview_schedule); by each episode's air time when omitted"`
	LockVersion      *bool          `json:"lock_version,omitempty" jsonschema:"TV: after the first episode only take the same release group and resolution (on when omitted)"`
	StartEpisode     *int           `json:"start_episode,omitempty" jsonschema:"TV: follow from this episode on; earlier episodes are not wanted"`
	FollowNextSeason *bool          `json:"follow_next_season,omitempty" jsonschema:"TV: follow the next season automatically when this one is done"`
	LibraryID        *uint          `json:"library_id,omitempty" jsonschema:"media library to organize into (list_media_libraries); picked by type when omitted"`
	FromTorrent      *fromTorrentIn `json:"from_torrent,omitempty" jsonschema:"TV: start following from this torrent (site and torrent_id from search_torrents): the start episode defaults to its episode and it is pushed right after creation"`
	Confirm          bool           `json:"confirm,omitempty" jsonschema:"true to run it; without it nothing changes and you get a preview (changes and impact) to show the user first"`
}

// body 是新建订阅的请求体（App API 的 AppSubscriptionCreate）。
func (in addSubIn) body() map[string]any {
	b := map[string]any{"media_type": in.MediaType, "tmdb_id": in.TMDBID, "season": in.Season, "upgrade": in.Upgrade, "profile_id": in.ProfileID}
	if len(in.Sites) > 0 {
		b["sites"] = in.Sites
	}
	if in.DownloaderID != 0 {
		b["downloader_id"] = in.DownloaderID
	}
	for k, v := range map[string]string{"category": in.Category, "tags": in.Tags, "save_path": in.SavePath} {
		if strings.TrimSpace(v) != "" {
			b[k] = v
		}
	}
	if in.Schedule != nil {
		b["schedule"] = *in.Schedule
	}
	if in.LockVersion != nil {
		b["lock_version"] = *in.LockVersion
	}
	if in.StartEpisode != nil {
		b["start_episode"] = *in.StartEpisode
	}
	if in.FollowNextSeason != nil {
		b["follow_next_season"] = *in.FollowNextSeason
	}
	if in.LibraryID != nil {
		b["library_id"] = *in.LibraryID
	}
	if in.FromTorrent != nil {
		b["from_torrent"] = *in.FromTorrent
	}
	return b
}

type subIDIn struct {
	ID uint `json:"id" jsonschema:"subscription id from list_subscriptions"`
}

type subStatusIn struct {
	ID      uint   `json:"id" jsonschema:"subscription id from list_subscriptions"`
	Status  string `json:"status" jsonschema:"active to start (or keep) searching, paused to stop"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"true to run it; without it nothing changes and you get a preview (changes and impact) to show the user first"`
}

type subWriteIn struct {
	ID      uint `json:"id" jsonschema:"subscription id from list_subscriptions"`
	Confirm bool `json:"confirm,omitempty" jsonschema:"true to run it; without it nothing changes and you get a preview (changes and impact) to show the user first"`
}

type recognizeIn struct {
	Title    string `json:"title" jsonschema:"torrent title (release name) to parse"`
	Subtitle string `json:"subtitle,omitempty" jsonschema:"the site's subtitle for the torrent, often the Chinese title"`
	IMDbID   string `json:"imdb_id,omitempty" jsonschema:"IMDb id the site gives (tt1234567), if any"`
}

type mediaHistoryIn struct {
	Status   string `json:"status,omitempty" jsonschema:"only records in this status"`
	Keyword  string `json:"keyword,omitempty" jsonschema:"only records whose title or torrent name contains this keyword"`
	Page     int    `json:"page,omitempty" jsonschema:"page number starting at 1"`
	PageSize int    `json:"page_size,omitempty" jsonschema:"items per page, 1 to 100 (default 20)"`
}

type organizePreviewIn struct {
	DownloaderID uint   `json:"downloader_id" jsonschema:"downloader id from list_downloader_torrents"`
	Hash         string `json:"hash" jsonschema:"info_hash from list_downloader_torrents (task_id when it has no info_hash)"`
	MediaType    string `json:"media_type,omitempty" jsonschema:"movie or tv, together with tmdb_id to override recognition"`
	TMDBID       int    `json:"tmdb_id,omitempty" jsonschema:"TMDB id (from recognize_media or explore_media) to override recognition"`
	LibraryID    uint   `json:"library_id,omitempty" jsonschema:"library id from list_media_libraries; chosen by media type when omitted"`
}

type organizeIn struct {
	DownloaderID uint   `json:"downloader_id" jsonschema:"downloader id from list_downloader_torrents"`
	Hash         string `json:"hash" jsonschema:"info_hash from list_downloader_torrents (task_id when it has no info_hash)"`
	MediaType    string `json:"media_type,omitempty" jsonschema:"movie or tv, together with tmdb_id to override recognition"`
	TMDBID       int    `json:"tmdb_id,omitempty" jsonschema:"TMDB id (from recognize_media or explore_media) to override recognition"`
	LibraryID    uint   `json:"library_id,omitempty" jsonschema:"library id from list_media_libraries; chosen by media type when omitted"`
	Confirm      bool   `json:"confirm,omitempty" jsonschema:"true to run it; without it nothing changes and you get a preview (changes and impact) to show the user first"`
}

type historyRetryIn struct {
	ID      uint `json:"id" jsonschema:"record id from list_media_history"`
	Confirm bool `json:"confirm,omitempty" jsonschema:"true to run it; without it nothing changes and you get a preview (changes and impact) to show the user first"`
}

// pushTarget 是推送的站点与种子编号：给了下载地址的按已配置的站点认出来。
func pushTarget(d *Deps, in pushIn) (site, id string, err error) {
	site, id = strings.TrimSpace(in.Site), strings.TrimSpace(in.TorrentID)
	if raw := strings.TrimSpace(in.TorrentURL); raw != "" {
		if site != "" || id != "" {
			return "", "", errors.New("torrent_url 与 site、torrent_id 只给一种")
		}
		var ok bool
		if d.ResolveURL != nil {
			site, id, ok = d.ResolveURL(raw)
		}
		if !ok {
			return "", "", errors.New("认不出这个地址：只收已配置站点的种子下载地址（磁力链接不收）")
		}
	}
	if site == "" || id == "" {
		return "", "", errors.New("要给 site 与 torrent_id（从 search_torrents 取），或者 torrent_url")
	}
	return site, id, nil
}

// organizeBody 是整理与整理预览的请求体（App API 的 AppOrganizeInput）。
func organizeBody(downloaderID uint, hash, mediaType string, tmdbID int, libraryID uint) (map[string]any, error) {
	hash = strings.TrimSpace(hash)
	if downloaderID == 0 || hash == "" {
		return nil, errors.New("要给 downloader_id 与 hash（从 list_downloader_torrents 取）")
	}
	if (tmdbID > 0) != (mediaType != "") {
		return nil, errors.New("指定条目时 media_type 与 tmdb_id 一起给")
	}
	return map[string]any{
		"downloader_id": downloaderID, "hash": hash, "media_type": mediaType, "tmdb_id": tmdbID, "library_id": libraryID,
	}, nil
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

	registerWrite(s, d, spec{
		min:  map[string]float64{"downloader_id": 1},
		name: "pause_torrent", title: "Pause a torrent",
		description: "Pause one torrent in a downloader. Changes the user's downloader: ask first and pass confirm=true.",
	}, func(ctx context.Context, d *Deps, c Caller, in torrentIn) (Preview, error) {
		return torrentPreview(ctx, d, c, "pause_torrent", in.DownloaderID, in.TaskID, false)
	}, func(in torrentIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		return torrentAction("pause", in.DownloaderID, in.TaskID)
	})

	registerWrite(s, d, spec{
		min:  map[string]float64{"downloader_id": 1},
		name: "resume_torrent", title: "Resume a torrent",
		description: "Resume one paused torrent in a downloader. Changes the user's downloader: ask first and pass confirm=true.",
	}, func(ctx context.Context, d *Deps, c Caller, in torrentIn) (Preview, error) {
		return torrentPreview(ctx, d, c, "resume_torrent", in.DownloaderID, in.TaskID, false)
	}, func(in torrentIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		return torrentAction("resume", in.DownloaderID, in.TaskID)
	})

	registerWrite(s, d, spec{
		min:  map[string]float64{"downloader_id": 1},
		name: "delete_torrent", title: "Delete a torrent", destructive: true,
		description: "Remove one torrent from a downloader, optionally deleting its files (remove_data). Cannot be undone: ask first and pass confirm=true.",
	}, func(ctx context.Context, d *Deps, c Caller, in deleteIn) (Preview, error) {
		return torrentPreview(ctx, d, c, "delete_torrent", in.DownloaderID, in.TaskID, in.RemoveData)
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

	registerWrite(s, d, spec{
		name: "push_torrent", title: "Push a torrent to a downloader", openWorld: true,
		description: "Download a torrent through pt-tools' own site configuration and add it to a downloader, applying the disk-space and site-capacity checks. " +
			"Give site and torrent_id from search_torrents, or the torrent's download URL on a configured site. Magnet links and other URLs are not accepted. " +
			"Changes the user's downloader: ask first and pass confirm=true.",
	}, func(ctx context.Context, d *Deps, c Caller, in pushIn) (Preview, error) {
		site, id, err := pushTarget(d, in)
		if err != nil {
			return Preview{}, err
		}
		return pushPreview(ctx, d, c, site, id, in), nil
	}, func(in pushIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		site, id, err := pushTarget(d, in)
		if err != nil {
			return call{}, err
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
		description: "Show the user's account on each configured PT site: upload, billed download, ratio, bonus, seeding count, unread messages, login status and check-in result (cached; updated_at tells when). " +
			"user.downloaded is the download the site bills after free and discount promotions (the figure that counts toward ratio), not the amount actually transferred.",
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

	registerWrite(s, d, spec{
		name: "add_subscription", title: "Subscribe to a movie or TV show", openWorld: true,
		min: map[string]float64{"tmdb_id": 1, "season": 0, "profile_id": 0, "downloader_id": 0, "start_episode": 0, "library_id": 0},
		max: map[string]float64{"start_episode": 2000},
		description: "Subscribe to a movie or a TV season by TMDB id (from explore_media): pt-tools then searches the user's sites and downloads it. " +
			"A TV season is followed episode by episode: by default it searches by each episode's air time and locks the version (release group and resolution) " +
			"of the first download; schedule, lock_version, start_episode, from_torrent and the other fields change that. " +
			"Changes the user's setup: ask first and pass confirm=true.",
		enums: map[string][]any{"media_type": {"movie", "tv"}},
	}, func(_ context.Context, _ *Deps, _ Caller, in addSubIn) (Preview, error) {
		if in.TMDBID <= 0 {
			return Preview{}, errors.New("tmdb_id 要填（从 explore_media 取）")
		}
		return addSubscriptionPreview(in), nil
	}, func(in addSubIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		if in.TMDBID <= 0 {
			return call{}, errors.New("tmdb_id 要填（从 explore_media 取）")
		}
		body := in.body()
		return call{
			req:   Request{Method: http.MethodPost, Path: "/subscriptions", Body: body},
			audit: map[string]any{"media_type": in.MediaType, "tmdb_id": in.TMDBID, "season": in.Season, "fields": sortedKeys(body)},
		}, nil
	})

	registerMediaTools(s, d)
	registerFollowTools(s, d)
	registerConfigTools(s, d)
}

// registerMediaTools 注册订阅管理、媒体识别与整理入库的工具。
func registerMediaTools(s *sdk.Server, d *Deps) {
	register(s, d, spec{
		name: "get_subscription", title: "Subscription details",
		min: map[string]float64{"id": 1},
		description: "Show one subscription: its settings (schedule, version lock, sites, downloader, library), progress, each episode's state " +
			"(library, downloading, missing, waiting_version, upcoming, skipped) and the torrents it downloaded with how each file was organized and scraped " +
			"(can_organize and can_rescrape say whether reorganize_subscription_torrent and rescrape_subscription_torrent apply).",
	}, func(in subIDIn) (call, error) {
		return call{req: Request{Method: http.MethodGet, Path: "/subscriptions/" + utoa(in.ID)}}, nil
	})

	registerWrite(s, d, spec{
		name: "set_subscription_status", title: "Pause or resume a subscription",
		min:         map[string]float64{"id": 1},
		enums:       map[string][]any{"status": {"active", "paused"}},
		description: "Pause a subscription (stop searching) or set it active again. Changes the user's subscriptions: ask first and pass confirm=true.",
	}, func(ctx context.Context, d *Deps, c Caller, in subStatusIn) (Preview, error) {
		return subscriptionPreview(ctx, d, c, "set_subscription_status", in.ID, in.Status)
	}, func(in subStatusIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		return call{
			req:   Request{Method: http.MethodPost, Path: "/subscriptions/" + utoa(in.ID) + "/status", Body: map[string]any{"status": in.Status}},
			audit: map[string]any{"id": in.ID, "status": in.Status},
		}, nil
	})

	registerWrite(s, d, spec{
		name: "search_subscription", title: "Search a subscription now", openWorld: true,
		min: map[string]float64{"id": 1},
		description: "Search the user's sites for a subscription right now instead of waiting for the next scheduled search; matching torrents are downloaded. " +
			"Changes the user's downloader: ask first and pass confirm=true.",
	}, func(ctx context.Context, d *Deps, c Caller, in subWriteIn) (Preview, error) {
		return subscriptionPreview(ctx, d, c, "search_subscription", in.ID, "")
	}, func(in subWriteIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		return call{req: Request{Method: http.MethodPost, Path: "/subscriptions/" + utoa(in.ID) + "/search"}, audit: map[string]any{"id": in.ID}}, nil
	})

	registerWrite(s, d, spec{
		name: "delete_subscription", title: "Delete a subscription", destructive: true,
		min:         map[string]float64{"id": 1},
		description: "Delete a subscription. Downloaded torrents and library files stay. Cannot be undone: ask first and pass confirm=true.",
	}, func(ctx context.Context, d *Deps, c Caller, in subWriteIn) (Preview, error) {
		return subscriptionPreview(ctx, d, c, "delete_subscription", in.ID, "")
	}, func(in subWriteIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		return call{req: Request{Method: http.MethodDelete, Path: "/subscriptions/" + utoa(in.ID)}, audit: map[string]any{"id": in.ID}}, nil
	})

	register(s, d, spec{
		name: "recognize_media", title: "Recognize a torrent title", openWorld: true,
		description: "Parse a torrent title (name, year, season and episode, resolution, source, codec, group) and match it to a TMDB movie or TV show, " +
			"applying the user's corrections and recognition words. Returns the match, how it was found and the best candidates.",
	}, func(in recognizeIn) (call, error) {
		if strings.TrimSpace(in.Title) == "" && strings.TrimSpace(in.Subtitle) == "" {
			return call{}, errors.New("title 与 subtitle 至少给一个")
		}
		return call{req: Request{Method: http.MethodPost, Path: "/media/recognize", Body: map[string]any{
			"title": in.Title, "subtitle": in.Subtitle, "imdb_id": in.IMDbID,
		}}}, nil
	})

	register(s, d, spec{
		name: "list_media_libraries", title: "List media libraries",
		description: "List the user's media libraries (movie or TV, directory, organize mode, scraping) that organize_media puts files into.",
	}, func(struct{}) (call, error) {
		return call{req: Request{Method: http.MethodGet, Path: "/media/libraries"}}, nil
	})

	register(s, d, spec{
		name: "list_media_history", title: "List library organize history",
		min: map[string]float64{"page": 1, "page_size": 1}, max: map[string]float64{"page_size": 100},
		enums:       map[string][]any{"status": {"done", "failed", "skipped", "removed"}},
		description: "List organize records, newest first: one per video file put into a library (or skipped, failed), with the target path and the reason when it failed. Failed records can be retried with retry_media_organize.",
	}, func(in mediaHistoryIn) (call, error) {
		return call{req: Request{Method: http.MethodGet, Path: withQuery("/media/history",
			"status", in.Status, "q", in.Keyword, "page", itoa(in.Page), "page_size", itoa(in.PageSize))}}, nil
	})

	register(s, d, spec{
		name: "preview_media_organize", title: "Preview organizing a torrent", openWorld: true,
		min:   map[string]float64{"downloader_id": 1, "tmdb_id": 0, "library_id": 0},
		enums: map[string][]any{"media_type": {"movie", "tv"}},
		description: "Show what organize_media would do with a downloaded torrent, without changing anything: the recognized TMDB entry, the library, " +
			"and each video's target path and status (pending, done, exists, replace, failed).",
	}, func(in organizePreviewIn) (call, error) {
		body, err := organizeBody(in.DownloaderID, in.Hash, in.MediaType, in.TMDBID, in.LibraryID)
		if err != nil {
			return call{}, err
		}
		return call{req: Request{Method: http.MethodPost, Path: "/media/organize/preview", Body: body}}, nil
	})

	registerWrite(s, d, spec{
		name: "organize_media", title: "Organize a torrent into the library", openWorld: true,
		min:   map[string]float64{"downloader_id": 1, "tmdb_id": 0, "library_id": 0},
		enums: map[string][]any{"media_type": {"movie", "tv"}},
		description: "Put a downloaded torrent's videos into a media library with the naming template (hardlink, copy or move as the library says), " +
			"write NFO and posters, and ask the media servers to scan. Use preview_media_organize first. Changes the user's library: ask first and pass confirm=true.",
	}, organizePreview, func(in organizeIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		body, err := organizeBody(in.DownloaderID, in.Hash, in.MediaType, in.TMDBID, in.LibraryID)
		if err != nil {
			return call{}, err
		}
		return call{
			req:   Request{Method: http.MethodPost, Path: "/media/organize", Body: body},
			audit: map[string]any{"downloader_id": in.DownloaderID, "hash": strings.TrimSpace(in.Hash), "tmdb_id": in.TMDBID, "library_id": in.LibraryID},
		}, nil
	})

	registerWrite(s, d, spec{
		name: "retry_media_organize", title: "Retry organizing a record", openWorld: true,
		min:         map[string]float64{"id": 1},
		description: "Organize again the torrent of one record from list_media_history (usually a failed one). Changes the user's library: ask first and pass confirm=true.",
	}, func(ctx context.Context, d *Deps, c Caller, in historyRetryIn) (Preview, error) {
		return retryPreview(ctx, d, c, in.ID)
	}, func(in historyRetryIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		return call{req: Request{Method: http.MethodPost, Path: "/media/history/" + utoa(in.ID) + "/retry"}, audit: map[string]any{"id": in.ID}}, nil
	})
}
