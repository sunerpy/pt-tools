package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// 追剧的工具：改订阅的设置（定时、版本锁、起始集、站点、下载器、媒体库……）、每一集跳过与换版本、运行记录、集号规则、
// 定时预览，以及订阅下载的种子重新整理、重新刮削。都经 App API 的订阅接口，和网页、App 上的操作一样。

// scheduleIn 是追剧的定时（和 App API 的 AppSchedule 一样）。
type scheduleIn struct {
	Kind            string `json:"kind" jsonschema:"airtime (by each episode's air time; TV only), interval (every interval_minutes), daily, every_days, weekly, monthly or cron; an empty string uses the global search interval"`
	At              string `json:"at,omitempty" jsonschema:"time of day HH:MM in pt-tools' time zone for daily, every_days, weekly and monthly (20:00 when empty); for airtime the time the episode airs"`
	Every           int    `json:"every,omitempty" jsonschema:"every_days: days between searches, 1 to 30"`
	Days            []int  `json:"days,omitempty" jsonschema:"weekly: weekdays 0 to 6 (0 is Sunday); monthly: days of the month 1 to 31 (short months use their last day)"`
	AirDelayHours   int    `json:"air_delay_hours,omitempty" jsonschema:"airtime: hours after airing to start searching, 0 to 72 (0 means 2)"`
	AirRetryHours   int    `json:"air_retry_hours,omitempty" jsonschema:"airtime: hours between retries while not found, 0 to 48 (0 means 6)"`
	AirWindowDays   int    `json:"air_window_days,omitempty" jsonschema:"airtime: days after airing to keep retrying at that pace, 0 to 60 (0 means 7); after that it searches once a day"`
	IntervalMinutes int    `json:"interval_minutes,omitempty" jsonschema:"interval: minutes between searches, 30 to 10080 (7 days), counted from the last search"`
	Cron            string `json:"cron,omitempty" jsonschema:"cron: 5-field cron expressions (minute hour day month weekday) in pt-tools' time zone, several separated by ; for example '0 20 * * 5' (Fridays 20:00) or '0 9,21 * * *' (twice a day)"`
}

type fromTorrentIn struct {
	Site      string `json:"site" jsonschema:"site ID from search_torrents"`
	TorrentID string `json:"torrent_id" jsonschema:"torrent ID from search_torrents"`
	Title     string `json:"title,omitempty" jsonschema:"the torrent's title, used to search that site only when the search cache no longer has it"`
}

type updateSubIn struct {
	ID               uint        `json:"id" jsonschema:"subscription id from list_subscriptions"`
	ProfileID        *uint       `json:"profile_id,omitempty" jsonschema:"quality profile id (get_config resource quality_profiles); 0 uses the default profile"`
	Upgrade          *bool       `json:"upgrade,omitempty" jsonschema:"keep looking for better releases after the first download"`
	Sites            *[]string   `json:"sites,omitempty" jsonschema:"only search these sites (site IDs); an empty array means every enabled site"`
	DownloaderID     *uint       `json:"downloader_id,omitempty" jsonschema:"downloader to push to (ids from get_downloader_stats); 0 uses the subscription settings' default"`
	Category         *string     `json:"category,omitempty" jsonschema:"downloader category for pushes"`
	Tags             *string     `json:"tags,omitempty" jsonschema:"comma-separated downloader tags for pushes"`
	SavePath         *string     `json:"save_path,omitempty" jsonschema:"save directory in the downloader; empty uses the downloader's default"`
	Schedule         *scheduleIn `json:"schedule,omitempty" jsonschema:"when to search actively (check it with preview_schedule)"`
	LockVersion      *bool       `json:"lock_version,omitempty" jsonschema:"TV: after the first episode only take the same release group and resolution (wait rather than switch versions)"`
	LockGroup        *string     `json:"lock_group,omitempty" jsonschema:"TV: the locked release group; empty strings for both lock_group and lock_resolution clear the locked version (the next download decides it again)"`
	LockResolution   *string     `json:"lock_resolution,omitempty" jsonschema:"TV: the locked resolution such as 1080p"`
	StartEpisode     *int        `json:"start_episode,omitempty" jsonschema:"TV: follow from this episode on (0 and 1 mean the whole season)"`
	FollowNextSeason *bool       `json:"follow_next_season,omitempty" jsonschema:"TV: follow the next season automatically when this one is done"`
	LibraryID        *uint       `json:"library_id,omitempty" jsonschema:"media library to organize into (list_media_libraries); 0 picks one by type"`
	TotalEpisodes    *int        `json:"total_episodes,omitempty" jsonschema:"TV: episodes in this season when TMDB's count is wrong; 0 goes back to TMDB's count"`
	Confirm          bool        `json:"confirm,omitempty" jsonschema:"true to run it; without it nothing changes and you get a preview (changes and impact) to show the user first"`
}

type subRunsIn struct {
	ID       uint `json:"id" jsonschema:"subscription id from list_subscriptions"`
	Page     int  `json:"page,omitempty" jsonschema:"page number starting at 1"`
	PageSize int  `json:"page_size,omitempty" jsonschema:"items per page, 1 to 100 (default 20)"`
}

type episodeSkipIn struct {
	ID      uint `json:"id" jsonschema:"subscription id from list_subscriptions"`
	Episode int  `json:"episode" jsonschema:"episode number in the season (from get_subscription episodes)"`
	Skip    bool `json:"skip" jsonschema:"true to skip the episode (not followed: not missing and does not hold up completion), false to want it again"`
	Confirm bool `json:"confirm,omitempty" jsonschema:"true to run it; without it nothing changes and you get a preview (changes and impact) to show the user first"`
}

type episodeUnlockIn struct {
	ID      uint `json:"id" jsonschema:"subscription id from list_subscriptions"`
	Episode int  `json:"episode" jsonschema:"episode number in the season (from get_subscription episodes)"`
	Unlock  bool `json:"unlock" jsonschema:"true to let this episode use another version than the locked one (pt-tools searches once right away), false to put it back under the lock"`
	Confirm bool `json:"confirm,omitempty" jsonschema:"true to run it; without it nothing changes and you get a preview (changes and impact) to show the user first"`
}

