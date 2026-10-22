package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// 订阅接口：订阅设置、质量档案、订阅、豆瓣想看来源、探索。全部只认 session（s.auth），请求体严格解析。

// subscribeSearchTimeout 是「立即搜索」的时限（要在各站点搜索、下载种子文件并推送）。
const subscribeSearchTimeout = 3 * time.Minute

// SetSubscribeService 接上订阅服务（cmd 里建好后调用）。
func (s *Server) SetSubscribeService(svc *subscribe.Service) { s.subscriber = svc }

func (s *Server) registerSubscribeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/media/subscribe/settings", s.auth(s.apiSubscribeSettingsGet))
	mux.HandleFunc("PUT /api/media/subscribe/settings", s.auth(s.apiSubscribeSettingsPut))
	mux.HandleFunc("GET /api/media/quality-profiles", s.auth(s.apiQualityProfiles))
	mux.HandleFunc("POST /api/media/quality-profiles", s.auth(s.apiQualityProfileSave))
	mux.HandleFunc("PUT /api/media/quality-profiles/{id}", s.auth(s.apiQualityProfileSave))
	mux.HandleFunc("DELETE /api/media/quality-profiles/{id}", s.auth(s.apiQualityProfileDelete))
	mux.HandleFunc("GET /api/media/subscriptions", s.auth(s.apiSubscriptions))
	mux.HandleFunc("POST /api/media/subscriptions", s.auth(s.apiSubscriptionCreate))
	mux.HandleFunc("POST /api/media/subscriptions/schedule-preview", s.auth(s.apiSchedulePreview))
	mux.HandleFunc("GET /api/media/subscriptions/{id}", s.auth(s.apiSubscriptionGet))
	mux.HandleFunc("PUT /api/media/subscriptions/{id}", s.auth(s.apiSubscriptionUpdate))
	mux.HandleFunc("DELETE /api/media/subscriptions/{id}", s.auth(s.apiSubscriptionDelete))
	mux.HandleFunc("POST /api/media/subscriptions/{id}/status", s.auth(s.apiSubscriptionStatus))
	mux.HandleFunc("POST /api/media/subscriptions/{id}/search", s.auth(s.apiSubscriptionSearch))
	mux.HandleFunc("POST /api/media/subscriptions/{id}/torrents/{tid}/organize", s.auth(s.apiSubscriptionTorrentOrganize))
	mux.HandleFunc("POST /api/media/subscriptions/{id}/torrents/{tid}/scrape", s.auth(s.apiSubscriptionTorrentScrape))
	mux.HandleFunc("GET /api/media/subscriptions/{id}/episodes", s.auth(s.apiSubscriptionEpisodes))
	mux.HandleFunc("POST /api/media/subscriptions/{id}/episodes/{n}/skip", s.auth(s.apiSubscriptionEpisodeSkip))
	mux.HandleFunc("POST /api/media/subscriptions/{id}/episodes/{n}/unlock", s.auth(s.apiSubscriptionEpisodeUnlock))
	mux.HandleFunc("GET /api/media/subscriptions/{id}/runs", s.auth(s.apiSubscriptionRuns))
	mux.HandleFunc("GET /api/media/subscriptions/{id}/episode-rules", s.auth(s.apiEpisodeRules))
	mux.HandleFunc("PUT /api/media/subscriptions/{id}/episode-rules", s.auth(s.apiEpisodeRuleSave))
	mux.HandleFunc("DELETE /api/media/subscriptions/{id}/episode-rules", s.auth(s.apiEpisodeRuleDelete))
	mux.HandleFunc("GET /api/media/douban-sources", s.auth(s.apiDoubanSources))
	mux.HandleFunc("POST /api/media/douban-sources", s.auth(s.apiDoubanSourceSave))
	mux.HandleFunc("PUT /api/media/douban-sources/{id}", s.auth(s.apiDoubanSourceSave))
	mux.HandleFunc("DELETE /api/media/douban-sources/{id}", s.auth(s.apiDoubanSourceDelete))
	mux.HandleFunc("POST /api/media/douban-sources/{id}/fetch", s.auth(s.apiDoubanSourceFetch))
	mux.HandleFunc("GET /api/media/douban-sources/{id}/items", s.auth(s.apiDoubanSourceItems))
	mux.HandleFunc("GET /api/media/explore", s.auth(s.apiMediaExplore))
}

