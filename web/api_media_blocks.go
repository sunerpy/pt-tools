package web

import (
	"net/http"

	"github.com/sunerpy/pt-tools/internal/media/subscribe"
)

// 种子黑名单（媒体中心 M2b）：「不要」的种子，订阅不再选，按条目搜索标成已拉黑。只认会话。

func (s *Server) registerBlockRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/media/blocks", s.auth(s.apiMediaBlocks))
	mux.HandleFunc("POST /api/media/blocks", s.auth(s.apiMediaBlockAdd))
	mux.HandleFunc("DELETE /api/media/blocks/{id}", s.auth(s.apiMediaBlockRemove))
	mux.HandleFunc("POST /api/media/subscriptions/{id}/torrents/{tid}/block", s.auth(s.apiSubscriptionTorrentBlock))
}

// apiMediaBlocks handles GET /api/media/blocks：全部拉黑的种子，新的在前。
func (s *Server) apiMediaBlocks(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	rows, err := svc.Blocks(r.Context())
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": rows})
}

// apiMediaBlockAdd handles POST /api/media/blocks：拉黑一个或几个种子（见 subscribe.BlockInput）。
func (s *Server) apiMediaBlockAdd(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	var in subscribe.BlockInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.AddBlocks(r.Context(), in)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiMediaBlockRemove handles DELETE /api/media/blocks/{id}：移出黑名单。
func (s *Server) apiMediaBlockRemove(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := svc.RemoveBlock(r.Context(), id); err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// apiSubscriptionTorrentBlock handles POST /api/media/subscriptions/{id}/torrents/{tid}/block：追剧详情里「不要这个种子」。
func (s *Server) apiSubscriptionTorrentBlock(w http.ResponseWriter, r *http.Request) {
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
	var in subscribe.BlockTorrentInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.BlockSubscriptionTorrent(r.Context(), id, tid, in)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}
