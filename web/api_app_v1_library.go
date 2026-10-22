package web

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/server"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// App API v1 的媒体库：媒体库列表、识别标题、整理预览与执行、重试整理记录（App 与 MCP 工具共用）。
// 网页上的同名功能在 /api/media/*（api_media.go、api_media_library.go），这里只换成 App 的 DTO。

// appMaxCandidates 是识别结果最多给几个候选
const appMaxCandidates = 5

// AppMediaLibrary 是 GET /media/libraries 的一项。
type AppMediaLibrary struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Anime bool   `json:"anime"`
	Path  string `json:"path"`
	// Mode 是整理方式，空的是整理设置里的默认方式
	Mode          string `json:"mode"`
	Scrape        bool   `json:"scrape"`
	ClassifyDir   bool   `json:"classify_dir"`
	OverwriteMode string `json:"overwrite_mode"`
	Enabled       bool   `json:"enabled"`
}

// AppMediaMeta 是从种子标题解析出来的信息。
type AppMediaMeta struct {
	NameCN     string   `json:"name_cn,omitempty"`
	NameEN     string   `json:"name_en,omitempty"`
	Year       int      `json:"year,omitempty"`
	Type       string   `json:"type,omitempty"`
	Season     int      `json:"season,omitempty"`
	SeasonEnd  int      `json:"season_end,omitempty"`
	Episode    int      `json:"episode,omitempty"`
	EpisodeEnd int      `json:"episode_end,omitempty"`
	Complete   bool     `json:"complete,omitempty"`
	Resolution string   `json:"resolution,omitempty"`
	Source     string   `json:"source,omitempty"`
	VideoCodec string   `json:"video_codec,omitempty"`
	HDR        []string `json:"hdr,omitempty"`
	Audio      []string `json:"audio,omitempty"`
	Group      string   `json:"group,omitempty"`
	Edition    []string `json:"edition,omitempty"`
}

// AppMediaMatch 是 TMDB 上的一个条目：识别出来的，或者候选（候选带 score）。
type AppMediaMatch struct {
	ID            int     `json:"id"`
	MediaType     string  `json:"media_type"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title,omitempty"`
	Year          int     `json:"year,omitempty"`
	Overview      string  `json:"overview,omitempty"`
	PosterPath    string  `json:"poster_path,omitempty"`
	VoteAverage   float64 `json:"vote_average,omitempty"`
	IMDbID        string  `json:"imdb_id,omitempty"`
	Score         float64 `json:"score,omitempty"`
}

// AppRecognition 是 POST /media/recognize 的结果：Match 为空时没有可靠的匹配（看 candidates，或者到网页上手动纠正）。
type AppRecognition struct {
	Summary string         `json:"summary"`
	Meta    AppMediaMeta   `json:"meta"`
	Match   *AppMediaMatch `json:"match,omitempty"`
	// Source 是怎么认出来的：override（手动纠正过）、imdb（按站点给的 IMDb 编号）、search（按名字搜）、none（没认出来）
	Source     string          `json:"source"`
	Score      float64         `json:"score,omitempty"`
	Candidates []AppMediaMatch `json:"candidates"`
	Message    string          `json:"message,omitempty"`
	Error      string          `json:"error,omitempty"`
}

// AppRecognizeInput 是 POST /media/recognize：种子标题与副标题（至少一个）、站点给的 IMDb 编号（可选）。
type AppRecognizeInput struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	IMDbID   string `json:"imdb_id"`
}

// AppOrganizeInput 是 POST /media/organize 与 /media/organize/preview：下载器里的一个种子（hash 是 info hash，没有时用 task_id）。
// 可以指定 TMDB 条目（media_type 与 tmdb_id 一起给）与媒体库；不指定时自动识别、按类型选库。
type AppOrganizeInput struct {
	DownloaderID uint   `json:"downloader_id"`
	Hash         string `json:"hash"`
	MediaType    string `json:"media_type,omitempty"`
	TMDBID       int    `json:"tmdb_id,omitempty"`
	LibraryID    uint   `json:"library_id,omitempty"`
}

