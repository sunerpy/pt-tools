package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// /api/version 告诉网页这个库是不是从 v1 升级上来的：「v2.0 升级完成」的说明只给升级上来的用户看，全新安装不显示
func TestApiVersionReportsUpgradeFromV1(t *testing.T) {
	get := func() map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		(&Server{}).apiVersion(w, httptest.NewRequest(http.MethodGet, "/api/version", nil))
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp
	}

	orig := global.GlobalDB
	t.Cleanup(func() { global.GlobalDB = orig })
	global.GlobalDB = nil
	assert.Equal(t, false, get()["upgraded_from_v1"], "没有库时当成全新安装")

	db, err := core.OpenTestDB(t)
	require.NoError(t, err)
	global.GlobalDB = db
	resp := get()
	assert.Equal(t, false, resp["upgraded_from_v1"], "全新安装没有 v2 迁移的记录")
	assert.Contains(t, resp, "version")

	require.NoError(t, db.DB.AutoMigrate(&models.MigrationState{}))
	assert.Equal(t, false, get()["upgraded_from_v1"])
	require.NoError(t, models.UpsertMigrationState(db.DB, core.V2BroadcastSchemaVersion, time.Now()))
	assert.Equal(t, true, get()["upgraded_from_v1"])
}
