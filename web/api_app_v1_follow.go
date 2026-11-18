package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/models"
)

// App API v1 的追剧：修改订阅的设置、每一集、跳过与换版本、运行记录、集号规则。

// AppSubscriptionUpdate 是 PUT /subscriptions/{id}：只改给了的字段（没给的不变）。条目与季不能改。
type AppSubscriptionUpdate struct {
	IncludeKeywords *string `json:"include_keywords,omitempty"`
	ExcludeKeywords *string `json:"exclude_keywords,omitempty"`
	ProfileID       *uint   `json:"profile_id,omitempty"`
	Upgrade         *bool   `json:"upgrade,omitempty"`
	// Sites（空数组是全部站点）、DownloaderID（0 用设置里的）、Category、Tags、SavePath 是在哪里找、推到哪里
	Sites            *[]string    `json:"sites,omitempty"`
	DownloaderID     *uint        `json:"downloader_id,omitempty"`
	Category         *string      `json:"category,omitempty"`
	Tags             *string      `json:"tags,omitempty"`
	SavePath         *string      `json:"save_path,omitempty"`
	Schedule         *AppSchedule `json:"schedule,omitempty"`
	LockVersion      *bool        `json:"lock_version,omitempty"`
	LockGroup        *string      `json:"lock_group,omitempty"`
	LockResolution   *string      `json:"lock_resolution,omitempty"`
	StartEpisode     *int         `json:"start_episode,omitempty"`
	FollowNextSeason *bool        `json:"follow_next_season,omitempty"`
	LibraryID        *uint        `json:"library_id,omitempty"`
	// TotalEpisodes 大于 0 时以它为这一季的集数，0 改回按 TMDB
	TotalEpisodes *int `json:"total_episodes,omitempty"`
}

func (s *Server) appSubscriptionUpdate(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	id, ok := appID(w, r)
	if !ok {
		return
	}
	var in AppSubscriptionUpdate
	if !appDecode(w, r, &in) {
		return
	}
	cur, err := svc.Subscription(r.Context(), id)
	if err != nil {
		appSubscribeError(w, err)
		return
	}
	// 修改接口按整份设置保存：没给的字段用现在的
	input := subscribe.SubscriptionInput{
		IncludeKeywords: in.IncludeKeywords, ExcludeKeywords: in.ExcludeKeywords,
		ProfileID: cur.ProfileID, Sites: cur.Sites, DownloaderID: cur.DownloaderID, Category: cur.Category, Tags: cur.Tags,
		SavePath: cur.SavePath, Upgrade: cur.Upgrade,
		LockVersion: in.LockVersion, LockGroup: in.LockGroup, LockResolution: in.LockResolution, StartEpisode: in.StartEpisode,
		FollowNextSeason: in.FollowNextSeason, LibraryID: in.LibraryID, TotalEpisodes: in.TotalEpisodes,
	}
	if in.ProfileID != nil {
		input.ProfileID = *in.ProfileID
	}
	if in.Upgrade != nil {
		input.Upgrade = *in.Upgrade
	}
	if in.Sites != nil {
		if bad := svc.UnknownSites(*in.Sites); len(bad) > 0 {
			appError(w, http.StatusBadRequest, "invalid_argument", "这些站点没有配置或没启用："+strings.Join(bad, "、"))
			return
		}
		input.Sites = *in.Sites
	}
	if in.DownloaderID != nil {
		if !appCheckDownloader(w, r, svc, *in.DownloaderID) {
			return
		}
		input.DownloaderID = *in.DownloaderID
	}
	if in.Category != nil {
		input.Category = *in.Category
	}
	if in.Tags != nil {
		input.Tags = *in.Tags
	}
	if in.SavePath != nil {
		input.SavePath = *in.SavePath
	}
	if in.Schedule != nil {
		input.Schedule = in.Schedule.schedule()
	}
	sub, err := svc.UpdateSubscription(r.Context(), id, input)
	if err != nil {
		appSubscribeError(w, err)
		return
	}
	appJSON(w, s.appSubscriptionView(r, svc, sub))
}