// AppLibraryRef 是整理计划用的媒体库。
type AppLibraryRef struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// AppOrganizeItem 是整理计划里的一个视频文件。Status：pending 要整理，done 库里已经有了，exists 目标被别的文件占着（不覆盖），
// replace 换掉 pt-tools 之前放进库的旧版本，failed 整理不了（message 写明原因）。
type AppOrganizeItem struct {
	Rel        string `json:"rel"`
	Size       int64  `json:"size"`
	Target     string `json:"target,omitempty"`
	Season     int    `json:"season,omitempty"`
	Episode    int    `json:"episode,omitempty"`
	EpisodeEnd int    `json:"episode_end,omitempty"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
	HistoryID  uint   `json:"history_id,omitempty"`
}

// AppOrganizePlan 是一个种子的整理计划（预览与执行共用）：problem 不为空时整个种子整理不了。
type AppOrganizePlan struct {
	DownloaderID uint              `json:"downloader_id"`
	Downloader   string            `json:"downloader"`
	Hash         string            `json:"hash"`
	Name         string            `json:"name"`
	Match        *AppMediaMatch    `json:"match,omitempty"`
	Library      *AppLibraryRef    `json:"library,omitempty"`
	Category     string            `json:"category,omitempty"`
	Mode         string            `json:"mode,omitempty"`
	Items        []AppOrganizeItem `json:"items"`
	// Skipped 是不整理的文件（样片、太小的、不是视频的）有几个
	Skipped int    `json:"skipped"`
	Problem string `json:"problem,omitempty"`
}

// AppOrganizeResult 是 POST /media/organize 与 /media/history/{id}/retry 的结果：queued 为真时整理还在后台进行，稍后看整理历史。
type AppOrganizeResult struct {
	Plan     *AppOrganizePlan `json:"plan,omitempty"`
	Created  int              `json:"created"`
	Done     int              `json:"done"`
	Skipped  int              `json:"skipped"`
	Failed   int              `json:"failed"`
	Messages []string         `json:"messages"`
	Queued   bool             `json:"queued"`
}

// appOrganizeError 写整理与识别的错误（同网页的 writeOrganizeError，换成 App 的错误码）。
func appOrganizeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, organize.ErrInvalid), errors.Is(err, recognize.ErrInvalid), errors.Is(err, server.ErrBadConfig),
		errors.Is(err, server.ErrUnauthorized), errors.Is(err, tmdb.ErrNoKey), errors.Is(err, tmdb.ErrUnauthorized):
		appError(w, http.StatusBadRequest, "invalid_argument", appRedactAddr(err.Error()))
	case errors.Is(err, organize.ErrNotFound), errors.Is(err, recognize.ErrNotFound), errors.Is(err, tmdb.ErrNotFound):
		appError(w, http.StatusNotFound, "not_found", appRedactAddr(err.Error()))
	case errors.Is(err, tmdb.ErrRateLimited):
		appError(w, http.StatusTooManyRequests, "rate_limited", appRedactAddr(err.Error()))
	case errors.Is(err, context.DeadlineExceeded):
		appError(w, http.StatusGatewayTimeout, "timeout", "整理或识别超时：稍后到整理历史里看结果")
	case errors.Is(err, server.ErrUnavailable), errors.Is(err, server.ErrRedirect), errors.Is(err, tmdb.ErrUnavailable):
		appError(w, http.StatusBadGateway, "upstream", appRedactAddr(err.Error()))
	default:
		appError(w, http.StatusInternalServerError, "internal", appRedactAddr(err.Error()))
	}
}

func (s *Server) appOrganizer(w http.ResponseWriter) (*organize.Service, bool) {
	if s.organizer == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "整理入库没有启动")
		return nil, false
	}
	return s.organizer, true
}

func (s *Server) appMediaLibraries(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appOrganizer(w)
	if !ok {
		return
	}
	list, err := svc.Libraries(r.Context())
	if err != nil {
		appOrganizeError(w, err)
		return
	}
	out := make([]AppMediaLibrary, 0, len(list))
	for _, l := range list {
		out = append(out, AppMediaLibrary{
			ID: l.ID, Name: l.Name, Kind: l.Kind, Anime: l.Anime, Path: l.Path, Mode: l.Mode, Scrape: l.Scrape,
			ClassifyDir: l.ClassifyDir, OverwriteMode: l.OverwriteMode, Enabled: l.Enabled,
		})
	}
	appJSON(w, out)
}

func appMetaOf(m meta.Meta) AppMediaMeta {
	return AppMediaMeta{
		NameCN: m.NameCN, NameEN: m.NameEN, Year: m.Year, Type: string(m.Type), Season: m.Season, SeasonEnd: m.SeasonEnd,
		Episode: m.Episode, EpisodeEnd: m.EpisodeEnd, Complete: m.Complete, Resolution: m.Resolution, Source: m.Source,
		VideoCodec: m.VideoCodec, HDR: m.HDR, Audio: m.Audio, Group: m.Group, Edition: m.Edition,
	}
}

func appMatchOf(r *tmdb.Result, score float64) *AppMediaMatch {
	if r == nil {
		return nil
	}
	return &AppMediaMatch{
		ID: r.ID, MediaType: r.MediaType, Title: r.Title, OriginalTitle: r.OriginalTitle, Year: r.Year, Overview: r.Overview,
		PosterPath: r.PosterPath, VoteAverage: r.VoteAverage, IMDbID: r.IMDbID, Score: score,
	}
}

func (s *Server) appMediaRecognize(w http.ResponseWriter, r *http.Request) {
	if s.media == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "媒体识别没有启动")
		return
	}
	var in AppRecognizeInput
	if !appDecode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), mediaAPITimeout)
	defer cancel()
	res, err := s.media.Recognize(ctx, recognize.Input{Title: in.Title, Subtitle: in.Subtitle, IMDbID: in.IMDbID})
	if err != nil {
		appOrganizeError(w, err)
		return
	}
	out := AppRecognition{
		Summary: res.Summary, Meta: appMetaOf(res.Meta), Match: appMatchOf(res.Match, 0), Source: res.Source, Score: res.Score,
		Candidates: []AppMediaMatch{}, Message: res.Message, Error: appRedactAddr(res.Error),
	}
	for i := range res.Candidates {
		if i == appMaxCandidates {
			break
		}
		out.Candidates = append(out.Candidates, *appMatchOf(&res.Candidates[i].Result, res.Candidates[i].Score))
	}
	appJSON(w, out)
}

// appOrganizeRequest 检查整理的参数，换成整理服务的请求。
func appOrganizeRequest(w http.ResponseWriter, r *http.Request) (organize.Request, bool) {
	var in AppOrganizeInput
	if !appDecode(w, r, &in) {
		return organize.Request{}, false
	}
	in.Hash = strings.TrimSpace(in.Hash)
	switch {
	case in.DownloaderID == 0 || in.Hash == "" || len(in.Hash) > 128:
		appError(w, http.StatusBadRequest, "invalid_argument", "要给 downloader_id 与 hash（list_downloader_torrents 的 info_hash，没有时用 task_id）")
		return organize.Request{}, false
	case in.MediaType != "" && in.MediaType != models.MediaKindMovie && in.MediaType != models.MediaKindTV:
		appError(w, http.StatusBadRequest, "invalid_argument", "media_type 要选 movie 或 tv")
		return organize.Request{}, false
	case (in.TMDBID > 0) != (in.MediaType != ""):
		appError(w, http.StatusBadRequest, "invalid_argument", "指定条目时 media_type 与 tmdb_id 一起给")
		return organize.Request{}, false
	}
	return organize.Request{
		DownloaderID: in.DownloaderID, Hash: in.Hash, MediaType: in.MediaType, TMDBID: in.TMDBID, LibraryID: in.LibraryID,
	}, true
}

func appPlanOf(p *organize.Plan) *AppOrganizePlan {
	if p == nil {
		return nil
	}
	out := &AppOrganizePlan{
		DownloaderID: p.DownloaderID, Downloader: p.DownloaderName, Hash: p.Hash, Name: p.Name, Match: appMatchOf(p.Match, 0),
		Category: p.Category, Mode: p.Mode, Items: make([]AppOrganizeItem, 0, len(p.Items)), Skipped: len(p.Skipped),
		Problem: appRedactAddr(p.Problem),
	}
	if p.Library != nil {
		out.Library = &AppLibraryRef{ID: p.Library.ID, Name: p.Library.Name}
	}
	for _, it := range p.Items {
		out.Items = append(out.Items, AppOrganizeItem{
			Rel: it.Rel, Size: it.Size, Target: it.Target, Season: it.Season, Episode: it.Episode, EpisodeEnd: it.EpisodeEnd,
			Status: it.Status, Message: appRedactAddr(it.Message), HistoryID: it.HistoryID,
		})
	}
	return out
}

func appResultOf(res *organize.Result) AppOrganizeResult {
	out := AppOrganizeResult{Messages: []string{}}
	if res == nil {
		return out
	}
	out.Plan, out.Created, out.Done, out.Skipped, out.Failed, out.Queued = appPlanOf(res.Plan), res.Created, res.Done, res.Skipped, res.Failed, res.Queued
	for _, m := range res.Messages {
		out.Messages = append(out.Messages, appRedactAddr(m))
	}
	return out
}

func (s *Server) appOrganizePreview(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appOrganizer(w)
	if !ok {
		return
	}
	req, ok := appOrganizeRequest(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), organizeAPITimeout)
	defer cancel()
	plan, err := svc.Preview(ctx, req)
	if err != nil {
		appOrganizeError(w, err)
		return
	}
	appJSON(w, appPlanOf(plan))
}

func (s *Server) appOrganize(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appOrganizer(w)
	if !ok {
		return
	}
	req, ok := appOrganizeRequest(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), organizeAPITimeout)
	defer cancel()
	res, err := svc.Organize(ctx, req)
	if err != nil {
		appOrganizeError(w, err)
		return
	}
	out := appResultOf(res)
	if out.Failed > 0 && out.Created == 0 && out.Done == 0 {
		appSetOutcome(r, "error:organize_failed")
	}
	appJSON(w, out)
}

func (s *Server) appMediaHistoryRetry(w http.ResponseWriter, r *http.Request) {
	if !appNoBody(w, r) {
		return
	}
	svc, ok := s.appOrganizer(w)
	if !ok {
		return
	}
	id, ok := appID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), organizeAPITimeout)
	defer cancel()
	res, err := svc.Retry(ctx, id)
	if err != nil {
		appOrganizeError(w, err)
		return
	}
	out := appResultOf(res)
	if out.Failed > 0 && out.Created == 0 && out.Done == 0 {
		appSetOutcome(r, "error:organize_failed")
	}
	appJSON(w, out)
}
