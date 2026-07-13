package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/extension"
)

func TestApiExtensionActionsRouter_Dispatch(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	require.NoError(t, extension.Enqueue(global.GlobalDB.DB, extension.PendingAction{
		Type:      extension.ActionOpenTab,
		TargetURL: "https://hdsky.me/",
		SiteName:  "hdsky",
	}))

	t.Run("empty rest", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiExtensionActionsRouter(rec, authedRequest(http.MethodPost, "/api/extension/actions/", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("non-ack suffix", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiExtensionActionsRouter(rec, authedRequest(http.MethodPost, "/api/extension/actions/1/other", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("invalid id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiExtensionActionsRouter(rec, authedRequest(http.MethodPost, "/api/extension/actions/abc/ack", nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ack existing via router", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiExtensionActionsRouter(rec, authedRequest(http.MethodPost, "/api/extension/actions/1/ack", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "acked")
	})

	t.Run("ack wrong method", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.handleExtensionActionAck(rec, authedRequest(http.MethodGet, "/api/extension/actions/1/ack", nil), 1)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})
}

func TestRequireDB_NilDB(t *testing.T) {
	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })

	s := &Server{}
	w := httptest.NewRecorder()
	db, ok := s.requireDB(w)
	assert.Nil(t, db)
	assert.False(t, ok)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestApiExtensionActionsPending_BadSince(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiExtensionActionsPending(rec, authedRequest(http.MethodPost, "/api/extension/actions/pending", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("bad since", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiExtensionActionsPending(rec, authedRequest(http.MethodGet, "/api/extension/actions/pending?since=-1", nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}
