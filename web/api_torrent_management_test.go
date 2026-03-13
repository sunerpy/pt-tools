package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// TestApiDeleteTasks 测试批量删除任务API
func TestApiDeleteTasks(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*testing.T, *gorm.DB)
		request   *DeleteTasksRequest
		checkResp func(*testing.T, *http.Response, *DeleteTasksResponse)
	}{
		{
			name: "Delete single unpushed record",
			setup: func(t *testing.T, db *gorm.DB) {
				// 创建一条未推送的记录
				torrent := models.TorrentInfo{
					SiteName:     "test-site",
					TorrentID:    "123",
					Title:        "Test Torrent 1",
					IsPushed:     boolPtr(false),
					IsFree:       true,
					IsDownloaded: false,
				}
				if err := db.Create(&torrent).Error; err != nil {
					t.Fatalf("failed to create test torrent: %v", err)
				}
			},
			request: &DeleteTasksRequest{IDs: []uint{1}},
			checkResp: func(t *testing.T, resp *http.Response, body *DeleteTasksResponse) {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				assert.Equal(t, 1, body.Success)
				assert.Equal(t, 0, body.Failed)
				assert.Empty(t, body.FailedIDs)
				assert.Empty(t, body.FailedErrors)

				// 验证数据库中记录已删除
				var count int64
				global.GlobalDB.DB.Model(&models.TorrentInfo{}).Where("id = ?", 1).Count(&count)
				assert.Equal(t, int64(0), count)
			},
		},
		{
			name: "Delete multiple unpushed records",
			setup: func(t *testing.T, db *gorm.DB) {
				// 创建3条未推送的记录
				torrents := []models.TorrentInfo{
					{
						SiteName:     "test-site",
						TorrentID:    "123",
						Title:        "Test Torrent 1",
						IsPushed:     boolPtr(false),
						IsFree:       true,
						IsDownloaded: false,
					},
					{
						SiteName:     "test-site",
						TorrentID:    "124",
						Title:        "Test Torrent 2",
						IsPushed:     boolPtr(false),
						IsFree:       false,
						IsDownloaded: false,
					},
					{
						SiteName:     "test-site",
						TorrentID:    "125",
						Title:        "Test Torrent 3",
						IsPushed:     boolPtr(false),
						IsFree:       true,
						IsDownloaded: true,
					},
				}
				for _, torr := range torrents {
					if err := db.Create(&torr).Error; err != nil {
						panic("failed to create test torrents")
					}
				}
			},
			request: &DeleteTasksRequest{IDs: []uint{1, 2, 3}},
			checkResp: func(t *testing.T, resp *http.Response, body *DeleteTasksResponse) {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				assert.Equal(t, 3, body.Success)
				assert.Equal(t, 0, body.Failed)
				assert.Empty(t, body.FailedIDs)
				assert.Empty(t, body.FailedErrors)

				// 验证所有记录已删除
				var count int64
				global.GlobalDB.DB.Model(&models.TorrentInfo{}).Count(&count)
				assert.Equal(t, int64(0), count)
			},
		},
		{
			name: "Attempt to delete pushed record",
			setup: func(t *testing.T, db *gorm.DB) {
				// 创建2条记录：一条已推送，一条未推送
				unpushed := models.TorrentInfo{
					SiteName:     "test-site",
					TorrentID:    "126",
					Title:        "Unpushed Torrent",
					IsPushed:     boolPtr(false),
					IsFree:       true,
					IsDownloaded: false,
				}
				pushed := models.TorrentInfo{
					SiteName:     "test-site",
					TorrentID:    "127",
					Title:        "Pushed Torrent",
					IsPushed:     boolPtr(true),
					IsFree:       true,
					IsDownloaded: false,
				}
				if err := db.Create(&unpushed).Error; err != nil {
					t.Fatalf("failed to create unpushed torrent: %v", err)
				}
				if err := db.Create(&pushed).Error; err != nil {
					t.Fatalf("failed to create pushed torrent: %v", err)
				}
			},
			request: &DeleteTasksRequest{IDs: []uint{1, 2}},
			checkResp: func(t *testing.T, resp *http.Response, body *DeleteTasksResponse) {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				// Query filters is_pushed != true, so pushed record (ID:2) is not returned
				// Only unpushed record (ID:1) is deleted
				assert.Equal(t, 1, body.Success)
				assert.Equal(t, 0, body.Failed)
				assert.Empty(t, body.FailedIDs)
				assert.Empty(t, body.FailedErrors)

				// Verify pushed record still exists
				var count int64
				global.GlobalDB.DB.Model(&models.TorrentInfo{}).Count(&count)
				assert.Equal(t, int64(1), count) // Only pushed record remains
			},
		},
		{
			name: "Delete with empty IDs array",
			setup: func(t *testing.T, db *gorm.DB) {
				// Create 2 unpushed records
				torrents := []models.TorrentInfo{
					{
						SiteName:     "test-site",
						TorrentID:    "128",
						Title:        "Test Torrent 1",
						IsPushed:     boolPtr(false),
						IsFree:       true,
						IsDownloaded: false,
					},
					{
						SiteName:     "test-site",
						TorrentID:    "129",
						Title:        "Test Torrent 2",
						IsPushed:     boolPtr(false),
						IsFree:       false,
						IsDownloaded: false,
					},
				}
				for _, torr := range torrents {
					if err := db.Create(&torr).Error; err != nil {
						t.Fatalf("failed to create test torrent: %v", err)
					}
				}
			},
			request: &DeleteTasksRequest{IDs: []uint{}},
			checkResp: func(t *testing.T, resp *http.Response, body *DeleteTasksResponse) {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				// Empty IDs means delete ALL unpushed records (no ID filter)
				assert.Equal(t, 2, body.Success)
				assert.Equal(t, 0, body.Failed)
				assert.Empty(t, body.FailedIDs)
				assert.Empty(t, body.FailedErrors)

				// Verify all records deleted
				var count int64
				global.GlobalDB.DB.Model(&models.TorrentInfo{}).Count(&count)
				assert.Equal(t, int64(0), count)
			},
		},
		{
			name: "Delete record with nil IsPushed (default value)",
			setup: func(t *testing.T, db *gorm.DB) {
				// 创建 IsPushed 为 nil 的记录（真实场景默认值）
				torrent := models.TorrentInfo{
					SiteName:     "test-site",
					TorrentID:    "130",
					Title:        "Nil IsPushed Torrent",
					IsPushed:     nil, // 未设置，数据库中为 NULL
					IsFree:       true,
					IsDownloaded: false,
				}
				if err := db.Create(&torrent).Error; err != nil {
					t.Fatalf("failed to create test torrent: %v", err)
				}
			},
			request: &DeleteTasksRequest{IDs: []uint{1}},
			checkResp: func(t *testing.T, resp *http.Response, body *DeleteTasksResponse) {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				assert.Equal(t, 1, body.Success)
				assert.Equal(t, 0, body.Failed)
				assert.Empty(t, body.FailedIDs)
				assert.Empty(t, body.FailedErrors)

				// 验证数据库中记录已删除
				var count int64
				global.GlobalDB.DB.Model(&models.TorrentInfo{}).Where("id = ?", 1).Count(&count)
				assert.Equal(t, int64(0), count)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 建立测试服务器和数据库
			server, db := setupTestServer(t)

			// 迁移TorrentInfo表
			if err := db.AutoMigrate(&models.TorrentInfo{}); err != nil {
				t.Fatalf("failed to migrate table: %v", err)
			}

			// 设置测试数据
			tt.setup(t, db)

			// 准备请求
			reqBody, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/api/tasks/batch-delete", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// 执行API调用
			server.apiDeleteTasks(w, req)

			// 解析响应
			var respBody DeleteTasksResponse
			err := json.NewDecoder(w.Body).Decode(&respBody)
			if err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			// 验证响应
			tt.checkResp(t, w.Result(), &respBody)
		})
	}
}

// boolPtr 辅助函数，将bool转换为*bool
func boolPtr(b bool) *bool {
	return &b
}