// appCheckDownloader 查给的下载器编号存在（0 是用设置里的）；不存在时回 400。
func appCheckDownloader(w http.ResponseWriter, r *http.Request, svc *subscribe.Service, id uint) bool {
	bad, err := svc.UnknownDownloader(r.Context(), id)
	if err != nil {
		appError(w, http.StatusInternalServerError, "internal", "读取下载器失败")
		return false
	}
	if bad {
		appError(w, http.StatusBadRequest, "invalid_argument", "下载器不存在："+strconv.Itoa(int(id)))
		return false
	}
	return true
}

func appEpisodes(eps []subscribe.EpisodeState) []AppEpisode {
	out := make([]AppEpisode, 0, len(eps))
	for _, e := range eps {
		out = append(out, appEpisode(e))
	}
	return out
}

func (s *Server) appSubscriptionEpisodes(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	id, ok := appID(w, r)
	if !ok {
		return
	}
	eps, err := svc.Episodes(r.Context(), id)
	if err != nil {
		appSubscribeError(w, err)
		return
	}
	appJSON(w, appEpisodes(eps))
}

// appEpisodeNumber 读路径里的集号。
func appEpisodeNumber(w http.ResponseWriter, r *http.Request) (int, bool) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 1 {
		appError(w, http.StatusBadRequest, "invalid_argument", "集号要是正整数")
		return 0, false
	}
	return n, true
}

// AppEpisodeSkip 是 POST /subscriptions/{id}/episodes/{n}/skip：true 跳过这一集，false 重新要。
type AppEpisodeSkip struct {
	Skip *bool `json:"skip"`
}

// AppEpisodeUnlock 是 POST /subscriptions/{id}/episodes/{n}/unlock：true 这一集不受版本锁限制（并马上搜一次），false 恢复。
type AppEpisodeUnlock struct {
	Unlock *bool `json:"unlock"`
}

func (s *Server) appSubscriptionEpisodeSkip(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	id, ok := appID(w, r)
	if !ok {
		return
	}
	n, ok := appEpisodeNumber(w, r)
	if !ok {
		return
	}
	var in AppEpisodeSkip
	if !appDecode(w, r, &in) {
		return
	}
	if in.Skip == nil {
		appError(w, http.StatusBadRequest, "invalid_argument", "要给 skip")
		return
	}
	appSetAuditArgs(r, map[string]any{"episode": n, "skip": *in.Skip})
	eps, err := svc.SkipEpisode(r.Context(), id, n, *in.Skip)
	if err != nil {
		appSubscribeError(w, err)
		return
	}
	appJSON(w, appEpisodes(eps))
}

func (s *Server) appSubscriptionEpisodeUnlock(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	id, ok := appID(w, r)
	if !ok {
		return
	}
	n, ok := appEpisodeNumber(w, r)
	if !ok {
		return
	}
	var in AppEpisodeUnlock
	if !appDecode(w, r, &in) {
		return
	}
	if in.Unlock == nil {
		appError(w, http.StatusBadRequest, "invalid_argument", "要给 unlock")
		return
	}
	appSetAuditArgs(r, map[string]any{"episode": n, "unlock": *in.Unlock})
	eps, err := svc.UnlockEpisode(r.Context(), id, n, *in.Unlock)
	if err != nil {
		appSubscribeError(w, err)
		return
	}
	appJSON(w, appEpisodes(eps))
}

// AppRunStep 是一次运行里的一步：search、reject、lock、push、fail、adopt、wait 或 info。
type AppRunStep struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// AppSubscriptionRun 是订阅的一次运行（主动搜索、RSS 命中、建立、手动操作）。
type AppSubscriptionRun struct {
	ID             uint         `json:"id"`
	Trigger        string       `json:"trigger"`
	StartedAt      int64        `json:"started_at"`
	FinishedAt     *int64       `json:"finished_at,omitempty"`
	Status         string       `json:"status"`
	Summary        string       `json:"summary"`
	Steps          []AppRunStep `json:"steps"`
	StepsTruncated bool         `json:"steps_truncated"`
}

