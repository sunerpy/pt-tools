package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateFromConfigNoFactory(t *testing.T) {
	dm := NewDownloaderManager()
	_, err := dm.CreateFromConfig(&MockConfig{Type: DownloaderQBittorrent, URL: "http://x"}, "temp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no factory registered")
}

func TestCreateFromConfigSuccess(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)
	dl, err := dm.CreateFromConfig(&MockConfig{Type: DownloaderQBittorrent, URL: "http://x"}, "temp")
	require.NoError(t, err)
	assert.Equal(t, "temp", dl.GetName())
	assert.Equal(t, DownloaderQBittorrent, dl.GetType())

	assert.Empty(t, dm.ListDownloaders(), "CreateFromConfig must NOT register the instance")
}

func TestHasFactory(t *testing.T) {
	dm := NewDownloaderManager()
	assert.False(t, dm.HasFactory(DownloaderQBittorrent))
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)
	assert.True(t, dm.HasFactory(DownloaderQBittorrent))
	assert.False(t, dm.HasFactory(DownloaderTransmission))
}

func TestSyncFromDBUnknownTypeSkipped(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)

	records := []DownloaderDBRecord{
		{Name: "known", Type: DownloaderQBittorrent, URL: "http://k", Enabled: true, IsDefault: true},
		{Name: "unknown", Type: DownloaderType("mystery"), URL: "http://u", Enabled: true},
	}
	dm.SyncFromDB(records)

	names := dm.ListDownloaders()
	assert.Contains(t, names, "known")
	assert.NotContains(t, names, "unknown", "records with unregistered type must be skipped")
}

func TestSyncFromDBClearsSiteMappingForRemoved(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)
	dm.RegisterConfig("qbit-1", &MockConfig{Type: DownloaderQBittorrent, URL: "http://x"}, true)
	dm.SetSiteDownloader("siteA", "qbit-1")

	dm.SyncFromDB([]DownloaderDBRecord{})

	_, err := dm.GetDownloaderForSite("siteA")
	require.Error(t, err, "site mapping to a removed downloader must be cleared")
}

func TestSyncFromDBConfigUnchangedKeepsInstance(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)
	dm.RegisterConfig("qbit-1", &GenericConfig{Type: DownloaderQBittorrent, URL: "http://same:8080", Username: "u", Password: "p", AutoStart: true}, true)
	dl, _ := dm.GetDownloader("qbit-1")

	records := []DownloaderDBRecord{
		{Name: "qbit-1", Type: DownloaderQBittorrent, URL: "http://same:8080", Username: "u", Password: "p", AutoStart: true, Enabled: true, IsDefault: true},
	}
	dm.SyncFromDB(records)

	newDl, err := dm.GetDownloader("qbit-1")
	require.NoError(t, err)
	assert.Same(t, dl, newDl, "unchanged config must keep the existing instance")
}

func TestConfigChangedDetectsEachField(t *testing.T) {
	dm := NewDownloaderManager()
	base := &GenericConfig{Type: DownloaderQBittorrent, URL: "http://x", Username: "u", Password: "p", AutoStart: true}
	rec := DownloaderDBRecord{Type: DownloaderQBittorrent, URL: "http://x", Username: "u", Password: "p", AutoStart: true}

	assert.False(t, dm.configChanged(base, rec), "identical → unchanged")

	assert.True(t, dm.configChanged(base, withRec(rec, func(r *DownloaderDBRecord) { r.Type = DownloaderTransmission })))
	assert.True(t, dm.configChanged(base, withRec(rec, func(r *DownloaderDBRecord) { r.URL = "http://y" })))
	assert.True(t, dm.configChanged(base, withRec(rec, func(r *DownloaderDBRecord) { r.Username = "u2" })))
	assert.True(t, dm.configChanged(base, withRec(rec, func(r *DownloaderDBRecord) { r.Password = "p2" })))
	assert.True(t, dm.configChanged(base, withRec(rec, func(r *DownloaderDBRecord) { r.AutoStart = false })))
}

