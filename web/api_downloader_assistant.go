package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/internal/dlassistant"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// assistantMaxItems 是一次执行最多处理的种子数，防止一次请求跑太久。
const assistantMaxItems = 2000

// assistantTimeout 是一次扫描或执行的总时限；到时底层的下载器请求一起取消。
const assistantTimeout = 10 * time.Minute

// assistantBusy 记录正在扫描或执行的下载器：同一台下载器同一时间只跑一个助手操作，
// 连点或多个页面同时扫描时回 409，不叠加请求压在下载器上。
var assistantBusy sync.Map

// acquireAssistant 占住这台下载器；已有操作在跑时返回 false 并写好 409。
func acquireAssistant(w http.ResponseWriter, id uint) (func(), bool) {
	if _, loaded := assistantBusy.LoadOrStore(id, struct{}{}); loaded {
		http.Error(w, "这台下载器正在扫描或执行，请等它结束再试", http.StatusConflict)
		return nil, false
	}
	return func() { assistantBusy.Delete(id) }, true
}

// registerDownloaderAssistantRoutes 注册下载器助手的接口（全部要求登录）。
//
//	GET  /api/downloader-assistant/site-tags?downloader_id=   预览缺站点标签的种子
//	POST /api/downloader-assistant/site-tags                  补标签 {downloader_id, items:[{hash, site}]}
//	GET  /api/downloader-assistant/trackers?downloader_id=&from=&to=   预览 tracker 替换
//	POST /api/downloader-assistant/trackers                   执行替换 {downloader_id, from, to, selections:[{hash, id}]}
//	GET  /api/downloader-assistant/dead?downloader_id=        列出失效种子
//	POST /api/downloader-assistant/dead                       删除 {downloader_id, hashes, remove_data}
//	GET  /api/downloader-assistant/dead-scan                  定时扫描设置；PUT 保存
func (s *Server) registerDownloaderAssistantRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/downloader-assistant/site-tags", s.auth(s.apiAssistantSiteTags))
	mux.HandleFunc("/api/downloader-assistant/trackers", s.auth(s.apiAssistantTrackers))
	mux.HandleFunc("/api/downloader-assistant/dead", s.auth(s.apiAssistantDead))
	mux.HandleFunc("/api/downloader-assistant/dead-scan", s.auth(s.apiAssistantDeadScan))
}

type assistantSiteTagsRequest struct {
	DownloaderID uint                      `json:"downloader_id"`
	Items        []dlassistant.SiteTagItem `json:"items"`
}

type assistantTrackersRequest struct {
	DownloaderID uint                           `json:"downloader_id"`
	From         string                         `json:"from"`
	To           string                         `json:"to"`
	Selections   []dlassistant.TrackerSelection `json:"selections"`
}

type assistantDeadRequest struct {
	DownloaderID uint     `json:"downloader_id"`
	Hashes       []string `json:"hashes"`
	RemoveData   bool     `json:"remove_data"`
}

// assistantDownloader 按 ID 取已启用的下载器实例；失败时已经写好了响应。
func (s *Server) assistantDownloader(w http.ResponseWriter, ctx context.Context, id uint) (downloader.Downloader, bool) {
	if id == 0 {
		http.Error(w, "请选择下载器", http.StatusBadRequest)
		return nil, false
	}
	recs, err := s.listEnabledDownloaderRecords(&id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, false
	}
	if len(recs) == 0 {
		http.Error(w, "下载器不存在或未启用", http.StatusNotFound)
		return nil, false
	}
	dm := s.getDownloaderManager()
	if dm == nil {
		http.Error(w, "下载器管理器未初始化", http.StatusServiceUnavailable)
		return nil, false
	}
	dl, err := acquireDownloader(ctx, dm, recs[0].Name)
	if err != nil {
		http.Error(w, fmt.Sprintf("连不上下载器 %s: %v", recs[0].Name, err), http.StatusBadGateway)
		return nil, false
	}
	return dl, true
}

// withAssistant 取下载器实例、占住它并给整次操作套上总时限，再调用 fn；失败时已经写好了响应。
func (s *Server) withAssistant(w http.ResponseWriter, r *http.Request, id uint, fn func(ctx context.Context, dl downloader.Downloader)) {
	dl, ok := s.assistantDownloader(w, r.Context(), id)
	if !ok {
		return
	}
	release, ok := acquireAssistant(w, id)
	if !ok {
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(r.Context(), assistantTimeout)
	defer cancel()
	fn(ctx, dl)
}

func writeAssistantError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		http.Error(w, "扫描超过时限，已停止；种子很多时请稍后再试", http.StatusGatewayTimeout)
		return
	}
	http.Error(w, err.Error(), http.StatusBadGateway)
}

