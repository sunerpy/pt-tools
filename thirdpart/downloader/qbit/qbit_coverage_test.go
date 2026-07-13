package qbit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// coverageTestClient builds a QbitClient pointed at the given test-server URL,
// bypassing NewQbitClient's network Authenticate() so per-method tests stay
// fast and hermetic. version==5.2.0+ toggles the v520 success-status path.
func coverageTestClient(baseURL string, v520 bool) *QbitClient {
	c := &QbitClient{
		name:    "test-qbit",
		baseURL: baseURL,
		client:  &standardHTTPDoer{client: &http.Client{}},
		healthy: true,
	}
	c.versionMu.Lock()
	c.isV520Plus = v520
	if v520 {
		c.appVersion = "v5.2.0"
	}
	c.versionMu.Unlock()
	return c
}

func TestQbitPing(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/app/version", r.URL.Path)
			_, _ = w.Write([]byte("v4.6.5"))
		}))
		defer srv.Close()

		ok, err := coverageTestClient(srv.URL, false).Ping()
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("non-success status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		c := coverageTestClient(srv.URL, false)
		ok, err := c.Ping()
		require.NoError(t, err)
		assert.False(t, ok)
		assert.False(t, c.IsHealthy())
	})

	t.Run("network error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := srv.URL
		srv.Close() // server closed -> connection refused

		ok, err := coverageTestClient(url, false).Ping()
		require.Error(t, err)
		assert.False(t, ok)
	})
}

func TestQbitGetClientVersion(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("v5.0.0"))
		}))
		defer srv.Close()

		v, err := coverageTestClient(srv.URL, false).GetClientVersion()
		require.NoError(t, err)
		assert.Equal(t, "v5.0.0", v)
	})

	t.Run("error status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetClientVersion()
		require.Error(t, err)
	})
}

func TestQbitGetClientStatus(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"server_state": map[string]any{
					"up_info_speed": float64(100),
					"alltime_ul":    float64(2000),
					"dl_info_speed": float64(200),
					"alltime_dl":    float64(4000),
					"up_info_data":  float64(50),
					"dl_info_data":  float64(60),
				},
			})
		}))
		defer srv.Close()

		st, err := coverageTestClient(srv.URL, false).GetClientStatus()
		require.NoError(t, err)
		assert.Equal(t, int64(100), st.UpSpeed)
		assert.Equal(t, int64(2000), st.UpData)
		assert.Equal(t, int64(200), st.DlSpeed)
		assert.Equal(t, int64(4000), st.DlData)
		assert.Equal(t, int64(50), st.SessionUpData)
		assert.Equal(t, int64(60), st.SessionDlData)
	})

	t.Run("missing server_state", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"other":1}`))
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetClientStatus()
		require.Error(t, err)
	})

	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`not json`))
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetClientStatus()
		require.Error(t, err)
	})

	t.Run("error status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetClientStatus()
		require.Error(t, err)
	})
}

func TestQbitGetClientFreeSpace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":1073741824}}`))
	}))
	defer srv.Close()

	got, err := coverageTestClient(srv.URL, false).GetClientFreeSpace(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1073741824), got)
}