func (s *Server) appSubscriptionRuns(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	id, ok := appID(w, r)
	if !ok {
		return
	}
	page, size, ok := appPaging(r, 100)
	if !ok {
		appError(w, http.StatusBadRequest, "invalid_argument", "page 从 1 开始，page_size 在 1 到 100 之间")
		return
	}
	res, err := svc.Runs(r.Context(), id, size, (page-1)*size)
	if err != nil {
		appSubscribeError(w, err)
		return
	}
	out := AppPage[AppSubscriptionRun]{Items: make([]AppSubscriptionRun, 0, len(res.Items)), Total: int(res.Total), Page: page, PageSize: size}
	for _, run := range res.Items {
		v := AppSubscriptionRun{
			ID: run.ID, Trigger: run.Trigger, StartedAt: run.StartedAt.Unix(), FinishedAt: unixPtr(run.FinishedAt), Status: run.Status,
			Summary: appRedactAddr(run.Summary), Steps: make([]AppRunStep, 0, len(run.Steps)), StepsTruncated: run.StepsTruncated,
		}
		for _, st := range run.Steps {
			v.Steps = append(v.Steps, AppRunStep{Kind: st.Kind, Text: appRedactAddr(st.Text)})
		}
		out.Items = append(out.Items, v)
	}
	appJSON(w, out)
}

// AppEpisodeRule 是追剧对一个制作组（空 = 所有组）的集号规则：mode 是 tmdb、period_part（按期号与上下）、date（按播出日期）
// 或 offset（TMDB 的集号 = 标题里的集号 + offset）。
type AppEpisodeRule struct {
	Group     string `json:"group"`
	Mode      string `json:"mode"`
	Offset    int    `json:"offset"`
	Source    string `json:"source"`
	Note      string `json:"note,omitempty"`
	UpdatedAt int64  `json:"updated_at"`
}

// AppEpisodeRuleInput 是 PUT /subscriptions/{id}/episode-rules：同一个制作组已有规则时替换。
type AppEpisodeRuleInput struct {
	Group  string `json:"group"`
	Mode   string `json:"mode"`
	Offset int    `json:"offset"`
	Note   string `json:"note"`
}

func appEpisodeRules(rows []models.MediaEpisodeRule) []AppEpisodeRule {
	out := make([]AppEpisodeRule, 0, len(rows))
	for _, r := range rows {
		out = append(out, AppEpisodeRule{Group: r.Group, Mode: r.Mode, Offset: r.Offset, Source: r.Source, Note: r.Note, UpdatedAt: r.UpdatedAt.Unix()})
	}
	return out
}

func (s *Server) appEpisodeRules(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	id, ok := appID(w, r)
	if !ok {
		return
	}
	rows, err := svc.EpisodeRules(r.Context(), id)
	if err != nil {
		appSubscribeError(w, err)
		return
	}
	appJSON(w, appEpisodeRules(rows))
}

func (s *Server) appEpisodeRuleSave(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	id, ok := appID(w, r)
	if !ok {
		return
	}
	var in AppEpisodeRuleInput
	if !appDecode(w, r, &in) {
		return
	}
	appSetAuditArgs(r, map[string]any{"group": in.Group, "mode": in.Mode})
	rows, err := svc.SaveEpisodeRule(r.Context(), id, subscribe.EpisodeRuleInput{Group: in.Group, Mode: in.Mode, Offset: in.Offset, Note: in.Note})
	if err != nil {
		appSubscribeError(w, err)
		return
	}
	appJSON(w, appEpisodeRules(rows))
}

func (s *Server) appEpisodeRuleDelete(w http.ResponseWriter, r *http.Request) {
	if !appNoBody(w, r) {
		return
	}
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	id, ok := appID(w, r)
	if !ok {
		return
	}
	group := r.URL.Query().Get("group")
	appSetAuditArgs(r, map[string]any{"group": group})
	rows, err := svc.DeleteEpisodeRule(r.Context(), id, group)
	if err != nil {
		appSubscribeError(w, err)
		return
	}
	appJSON(w, appEpisodeRules(rows))
}