type episodeRuleIn struct {
	ID      uint   `json:"id" jsonschema:"subscription id from list_subscriptions"`
	Group   string `json:"group,omitempty" jsonschema:"release group the rule applies to (as in the torrent titles); empty applies to every group"`
	Mode    string `json:"mode" jsonschema:"tmdb (the title's episode number is TMDB's), period_part (issue and part, e.g. a variety show's 'issue 1 part 2' is TMDB episode 2), date (by air date) or offset (TMDB episode = title's number + offset)"`
	Offset  int    `json:"offset,omitempty" jsonschema:"offset mode: the number to add, -2000 to 2000 (not 0)"`
	Note    string `json:"note,omitempty" jsonschema:"why the rule is needed"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"true to run it; without it nothing changes and you get a preview (changes and impact) to show the user first"`
}

type episodeRuleDeleteIn struct {
	ID      uint   `json:"id" jsonschema:"subscription id from list_subscriptions"`
	Group   string `json:"group,omitempty" jsonschema:"release group of the rule to delete; empty is the rule for every group"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"true to run it; without it nothing changes and you get a preview (changes and impact) to show the user first"`
}

type schedulePreviewIn struct {
	Schedule       scheduleIn `json:"schedule" jsonschema:"the schedule to check"`
	MediaType      string     `json:"media_type,omitempty" jsonschema:"movie or tv (airtime is TV only); taken from subscription_id when given, tv when omitted"`
	SubscriptionID uint       `json:"subscription_id,omitempty" jsonschema:"work it out for this subscription (every_days counts from the day it was created)"`
	Count          int        `json:"count,omitempty" jsonschema:"how many upcoming searches, 1 to 10 (default 5)"`
}

type subTorrentIn struct {
	ID                    uint `json:"id" jsonschema:"subscription id from list_subscriptions"`
	SubscriptionTorrentID uint `json:"subscription_torrent_id" jsonschema:"id of the torrent in get_subscription's torrents (not the site's torrent_id)"`
	Confirm               bool `json:"confirm,omitempty" jsonschema:"true to run it; without it nothing changes and you get a preview (changes and impact) to show the user first"`
}

// subPath 是一个订阅的接口地址。
func subPath(id uint, suffix string) string {
	return "/subscriptions/" + utoa(id) + suffix
}

// updateBody 是改订阅的请求体：只放给了的字段。
func (in updateSubIn) body() map[string]any {
	b := map[string]any{}
	set := func(k string, v any, given bool) {
		if given {
			b[k] = v
		}
	}
	set("profile_id", ptrValue(in.ProfileID), in.ProfileID != nil)
	set("upgrade", ptrValue(in.Upgrade), in.Upgrade != nil)
	if in.Sites != nil {
		b["sites"] = append([]string{}, (*in.Sites)...)
	}
	set("downloader_id", ptrValue(in.DownloaderID), in.DownloaderID != nil)
	set("category", ptrValue(in.Category), in.Category != nil)
	set("tags", ptrValue(in.Tags), in.Tags != nil)
	set("save_path", ptrValue(in.SavePath), in.SavePath != nil)
	if in.Schedule != nil {
		b["schedule"] = *in.Schedule
	}
	set("lock_version", ptrValue(in.LockVersion), in.LockVersion != nil)
	set("lock_group", ptrValue(in.LockGroup), in.LockGroup != nil)
	set("lock_resolution", ptrValue(in.LockResolution), in.LockResolution != nil)
	set("start_episode", ptrValue(in.StartEpisode), in.StartEpisode != nil)
	set("follow_next_season", ptrValue(in.FollowNextSeason), in.FollowNextSeason != nil)
	set("library_id", ptrValue(in.LibraryID), in.LibraryID != nil)
	set("total_episodes", ptrValue(in.TotalEpisodes), in.TotalEpisodes != nil)
	return b
}