func withRec(r DownloaderDBRecord, mut func(*DownloaderDBRecord)) DownloaderDBRecord {
	mut(&r)
	return r
}

func TestToAddTorrentOptions(t *testing.T) {
	cfg := &GenericConfig{Type: DownloaderQBittorrent, URL: "http://x", AutoStart: true}
	opt := ToAddTorrentOptions(cfg, "movies", "hd,free", "/downloads")
	assert.False(t, opt.AddAtPaused, "autoStart=true → AddAtPaused=false")
	assert.Equal(t, "movies", opt.Category)
	assert.Equal(t, "hd,free", opt.Tags)
	assert.Equal(t, "/downloads", opt.SavePath)

	cfg2 := &GenericConfig{Type: DownloaderQBittorrent, URL: "http://x", AutoStart: false}
	opt2 := ToAddTorrentOptions(cfg2, "", "", "")
	assert.True(t, opt2.AddAtPaused, "autoStart=false → AddAtPaused=true")
}

func TestGenericConfigGetters(t *testing.T) {
	c := NewGenericConfig(DownloaderTransmission, "http://t:9091", "user", "pass", true)
	assert.Equal(t, DownloaderTransmission, c.GetType())
	assert.Equal(t, "http://t:9091", c.GetURL())
	assert.Equal(t, "user", c.GetUsername())
	assert.Equal(t, "pass", c.GetPassword())
	assert.True(t, c.GetAutoStart())
	assert.NoError(t, c.Validate())
}

func TestGenericConfigValidate(t *testing.T) {
	assert.ErrorIs(t, (&GenericConfig{Type: DownloaderQBittorrent}).Validate(), ErrInvalidConfig)
	assert.ErrorIs(t, (&GenericConfig{URL: "http://x"}).Validate(), ErrInvalidConfig)
	assert.NoError(t, (&GenericConfig{Type: DownloaderQBittorrent, URL: "http://x"}).Validate())
}

func TestAddTorrentOptionsEffectiveLimits(t *testing.T) {
	assert.Equal(t, int64(2048), AddTorrentOptions{UploadSpeedLimitKBs: 2}.EffectiveUploadLimitBytes())
	assert.Equal(t, int64(3*1024*1024), AddTorrentOptions{UploadSpeedLimitMB: 3}.EffectiveUploadLimitBytes())
	assert.Equal(t, int64(0), AddTorrentOptions{}.EffectiveUploadLimitBytes())
	assert.Equal(t, int64(5*1024), AddTorrentOptions{DownloadSpeedLimitKBs: 5}.EffectiveDownloadLimitBytes())
	assert.Equal(t, int64(0), AddTorrentOptions{}.EffectiveDownloadLimitBytes())

	both := AddTorrentOptions{UploadSpeedLimitKBs: 2, UploadSpeedLimitMB: 3}
	assert.Equal(t, int64(2048), both.EffectiveUploadLimitBytes(), "KBs takes priority over MB")
}

func TestRequestsHTTPDoerRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "bar", r.Header.Get("X-Foo"))
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pong"))
	}))
	defer srv.Close()

	doer := NewRequestsHTTPDoer(srv.URL, 5*time.Second)
	defer doer.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL, strings.NewReader("ping"))
	require.NoError(t, err)
	req.Header.Set("X-Foo", "bar")

	resp, err := doer.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	buf := make([]byte, 4)
	_, _ = resp.Body.Read(buf)
	assert.Equal(t, "pong", string(buf))
}

func TestRequestsHTTPDoerGetNoBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	doer := NewRequestsHTTPDoer(srv.URL, 5*time.Second)
	defer doer.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := doer.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRequestsHTTPDoerCloseNilSession(t *testing.T) {
	d := &RequestsHTTPDoer{}
	assert.NoError(t, d.Close())
}