func queryDownloaderID(r *http.Request) uint {
	v, err := strconv.ParseUint(strings.TrimSpace(r.URL.Query().Get("downloader_id")), 10, 64)
	if err != nil {
		return 0
	}
	return uint(v)
}

func decodeAssistantBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(dst); err != nil {
		http.Error(w, "请求格式错误: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func tooManyItems(w http.ResponseWriter, n int) bool {
	if n == 0 {
		http.Error(w, "没有选中任何种子", http.StatusBadRequest)
		return true
	}
	if n > assistantMaxItems {
		http.Error(w, fmt.Sprintf("一次最多处理 %d 项", assistantMaxItems), http.StatusBadRequest)
		return true
	}
	return false
}

func (s *Server) apiAssistantSiteTags(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.withAssistant(w, r, queryDownloaderID(r), func(ctx context.Context, dl downloader.Downloader) {
			items, info, err := dlassistant.FindMissingSiteTags(ctx, dl, v2.NewTrackerResolver())
			if err != nil {
				writeAssistantError(w, err)
				return
			}
			writeJSON(w, map[string]any{"items": items, "total": info.Total, "scanned": info.Scanned})
		})
	case http.MethodPost:
		var req assistantSiteTagsRequest
		if !decodeAssistantBody(w, r, &req) || tooManyItems(w, len(req.Items)) {
			return
		}
		s.withAssistant(w, r, req.DownloaderID, func(ctx context.Context, dl downloader.Downloader) {
			res, err := dlassistant.ApplySiteTags(ctx, dl, v2.NewTrackerResolver(), req.Items)
			if err != nil {
				writeAssistantError(w, err)
				return
			}
			writeJSON(w, res)
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) apiAssistantTrackers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		from, to := q.Get("from"), q.Get("to")
		if err := dlassistant.ValidateTrackerReplace(from, to); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.withAssistant(w, r, queryDownloaderID(r), func(ctx context.Context, dl downloader.Downloader) {
			_, editable := dl.(downloader.TrackerEditor)
			items, info, err := dlassistant.PreviewTrackerReplace(ctx, dl, from, to)
			if err != nil {
				writeAssistantError(w, err)
				return
			}
			writeJSON(w, map[string]any{"items": items, "supported": editable, "total": info.Total, "scanned": info.Scanned})
		})
	case http.MethodPost:
		var req assistantTrackersRequest
		if !decodeAssistantBody(w, r, &req) {
			return
		}
		if err := dlassistant.ValidateTrackerReplace(req.From, req.To); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if tooManyItems(w, len(req.Selections)) {
			return
		}
		s.withAssistant(w, r, req.DownloaderID, func(ctx context.Context, dl downloader.Downloader) {
			res, err := dlassistant.ApplyTrackerReplace(ctx, dl, req.From, req.To, req.Selections)
			switch {
			case errors.Is(err, downloader.ErrCapabilityUnsupported):
				http.Error(w, "这台下载器不支持修改 tracker", http.StatusBadRequest)
				return
			case err != nil:
				writeAssistantError(w, err)
				return
			}
			writeJSON(w, res)
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) apiAssistantDead(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.withAssistant(w, r, queryDownloaderID(r), func(ctx context.Context, dl downloader.Downloader) {
			items, info, err := dlassistant.ScanDeadTorrents(ctx, dl, v2.NewTrackerResolver())
			if err != nil {
				writeAssistantError(w, err)
				return
			}
			writeJSON(w, map[string]any{"items": items, "total": info.Total, "scanned": info.Scanned})
		})
	case http.MethodPost:
		var req assistantDeadRequest
		if !decodeAssistantBody(w, r, &req) || tooManyItems(w, len(req.Hashes)) {
			return
		}
		s.withAssistant(w, r, req.DownloaderID, func(ctx context.Context, dl downloader.Downloader) {
			res, err := dlassistant.DeleteDeadTorrents(ctx, dl, req.Hashes, req.RemoveData)
			if err != nil {
				writeAssistantError(w, err)
				return
			}
			writeJSON(w, res)
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) apiAssistantDeadScan(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "配置存储未初始化", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg, err := s.store.DeadTorrentScanSettings()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, cfg)
	case http.MethodPut:
		var in core.DeadTorrentScanSettings
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
			http.Error(w, "请求格式错误: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.store.SaveDeadTorrentScanSettings(in); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, core.ErrDeadTorrentScanInvalid) || errors.Is(err, core.ErrDeadTorrentScanNoChannel) {
				status = http.StatusBadRequest
			}
			http.Error(w, err.Error(), status)
			return
		}
		cfg, err := s.store.DeadTorrentScanSettings()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, cfg)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
