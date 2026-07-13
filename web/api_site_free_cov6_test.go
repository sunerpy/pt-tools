package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApiSiteFreeTorrents_EmptyAndMethod(t *testing.T) {
	s := &Server{}

	t.Run("list method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/site/hdsky/free-torrents", nil)
		s.apiSiteFreeTorrentsList(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("list empty site id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site//free-torrents", nil)
		s.apiSiteFreeTorrentsList(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("list ok", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/free-torrents", nil)
		s.apiSiteFreeTorrentsList(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("download empty site id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site//free-torrents/download", nil)
		s.apiSiteFreeTorrentsDownload(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("download bad archive type", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/free-torrents/download?type=rar", nil)
		s.apiSiteFreeTorrentsDownload(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("download zip ok", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/free-torrents/download?type=zip", nil)
		s.apiSiteFreeTorrentsDownload(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}
