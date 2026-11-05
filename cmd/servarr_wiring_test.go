package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/web"
)

// Servarr 兼容入口：订阅或令牌没有起来时不建；建好后挂在 Web 服务上，根目录是整理服务里启用的媒体库。
func TestWireServarr(t *testing.T) {
	db := organizeTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&models.MediaQualityProfile{}, &models.MediaSubscription{}, &models.MediaSubscriptionTorrent{},
		&models.MediaSubscribeSetting{}, &models.MediaDoubanSource{}, &models.MediaDoubanItem{},
		&models.MediaSubscriptionEpisode{}, &models.MediaEpisodeRule{}, &models.MediaSubscriptionRun{},
		&models.MediaTorrentBlock{}, &models.APIToken{}, &models.ServarrTag{}, &models.ServarrTagLink{}, &models.ActionAudit{},
	))
	store := core.NewConfigStore(global.GlobalDB)
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	ctx := context.Background()
	// 识别服务接上假 TMDB：兼容入口查找电影时用的是它
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/3/movie/278" {
			_, _ = w.Write([]byte(`{"id":278,"title":"肖申克的救赎","original_title":"The Shawshank Redemption","release_date":"1994-09-23"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(fake.Close)
	media := recognize.New(recognize.Config{DB: db, Cipher: subscribeTestCipher{}, BaseURL: fake.URL + "/3", RatePerSecond: 1000})
	tmdbKey := "0123456789abcdef0123456789abcdef"
	_, err := media.SaveSettings(ctx, recognize.SettingsInput{TMDBKey: &tmdbKey})
	require.NoError(t, err)
	subs := newSubscribeService(ctx, mgr, media, nil, nil, nil)
	require.NotNil(t, subs)
	tokens, audit := apitoken.New(db), app.NewAuditService(db)
	srv := web.NewServer(store, mgr)

	assert.Nil(t, wireServarr(db, srv, nil, nil, media, tokens, audit), "没有订阅服务")
	assert.Nil(t, wireServarr(db, srv, subs, nil, media, nil, audit), "没有令牌库")
	assert.Nil(t, wireServarr(nil, srv, subs, nil, media, tokens, audit))

	organizer := newOrganizeService(store, mgr, media, nil, nil)
	require.NotNil(t, organizer)
	t.Cleanup(organizer.Stop)
	lib, err := organizer.SaveLibrary(ctx, 0, organize.LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: t.TempDir(), Enabled: true})
	require.NoError(t, err)
	sv := wireServarr(db, srv, subs, organizer, media, tokens, audit)
	require.NotNil(t, sv)
	_, key, err := tokens.Create(ctx, apitoken.CreateInput{Name: "seerr", Scopes: []string{apitoken.ScopeServarr}}, "admin")
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	sv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/servarr/radarr/api/v3/rootfolder?apikey="+key, nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var folders []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &folders))
	require.Len(t, folders, 2)
	assert.Equal(t, "auto", folders[0]["path"])
	assert.Equal(t, lib.Path, folders[1]["path"])

	rec = httptest.NewRecorder()
	sv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/servarr/radarr/api/v3/movie/lookup?term=tmdb:278&apikey="+key, nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "肖申克的救赎")

	// 写操作进操作审计（通道 servarr）
	rec = httptest.NewRecorder()
	sv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/servarr/radarr/api/v3/tag?apikey="+key, strings.NewReader(`{"label":"1-admin"}`)))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var row models.ActionAudit
	require.NoError(t, db.Where("channel_type = ?", "servarr").First(&row).Error)
	assert.Equal(t, "POST /servarr/radarr/api/v3/tag", row.Command)
	assert.Equal(t, "success", row.Result)
}
