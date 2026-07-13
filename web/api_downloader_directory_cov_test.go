package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// setupDirectoryServer prepares a server + DB with a downloader and directory tables migrated.
func setupDirectoryServer(t *testing.T) (*Server, *gorm.DB, uint) {
	t.Helper()
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.DownloaderDirectory{}))

	dl := models.DownloaderSetting{Name: "qbit-1", Type: "qbittorrent", Enabled: true}
	require.NoError(t, db.Create(&dl).Error)
	return server, db, dl.ID
}

func doDirReq(t *testing.T, h http.HandlerFunc, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func TestCreateAndListDownloaderDirectory(t *testing.T) {
	server, _, dlID := setupDirectoryServer(t)
	base := "/api/downloaders/1/directories"

	t.Run("create first directory becomes default", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectories, http.MethodPost, base, DownloaderDirectoryRequest{Path: "/downloads/a", Alias: "A"})
		require.Equal(t, http.StatusOK, w.Code)

		var resp DownloaderDirectoryResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "/downloads/a", resp.Path)
		assert.True(t, resp.IsDefault, "first directory must be default")
	})

	t.Run("duplicate path rejected", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectories, http.MethodPost, base, DownloaderDirectoryRequest{Path: "/downloads/a"})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty path rejected", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectories, http.MethodPost, base, DownloaderDirectoryRequest{Path: ""})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("list returns directories", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectories, http.MethodGet, base, nil)
		require.Equal(t, http.StatusOK, w.Code)

		var resp []DownloaderDirectoryResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Len(t, resp, 1)
	})

	_ = dlID
}

