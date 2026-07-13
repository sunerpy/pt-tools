package v2

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMTorrentDriver_GetSiteDefinition(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	assert.Nil(t, d.GetSiteDefinition())
	def := &SiteDefinition{ID: "mteam"}
	d.SetSiteDefinition(def)
	assert.Equal(t, def, d.GetSiteDefinition())
	assert.Equal(t, "mteam", d.getSiteID())
}

func TestMTorrentDriver_getSiteID_Default(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	assert.NotEmpty(t, d.getSiteID())
}

func TestNewMTorrentDriverWithFailover(t *testing.T) {
	d := NewMTorrentDriverWithFailover("apikey")
	require.NotNil(t, d)
	assert.Equal(t, "apikey", d.APIKey)
}

func TestTorrentResponsePreview(t *testing.T) {
	assert.Equal(t, "hello world", torrentResponsePreview([]byte("hello   world")))
	assert.Contains(t, torrentResponsePreview([]byte("d4:info")), "d4:info")
	long := strings.Repeat("a", 300)
	assert.LessOrEqual(t, len([]rune(torrentResponsePreview([]byte(long)))), 160)
}

func TestMTorrentDriver_ParseDownload_Errors(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})

	// API error code
	_, err := d.ParseDownload(MTorrentResponse{Code: FlexibleCode("1"), Message: "bad"})
	assert.Error(t, err)

	// bad JSON download URL
	_, err = d.ParseDownload(MTorrentResponse{Code: FlexibleCode("0"), Data: json.RawMessage(`{invalid`)})
	assert.Error(t, err)

	// empty URL
	_, err = d.ParseDownload(MTorrentResponse{Code: FlexibleCode("0"), Data: json.RawMessage(`""`)})
	assert.Error(t, err)
}

func TestMTorrentDriver_GetBonusPerHour(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "mybonus")
		w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"formulaParams":{"finalBs":"12.5"}}}`))
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	bph, err := d.GetBonusPerHour(context.Background())
	require.NoError(t, err)
	assert.InDelta(t, 12.5, bph, 0.01)
}

func TestMTorrentDriver_GetUnreadMessageCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"unMake":"3","count":"10"}}`))
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	unread, total, err := d.GetUnreadMessageCount(context.Background())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, unread, 0)
	assert.GreaterOrEqual(t, total, 0)
}

func TestMTorrentDriver_GetPeerStatistics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"seederCount":"10","seederSize":"1073741824","leecherCount":"2","leecherSize":"536870912"}}`))
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	stats, err := d.GetPeerStatistics(context.Background())
	require.NoError(t, err)
	require.NotNil(t, stats)
	assert.Equal(t, 10, stats.SeederCount)
	assert.Equal(t, int64(1073741824), stats.SeederSize)
}

func TestMTorrentDriver_GetUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "member/profile"):
			w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{
				"id":"1001","username":"tester","role":"2","createdDate":"2020-01-01 00:00:00",
				"memberCount":{"uploaded":"10737418240","downloaded":"1073741824","shareRate":"10.0","bonus":"5000"},
				"memberStatus":{"lastBrowse":"2024-06-01 12:00:00"}
			}}`))
		case strings.Contains(r.URL.Path, "mybonus"):
			w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"formulaParams":{"finalBs":"8.8"}}}`))
		case strings.Contains(r.URL.Path, "notify/statistic"):
			w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"unMake":"2","count":"5"}}`))
		case strings.Contains(r.URL.Path, "myPeerStatistics"):
			w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"seederCount":"20","seederSize":"2048","leecherCount":"1","leecherSize":"512"}}`))
		default:
			w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{}}`))
		}
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "1001", info.UserID)
	assert.Equal(t, "tester", info.Username)
	assert.Equal(t, "Power User", info.Rank)
	assert.Equal(t, int64(10737418240), info.Uploaded)
	assert.Equal(t, 20, info.SeederCount)
	assert.Greater(t, info.LastAccess, int64(0))
}

func TestMTorrentDriver_GetUserInfo_ProfileError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":"1","message":"denied"}`))
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	_, err := d.GetUserInfo(context.Background())
	assert.Error(t, err)
}

func TestMTorrentDriver_GetTorrentDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "torrent/detail")
		w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{
			"id":"555","name":"Detail.Movie","size":"2147483648","smallDescr":"desc",
			"status":{"seeders":30,"leechers":3,"timesCompleted":99,"discount":"FREE"}
		}}`))
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	item, err := d.GetTorrentDetail(context.Background(), "555", "", "")
	require.NoError(t, err)
	assert.Equal(t, "555", item.ID)
	assert.Equal(t, "Detail.Movie", item.Title)
	assert.Equal(t, int64(2147483648), item.SizeBytes)
	assert.Equal(t, DiscountFree, item.DiscountLevel)
	assert.Equal(t, []string{"desc"}, item.Tags)
	assert.Equal(t, 30, item.Seeders)
}

func TestMTorrentDriver_GetTorrentDetail_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":"500","message":"error"}`))
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	_, err := d.GetTorrentDetail(context.Background(), "1", "", "")
	assert.Error(t, err)
}