func ptrValue[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

// sortedKeys 是请求体里给了的字段（审计只记字段名）。
func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func registerFollowTools(s *sdk.Server, d *Deps) {
	registerWrite(s, d, spec{
		name: "update_subscription", title: "Change a subscription's settings", openWorld: true,
		min: map[string]float64{"id": 1, "profile_id": 0, "downloader_id": 0, "start_episode": 0, "library_id": 0, "total_episodes": 0},
		max: map[string]float64{"start_episode": 2000, "total_episodes": 2000},
		description: "Change a subscription's settings; only the fields given change (the title and season cannot): where to search and push (sites, downloader, " +
			"category, tags, save path, quality profile, upgrade) and, for a followed TV season, the schedule, version lock, start episode, next season, " +
			"media library and episode count. Changing the schedule, lock, start episode or episode count works out every episode again and searches soon. " +
			"Changes the user's subscriptions: ask first and pass confirm=true.",
	}, updateSubscriptionPreview, func(in updateSubIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		body := in.body()
		if len(body) == 0 {
			return call{}, errors.New("没有要改的字段")
		}
		return call{
			req:   Request{Method: http.MethodPut, Path: subPath(in.ID, ""), Body: body},
			audit: map[string]any{"id": in.ID, "fields": sortedKeys(body)},
		}, nil
	})

	register(s, d, spec{
		name: "list_subscription_runs", title: "Subscription run history",
		min: map[string]float64{"id": 1, "page": 1, "page_size": 1}, max: map[string]float64{"page_size": 100},
		description: "List a subscription's runs, newest first: every active search, RSS match, creation and change by hand, " +
			"with what was found on which sites, which torrents were rejected and why (quality profile, not the locked version, episode number mismatch), " +
			"what was pushed and what failed. Use it to find out why something was not downloaded.",
	}, func(in subRunsIn) (call, error) {
		return call{req: Request{Method: http.MethodGet, Path: withQuery(subPath(in.ID, "/runs"), "page", itoa(in.Page), "page_size", itoa(in.PageSize))}}, nil
	})

	registerWrite(s, d, spec{
		name: "set_episode_skip", title: "Skip or want an episode",
		min:         map[string]float64{"id": 1, "episode": 1},
		description: "Skip one episode of a followed TV season (it is not followed: not missing and does not hold up completion), or want it again. Changes the user's subscriptions: ask first and pass confirm=true.",
	}, func(ctx context.Context, d *Deps, c Caller, in episodeSkipIn) (Preview, error) {
		return episodePreview(ctx, d, c, "set_episode_skip", in.ID, in.Episode, in.Skip)
	}, func(in episodeSkipIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		return call{
			req:   Request{Method: http.MethodPost, Path: subPath(in.ID, "/episodes/"+strconv.Itoa(in.Episode)+"/skip"), Body: map[string]any{"skip": in.Skip}},
			audit: map[string]any{"id": in.ID, "episode": in.Episode, "skip": in.Skip},
		}, nil
	})

	registerWrite(s, d, spec{
		name: "unlock_episode", title: "Let an episode use another version", openWorld: true,
		min: map[string]float64{"id": 1, "episode": 1},
		description: "Let one episode of a followed TV season ignore the version lock (for when the locked release group never releases it; pt-tools searches once right away), " +
			"or put it back under the lock. Changes the user's subscriptions: ask first and pass confirm=true.",
	}, func(ctx context.Context, d *Deps, c Caller, in episodeUnlockIn) (Preview, error) {
		return episodePreview(ctx, d, c, "unlock_episode", in.ID, in.Episode, in.Unlock)
	}, func(in episodeUnlockIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		return call{
			req:   Request{Method: http.MethodPost, Path: subPath(in.ID, "/episodes/"+strconv.Itoa(in.Episode)+"/unlock"), Body: map[string]any{"unlock": in.Unlock}},
			audit: map[string]any{"id": in.ID, "episode": in.Episode, "unlock": in.Unlock},
		}, nil
	})

	register(s, d, spec{
		name: "get_episode_rules", title: "Episode-number rules",
		min:         map[string]float64{"id": 1},
		description: "List a followed TV season's episode-number rules: how the episode number in a release group's torrent titles maps to the TMDB episode (variety shows often number by issue and part, or by air date).",
	}, func(in subIDIn) (call, error) {
		return call{req: Request{Method: http.MethodGet, Path: subPath(in.ID, "/episode-rules")}}, nil
	})

	registerWrite(s, d, spec{
		name: "save_episode_rule", title: "Set an episode-number rule", openWorld: true,
		min: map[string]float64{"id": 1, "offset": -2000}, max: map[string]float64{"offset": 2000},
		enums: map[string][]any{"mode": {"tmdb", "period_part", "date", "offset"}},
		description: "Set how one release group's episode numbers map to TMDB episodes for a followed TV season (replacing that group's rule); " +
			"use it when list_subscription_runs says the episode number does not match. pt-tools searches again soon with the rule. " +
			"Changes the user's subscriptions: ask first and pass confirm=true.",
	}, episodeRulePreview, func(in episodeRuleIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		group := strings.TrimSpace(in.Group)
		return call{
			req: Request{Method: http.MethodPut, Path: subPath(in.ID, "/episode-rules"), Body: map[string]any{
				"group": group, "mode": in.Mode, "offset": in.Offset, "note": strings.TrimSpace(in.Note),
			}},
			audit: map[string]any{"id": in.ID, "group": group, "mode": in.Mode},
		}, nil
	})

	registerWrite(s, d, spec{
		name: "delete_episode_rule", title: "Delete an episode-number rule", destructive: true,
		min:         map[string]float64{"id": 1},
		description: "Delete one release group's episode-number rule of a followed TV season. Changes the user's subscriptions: ask first and pass confirm=true.",
	}, episodeRuleDeletePreview, func(in episodeRuleDeleteIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		group := strings.TrimSpace(in.Group)
		return call{
			req:   Request{Method: http.MethodDelete, Path: withQuery(subPath(in.ID, "/episode-rules"), "group", group)},
			audit: map[string]any{"id": in.ID, "group": group},
		}, nil
	})

	register(s, d, spec{
		name: "preview_schedule", title: "Preview a search schedule",
		min: map[string]float64{"subscription_id": 0, "count": 1}, max: map[string]float64{"count": 10},
		enums: map[string][]any{"media_type": {"movie", "tv"}},
		description: "Work out when a subscription schedule would search next, without saving anything: the next few times (next_text is in pt-tools' time zone). " +
			"An invalid schedule (for example a bad cron field) returns the reason. An airtime schedule has no fixed times; note explains how it searches.",
	}, func(in schedulePreviewIn) (call, error) {
		return call{
			req: Request{Method: http.MethodPost, Path: "/subscriptions/schedule/preview", Body: map[string]any{
				"schedule": in.Schedule, "media_type": in.MediaType, "subscription_id": in.SubscriptionID, "count": in.Count,
			}},
			shape: func(out map[string]any) map[string]any {
				next, _ := out["next"].([]any)
				texts := make([]string, 0, len(next))
				for _, v := range next {
					if sec, ok := v.(float64); ok {
						texts = append(texts, timeText(int64(sec)))
					}
				}
				out["next_text"] = texts
				return out
			},
		}, nil
	})

	registerWrite(s, d, spec{
		name: "reorganize_subscription_torrent", title: "Organize a subscription's torrent again", openWorld: true,
		min: map[string]float64{"id": 1, "subscription_torrent_id": 1},
		description: "Organize again one torrent a subscription downloaded (a season pack works the same as a single episode), using the subscription's title and library " +
			"instead of recognizing the name; files already organized stay. Use it when get_subscription shows files that failed or were skipped (can_organize). " +
			"Changes the user's library: ask first and pass confirm=true.",
	}, func(ctx context.Context, d *Deps, c Caller, in subTorrentIn) (Preview, error) {
		return subTorrentPreview(ctx, d, c, "reorganize_subscription_torrent", in.ID, in.SubscriptionTorrentID)
	}, func(in subTorrentIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		return call{
			req:   Request{Method: http.MethodPost, Path: subPath(in.ID, "/torrents/"+utoa(in.SubscriptionTorrentID)+"/organize")},
			audit: map[string]any{"id": in.ID, "torrent": in.SubscriptionTorrentID},
		}, nil
	})

	registerWrite(s, d, spec{
		name: "rescrape_subscription_torrent", title: "Scrape a subscription's torrent again", openWorld: true,
		min: map[string]float64{"id": 1, "subscription_torrent_id": 1},
		description: "Rewrite the NFO and images of the movie or show one subscription torrent was organized into, with the library's current scrape settings (can_rescrape in get_subscription). " +
			"Changes the user's library: ask first and pass confirm=true.",
	}, func(ctx context.Context, d *Deps, c Caller, in subTorrentIn) (Preview, error) {
		return subTorrentPreview(ctx, d, c, "rescrape_subscription_torrent", in.ID, in.SubscriptionTorrentID)
	}, func(in subTorrentIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		return call{
			req:   Request{Method: http.MethodPost, Path: subPath(in.ID, "/torrents/"+utoa(in.SubscriptionTorrentID)+"/scrape")},
			audit: map[string]any{"id": in.ID, "torrent": in.SubscriptionTorrentID},
		}, nil
	})

	registerWrite(s, d, spec{
		name: "rescrape_media", title: "Scrape a library record again", openWorld: true,
		min: map[string]float64{"id": 1},
		description: "Rewrite the NFO and images of the movie or show of one organized record from list_media_history (every record of the same title in that library), " +
			"with the library's current scrape settings; fields locked in the media server stay. Changes the user's library: ask first and pass confirm=true.",
	}, func(ctx context.Context, d *Deps, c Caller, in historyRetryIn) (Preview, error) {
		return rescrapePreview(ctx, d, c, in.ID)
	}, func(in historyRetryIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		return call{req: Request{Method: http.MethodPost, Path: "/media/history/" + utoa(in.ID) + "/scrape"}, audit: map[string]any{"id": in.ID}}, nil
	})
}

// timeText 是 Unix 秒写成 pt-tools 所在时区的「2026-10-23 周五 20:00」。
func timeText(sec int64) string {
	t := time.Unix(sec, 0).In(time.Local)
	return t.Format("2006-01-02 ") + weekdayNames[t.Weekday()] + t.Format(" 15:04")
}

var weekdayNames = [...]string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

// scheduleLabel 是定时的中文说明（预览里用）。
func scheduleLabel(sc scheduleIn) string {
	at := sc.At
	if at == "" {
		at = "20:00"
	}
	switch sc.Kind {
	case "":
		return "按全局的搜索间隔"
	case "airtime":
		return "按每一集的播出时间"
	case "interval":
		m := sc.IntervalMinutes
		switch {
		case m > 0 && m%(24*60) == 0:
			return fmt.Sprintf("每 %d 天", m/(24*60))
		case m > 0 && m%60 == 0:
			return fmt.Sprintf("每 %d 小时", m/60)
		default:
			return fmt.Sprintf("每 %d 分钟", m)
		}
	case "daily":
		return "每天 " + at
	case "every_days":
		return fmt.Sprintf("每 %d 天 %s", max(sc.Every, 1), at)
	case "weekly":
		days := make([]string, 0, len(sc.Days))
		for _, d := range sc.Days {
			if d >= 0 && d < 7 {
				days = append(days, weekdayNames[d])
			}
		}
		return "每" + strings.Join(days, "、") + " " + at
	case "monthly":
		days := make([]string, 0, len(sc.Days))
		for _, d := range sc.Days {
			days = append(days, strconv.Itoa(d))
		}
		return "每月 " + strings.Join(days, "、") + " 号 " + at
	case "cron":
		return "按 cron：" + sc.Cron
	default:
		return sc.Kind
	}
}

// followNow 是订阅的现状（App API 的订阅详情里预览要的几项）。
type followNow struct {
	subscriptionNow
	ProfileID        uint            `json:"profile_id"`
	Sites            []string        `json:"sites"`
	DownloaderID     uint            `json:"downloader_id"`
	Category         string          `json:"category"`
	Tags             string          `json:"tags"`
	SavePath         string          `json:"save_path"`
	Schedule         scheduleIn      `json:"schedule"`
	LockVersion      bool            `json:"lock_version"`
	LockGroup        string          `json:"lock_group"`
	LockResolution   string          `json:"lock_resolution"`
	StartEpisode     int             `json:"start_episode"`
	FollowNextSeason bool            `json:"follow_next_season"`
	LibraryID        uint            `json:"library_id"`
	TotalEpisodes    int             `json:"total_episodes"`
	Episodes         []episodeNow    `json:"episodes"`
	Torrents         []subTorrentNow `json:"torrents"`
}

type episodeNow struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
	State  string `json:"state"`
}

