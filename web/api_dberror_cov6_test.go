package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// closedDBServer returns a server whose GlobalDB points at a closed sql.DB so
// that any query returns an error, exercising the 500 error branches.
func closedDBServer(t *testing.T) *Server {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.SiteTemplate{}, &models.SiteLoginState{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	prev := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}
	t.Cleanup(func() { global.GlobalDB = prev })
	return &Server{sessions: map[string]string{"sess-test": "admin"}}
}

func TestListDynamicSites_DBError(t *testing.T) {
	srv := closedDBServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/dynamic", nil)
	srv.listDynamicSites(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestApiSiteTemplates_DBError(t *testing.T) {
	srv := closedDBServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/templates", nil)
	srv.apiSiteTemplates(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestApiSiteLoginStateList_DBError(t *testing.T) {
	srv := closedDBServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state", nil)
	srv.apiSiteLoginStateList(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleCloakConfigPut_BadJSON(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/cloak/config", strings.NewReader(`{bad`))
	req.AddCookie(&http.Cookie{Name: "session", Value: "sess-test"})
	srv.handleCloakConfigPut(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
