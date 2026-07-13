package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func closedTorrentServer(t *testing.T) *Server {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.TorrentInfoArchive{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	prev := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}
	t.Cleanup(func() { global.GlobalDB = prev })
	return &Server{sessions: map[string]string{"sess-test": "admin"}}
}

func TestApiPausedTorrents_DBError(t *testing.T) {
	srv := closedTorrentServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/torrents/paused", nil)
	srv.apiPausedTorrents(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestApiArchiveTorrents_DBError(t *testing.T) {
	srv := closedTorrentServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/torrents/archive", nil)
	srv.apiArchiveTorrents(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestApiDeleteTasks_DBError(t *testing.T) {
	srv := closedTorrentServer(t)
	body, _ := json.Marshal(DeleteTasksRequest{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/batch-delete", bytes.NewReader(body))
	srv.apiDeleteTasks(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestApiDeletePausedTorrents_DBError(t *testing.T) {
	srv := closedTorrentServer(t)
	body, _ := json.Marshal(DeletePausedRequest{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/torrents/delete-paused", bytes.NewReader(body))
	srv.apiDeletePausedTorrents(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