func (s *Server) subscribeService(w http.ResponseWriter) (*subscribe.Service, bool) {
	if s != nil && s.subscriber != nil {
		return s.subscriber, true
	}
	http.Error(w, "订阅服务没有启动", http.StatusServiceUnavailable)
	return nil, false
}

func writeSubscribeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, subscribe.ErrInvalid), errors.Is(err, tmdb.ErrNoKey), errors.Is(err, tmdb.ErrUnauthorized):
		status = http.StatusBadRequest
	case errors.Is(err, subscribe.ErrNotFound), errors.Is(err, tmdb.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, subscribe.ErrBusy):
		status = http.StatusConflict
	case errors.Is(err, tmdb.ErrRateLimited):
		status = http.StatusTooManyRequests
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
	case errors.Is(err, tmdb.ErrUnavailable):
		status = http.StatusBadGateway
	}
	http.Error(w, err.Error(), status)
}

// apiSubscribeSettingsGet handles GET /api/media/subscribe/settings
func (s *Server) apiSubscribeSettingsGet(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	got, err := svc.Settings(r.Context())
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiSubscribeSettingsPut handles PUT /api/media/subscribe/settings
func (s *Server) apiSubscribeSettingsPut(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	var in subscribe.Settings
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.SaveSettings(r.Context(), in)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiQualityProfiles handles GET /api/media/quality-profiles：档案列表，带上分辨率、来源、编码的可选值。
func (s *Server) apiQualityProfiles(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	list, err := svc.Profiles(r.Context())
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, map[string]any{
		"items": list, "resolutions": subscribe.Resolutions, "sources": subscribe.Sources, "codecs": subscribe.Codecs,
	})
}

// apiQualityProfileSave handles POST /api/media/quality-profiles and PUT /api/media/quality-profiles/{id}
func (s *Server) apiQualityProfileSave(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := optionalID(w, r)
	if !ok {
		return
	}
	var in subscribe.ProfileInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.SaveProfile(r.Context(), id, in)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiQualityProfileDelete handles DELETE /api/media/quality-profiles/{id}
func (s *Server) apiQualityProfileDelete(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := svc.DeleteProfile(r.Context(), id); err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// apiSubscriptions handles GET /api/media/subscriptions?status=&q=
func (s *Server) apiSubscriptions(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	q := r.URL.Query()
	list, err := svc.Subscriptions(r.Context(), subscribe.SubscriptionQuery{Status: q.Get("status"), Keyword: q.Get("q")})
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, list)
}

// apiSubscriptionCreate handles POST /api/media/subscriptions：剧集没指定定时与版本锁时按追剧的默认设置（按播出时间、开着版本锁）。
// 带 from_torrent 时从那个种子开始（要在各站点取种子并推送，时限同「立即搜索」）。
func (s *Server) apiSubscriptionCreate(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	var in subscribe.SubscriptionInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	subscribe.FollowDefaults(&in)
	ctx, cancel := context.WithTimeout(r.Context(), subscribeSearchTimeout)
	defer cancel()
	got, err := svc.CreateSubscription(ctx, in, models.MediaSubFromManual)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiSubscriptionGet handles GET /api/media/subscriptions/{id}
func (s *Server) apiSubscriptionGet(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	got, err := svc.Subscription(r.Context(), id)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiSubscriptionUpdate handles PUT /api/media/subscriptions/{id}
func (s *Server) apiSubscriptionUpdate(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in subscribe.SubscriptionInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.UpdateSubscription(r.Context(), id, in)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiSubscriptionDelete handles DELETE /api/media/subscriptions/{id}
func (s *Server) apiSubscriptionDelete(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := svc.DeleteSubscription(r.Context(), id); err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// apiSubscriptionStatus handles POST /api/media/subscriptions/{id}/status {status: active|paused}
func (s *Server) apiSubscriptionStatus(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.SetStatus(r.Context(), id, in.Status)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiSubscriptionSearch handles POST /api/media/subscriptions/{id}/search：立即搜索一次，回结果说明。
func (s *Server) apiSubscriptionSearch(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), subscribeSearchTimeout)
	defer cancel()
	msg, err := svc.SearchNow(ctx, id)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"message": msg})
}

// apiDoubanSources handles GET /api/media/douban-sources
func (s *Server) apiDoubanSources(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	list, err := svc.DoubanSources(r.Context())
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, list)
}

// apiDoubanSourceSave handles POST /api/media/douban-sources and PUT /api/media/douban-sources/{id}
func (s *Server) apiDoubanSourceSave(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := optionalID(w, r)
	if !ok {
		return
	}
	var in subscribe.DoubanSourceInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.SaveDoubanSource(r.Context(), id, in)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiDoubanSourceDelete handles DELETE /api/media/douban-sources/{id}
func (s *Server) apiDoubanSourceDelete(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := svc.DeleteDoubanSource(r.Context(), id); err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// apiDoubanSourceFetch handles POST /api/media/douban-sources/{id}/fetch：立即拉一次，回新建的订阅数。
func (s *Server) apiDoubanSourceFetch(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), subscribeSearchTimeout)
	defer cancel()
	n, err := svc.FetchDouban(ctx, id)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"created": n})
}

