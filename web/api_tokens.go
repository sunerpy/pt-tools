package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/sunerpy/pt-tools/internal/apitoken"
)

// API 令牌的管理接口（路线图 M12）：只认 session，令牌自己不能新建或撤销令牌。

// SetAPITokens 接上令牌库与写操作的审计（cmd 在启动时调用）。
func (s *Server) SetAPITokens(store *apitoken.Store, audit appAuditRecorder) {
	s.tokens = store
	s.appAudit = audit
}

func (s *Server) registerTokenRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tokens", s.auth(s.apiTokenList))
	mux.HandleFunc("POST /api/tokens", s.auth(s.apiTokenCreate))
	mux.HandleFunc("DELETE /api/tokens/{id}", s.auth(s.apiTokenRevoke))
}

// TokenCreated 是新建令牌的回应：明文只在这里出现一次。
type TokenCreated struct {
	Token     apitoken.Token `json:"token"`
	Plaintext string         `json:"plaintext"`
}

func writeTokenError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, apitoken.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, apitoken.ErrNotFound):
		status = http.StatusNotFound
	}
	http.Error(w, err.Error(), status)
}

func (s *Server) tokenStore(w http.ResponseWriter) (*apitoken.Store, bool) {
	if s.tokens == nil {
		http.Error(w, "API 令牌没有启用", http.StatusServiceUnavailable)
		return nil, false
	}
	return s.tokens, true
}

func (s *Server) apiTokenList(w http.ResponseWriter, r *http.Request) {
	store, ok := s.tokenStore(w)
	if !ok {
		return
	}
	list, err := store.List(r.Context())
	if err != nil {
		writeTokenError(w, err)
		return
	}
	writeJSON(w, list)
}

func (s *Server) apiTokenCreate(w http.ResponseWriter, r *http.Request) {
	store, ok := s.tokenStore(w)
	if !ok {
		return
	}
	var in apitoken.CreateInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	by := ""
	if c, err := r.Cookie("session"); err == nil {
		by, _ = s.sessions.lookup(c.Value)
	}
	tok, plain, err := store.Create(r.Context(), in, by)
	if err != nil {
		writeTokenError(w, err)
		return
	}
	writeJSON(w, TokenCreated{Token: tok, Plaintext: plain})
}

func (s *Server) apiTokenRevoke(w http.ResponseWriter, r *http.Request) {
	store, ok := s.tokenStore(w)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, "令牌编号不对", http.StatusBadRequest)
		return
	}
	if err := store.Revoke(r.Context(), uint(id)); err != nil {
		writeTokenError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
