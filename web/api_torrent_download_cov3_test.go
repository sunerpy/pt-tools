package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// plainV2Site implements v2.Site WITHOUT HashDownloader, to exercise the
// download-hash fallback branch in apiSiteTorrentDownload.
type plainV2Site struct {
	id   string
	data []byte
	err  error
}

func (p *plainV2Site) ID() string        { return p.id }
func (p *plainV2Site) Name() string      { return p.id }
func (p *plainV2Site) Kind() v2.SiteKind { return v2.SiteNexusPHP }
func (p *plainV2Site) Login(context.Context, v2.Credentials) error {
	return nil
}

func (p *plainV2Site) Search(context.Context, v2.SearchQuery) ([]v2.TorrentItem, error) {
	return nil, nil
}

func (p *plainV2Site) GetUserInfo(context.Context) (v2.UserInfo, error) { return v2.UserInfo{}, nil }
func (p *plainV2Site) Download(context.Context, string) ([]byte, error) { return p.data, p.err }
func (p *plainV2Site) Close() error                                     { return nil }

func TestApiSiteTorrentDownload_HashFallbackNonHashSite(t *testing.T) {
	withOrchestrator(t, &plainV2Site{id: "plainsite", data: []byte("plaindata")})

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/site/plainsite/torrent/1/download?downhash=abc", nil)
	s.apiSiteTorrentDownload(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "plaindata", w.Body.String())
}

func TestApiSiteTorrentDownload_EmptyIDs(t *testing.T) {
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky"})
	s := &Server{}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/site//torrent//download", nil)
	s.apiSiteTorrentDownload(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestApiSiteTorrentDownload_OrchestratorNil(t *testing.T) {
	prev := searchOrchestrator
	searchOrchestrator = nil
	t.Cleanup(func() { searchOrchestrator = prev })

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/torrent/1/download", nil)
	s.apiSiteTorrentDownload(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestApiBatchTorrentDownload_DownloadErrorPerItem(t *testing.T) {
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: []byte("ok")},
		&fakeV2Site{id: "badsite", name: "Bad", err: assertErr("dlfail")})

	s := &Server{}
	body := `{"torrents":[{"siteId":"hdsky","torrentId":"1","title":"A"},{"siteId":"badsite","torrentId":"2"}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-download", strings.NewReader(body))
	s.apiBatchTorrentDownload(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/gzip", w.Header().Get("Content-Type"))
}