// apiDoubanSourceItems handles GET /api/media/douban-sources/{id}/items
func (s *Server) apiDoubanSourceItems(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	list, err := svc.DoubanItems(r.Context(), id)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, list)
}

// apiMediaExplore handles GET /api/media/explore?kind=movie|tv&list=trending|popular|search&page=&q=
func (s *Server) apiMediaExplore(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	q := r.URL.Query()
	page := 1
	if v := q.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			http.Error(w, "page 要是正整数", http.StatusBadRequest)
			return
		}
		page = n
	}
	got, err := svc.Explore(r.Context(), q.Get("kind"), q.Get("list"), page, q.Get("q"))
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// episodeNumber 读路径里的集号。
func episodeNumber(w http.ResponseWriter, r *http.Request) (int, bool) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 1 {
		http.Error(w, "集号要是正整数", http.StatusBadRequest)
		return 0, false
	}
	return n, true
}

// apiSubscriptionEpisodes handles GET /api/media/subscriptions/{id}/episodes：追剧每一集的情况。
func (s *Server) apiSubscriptionEpisodes(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	got, err := svc.Episodes(r.Context(), id)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiSubscriptionEpisodeSkip handles POST /api/media/subscriptions/{id}/episodes/{n}/skip {skip: bool}：跳过或重新要这一集。
func (s *Server) apiSubscriptionEpisodeSkip(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	n, ok := episodeNumber(w, r)
	if !ok {
		return
	}
	var in struct {
		Skip *bool `json:"skip"`
	}
	if !decodeStrictBody(w, r, &in) {
		return
	}
	if in.Skip == nil {
		http.Error(w, "要给 skip（true 跳过、false 重新要）", http.StatusBadRequest)
		return
	}
	got, err := svc.SkipEpisode(r.Context(), id, n, *in.Skip)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiSubscriptionEpisodeUnlock handles POST /api/media/subscriptions/{id}/episodes/{n}/unlock {unlock: bool}：这一集换版本（不受版本锁限制）。
func (s *Server) apiSubscriptionEpisodeUnlock(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	n, ok := episodeNumber(w, r)
	if !ok {
		return
	}
	var in struct {
		Unlock *bool `json:"unlock"`
	}
	if !decodeStrictBody(w, r, &in) {
		return
	}
	if in.Unlock == nil {
		http.Error(w, "要给 unlock（true 这一集换版本、false 恢复版本锁）", http.StatusBadRequest)
		return
	}
	got, err := svc.UnlockEpisode(r.Context(), id, n, *in.Unlock)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiSubscriptionRuns handles GET /api/media/subscriptions/{id}/runs?limit=&offset=：运行记录（新的在前）。
func (s *Server) apiSubscriptionRuns(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	limit, offset := 20, 0
	q := r.URL.Query()
	for _, v := range []struct {
		name string
		dst  *int
		max  int
	}{{"limit", &limit, 100}, {"offset", &offset, 100000}} {
		if raw := q.Get(v.name); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 || n > v.max {
				http.Error(w, v.name+" 不对", http.StatusBadRequest)
				return
			}
			*v.dst = n
		}
	}
	got, err := svc.Runs(r.Context(), id, limit, offset)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiEpisodeRules handles GET /api/media/subscriptions/{id}/episode-rules
func (s *Server) apiEpisodeRules(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	got, err := svc.EpisodeRules(r.Context(), id)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiEpisodeRuleSave handles PUT /api/media/subscriptions/{id}/episode-rules：设一个制作组的集号规则（同组替换）。
func (s *Server) apiEpisodeRuleSave(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in subscribe.EpisodeRuleInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.SaveEpisodeRule(r.Context(), id, in)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiEpisodeRuleDelete handles DELETE /api/media/subscriptions/{id}/episode-rules?group=：删掉一个制作组的集号规则。
func (s *Server) apiEpisodeRuleDelete(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	got, err := svc.DeleteEpisodeRule(r.Context(), id, r.URL.Query().Get("group"))
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// SchedulePreviewInput 是 POST /api/media/subscriptions/schedule-preview：改定时的时候看接下来几次什么时候搜。
// SubscriptionID 给了时按那个订阅的类型与建立时间算（每隔几天从建立那天对齐）。
type SchedulePreviewInput struct {
	Schedule       subscribe.Schedule `json:"schedule"`
	MediaType      string             `json:"media_type,omitempty"`
	SubscriptionID uint               `json:"subscription_id,omitempty"`
	Count          int                `json:"count,omitempty"`
}

// SchedulePreviewView 是预览的结果：接下来几次搜索的时间；按播出时间的不算，Note 说明为什么。
type SchedulePreviewView struct {
	Next []time.Time `json:"next"`
	Note string      `json:"note,omitempty"`
}

// schedulePreview 算定时的预览（网页与 App API 共用）。
func schedulePreview(ctx context.Context, svc *subscribe.Service, in SchedulePreviewInput) (SchedulePreviewView, error) {
	var anchor time.Time
	if in.SubscriptionID != 0 {
		sub, err := svc.Subscription(ctx, in.SubscriptionID)
		if err != nil {
			return SchedulePreviewView{}, err
		}
		anchor = sub.CreatedAt
		if in.MediaType == "" {
			in.MediaType = sub.MediaType
		}
	}
	count := in.Count
	if count <= 0 {
		count = 5
	}
	next, err := svc.SchedulePreview(ctx, in.Schedule, in.MediaType, anchor, count)
	if err != nil {
		return SchedulePreviewView{}, err
	}
	out := SchedulePreviewView{Next: next}
	if out.Next == nil {
		out.Next = []time.Time{}
	}
	if in.Schedule.Kind == models.MediaScheduleAirtime {
		out.Note = "按每一集的播出时间算：播出前不搜，播出后隔一段再搜，具体时间看每集的播出日期"
	}
	return out, nil
}

// apiSchedulePreview handles POST /api/media/subscriptions/schedule-preview
func (s *Server) apiSchedulePreview(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	var in SchedulePreviewInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	out, err := schedulePreview(r.Context(), svc, in)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, out)
}

// writeSubscribeOrganizeError 是订阅里整理、刮削的错误：订阅的错误按订阅的，整理入库回的按整理的。
func writeSubscribeOrganizeError(w http.ResponseWriter, err error) {
	if errors.Is(err, subscribe.ErrInvalid) || errors.Is(err, subscribe.ErrNotFound) {
		writeSubscribeError(w, err)
		return
	}
	writeOrganizeError(w, err)
}

// pathTorrentID 是路径里订阅种子的编号（订阅详情 torrent_list 里的 id）。
func pathTorrentID(w http.ResponseWriter, r *http.Request) (uint, bool) {
	id, err := strconv.ParseUint(r.PathValue("tid"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, "种子编号无效", http.StatusBadRequest)
		return 0, false
	}
	return uint(id), true
}

// apiSubscriptionTorrentOrganize handles POST /api/media/subscriptions/{id}/torrents/{tid}/organize：
// 重新整理订阅下载的一个种子（整季包与单集一样），按订阅的条目与媒体库。
func (s *Server) apiSubscriptionTorrentOrganize(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tid, ok := pathTorrentID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), organizeAPITimeout)
	defer cancel()
	got, err := svc.OrganizeTorrent(ctx, id, tid)
	if err != nil {
		writeSubscribeOrganizeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiSubscriptionTorrentScrape handles POST /api/media/subscriptions/{id}/torrents/{tid}/scrape：
// 重新刮削订阅下载的一个种子整理进的电影或剧集。
func (s *Server) apiSubscriptionTorrentScrape(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tid, ok := pathTorrentID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), organizeAPITimeout)
	defer cancel()
	got, err := svc.RescrapeTorrent(ctx, id, tid)
	if err != nil {
		writeSubscribeOrganizeError(w, err)
		return
	}
	writeJSON(w, got)
}
