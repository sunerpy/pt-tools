package web

import (
	"net/http"
	"strconv"

	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/models"
)

// App API v1 的追剧：修改订阅的设置、每一集、跳过与换版本、运行记录、集号规则。

// AppSubscriptionUpdate 是 PUT /subscriptions/{id}：只改给了的字段（没给的不变）。条目与季不能改。
type AppSubscriptionUpdate struct {
	ProfileID        *uint        `json:"profile_id,omitempty"`
	Upgrade          *bool        `json:"upgrade,omitempty"`
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