func TestDownloaderDirectories_Errors(t *testing.T) {
	server, _, _ := setupDirectoryServer(t)

	t.Run("invalid downloader id", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectories, http.MethodGet, "/api/downloaders/abc/directories", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("downloader not found", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectories, http.MethodGet, "/api/downloaders/999/directories", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid path shape", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectories, http.MethodGet, "/api/downloaders/1/notdirectories", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectories, http.MethodDelete, "/api/downloaders/1/directories", nil)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestUpdateDownloaderDirectory(t *testing.T) {
	server, db, dlID := setupDirectoryServer(t)

	d1 := models.DownloaderDirectory{DownloaderID: dlID, Path: "/a", IsDefault: true}
	d2 := models.DownloaderDirectory{DownloaderID: dlID, Path: "/b", IsDefault: false}
	require.NoError(t, db.Create(&d1).Error)
	require.NoError(t, db.Create(&d2).Error)

	t.Run("update alias", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectoryDetail, http.MethodPut,
			"/api/downloaders/1/directories/2", DownloaderDirectoryRequest{Path: "/b", Alias: "Beta", IsDefault: false})
		require.Equal(t, http.StatusOK, w.Code)
		var resp DownloaderDirectoryResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "Beta", resp.Alias)
	})

	t.Run("cannot unset only default", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectoryDetail, http.MethodPut,
			"/api/downloaders/1/directories/1", DownloaderDirectoryRequest{Path: "/a", IsDefault: false})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("set d2 as default clears d1", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectoryDetail, http.MethodPut,
			"/api/downloaders/1/directories/2", DownloaderDirectoryRequest{Path: "/b", IsDefault: true})
		require.Equal(t, http.StatusOK, w.Code)

		var d1After models.DownloaderDirectory
		require.NoError(t, db.First(&d1After, d1.ID).Error)
		assert.False(t, d1After.IsDefault)
	})

	t.Run("not found", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectoryDetail, http.MethodPut,
			"/api/downloaders/1/directories/999", DownloaderDirectoryRequest{Path: "/x"})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/1/directories/2", bytes.NewBufferString(`{bad`))
		w := httptest.NewRecorder()
		server.apiDownloaderDirectoryDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestDeleteDownloaderDirectory(t *testing.T) {
	server, db, dlID := setupDirectoryServer(t)

	d1 := models.DownloaderDirectory{DownloaderID: dlID, Path: "/a", IsDefault: true}
	d2 := models.DownloaderDirectory{DownloaderID: dlID, Path: "/b", IsDefault: false}
	require.NoError(t, db.Create(&d1).Error)
	require.NoError(t, db.Create(&d2).Error)

	t.Run("cannot delete default when others exist", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectoryDetail, http.MethodDelete, "/api/downloaders/1/directories/1", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("delete non-default succeeds", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectoryDetail, http.MethodDelete, "/api/downloaders/1/directories/2", nil)
		require.Equal(t, http.StatusOK, w.Code)

		var count int64
		db.Model(&models.DownloaderDirectory{}).Where("id = ?", d2.ID).Count(&count)
		assert.Equal(t, int64(0), count)
	})

	t.Run("delete non-existent", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectoryDetail, http.MethodDelete, "/api/downloaders/1/directories/999", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestSetDefaultDirectory(t *testing.T) {
	server, db, dlID := setupDirectoryServer(t)

	d1 := models.DownloaderDirectory{DownloaderID: dlID, Path: "/a", IsDefault: true}
	d2 := models.DownloaderDirectory{DownloaderID: dlID, Path: "/b", IsDefault: false}
	require.NoError(t, db.Create(&d1).Error)
	require.NoError(t, db.Create(&d2).Error)

	t.Run("set-default via router", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectoryDetail, http.MethodPost, "/api/downloaders/1/directories/2/set-default", nil)
		require.Equal(t, http.StatusOK, w.Code)

		var resp DownloaderDirectoryResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.True(t, resp.IsDefault)

		var d1After models.DownloaderDirectory
		require.NoError(t, db.First(&d1After, d1.ID).Error)
		assert.False(t, d1After.IsDefault)
	})

	t.Run("set-default not found", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectoryDetail, http.MethodPost, "/api/downloaders/1/directories/999/set-default", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("set-default wrong method", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1/directories/2/set-default", nil)
		w := httptest.NewRecorder()
		server.setDefaultDirectory(w, req, dlID, d2.ID)
		// setDefaultDirectory requires POST
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestDownloaderDirectoryDetail_Errors(t *testing.T) {
	server, _, _ := setupDirectoryServer(t)

	t.Run("invalid path", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectoryDetail, http.MethodPut, "/api/downloaders/1/nope", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid downloader id", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectoryDetail, http.MethodPut, "/api/downloaders/abc/directories/1", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid dir id", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectoryDetail, http.MethodPut, "/api/downloaders/1/directories/abc", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid dir id in set-default", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectoryDetail, http.MethodPost, "/api/downloaders/1/directories/abc/set-default", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderDirectoryDetail, http.MethodPatch, "/api/downloaders/1/directories/1", nil)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiDownloaderRouter_Dispatch(t *testing.T) {
	server, db, dlID := setupDirectoryServer(t)
	require.NoError(t, db.Create(&models.DownloaderDirectory{DownloaderID: dlID, Path: "/a", IsDefault: true}).Error)

	t.Run("directories list", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderRouter, http.MethodGet, "/api/downloaders/1/directories", nil)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("directory detail", func(t *testing.T) {
		w := doDirReq(t, server.apiDownloaderRouter, http.MethodPut, "/api/downloaders/1/directories/1", DownloaderDirectoryRequest{Path: "/a", IsDefault: true})
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestApiAllDownloaderDirectories(t *testing.T) {
	server, db, dlID := setupDirectoryServer(t)
	require.NoError(t, db.Create(&models.DownloaderDirectory{DownloaderID: dlID, Path: "/a", IsDefault: true}).Error)

	t.Run("GET returns map", func(t *testing.T) {
		w := doDirReq(t, server.apiAllDownloaderDirectories, http.MethodGet, "/api/downloaders/all-directories", nil)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string][]DownloaderDirectoryResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := doDirReq(t, server.apiAllDownloaderDirectories, http.MethodPost, "/api/downloaders/all-directories", nil)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	_ = global.GlobalDB
}
