package web

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestListRSSNotifications_AllFilters(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	setupChatOpsDB(t)
	now := time.Now()
	require.NoError(t, global.GlobalDB.DB.Create(&models.RSSNotificationLog{
		RSSID: 1, SiteName: "hdsky", TorrentID: "t1", NotifyKind: "new",
		NotificationConfID: 5, Result: "pending", NextRetryAt: &now,
	}).Error)

	resp := chatopsReq(t, srv, http.MethodGet,
		"/api/chatops/rss-notifications?rss_id=1&kind=new&result=pending&conf_id=5&page=1&page_size=10",
		tok, nil)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Total int64 `json:"total"`
	}
	decodeBody(t, resp, &body)
	assert.Equal(t, int64(1), body.Total)
}
