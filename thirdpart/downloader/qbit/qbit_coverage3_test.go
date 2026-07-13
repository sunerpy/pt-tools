package qbit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestQbitAddTorrentFileEx_LegacyErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	res, err := coverageTestClient(srv.URL, false).AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

func TestQbitAddTorrentFileEx_V520Fails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Fails."))
	}))
	defer srv.Close()

	res, err := coverageTestClient(srv.URL, true).AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

func TestQbitAddTorrentFileEx_V520PlainOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Ok."))
	}))
	defer srv.Close()

	res, err := coverageTestClient(srv.URL, true).AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{})
	require.NoError(t, err)
	assert.True(t, res.Success)
}

func TestQbitAddTorrentFileEx_V520ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	res, err := coverageTestClient(srv.URL, true).AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

func TestQbitAddTorrentEx_V520Fails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Fails."))
	}))
	defer srv.Close()

	res, err := coverageTestClient(srv.URL, true).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

func TestQbitGetSpeedLimit_DownloadLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/transfer/speedLimitsMode":
			_, _ = w.Write([]byte("1"))
		case "/api/v2/transfer/downloadLimit":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	_, err := coverageTestClient(srv.URL, false).GetSpeedLimit()
	require.Error(t, err)
}

func TestQbitGetSpeedLimit_UploadLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/transfer/speedLimitsMode":
			_, _ = w.Write([]byte("1"))
		case "/api/v2/transfer/downloadLimit":
			_, _ = w.Write([]byte("100"))
		case "/api/v2/transfer/uploadLimit":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	_, err := coverageTestClient(srv.URL, false).GetSpeedLimit()
	require.Error(t, err)
}

func TestQbitSetSpeedLimit_UploadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/transfer/setUploadLimit" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := coverageTestClient(srv.URL, false).SetSpeedLimit(downloader.SpeedLimit{UploadLimit: 1})
	require.Error(t, err)
}

func TestQbitCheckTorrentExists(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"save_path":"/downloads"}`))
		}))
		defer srv.Close()

		got, err := coverageTestClient(srv.URL, false).CheckTorrentExists("hash1")
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("not found", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		got, err := coverageTestClient(srv.URL, false).CheckTorrentExists("hash1")
		require.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("error status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).CheckTorrentExists("hash1")
		require.Error(t, err)
	})

	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{bad`))
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).CheckTorrentExists("hash1")
		require.Error(t, err)
	})
}
