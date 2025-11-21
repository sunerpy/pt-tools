package site

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gocolly/colly"
	"github.com/stretchr/testify/require"
	"github.com/sunerpy/pt-tools/models"
)

func TestCommonFetchMultiTorrents_SuccessAndError(t *testing.T) {
	// one OK html page, one error page
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html><body><div>ok</div></body></html>"))
	}))
	defer okSrv.Close()
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer errSrv.Close()
	c := colly.NewCollector()
	conf := NewSiteMapConfig(models.CMCT, "cookie=1", models.SiteConfig{}, NewCMCTParser())
	urls := []string{okSrv.URL, errSrv.URL}
	out, e := CommonFetchMultiTorrents(context.Background(), c, conf, urls)
	require.NotNil(t, out)
	require.Len(t, out, 1)
	require.Error(t, e)
}

func TestCommonFetchMultiTorrents_Cancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html><body><div>ok</div></body></html>"))
	}))
	defer srv.Close()
	c := colly.NewCollector()
	conf := NewSiteMapConfig(models.CMCT, "cookie=1", models.SiteConfig{}, NewCMCTParser())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, e := CommonFetchMultiTorrents(ctx, c, conf, []string{srv.URL})
	require.Nil(t, out)
	require.Error(t, e)
}