// AppSchedulePreviewInput 是 POST /subscriptions/schedule/preview：看一个定时接下来几次什么时候搜（只读，不改订阅）。
// subscription_id 给了时按那个订阅的类型与建立时间算（每隔几天从建立那天对齐）；count 是几次（1–10，默认 5）。
type AppSchedulePreviewInput struct {
	Schedule       AppSchedule `json:"schedule"`
	MediaType      string      `json:"media_type,omitempty"`
	SubscriptionID uint        `json:"subscription_id,omitempty"`
	Count          int         `json:"count,omitempty"`
}

// AppSchedulePreview 是预览：接下来几次搜索的时间（Unix 秒，不算随机偏移）；按播出时间的不算，note 说明。
type AppSchedulePreview struct {
	Next []int64 `json:"next"`
	Note string  `json:"note,omitempty"`
}

func (s *Server) appSchedulePreview(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	var in AppSchedulePreviewInput
	if !appDecode(w, r, &in) {
		return
	}
	if in.Count < 0 || in.Count > subscribe.PreviewMax {
		appError(w, http.StatusBadRequest, "invalid_argument", "count 要在 1 到 10 之间")
		return
	}
	got, err := schedulePreview(r.Context(), svc, SchedulePreviewInput{
		Schedule: *in.Schedule.schedule(), MediaType: in.MediaType, SubscriptionID: in.SubscriptionID, Count: in.Count,
	})
	if err != nil {
		appSubscribeError(w, err)
		return
	}
	out := AppSchedulePreview{Next: make([]int64, 0, len(got.Next)), Note: got.Note}
	for _, t := range got.Next {
		out.Next = append(out.Next, t.Unix())
	}
	appJSON(w, out)
}

// appSubscriptionTorrentAction 是订阅种子的重新整理、重新刮削：读路径里的订阅与种子编号。
func (s *Server) appSubscriptionTorrentIDs(w http.ResponseWriter, r *http.Request) (*subscribe.Service, uint, uint, bool) {
	if !appNoBody(w, r) {
		return nil, 0, 0, false
	}
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return nil, 0, 0, false
	}
	id, ok := appID(w, r)
	if !ok {
		return nil, 0, 0, false
	}
	tid, err := strconv.ParseUint(r.PathValue("tid"), 10, 64)
	if err != nil || tid == 0 {
		appError(w, http.StatusBadRequest, "invalid_argument", "种子编号要是正整数（订阅详情 torrents 里的 id）")
		return nil, 0, 0, false
	}
	appSetAuditArgs(r, map[string]any{"torrent": tid})
	return svc, id, uint(tid), true
}

// appSubscribeOrganizeError 是订阅里整理、刮削的错误：订阅的按订阅的，整理入库回的按整理的。
func appSubscribeOrganizeError(w http.ResponseWriter, err error) {
	if errors.Is(err, subscribe.ErrInvalid) || errors.Is(err, subscribe.ErrNotFound) {
		appSubscribeError(w, err)
		return
	}
	appOrganizeError(w, err)
}

// appSubscriptionTorrentOrganize 重新整理订阅下载的一个种子（整季包与单集一样）：按订阅的条目与媒体库，已经整理好的文件不动。
func (s *Server) appSubscriptionTorrentOrganize(w http.ResponseWriter, r *http.Request) {
	svc, id, tid, ok := s.appSubscriptionTorrentIDs(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), organizeAPITimeout)
	defer cancel()
	res, err := svc.OrganizeTorrent(ctx, id, tid)
	if err != nil {
		appSubscribeOrganizeError(w, err)
		return
	}
	out := appResultOf(res)
	if out.Failed > 0 && out.Created == 0 && out.Done == 0 {
		appSetOutcome(r, "error:organize_failed")
	}
	appJSON(w, out)
}

// appSubscriptionTorrentScrape 重新刮削订阅下载的一个种子整理进的电影或剧集。
func (s *Server) appSubscriptionTorrentScrape(w http.ResponseWriter, r *http.Request) {
	svc, id, tid, ok := s.appSubscriptionTorrentIDs(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), organizeAPITimeout)
	defer cancel()
	res, err := svc.RescrapeTorrent(ctx, id, tid)
	if err != nil {
		appSubscribeOrganizeError(w, err)
		return
	}
	appJSON(w, appRescrapeOf(res))
}