func TestQbitGetIncompletePendingBytes(t *testing.T) {
	t.Run("aggregates active states", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/torrents/info", r.URL.Path)
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"state": "downloading", "amount_left": float64(1000)},
				{"state": "pausedDL", "amount_left": float64(500)},
				{"state": "uploading", "amount_left": float64(999)}, // not counted
				{"state": "stalledDL", "amount_left": float64(0)},   // zero skipped
			})
		}))
		defer srv.Close()

		got, err := coverageTestClient(srv.URL, false).GetIncompletePendingBytes(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(1500), got)
	})

	t.Run("http error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetIncompletePendingBytes(context.Background())
		require.Error(t, err)
	})

	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{bad`))
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetIncompletePendingBytes(context.Background())
		require.Error(t, err)
	})
}

func TestQbitGetAllTorrentsAndMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"hash": "h1", "name": "torrent-one", "progress": float64(1.0),
				"ratio": float64(2.5), "added_on": float64(1600000000),
				"save_path": "/downloads", "category": "movies", "tags": "hd,new",
				"state": "uploading", "size": float64(1024), "amount_left": float64(0),
				"upspeed": float64(10), "dlspeed": float64(20), "uploaded": float64(2048),
				"downloaded": float64(4096), "eta": float64(0), "seeding_time": float64(3600),
				"tracker": "http://t.example", "completion_on": float64(1600001000),
				"num_seeds": float64(5), "num_leechs": float64(3), "availability": float64(1.5),
				"content_path": "/downloads/torrent-one",
			},
			{"hash": "h2", "name": "torrent-two", "progress": float64(0.5), "state": "downloading"},
		})
	}))
	defer srv.Close()

	torrents, err := coverageTestClient(srv.URL, false).GetAllTorrents()
	require.NoError(t, err)
	require.Len(t, torrents, 2)

	first := torrents[0]
	assert.Equal(t, "h1", first.ID)
	assert.Equal(t, "h1", first.InfoHash)
	assert.Equal(t, "torrent-one", first.Name)
	assert.True(t, first.IsCompleted)
	assert.Equal(t, 2.5, first.Ratio)
	assert.Equal(t, "/downloads", first.SavePath)
	assert.Equal(t, "movies", first.Category)
	assert.Equal(t, "movies", first.Label)
	assert.Equal(t, "hd,new", first.Tags)
	assert.Equal(t, downloader.TorrentSeeding, first.State)
	assert.Equal(t, int64(1024), first.TotalSize)
	assert.Equal(t, int64(10), first.UploadSpeed)
	assert.Equal(t, int64(20), first.DownloadSpeed)
	assert.Equal(t, int64(2048), first.TotalUploaded)
	assert.Equal(t, int64(4096), first.TotalDownloaded)
	assert.Equal(t, int64(3600), first.SeedingTime)
	assert.Equal(t, "http://t.example", first.Tracker)
	assert.Equal(t, 5, first.NumSeeds)
	assert.Equal(t, 3, first.NumPeers)
	assert.Equal(t, "test-qbit", first.ClientID)

	assert.False(t, torrents[1].IsCompleted)
	assert.Equal(t, downloader.TorrentDownloading, torrents[1].State)
}

func TestQbitGetAllTorrents_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := coverageTestClient(srv.URL, false).GetAllTorrents()
	require.Error(t, err)
}

func TestQbitMapQbitState(t *testing.T) {
	c := coverageTestClient("http://x", false)
	cases := map[string]downloader.TorrentState{
		"downloading":        downloader.TorrentDownloading,
		"forcedDL":           downloader.TorrentDownloading,
		"uploading":          downloader.TorrentSeeding,
		"forcedUP":           downloader.TorrentSeeding,
		"pausedDL":           downloader.TorrentPaused,
		"pausedUP":           downloader.TorrentPaused,
		"stoppedDL":          downloader.TorrentStopped,
		"stoppedUP":          downloader.TorrentStopped,
		"queuedDL":           downloader.TorrentQueued,
		"queuedUP":           downloader.TorrentQueued,
		"checkingDL":         downloader.TorrentChecking,
		"checkingResumeData": downloader.TorrentChecking,
		"error":              downloader.TorrentError,
		"missingFiles":       downloader.TorrentError,
		"somethingElse":      downloader.TorrentUnknown,
	}
	for state, want := range cases {
		assert.Equal(t, want, c.mapQbitState(state), state)
	}
}

func TestQbitGetTorrentsByAndGetTorrent(t *testing.T) {
	body := []map[string]any{
		{"hash": "h1", "name": "a", "progress": float64(1.0), "state": "uploading"},
		{"hash": "h2", "name": "b", "progress": float64(0.3), "state": "downloading"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()
	c := coverageTestClient(srv.URL, false)

	t.Run("no filter returns all", func(t *testing.T) {
		got, err := c.GetTorrentsBy(downloader.TorrentFilter{})
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})

	t.Run("filter by hash", func(t *testing.T) {
		got, err := c.GetTorrentsBy(downloader.TorrentFilter{Hashes: []string{"h2"}})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "h2", got[0].InfoHash)
	})

	t.Run("filter by id", func(t *testing.T) {
		got, err := c.GetTorrentsBy(downloader.TorrentFilter{IDs: []string{"h1"}})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "h1", got[0].ID)
	})

	t.Run("filter by complete", func(t *testing.T) {
		done := true
		got, err := c.GetTorrentsBy(downloader.TorrentFilter{Complete: &done})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.True(t, got[0].IsCompleted)
	})

	t.Run("filter by state", func(t *testing.T) {
		state := downloader.TorrentDownloading
		got, err := c.GetTorrentsBy(downloader.TorrentFilter{State: &state})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, downloader.TorrentDownloading, got[0].State)
	})

	t.Run("GetTorrent found", func(t *testing.T) {
		got, err := c.GetTorrent("h1")
		require.NoError(t, err)
		assert.Equal(t, "h1", got.InfoHash)
	})

	t.Run("GetTorrent not found", func(t *testing.T) {
		_, err := c.GetTorrent("missing")
		require.ErrorIs(t, err, downloader.ErrTorrentNotFound)
	})
}

func TestQbitAddTorrentEx(t *testing.T) {
	t.Run("legacy success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/torrents/add", r.URL.Path)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		res, err := coverageTestClient(srv.URL, false).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
		require.NoError(t, err)
		assert.True(t, res.Success)
	})

	t.Run("legacy error status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		res, err := coverageTestClient(srv.URL, false).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
		require.Error(t, err)
		assert.False(t, res.Success)
	})

	t.Run("v520 json result", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success_count":     1,
				"failure_count":     0,
				"added_torrent_ids": []string{"newhash"},
			})
		}))
		defer srv.Close()

		res, err := coverageTestClient(srv.URL, true).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{
			SavePath: "/dl", Category: "cat", Tags: "t", AddAtPaused: true,
		})
		require.NoError(t, err)
		assert.True(t, res.Success)
		assert.Equal(t, "newhash", res.Hash)
	})

	t.Run("v520 conflict", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
		}))
		defer srv.Close()

		res, err := coverageTestClient(srv.URL, true).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
		require.NoError(t, err)
		assert.False(t, res.Success)
	})
}

func TestQbitPauseResumeRemove(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := coverageTestClient(srv.URL, true)

	require.NoError(t, c.PauseTorrent("h1"))
	require.NoError(t, c.ResumeTorrent("h1"))
	require.NoError(t, c.RemoveTorrent("h1", true))
	require.NoError(t, c.PauseTorrents([]string{"h1", "h2"}))
	require.NoError(t, c.ResumeTorrents([]string{"h1", "h2"}))
	require.NoError(t, c.RemoveTorrents([]string{"h1"}, false))

	assert.Contains(t, paths, "/api/v2/torrents/stop")
	assert.Contains(t, paths, "/api/v2/torrents/start")
	assert.Contains(t, paths, "/api/v2/torrents/delete")
}

func TestQbitPauseResume_EmptyIDs(t *testing.T) {
	c := coverageTestClient("http://unused", true)
	assert.NoError(t, c.PauseTorrents(nil))
	assert.NoError(t, c.ResumeTorrents(nil))
	assert.NoError(t, c.RemoveTorrents(nil, true))
}

func TestQbitPauseResume_LegacyFallback(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		// modern endpoints 404 -> triggers legacy fallback
		if strings.HasSuffix(r.URL.Path, "/stop") || strings.HasSuffix(r.URL.Path, "/start") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := coverageTestClient(srv.URL, false)

	require.NoError(t, c.PauseTorrents([]string{"h1"}))
	require.NoError(t, c.ResumeTorrents([]string{"h1"}))
	assert.Contains(t, paths, "/api/v2/torrents/pause")
	assert.Contains(t, paths, "/api/v2/torrents/resume")
}

func TestQbitSetters(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := coverageTestClient(srv.URL, false)

	require.NoError(t, c.SetTorrentCategory("h1", "movies"))
	require.NoError(t, c.SetTorrentTags("h1", "tag1"))
	require.NoError(t, c.SetTorrentSavePath("h1", "/new/path"))
	require.NoError(t, c.RecheckTorrent("h1"))

	assert.Contains(t, seen, "/api/v2/torrents/setCategory")
	assert.Contains(t, seen, "/api/v2/torrents/addTags")
	assert.Contains(t, seen, "/api/v2/torrents/setLocation")
	assert.Contains(t, seen, "/api/v2/torrents/recheck")
}

func TestQbitSetters_ErrorPropagation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := coverageTestClient(srv.URL, false)

	require.Error(t, c.SetTorrentCategory("h1", "movies"))
	require.Error(t, c.SetTorrentTags("h1", "tag1"))
	require.Error(t, c.SetTorrentSavePath("h1", "/new/path"))
	require.Error(t, c.RecheckTorrent("h1"))
}

func TestQbitGetTorrentFiles(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Contains(t, r.URL.Path, "/api/v2/torrents/files")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"index": float64(0), "name": "a.mkv", "size": float64(1000), "progress": float64(0.5), "priority": float64(1)},
				{"index": float64(1), "name": "b.mkv", "size": float64(2000), "progress": float64(1.0), "priority": float64(7)},
			})
		}))
		defer srv.Close()

		files, err := coverageTestClient(srv.URL, false).GetTorrentFiles("h1")
		require.NoError(t, err)
		require.Len(t, files, 2)
		assert.Equal(t, "a.mkv", files[0].Name)
		assert.Equal(t, int64(1000), files[0].Size)
		assert.Equal(t, 0.5, files[0].Progress)
		assert.Equal(t, 1, files[0].Priority)
		assert.Equal(t, 1, files[1].Index)
	})

	t.Run("error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetTorrentFiles("h1")
		require.Error(t, err)
	})
}

func TestQbitGetTorrentTrackers(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Contains(t, r.URL.Path, "/api/v2/torrents/trackers")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"url": "http://tracker1", "status": float64(2), "num_peers": float64(10),
					"num_seeds": float64(5), "num_leeches": float64(3), "msg": "working",
				},
				{"url": "http://tracker2", "message": "alt-message-field"},
			})
		}))
		defer srv.Close()

		trackers, err := coverageTestClient(srv.URL, false).GetTorrentTrackers("h1")
		require.NoError(t, err)
		require.Len(t, trackers, 2)
		assert.Equal(t, "http://tracker1", trackers[0].URL)
		assert.Equal(t, 2, trackers[0].Status)
		assert.Equal(t, 10, trackers[0].Peers)
		assert.Equal(t, 5, trackers[0].Seeds)
		assert.Equal(t, 3, trackers[0].Leeches)
		assert.Equal(t, "working", trackers[0].Message)
		assert.Equal(t, "alt-message-field", trackers[1].Message)
	})

	t.Run("error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetTorrentTrackers("h1")
		require.Error(t, err)
	})
}

func TestQbitGetDiskInfo(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":5000,"default_save_path":"/dl"}}`))
		}))
		defer srv.Close()

		info, err := coverageTestClient(srv.URL, false).GetDiskInfo()
		require.NoError(t, err)
		assert.Equal(t, int64(5000), info.FreeSpace)
		assert.Equal(t, "/dl", info.Path)
	})

	t.Run("missing server_state", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"x":1}`))
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetDiskInfo()
		require.Error(t, err)
	})
}

func TestQbitGetSpeedLimit(t *testing.T) {
	t.Run("success enabled", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v2/transfer/speedLimitsMode":
				_, _ = w.Write([]byte("1"))
			case "/api/v2/transfer/downloadLimit":
				_, _ = w.Write([]byte("1024"))
			case "/api/v2/transfer/uploadLimit":
				_, _ = w.Write([]byte("2048"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		lim, err := coverageTestClient(srv.URL, false).GetSpeedLimit()
		require.NoError(t, err)
		assert.Equal(t, int64(1024), lim.DownloadLimit)
		assert.Equal(t, int64(2048), lim.UploadLimit)
		assert.True(t, lim.LimitEnabled)
	})

	t.Run("mode request error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetSpeedLimit()
		require.Error(t, err)
	})
}

func TestQbitSetSpeedLimit(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		switch r.URL.Path {
		case "/api/v2/transfer/speedLimitsMode":
			_, _ = w.Write([]byte("0")) // currently disabled
		case "/api/v2/transfer/downloadLimit":
			_, _ = w.Write([]byte("0"))
		case "/api/v2/transfer/uploadLimit":
			_, _ = w.Write([]byte("0"))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	err := coverageTestClient(srv.URL, false).SetSpeedLimit(downloader.SpeedLimit{
		DownloadLimit: 1000, UploadLimit: 2000, LimitEnabled: true,
	})
	require.NoError(t, err)
	assert.Contains(t, seen, "/api/v2/transfer/setDownloadLimit")
	assert.Contains(t, seen, "/api/v2/transfer/setUploadLimit")
	assert.Contains(t, seen, "/api/v2/transfer/toggleSpeedLimitsMode")
}

func TestQbitSetSpeedLimit_DownloadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := coverageTestClient(srv.URL, false).SetSpeedLimit(downloader.SpeedLimit{DownloadLimit: 1})
	require.Error(t, err)
}

func TestQbitGetClientPaths(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/app/preferences", r.URL.Path)
			_, _ = w.Write([]byte(`{"save_path":"/downloads"}`))
		}))
		defer srv.Close()

		paths, err := coverageTestClient(srv.URL, false).GetClientPaths()
		require.NoError(t, err)
		assert.Equal(t, []string{"/downloads"}, paths)
	})

	t.Run("error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetClientPaths()
		require.Error(t, err)
	})
}

func TestQbitGetClientLabels(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/torrents/categories", r.URL.Path)
			_, _ = w.Write([]byte(`{"movies":{"name":"movies"},"tv":{"name":"tv"}}`))
		}))
		defer srv.Close()

		labels, err := coverageTestClient(srv.URL, false).GetClientLabels()
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"movies", "tv"}, labels)
	})

	t.Run("error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetClientLabels()
		require.Error(t, err)
	})
}

func TestQbitEnsureTorrentStarted(t *testing.T) {
	t.Run("no autostart returns nil", func(t *testing.T) {
		c := coverageTestClient("http://unused", false)
		c.autoStart = false
		require.NoError(t, c.EnsureTorrentStarted("h1"))
	})

	t.Run("resumes paused torrent", func(t *testing.T) {
		var resumed bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.Contains(r.URL.Path, "/api/v2/torrents/info"):
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"name": "t", "state": "pausedDL"},
				})
			case strings.Contains(r.URL.Path, "/api/v2/torrents/resume"):
				resumed = true
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer srv.Close()

		c := coverageTestClient(srv.URL, false)
		c.autoStart = true
		require.NoError(t, c.EnsureTorrentStarted("h1"))
		assert.True(t, resumed)
	})

	t.Run("already running", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode([]map[string]any{{"name": "t", "state": "downloading"}})
		}))
		defer srv.Close()

		c := coverageTestClient(srv.URL, false)
		c.autoStart = true
		require.NoError(t, c.EnsureTorrentStarted("h1"))
	})

	t.Run("torrent not found", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`[]`))
		}))
		defer srv.Close()

		c := coverageTestClient(srv.URL, false)
		c.autoStart = true
		require.Error(t, c.EnsureTorrentStarted("h1"))
	})

	t.Run("info error status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		c := coverageTestClient(srv.URL, false)
		c.autoStart = true
		require.Error(t, c.EnsureTorrentStarted("h1"))
	})
}

func TestQbitWrapConnectionError(t *testing.T) {
	c := coverageTestClient("http://x", false)
	cases := []struct{ in, want string }{
		{"connection refused", "连接被拒绝"},
		{"no such host", "无法解析主机名"},
		{"timeout awaiting", "连接超时"},
		{"deadline exceeded", "连接超时"},
		{"x509 certificate signed", "SSL 证书错误"},
		{"some other error", "连接失败"},
	}
	for _, tc := range cases {
		err := c.wrapConnectionError(assertErr(tc.in))
		require.Error(t, err)
		assert.Contains(t, err.Error(), tc.want, tc.in)
	}
}

func TestQbitWrapStatusCodeError(t *testing.T) {
	c := coverageTestClient("http://x", false)
	assert.Contains(t, c.wrapStatusCodeError(http.StatusForbidden).Error(), "403")
	assert.Contains(t, c.wrapStatusCodeError(http.StatusNotFound).Error(), "404")
	assert.Contains(t, c.wrapStatusCodeError(http.StatusUnauthorized).Error(), "401")
	assert.Contains(t, c.wrapStatusCodeError(http.StatusTeapot).Error(), "418")
}

// assertErr is a tiny error helper carrying the given message.
type assertErr string

func (e assertErr) Error() string { return string(e) }
