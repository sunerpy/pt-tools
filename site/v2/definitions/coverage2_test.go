package definitions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// TestGetHDSkyLevelRequirements_ZeroJoinTime covers the joinTime==0 branch.
func TestGetHDSkyLevelRequirements_ZeroJoinTime(t *testing.T) {
	reqs := GetHDSkyLevelRequirements(0)
	require.NotEmpty(t, reqs)
	// zero join time should return the "new" requirements (same as a recent join)
	newReqs := GetHDSkyLevelRequirements(HDSkyNewRequirementsDate.Add(24 * 60 * 60 * 1e9).Unix())
	assert.Equal(t, len(newReqs), len(reqs))
}

// TestCreateRousiDriver_Errors covers createRousiDriver error/success branches.
func TestCreateRousiDriver_MissingPasskey(t *testing.T) {
	_, err := createRousiDriver(v2.SiteConfig{ID: "rousipro", Name: "RousiPro"}, zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Passkey")
}

func TestCreateRousiDriver_BadOptions(t *testing.T) {
	_, err := createRousiDriver(v2.SiteConfig{
		ID:      "rousipro",
		Options: json.RawMessage(`not json`),
	}, zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse Rousi options")
}

func TestCreateRousiDriver_Success(t *testing.T) {
	opts, _ := json.Marshal(v2.RousiOptions{Passkey: "FAKE_PK", Cookie: "c=1"})
	site, err := createRousiDriver(v2.SiteConfig{
		ID:      "rousipro",
		Name:    "RousiPro",
		BaseURL: "https://rousi.pro",
		Options: opts,
	}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, site)
	assert.Equal(t, "rousipro", site.ID())
}

// TestCreateRousiDriver_BaseURLFromDefinition exercises the fallback to definition URLs.
func TestCreateRousiDriver_BaseURLFromDefinition(t *testing.T) {
	opts, _ := json.Marshal(v2.RousiOptions{Passkey: "FAKE_PK"})
	site, err := createRousiDriver(v2.SiteConfig{
		ID:      "rousipro",
		Name:    "RousiPro",
		Options: opts,
		// BaseURL intentionally empty -> should use definition URLs
	}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, site)
}

// TestRousiDriver_ParseSearch_Full covers ParseSearch with items + promotion + times.
func TestRousiDriver_ParseSearch_Full(t *testing.T) {
	d := newTestRousiDriverWithURL("https://rousi.pro")

	body := `{"code":0,"message":"success","data":{"torrents":[
		{"id":1,"uuid":"u1","title":"Movie A","subtitle":"sub","size":1024,"seeders":3,"leechers":1,"downloads":9,
		 "created_at":"2025-01-15T08:30:00+08:00","category_name":"Movies",
		 "promotion":{"type":2,"is_active":true,"until":"2025-02-01T00:00:00+08:00"}},
		{"id":2,"uuid":"u2","title":"Movie B","size":2048,"seeders":5,"leechers":0,"downloads":2,
		 "created_at":"2025-01-16 09:00:00"}
	],"total":2}}`

	_ = d
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	d2 := newTestRousiDriverWithURL(server.URL)
	resp, err := d2.Execute(context.Background(), rousiRequest{Endpoint: "/api/v1/torrents"})
	require.NoError(t, err)
	items, err := d2.ParseSearch(resp)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "u1", items[0].ID)
	assert.Equal(t, "Movie A", items[0].Title)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Greater(t, items[0].UploadedAt, int64(0))
	// second item uses the "2006-01-02 15:04:05" CST fallback path
	assert.Greater(t, items[1].UploadedAt, int64(0))
	assert.Equal(t, v2.DiscountNone, items[1].DiscountLevel)
}

func TestRousiDriver_ParseSearch_APIError(t *testing.T) {
	d := newTestRousiDriverWithURL("https://rousi.pro")
	_, err := d.ParseSearch(rousiResponse{Code: 1, Message: "denied"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "denied")
}

func TestRousiDriver_ParseSearch_BadData(t *testing.T) {
	d := newTestRousiDriverWithURL("https://rousi.pro")
	_, err := d.ParseSearch(rousiResponse{Data: json.RawMessage(`not json`)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse search data")
}

// TestRousiDriver_ParsePromotion_Types covers the various promotion type mappings.
func TestRousiDriver_ParsePromotion_Types(t *testing.T) {
	d := newTestRousiDriverWithURL("https://rousi.pro")

	cases := []struct {
		promo *rousiPromotion
		want  v2.DiscountLevel
	}{
		{nil, v2.DiscountNone},
		{&rousiPromotion{IsActive: false, Type: 2}, v2.DiscountNone},
		{&rousiPromotion{IsActive: true, Type: 0}, v2.DiscountNone},
		{&rousiPromotion{IsActive: true, Type: 3}, v2.Discount2xUp},
		{&rousiPromotion{IsActive: true, Type: 4}, v2.Discount2xFree},
		{&rousiPromotion{IsActive: true, Type: 5}, v2.DiscountPercent50},
		{&rousiPromotion{IsActive: true, Type: 6}, v2.Discount2x50},
		{&rousiPromotion{IsActive: true, Type: 7}, v2.DiscountPercent30},
		{&rousiPromotion{IsActive: true, Type: 99, DownMultiplier: 0, UpMultiplier: 2}, v2.Discount2xFree},
		{&rousiPromotion{IsActive: true, Type: 99, DownMultiplier: 0, UpMultiplier: 1}, v2.DiscountFree},
		{&rousiPromotion{IsActive: true, Type: 99, DownMultiplier: 1, UpMultiplier: 1}, v2.DiscountNone},
	}
	for _, c := range cases {
		got, _ := d.parsePromotion(c.promo)
		assert.Equal(t, c.want, got)
	}
}

// TestRousiDriver_ParsePromotion_UntilLayouts exercises the different time layouts.
func TestRousiDriver_ParsePromotion_UntilLayouts(t *testing.T) {
	d := newTestRousiDriverWithURL("https://rousi.pro")

	_, end := d.parsePromotion(&rousiPromotion{IsActive: true, Type: 2, Until: "2025-02-01T00:00:00+08:00"})
	assert.False(t, end.IsZero())

	_, end2 := d.parsePromotion(&rousiPromotion{IsActive: true, Type: 2, Until: "2025-02-01 00:00:00"})
	assert.False(t, end2.IsZero())

	_, end3 := d.parsePromotion(&rousiPromotion{IsActive: true, Type: 2, Until: "garbage"})
	assert.True(t, end3.IsZero())
}

// TestRousiDriver_GetUserInfo_Full covers the full user data parse with seeding data + times.
func TestRousiDriver_GetUserInfo_Full(t *testing.T) {
	body := `{"code":0,"message":"success","data":{
		"id":9876,"username":"tester","level":2,"level_text":"Power User",
		"uploaded":1099511627776,"downloaded":107374182,"ratio":10.24,
		"karma":5000.5,"credits":123.4,"seeding_karma_per_hour":1.5,"seeding_points_per_hour":2.5,
		"registered_at":"2020-01-01T00:00:00+08:00","last_active_at":"2024-06-01T12:00:00+08:00",
		"seeding_leeching_data":{"seeding_count":120,"seeding_size":5497558138880}
	}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	d := newTestRousiDriverWithURL(server.URL)
	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "9876", info.UserID)
	assert.Equal(t, "tester", info.Username)
	assert.Equal(t, int64(1099511627776), info.Uploaded)
	assert.Equal(t, "Power User", info.LevelName)
	assert.Equal(t, 120, info.SeederCount)
	assert.Equal(t, int64(5497558138880), info.SeederSize)
	assert.Greater(t, info.JoinDate, int64(0))
	assert.Greater(t, info.LastAccess, int64(0))
	assert.InDelta(t, 5000.5, info.Bonus, 0.01)
}

// TestRousiDriver_GetUserInfo_AltTimeLayout covers the "-0700" layout fallback.
func TestRousiDriver_GetUserInfo_AltTimeLayout(t *testing.T) {
	body := `{"code":0,"message":"success","data":{
		"id":1,"username":"t","registered_at":"2020-01-01T00:00:00+0800","last_active_at":"2024-06-01T12:00:00+0800"
	}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	d := newTestRousiDriverWithURL(server.URL)
	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Greater(t, info.JoinDate, int64(0))
	assert.Greater(t, info.LastAccess, int64(0))
}

func TestRousiDriver_GetUserInfo_BadData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":"not-an-object"}`))
	}))
	defer server.Close()

	d := newTestRousiDriverWithURL(server.URL)
	_, err := d.GetUserInfo(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse user data")
}

func TestRousiDriver_GetUserInfo_ExecuteError(t *testing.T) {
	d := newTestRousiDriverWithURL("http://127.0.0.1:1") // connection refused
	_, err := d.GetUserInfo(context.Background())
	require.Error(t, err)
}

// TestRousiDriver_GetSiteID covers both branches of getSiteID.
func TestRousiDriver_GetSiteID(t *testing.T) {
	d := newTestRousiDriverWithURL("https://rousi.pro")
	assert.Equal(t, "rousipro", d.getSiteID())

	def, ok := v2.GetDefinitionRegistry().Get("rousipro")
	require.True(t, ok)
	d.siteDefinition = def
	assert.Equal(t, def.ID, d.getSiteID())
}

// TestExtractUUIDFromLink_Extra covers additional link shapes.
func TestExtractUUIDFromLink_Extra(t *testing.T) {
	assert.Equal(t, "xyz", extractUUIDFromLink("xyz"))
	assert.Equal(t, "last", extractUUIDFromLink("a/b/c/last"))
}
