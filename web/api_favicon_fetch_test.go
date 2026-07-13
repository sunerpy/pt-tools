package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestFaviconService_FetchAndSave(t *testing.T) {
	setupFaviconServer(t)

	pngBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	defer ts.Close()

	t.Run("fetches and stores favicon", func(t *testing.T) {
		require.NoError(t, faviconService.fetchAndSave("testsite", "TestSite", ts.URL+"/favicon.png"))

		cache, err := faviconService.GetFavicon("testsite")
		require.NoError(t, err)
		assert.NotEmpty(t, cache.Data)
		assert.NotEmpty(t, cache.ETag)
	})

	t.Run("updates existing record on refetch", func(t *testing.T) {
		require.NoError(t, faviconService.fetchAndSave("testsite", "TestSite", ts.URL+"/favicon.png"))
		var count int64
		global.GlobalDB.DB.Model(&models.FaviconCache{}).Where("site_id = ?", "testsite").Count(&count)
		assert.Equal(t, int64(1), count)
	})
}

func TestFaviconService_FetchAndSave_Errors(t *testing.T) {
	setupFaviconServer(t)

	t.Run("non-200 response", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer ts.Close()
		err := faviconService.fetchAndSave("s", "S", ts.URL)
		assert.Error(t, err)
	})

	t.Run("empty data", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()
		err := faviconService.fetchAndSave("s", "S", ts.URL)
		assert.Error(t, err)
	})
}
