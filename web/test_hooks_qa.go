//go:build qa

// Package web — QA test hook endpoints.
//
// This file is ONLY compiled when the `qa` build tag is set:
//
//	go build -tags qa ./...
//
// It exposes `/test/telegram/inject` and `/test/qq/inject` so QA scripts
// (scripts/qa/*.sh) can inject inbound messages directly into the
// chatops MessageChain, bypassing the real Telegram / OneBot transports.
//
// **Production builds MUST NOT include this file.** A vanilla
// `go build ./...` or `make build-local` will skip it; the routes return
// 404 from the standard mux because no handler is registered.
//
// Plan: T34 (Final Wave F3 prerequisite).
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/sunerpy/pt-tools/internal/notify"
)

// TestInboundProcessor is the minimal contract the QA endpoint requires.
// In qa builds, cmd/web.go (T32) constructs an *internal/chatops.MessageChain
// (which already has `Process(ctx, notify.InboundMessage) error`) and
// passes it via SetQAInboundProcessor.
type TestInboundProcessor interface {
	Process(ctx context.Context, msg notify.InboundMessage) error
}

// qaInbound holds the global QA inbound processor.
//
// A package-level atomic.Value (rather than a Server field) keeps this
// file fully self-contained — the qa build tag adds the file without
// requiring any modification to server.go.
var qaInbound atomic.Value

// qaInjectRequest is the JSON body shape accepted by both
// /test/telegram/inject and /test/qq/inject.
type qaInjectRequest struct {
	Text   string         `json:"text"`
	From   qaInjectSender `json:"from"`
	ConfID uint           `json:"conf_id"`
	ChatID string         `json:"chat_id"`
}

type qaInjectSender struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// SetQAInboundProcessor installs the chain processor used by the test
// hooks. Calling with a nil processor disables the hooks (returns 503).
func SetQAInboundProcessor(p TestInboundProcessor) {
	if p == nil {
		qaInbound.Store((*qaProcessorWrap)(nil))
		return
	}
	qaInbound.Store(&qaProcessorWrap{p: p})
}

type qaProcessorWrap struct{ p TestInboundProcessor }

func loadQAInbound() TestInboundProcessor {
	v := qaInbound.Load()
	if v == nil {
		return nil
	}
	w, _ := v.(*qaProcessorWrap)
	if w == nil {
		return nil
	}
	return w.p
}

// RegisterQATestHooks attaches /test/telegram/inject and /test/qq/inject
// to the given mux. T32 (cmd/web.go) is responsible for invoking this
// in qa builds after Server construction.
func RegisterQATestHooks(mux *http.ServeMux) {
	if mux == nil {
		panic("RegisterQATestHooks: nil mux")
	}
	mux.HandleFunc("POST /test/telegram/inject", qaInjectHandler("telegram"))
	mux.HandleFunc("POST /test/qq/inject", qaInjectHandler("qq_onebot"))
}

func qaInjectHandler(channelType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proc := loadQAInbound()
		if proc == nil {
			writeQAJSON(w, http.StatusServiceUnavailable, map[string]any{
				"error": "qa inbound processor not wired",
			})
			return
		}
		var req qaInjectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeQAJSON(w, http.StatusBadRequest, map[string]any{
				"error": fmt.Sprintf("invalid json: %v", err),
			})
			return
		}
		msg := notify.InboundMessage{
			ChannelType:   channelType,
			SourceConfID:  req.ConfID,
			ChannelUserID: fmt.Sprintf("%d", req.From.ID),
			Username:      req.From.Username,
			ChatID:        req.ChatID,
			Text:          req.Text,
		}
		if err := proc.Process(r.Context(), msg); err != nil {
			writeQAJSON(w, http.StatusInternalServerError, map[string]any{
				"error": err.Error(),
			})
			return
		}
		writeQAJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func writeQAJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