type subTorrentNow struct {
	ID     uint   `json:"id"`
	Site   string `json:"site"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Files  []struct {
		Name         string `json:"name"`
		Status       string `json:"status"`
		Message      string `json:"message"`
		ScrapeStatus string `json:"scrape_status"`
		Retrying     bool   `json:"retrying"`
	} `json:"files"`
	CanOrganize bool `json:"can_organize"`
	CanRescrape bool `json:"can_rescrape"`
}

// readFollow 读订阅的现状：没有这个订阅时回错误（写明怎么查）。
func readFollow(ctx context.Context, d *Deps, c Caller, id uint) (*followNow, error) {
	var sub followNow
	err := readJSON(ctx, d, c, http.MethodGet, subPath(id, ""), nil, &sub)
	if err != nil && notFound(err) {
		return nil, fmt.Errorf("没有编号 %d 的订阅：先用 list_subscriptions 查一下", id)
	}
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func onOff(b bool) string {
	if b {
		return "开"
	}
	return "关"
}

func sitesLabel(sites []string) string {
	if len(sites) == 0 {
		return "全部站点"
	}
	return strings.Join(sites, "、")
}

func idLabel(id uint, zero, prefix string) string {
	if id == 0 {
		return zero
	}
	return prefix + " " + utoa(id)
}

func textOr(s, empty string) string {
	if strings.TrimSpace(s) == "" {
		return empty
	}
	return s
}

func lockLabel(group, res string) string {
	if group == "" && res == "" {
		return "还没定（按下一个下载的版本定）"
	}
	return strings.TrimSpace(group + " " + res)
}

func startLabel(n int) string {
	if n <= 1 {
		return "整季"
	}
	return fmt.Sprintf("从第 %d 集开始", n)
}

func totalLabel(n int) string {
	if n <= 0 {
		return "按 TMDB"
	}
	return fmt.Sprintf("%d 集", n)
}

// updateSubscriptionPreview 是改订阅设置的预览：给了的每一项改以前、改以后（一样的不列），改了定时时列出接下来几次搜索。
func updateSubscriptionPreview(ctx context.Context, d *Deps, c Caller, in updateSubIn) (Preview, error) {
	p := Preview{Tool: "update_subscription"}
	if len(in.body()) == 0 {
		return p, errors.New("没有要改的字段：给出要改的设置（比如 schedule、lock_version、sites）")
	}
	sub, err := readFollow(ctx, d, c, in.ID)
	if err != nil {
		if strings.HasPrefix(err.Error(), "没有编号") {
			return p, err
		}
		p.Summary = fmt.Sprintf("修改订阅 %d 的设置", in.ID)
		p.Impact = append(p.Impact, unreadable("这个订阅", err))
		return p, nil
	}
	p.Summary = "修改订阅" + sub.label() + "的设置"
	add := func(label, before, after string) {
		if before != after {
			p.Changes = append(p.Changes, Change{Label: label, Before: before, After: after})
		}
	}
	recompute := false
	if in.ProfileID != nil {
		add("质量档案", idLabel(sub.ProfileID, "默认质量档案", "质量档案"), idLabel(*in.ProfileID, "默认质量档案", "质量档案"))
	}
	if in.Upgrade != nil {
		add("洗版", onOff(sub.Upgrade), onOff(*in.Upgrade))
	}
	if in.Sites != nil {
		add("在哪些站点找", sitesLabel(sub.Sites), sitesLabel(*in.Sites))
	}
	if in.DownloaderID != nil {
		add("推送到", idLabel(sub.DownloaderID, "订阅设置里的下载器", "下载器"), idLabel(*in.DownloaderID, "订阅设置里的下载器", "下载器"))
	}
	if in.Category != nil {
		add("分类", textOr(sub.Category, "不设"), textOr(*in.Category, "不设"))
	}
	if in.Tags != nil {
		add("标签", textOr(sub.Tags, "不设"), textOr(*in.Tags, "不设"))
	}
	if in.SavePath != nil {
		add("保存目录", textOr(sub.SavePath, "下载器的默认目录"), textOr(*in.SavePath, "下载器的默认目录"))
	}
	if in.Schedule != nil {
		before, after := scheduleLabel(sub.Schedule), scheduleLabel(*in.Schedule)
		if before != after {
			recompute = true
		}
		add("定时", before, after)
	}
	if in.LockVersion != nil {
		if *in.LockVersion != sub.LockVersion {
			recompute = true
		}
		add("版本锁", onOff(sub.LockVersion), onOff(*in.LockVersion))
	}
	if in.LockGroup != nil || in.LockResolution != nil {
		group, res := sub.LockGroup, sub.LockResolution
		if in.LockGroup != nil {
			group = strings.TrimSpace(*in.LockGroup)
		}
		if in.LockResolution != nil {
			res = strings.TrimSpace(*in.LockResolution)
		}
		before, after := lockLabel(sub.LockGroup, sub.LockResolution), lockLabel(group, res)
		if before != after {
			recompute = true
		}
		add("锁住的版本", before, after)
	}
	if in.StartEpisode != nil {
		if startLabel(*in.StartEpisode) != startLabel(sub.StartEpisode) {
			recompute = true
		}
		add("起始集", startLabel(sub.StartEpisode), startLabel(*in.StartEpisode))
	}
	if in.FollowNextSeason != nil {
		add("追完以后追下一季", onOff(sub.FollowNextSeason), onOff(*in.FollowNextSeason))
	}
	if in.LibraryID != nil {
		add("整理进的媒体库", idLabel(sub.LibraryID, "按类型自动选", "媒体库"), idLabel(*in.LibraryID, "按类型自动选", "媒体库"))
	}
	if in.TotalEpisodes != nil {
		if totalLabel(*in.TotalEpisodes) != totalLabel(sub.TotalEpisodes) {
			recompute = true
		}
		add("这一季的集数", totalLabel(sub.TotalEpisodes), totalLabel(*in.TotalEpisodes))
	}
	if len(p.Changes) == 0 {
		p.Impact = append(p.Impact, "给的设置和现在一样：保存了也不会有变化")
		return p, nil
	}
	// 只有在找的订阅会搜索：完成了、暂停着、在等上映的改了设置也不马上搜
	searching := sub.Status == "" || sub.Status == "active"
	switch {
	case !searching:
		p.Impact = append(p.Impact, notSearchingNote(sub.Status))
	case recompute:
		p.Impact = append(p.Impact, "马上按新的设置重新算每一集、尽快搜索一次，搜到符合的就推送到下载器；运行记录里记一条")
	}
	if in.Schedule != nil && in.Schedule.Kind != "airtime" {
		// 定时写错了时预览就说（不在找的订阅也查）；接下来几次只给在找的
		var next struct {
			Next []int64 `json:"next"`
		}
		body := map[string]any{"schedule": *in.Schedule, "subscription_id": in.ID, "count": 3}
		if err := readJSON(ctx, d, c, http.MethodPost, "/subscriptions/schedule/preview", body, &next); err != nil {
			var e *errAPI
			if errors.As(err, &e) && e.Status == http.StatusBadRequest {
				return p, fmt.Errorf("定时不对：%s", e.Message)
			}
			if searching {
				p.Impact = append(p.Impact, unreadable("接下来几次搜索的时间", err))
			}
		} else if len(next.Next) > 0 && searching {
			texts := make([]string, 0, len(next.Next))
			for _, sec := range next.Next {
				texts = append(texts, timeText(sec))
			}
			p.Impact = append(p.Impact, "接下来几次搜索："+strings.Join(texts, "，")+"（前后有几分钟的随机偏移）")
		}
	}
	if in.Sites != nil || in.DownloaderID != nil || in.Category != nil || in.Tags != nil || in.SavePath != nil || in.ProfileID != nil {
		p.Impact = append(p.Impact, "之后的搜索与推送按新的设置；已经推送的种子不动")
	}
	if in.LibraryID != nil {
		p.Impact = append(p.Impact, "之后下载的整理进这个媒体库；已经整理好的文件不动")
	}
	if in.Upgrade != nil && *in.Upgrade && !sub.Upgrade {
		p.Impact = append(p.Impact, "开着洗版：下完以后还会找更好的版本，找到就替换媒体库里的文件")
	}
	return p, nil
}

// notSearchingNote 是不在找的订阅改了设置以后的说明。
func notSearchingNote(status string) string {
	switch status {
	case "done":
		return "这个订阅已经完成，不会再搜索：改的设置在它重新在找以后才用上（TMDB 上这一季又有了新的集时会自动改回在找）"
	case "paused":
		return "订阅暂停着：恢复以后才按新的设置搜索"
	case "pending":
		return "订阅还没开始找（等上映或等确认）：开始找以后按新的设置搜索"
	}
	return "订阅现在不在找（" + subStatusName(status) + "）：开始找以后按新的设置搜索"
}

var episodeStateNames = map[string]string{
	"library": "已入库", "downloading": "下载中", "missing": "缺", "waiting_version": "等锁住的版本",
	"upcoming": "还没播", "skipped": "不追",
}

func episodeStateName(s string) string {
	if n := episodeStateNames[s]; n != "" {
		return n
	}
	return s
}

// episodePreview 是跳过一集、换版本的预览：那一集现在的情况与改了以后。
func episodePreview(ctx context.Context, d *Deps, c Caller, tool string, id uint, n int, on bool) (Preview, error) {
	p := Preview{Tool: tool}
	sub, err := readFollow(ctx, d, c, id)
	if err != nil {
		if strings.HasPrefix(err.Error(), "没有编号") {
			return p, err
		}
		p.Summary = fmt.Sprintf("订阅 %d 的第 %d 集", id, n)
		p.Impact = append(p.Impact, unreadable("这个订阅", err))
		return p, nil
	}
	if sub.MediaType != "tv" {
		return p, errors.New("电影没有分集")
	}
	idx := slices.IndexFunc(sub.Episodes, func(e episodeNow) bool { return e.Number == n })
	if idx < 0 {
		return p, fmt.Errorf("这一季没有第 %d 集（共 %d 集）：用 get_subscription 看每一集", n, len(sub.Episodes))
	}
	ep := sub.Episodes[idx]
	label := fmt.Sprintf("第 %d 集", n)
	if ep.Name != "" {
		label += "「" + ep.Name + "」"
	}
	state := episodeStateName(ep.State)
	switch tool {
	case "set_episode_skip":
		if on {
			p.Summary = "跳过" + sub.label() + "的" + label
			p.Changes = []Change{{Label: label, Before: state, After: "不追（跳过）"}}
			p.Impact = append(p.Impact, "不再找这一集，也不算缺；其余的集追完就算追完")
			if ep.State == "library" || ep.State == "downloading" {
				p.Impact = append(p.Impact, "已经下载、入库的文件不动")
			}
		} else {
			p.Summary = "重新要" + sub.label() + "的" + label
			p.Changes = []Change{{Label: label, Before: state, After: "要（接着找）"}}
			p.Impact = append(p.Impact, "这一集重新算进要追的集，之后的搜索会找它")
		}
	default: // unlock_episode
		lock := lockLabel(sub.LockGroup, sub.LockResolution)
		if on {
			p.Summary = sub.label() + "的" + label + "换版本"
			p.Changes = []Change{{Label: label + "的版本", Before: "只要锁住的版本（" + lock + "）", After: "不受版本锁限制"}}
			p.Impact = append(p.Impact, "马上搜一次这一集，按质量档案挑别的版本推送到下载器；其余的集照样受版本锁限制")
		} else {
			p.Summary = sub.label() + "的" + label + "恢复版本锁"
			p.Changes = []Change{{Label: label + "的版本", Before: "不受版本锁限制", After: "只要锁住的版本（" + lock + "）"}}
		}
	}
	return p, nil
}

type episodeRuleNow struct {
	Group  string `json:"group"`
	Mode   string `json:"mode"`
	Offset int    `json:"offset"`
	Note   string `json:"note"`
}

func ruleLabel(mode string, offset int) string {
	switch mode {
	case "tmdb":
		return "按 TMDB 的集号"
	case "period_part":
		return "按第几期与上下"
	case "date":
		return "按播出日期"
	case "offset":
		return fmt.Sprintf("集号偏移 %+d", offset)
	default:
		return mode
	}
}

func groupLabel(group string) string {
	if group == "" {
		return "所有制作组"
	}
	return "制作组 " + group
}

// readRules 读订阅的集号规则。
func readRules(ctx context.Context, d *Deps, c Caller, id uint) ([]episodeRuleNow, error) {
	var rules []episodeRuleNow
	if err := readJSON(ctx, d, c, http.MethodGet, subPath(id, "/episode-rules"), nil, &rules); err != nil {
		if notFound(err) {
			return nil, fmt.Errorf("没有编号 %d 的订阅：先用 list_subscriptions 查一下", id)
		}
		return nil, err
	}
	return rules, nil
}

func episodeRulePreview(ctx context.Context, d *Deps, c Caller, in episodeRuleIn) (Preview, error) {
	group := strings.TrimSpace(in.Group)
	p := Preview{Tool: "save_episode_rule", Summary: fmt.Sprintf("给订阅 %d 的%s设集号规则", in.ID, groupLabel(group))}
	if in.Mode == "offset" && in.Offset == 0 {
		return p, errors.New("offset 方式要给不是 0 的 offset")
	}
	after := ruleLabel(in.Mode, in.Offset)
	rules, err := readRules(ctx, d, c, in.ID)
	if err != nil {
		if strings.HasPrefix(err.Error(), "没有编号") {
			return p, err
		}
		p.Changes = []Change{{Label: groupLabel(group), After: after}}
		p.Impact = append(p.Impact, unreadable("现在的集号规则", err))
		return p, nil
	}
	before := ""
	for _, r := range rules {
		if r.Group == group {
			before = ruleLabel(r.Mode, r.Offset)
		}
	}
	p.Changes = []Change{{Label: groupLabel(group) + "的集号", Before: before, After: after}}
	p.Impact = append(p.Impact, "这个组的种子标题里的集号按这条规则对到 TMDB 的集；保存以后尽快按它搜索一次")
	return p, nil
}

func episodeRuleDeletePreview(ctx context.Context, d *Deps, c Caller, in episodeRuleDeleteIn) (Preview, error) {
	group := strings.TrimSpace(in.Group)
	p := Preview{Tool: "delete_episode_rule", Summary: fmt.Sprintf("删除订阅 %d 的%s的集号规则", in.ID, groupLabel(group))}
	rules, err := readRules(ctx, d, c, in.ID)
	if err != nil {
		if strings.HasPrefix(err.Error(), "没有编号") {
			return p, err
		}
		p.Impact = append(p.Impact, unreadable("现在的集号规则", err))
		return p, nil
	}
	for _, r := range rules {
		if r.Group == group {
			p.Changes = []Change{{Label: groupLabel(group) + "的集号", Before: ruleLabel(r.Mode, r.Offset), After: "删除"}}
			p.Impact = append(p.Impact, "之后这个组的集号按 pt-tools 自己的识别（综艺按期号、播出日期换算）；拿不准的不要")
			return p, nil
		}
	}
	return p, fmt.Errorf("这个订阅没有%s的集号规则：用 get_episode_rules 看有哪些", groupLabel(group))
}

// subTorrentPreview 是订阅的种子重新整理、重新刮削的预览：种子里每个文件现在怎样。
func subTorrentPreview(ctx context.Context, d *Deps, c Caller, tool string, id, tid uint) (Preview, error) {
	p := Preview{Tool: tool}
	sub, err := readFollow(ctx, d, c, id)
	if err != nil {
		if strings.HasPrefix(err.Error(), "没有编号") {
			return p, err
		}
		p.Summary = fmt.Sprintf("订阅 %d 的种子 %d", id, tid)
		p.Impact = append(p.Impact, unreadable("这个订阅", err))
		return p, nil
	}
	idx := slices.IndexFunc(sub.Torrents, func(t subTorrentNow) bool { return t.ID == tid })
	if idx < 0 {
		return p, fmt.Errorf("这个订阅没有编号 %d 的种子：subscription_torrent_id 用 get_subscription 的 torrents 里的 id", tid)
	}
	t := sub.Torrents[idx]
	if t.Status == "replaced" {
		return p, errors.New("这是洗版换掉的旧版本：不再整理、刮削")
	}
	for i, f := range t.Files {
		if i == 20 {
			p.Changes = append(p.Changes, Change{Label: "…", After: fmt.Sprintf("还有 %d 个文件", len(t.Files)-20)})
			break
		}
		before := historyStatusName(f.Status)
		if f.Retrying {
			before += "（正在自动重试）"
		}
		if f.Message != "" {
			before += "：" + oneLine(f.Message, 80)
		}
		after := "不动"
		switch {
		case tool == "reorganize_subscription_torrent" && f.Status != "done":
			after = "按订阅的条目重新整理"
		case tool == "rescrape_subscription_torrent" && f.Status == "done":
			after = "重写 NFO 与图片"
		}
		p.Changes = append(p.Changes, Change{Label: f.Name, Before: before, After: after})
	}
	if tool == "reorganize_subscription_torrent" {
		p.Summary = fmt.Sprintf("重新整理%s下载的「%s」", sub.label(), t.Title)
		if len(t.Files) == 0 {
			p.Changes = append(p.Changes, Change{Label: t.Title, Before: "还没有整理记录", After: "按订阅的条目整理"})
		}
		if !t.CanOrganize {
			p.Impact = append(p.Impact, "这个种子没有整理没成的文件（都整理好了，或者在自动重试）：多半不会有变化")
		}
		p.Impact = append(p.Impact, "按订阅的条目与媒体库整理（不靠识别），已经整理好的文件不动；目标位置已经有别的文件时不覆盖")
		p.Impact = append(p.Impact, "标成失败的种子改回等整理，整理成了马上标成已入库；运行记录里记一条")
		return p, nil
	}
	if !t.CanRescrape {
		return p, errors.New("这个种子还没有整理好的文件：先用 reorganize_subscription_torrent 重新整理")
	}
	p.Summary = fmt.Sprintf("重新刮削%s（「%s」整理进的那部）", sub.label(), t.Title)
	p.Impact = append(p.Impact, rescrapeImpact(sub.MediaType)...)
	p.Impact = append(p.Impact, "运行记录里记一条")
	return p, nil
}

var historyStatusNames = map[string]string{"done": "已整理", "failed": "失败", "skipped": "跳过", "removed": "已删除"}

func historyStatusName(s string) string {
	if n := historyStatusNames[s]; n != "" {
		return n
	}
	return s
}

// rescrapeImpact 是重新刮削的影响：mediaType 是 tv、movie，不知道时为空。
func rescrapeImpact(mediaType string) []string {
	what := "这条记录所在的电影或剧集"
	switch mediaType {
	case "tv":
		what = "这部剧在这个媒体库里整理好的每一集"
	case "movie":
		what = "这部电影"
	}
	return []string{
		"按媒体库现在的刮削设置重写" + what + "的 NFO 与图片：pt-tools 写的、之后没被改过的文件换成新的，自己放的文件只在媒体库开着覆盖时换",
		"媒体服务器里锁定了的字段不改；会访问 TMDB 等刮削来源",
	}
}

// rescrapePreview 是重新刮削一条整理记录的预览。
func rescrapePreview(ctx context.Context, d *Deps, c Caller, id uint) (Preview, error) {
	p := Preview{Tool: "rescrape_media", Summary: fmt.Sprintf("重新刮削记录 %d", id)}
	var page struct {
		Items []struct {
			ID        uint   `json:"id"`
			MediaType string `json:"media_type"`
			Title     string `json:"title"`
			Library   string `json:"library"`
			Status    string `json:"status"`
		} `json:"items"`
	}
	if err := readJSON(ctx, d, c, http.MethodGet, withQuery("/media/history", "page_size", "100"), nil, &page); err != nil {
		p.Impact = append(p.Impact, unreadable("整理记录", err))
		return p, nil
	}
	for _, it := range page.Items {
		if it.ID != id {
			continue
		}
		if it.Status != "done" {
			return p, fmt.Errorf("这条记录是%s的：只有整理好的记录能重新刮削（失败的先用 retry_media_organize）", historyStatusName(it.Status))
		}
		p.Summary = fmt.Sprintf("重新刮削「%s」", it.Title)
		lib := textOr(it.Library, "它所在的媒体库")
		p.Changes = []Change{{Label: "刮削", Before: "媒体库「" + lib + "」里现有的 NFO 与图片", After: "按现在的刮削设置重写"}}
		p.Impact = append(p.Impact, rescrapeImpact(it.MediaType)...)
		return p, nil
	}
	p.Impact = append(p.Impact, "最近 100 条整理记录里没有这一条：确认以后照样会按编号重新刮削")
	p.Impact = append(p.Impact, rescrapeImpact("")...)
	return p, nil
}

// followSkillGuide 是追剧技能的全文。
const followSkillGuide = `# pt-tools 追剧技能

订阅一部电影或追一部剧的一季以后，pt-tools 按定时在各站点搜索（RSS 里出现时也下载），按质量档案挑出最合适的种子推送到下载器，
下载完成后整理进媒体库。剧集是追剧：一集一集补齐，默认按每一集的播出时间去搜，第一集定下版本（制作组与分辨率）以后只要同一个版本。
读工具随便用；写工具先不带 confirm 拿预览给用户看，用户同意以后带 confirm=true 再调一次才执行。

## 建订阅

1. explore_media（kind=tv，query=剧名）找到 TMDB 编号；已经订阅了的条目会写明。
2. add_subscription：media_type=tv、tmdb_id、season（不填是最新一季）。常用的设置：
   - schedule：什么时候搜，见下面「定时」；不填按播出时间。
   - lock_version：版本锁，默认开；不想锁版本时给 false。
   - start_episode：从第几集开始追（之前的不要）。
   - from_torrent：用户指定了站点上的一个种子（search_torrents 的 site 与 torrent_id）时从它开始追：建好马上推送它，起始集取它的集号。
   - sites、downloader_id、category、tags、save_path、profile_id、library_id：在哪些站点找、推到哪里、整理进哪个媒体库；不填用订阅设置里的默认。
3. get_subscription 看每一集的情况：library（已入库）、downloading、missing（缺）、waiting_version（在等锁住的版本）、upcoming（还没播）、skipped（不追）。

## 定时（schedule）

- {"kind":"airtime"}：按每一集的播出时间（只用于剧集）。播出后 air_delay_hours 小时开始搜（默认 2），没找到隔 air_retry_hours 小时再搜（默认 6），
  播出后 air_window_days 天（默认 7）以后每天搜一次。at 是播出的时刻（TMDB 只有日期，默认 20:00）。
- {"kind":"interval","interval_minutes":180}：每隔多少分钟（30 到 10080），从上次搜索算起。
- {"kind":"daily","at":"21:30"}：每天几点。{"kind":"every_days","every":3,"at":"08:00"}：每隔几天。
- {"kind":"weekly","days":[5,6],"at":"22:00"}：每周几（0 是星期日）。{"kind":"monthly","days":[1,15]}：每月几号（小月按月末）。
- {"kind":"cron","cron":"0 22 * * 5; 0 9,13 * * 6"}：cron 表达式，5 段（分 时 日 月 星期），几条用分号分开，取最早到点的。
  例子：每周五 22:00 是 0 22 * * 5；每天 9 点与 21 点是 0 9,21 * * *；工作日 8:30 是 30 8 * * 1-5；每月 1 号是 0 9 1 * *。
- {"kind":""}：用订阅设置里的全局搜索间隔。

综艺多是固定星期更新：按播出时间，或者 weekly / cron 定在更新当晚与第二天上午。改之前用 preview_schedule 看接下来几次什么时候搜
（next_text 是 pt-tools 所在时区的时间）；表达式写错时它会写明哪一段不对。定时到点的搜索前后有几分钟的随机偏移，两次至少隔 30 分钟。

## 改设置

update_subscription 只改给了的字段：schedule、lock_version、lock_group 与 lock_resolution（两个都给空串是清掉锁住的版本，按下一个下载的重新定）、
start_episode、follow_next_season、library_id、total_episodes（TMDB 的集数不对时）、sites、downloader_id、category、tags、save_path、profile_id、upgrade。
改了定时、版本锁、起始集或集数时，pt-tools 马上重新算每一集、尽快搜一次。

## 每一集

- set_episode_skip：跳过一集（不追：不算缺、不影响追完），skip=false 重新要它。
- unlock_episode：锁住的版本久等不到时，unlock=true 让这一集换别的版本（pt-tools 马上搜一次）；其余的集照样受版本锁限制。

## 没下载、下错了

1. list_subscription_runs 看每次搜索的经过：在哪些站点搜到几个、哪些种子为什么没要（不合质量档案、不是锁住的版本、集号对不上……）、推送了哪个、哪个失败了。
2. 写着「集号对不上」的多是综艺：get_episode_rules 看现有的规则，save_episode_rule 给那个制作组设一条：
   mode=period_part（按第几期与上下，比如「第 1 期下」是 TMDB 的第 2 集）、date（按播出日期）、offset（TMDB 的集号 = 标题里的集号 + offset）或 tmdb。
   group 不填的对所有组；设了以后尽快按它搜一次。delete_episode_rule 删掉一条。
3. 推送失败（站点超时、种子文件下载失败）的种子没有记下：search_subscription 立即重新搜一次就会再推。暂停的订阅先 set_subscription_status 恢复。
4. 下载了但没入库：get_subscription 的 torrents 里每个文件写明整理得怎样；改好原因（路径映射、媒体库）以后用 reorganize_subscription_torrent
   重新整理那个种子（subscription_torrent_id 是 torrents 里的 id，整季包与单集一样）。整理好了但刮削不全的用 rescrape_subscription_torrent；
   整理历史里的单条记录用 retry_media_organize（重新整理）与 rescrape_media（重新刮削）。

## 全局设置

订阅的总开关、全局搜索间隔、默认质量档案与下载器、通知通道，以及质量档案、豆瓣想看，经 get_config / change_config 的 subscribe、
quality_profiles、douban_sources 改（令牌要有 app:config，先看 get_skill pt-tools-config）。订阅的总开关关着时不会主动搜索。
`
