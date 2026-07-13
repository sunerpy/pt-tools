package transmission

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// covClient builds a TransmissionClient pointed at the given test server with a
// preset sessionID so doRequest skips the 409 handshake — keeping per-method
// tests fast and hermetic (no real network, no retry sleeps).
func covClient(baseURL string) *TransmissionClient {
	return &TransmissionClient{
		name:      "test-tr",
		baseURL:   baseURL,
		client:    downloader.NewRequestsHTTPDoer(baseURL, 5_000_000_000),
		sessionID: "sess-1",
		healthy:   true,
	}
}

// rpcServer serves a handler keyed by RPC method, wrapping results in the
// standard {"result":"success","arguments":...} envelope.
func rpcServer(t *testing.T, handlers map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		args, ok := handlers[req.Method]
		if !ok {
			// default empty success
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success"})
			return
		}
		raw, _ := json.Marshal(args)
		_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success", Arguments: raw})
	}))
}

func TestTrPing(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"session-get": map[string]any{"download-dir": "/dl"}})
		defer srv.Close()

		ok, err := covClient(srv.URL).Ping()
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("rpc error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "failure"})
		}))
		defer srv.Close()

		ok, err := covClient(srv.URL).Ping()
		require.Error(t, err)
		assert.False(t, ok)
	})
}

func TestTrGetClientVersion(t *testing.T) {
	srv := rpcServer(t, map[string]any{"session-get": map[string]any{"version": "4.0.5"}})
	defer srv.Close()

	v, err := covClient(srv.URL).GetClientVersion()
	require.NoError(t, err)
	assert.Equal(t, "4.0.5", v)
}

func TestTrGetClientVersion_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := covClient(srv.URL).GetClientVersion()
	require.Error(t, err)
}

func TestTrGetClientStatus(t *testing.T) {
	srv := rpcServer(t, map[string]any{"session-stats": map[string]any{
		"uploadSpeed":      100,
		"downloadSpeed":    200,
		"cumulative-stats": map[string]any{"uploadedBytes": 5000, "downloadedBytes": 9000},
		"current-stats":    map[string]any{"uploadedBytes": 50, "downloadedBytes": 90},
	}})
	defer srv.Close()

	st, err := covClient(srv.URL).GetClientStatus()
	require.NoError(t, err)
	assert.Equal(t, int64(100), st.UpSpeed)
	assert.Equal(t, int64(200), st.DlSpeed)
	assert.Equal(t, int64(5000), st.UpData)
	assert.Equal(t, int64(9000), st.DlData)
	assert.Equal(t, int64(50), st.SessionUpData)
	assert.Equal(t, int64(90), st.SessionDlData)
}

func TestTrGetClientFreeSpace(t *testing.T) {
	srv := rpcServer(t, map[string]any{
		"session-get": map[string]any{"download-dir": "/downloads"},
		"free-space":  freeSpaceResponse{Path: "/downloads", SizeBytes: 2048},
	})
	defer srv.Close()

	got, err := covClient(srv.URL).GetClientFreeSpace(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(2048), got)
}

func TestTrGetDiskSpace_DefaultDirAndZeroSpace(t *testing.T) {
	t.Run("empty download dir uses default", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{
			"session-get": map[string]any{"download-dir": ""},
			"free-space":  freeSpaceResponse{Path: "/downloads", SizeBytes: 4096},
		})
		defer srv.Close()

		got, err := covClient(srv.URL).GetDiskSpace(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(4096), got)
	})

	t.Run("zero free space is an error", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{
			"session-get": map[string]any{"download-dir": "/dl"},
			"free-space":  freeSpaceResponse{Path: "/dl", SizeBytes: 0},
		})
		defer srv.Close()

		_, err := covClient(srv.URL).GetDiskSpace(context.Background())
		require.Error(t, err)
	})
}

func TestTrGetIncompletePendingBytes(t *testing.T) {
	srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{
		"torrents": []map[string]any{
			{"status": 4, "leftUntilDone": 1000},
			{"status": 0, "leftUntilDone": 500},
			{"status": 6, "leftUntilDone": 999}, // seeding, not counted
			{"status": 4, "leftUntilDone": 0},   // zero skipped
		},
	}})
	defer srv.Close()

	got, err := covClient(srv.URL).GetIncompletePendingBytes(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1500), got)
}

func TestTrGetAllTorrentsAndMapping(t *testing.T) {
	srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{
		"torrents": []map[string]any{
			{
				"id": 1, "name": "t1", "hashString": "h1", "status": 6,
				"percentDone": 1.0, "rateDownload": 10, "rateUpload": 20,
				"totalSize": 1024, "leftUntilDone": 0, "uploadedEver": 2048,
				"downloadedEver": 4096, "uploadRatio": 2.0, "addedDate": 1600000000,
				"downloadDir": "/dl", "labels": []string{"movies", "hd"},
				"eta": -1, "secondsSeeding": 3600, "doneDate": 1600001000,
				"peersSendingToUs": 3, "peersGettingFromUs": 5, "peersConnected": 8,
				"desiredAvailable": 1.5,
				"trackers":         []map[string]any{{"announce": "http://tr1"}},
			},
			{"id": 2, "name": "t2", "hashString": "h2", "status": 4, "percentDone": 0.5, "peersConnected": 2},
		},
	}})
	defer srv.Close()

	torrents, err := covClient(srv.URL).GetAllTorrents()
	require.NoError(t, err)
	require.Len(t, torrents, 2)

	first := torrents[0]
	assert.Equal(t, "1", first.ID)
	assert.Equal(t, "h1", first.InfoHash)
	assert.Equal(t, "t1", first.Name)
	assert.True(t, first.IsCompleted)
	assert.Equal(t, downloader.TorrentSeeding, first.State)
	assert.Equal(t, int64(1024), first.TotalSize)
	assert.Equal(t, "movies,hd", first.Tags)
	assert.Equal(t, "movies", first.Category)
	assert.Equal(t, "http://tr1", first.Tracker)
	assert.Equal(t, 5, first.NumSeeds)
	assert.Equal(t, 3, first.NumPeers)
	assert.Equal(t, filepath.Join("/dl", "t1"), first.ContentPath)
	assert.Equal(t, "test-tr", first.ClientID)

	// second torrent: NumPeers falls back to peersConnected
	assert.Equal(t, 2, torrents[1].NumPeers)
	assert.Equal(t, downloader.TorrentDownloading, torrents[1].State)
}

func TestTrMapTransmissionState(t *testing.T) {
	c := covClient("http://x")
	cases := map[int]downloader.TorrentState{
		0:  downloader.TorrentStopped,
		1:  downloader.TorrentChecking,
		2:  downloader.TorrentChecking,
		3:  downloader.TorrentQueued,
		4:  downloader.TorrentDownloading,
		5:  downloader.TorrentQueued,
		6:  downloader.TorrentSeeding,
		99: downloader.TorrentUnknown,
	}
	for status, want := range cases {
		assert.Equal(t, want, c.mapTransmissionState(status), "status=%d", status)
	}
}

func TestTrGetTorrentsByAndGetTorrent(t *testing.T) {
	body := map[string]any{"torrents": []map[string]any{
		{"id": 1, "name": "a", "hashString": "h1", "status": 6, "percentDone": 1.0},
		{"id": 2, "name": "b", "hashString": "h2", "status": 4, "percentDone": 0.3},
	}}
	srv := rpcServer(t, map[string]any{"torrent-get": body})
	defer srv.Close()
	c := covClient(srv.URL)

	all, err := c.GetTorrentsBy(downloader.TorrentFilter{})
	require.NoError(t, err)
	assert.Len(t, all, 2)

	byHash, err := c.GetTorrentsBy(downloader.TorrentFilter{Hashes: []string{"h2"}})
	require.NoError(t, err)
	require.Len(t, byHash, 1)
	assert.Equal(t, "h2", byHash[0].InfoHash)

	byID, err := c.GetTorrentsBy(downloader.TorrentFilter{IDs: []string{"1"}})
	require.NoError(t, err)
	require.Len(t, byID, 1)

	done := true
	byComplete, err := c.GetTorrentsBy(downloader.TorrentFilter{Complete: &done})
	require.NoError(t, err)
	require.Len(t, byComplete, 1)

	state := downloader.TorrentDownloading
	byState, err := c.GetTorrentsBy(downloader.TorrentFilter{State: &state})
	require.NoError(t, err)
	require.Len(t, byState, 1)

	tor, err := c.GetTorrent("h1")
	require.NoError(t, err)
	assert.Equal(t, "h1", tor.InfoHash)

	_, err = c.GetTorrent("missing")
	require.ErrorIs(t, err, downloader.ErrTorrentNotFound)
}

func TestTrCheckTorrentExists(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{
			"torrents": []map[string]any{{"id": 1, "name": "a", "hashString": "abc"}},
		}})
		defer srv.Close()

		got, err := covClient(srv.URL).CheckTorrentExists("abc")
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("not found", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{"torrents": []map[string]any{}}})
		defer srv.Close()

		got, err := covClient(srv.URL).CheckTorrentExists("abc")
		require.NoError(t, err)
		assert.False(t, got)
	})
}

func TestTrAddTorrentEx(t *testing.T) {
	t.Run("added", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-add": torrentAddResponse{
			TorrentAdded: &torrentInfo{ID: 7, Name: "n", HashString: "hh"},
		}})
		defer srv.Close()

		res, err := covClient(srv.URL).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{
			SavePath: "/dl", Category: "cat", Tags: "tag",
		})
		require.NoError(t, err)
		assert.True(t, res.Success)
		assert.Equal(t, "7", res.ID)
		assert.Equal(t, "hh", res.Hash)
	})

	t.Run("duplicate", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-add": torrentAddResponse{
			TorrentDuplicate: &torrentInfo{ID: 3, Name: "dup", HashString: "dd"},
		}})
		defer srv.Close()

		res, err := covClient(srv.URL).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
		require.NoError(t, err)
		assert.True(t, res.Success)
		assert.Equal(t, "dd", res.Hash)
	})

	t.Run("rpc error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "duplicate torrent"})
		}))
		defer srv.Close()

		res, err := covClient(srv.URL).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
		require.Error(t, err)
		assert.False(t, res.Success)
	})
}

func TestTrAddTorrentFileEx(t *testing.T) {
	t.Run("added no limits", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-add": torrentAddResponse{
			TorrentAdded: &torrentInfo{ID: 9, Name: "n", HashString: "hh"},
		}})
		defer srv.Close()

		res, err := covClient(srv.URL).AddTorrentFileEx([]byte("data"), downloader.AddTorrentOptions{
			SavePath: "/dl", Category: "cat", Tags: "tag",
		})
		require.NoError(t, err)
		assert.True(t, res.Success)
		assert.Equal(t, "hh", res.Hash)
	})

	t.Run("with speed limits triggers torrent-set and start", func(t *testing.T) {
		var sawSet, sawStart bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req rpcRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			resp := rpcResponse{Result: "success"}
			switch req.Method {
			case "torrent-add":
				raw, _ := json.Marshal(torrentAddResponse{TorrentAdded: &torrentInfo{ID: 11, HashString: "hh"}})
				resp.Arguments = raw
			case "torrent-set":
				sawSet = true
			case "torrent-start":
				sawStart = true
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer srv.Close()

		res, err := covClient(srv.URL).AddTorrentFileEx([]byte("data"), downloader.AddTorrentOptions{
			UploadSpeedLimitKBs:   500,
			DownloadSpeedLimitKBs: 1000,
			AddAtPaused:           false,
		})
		require.NoError(t, err)
		assert.True(t, res.Success)
		assert.True(t, sawSet, "torrent-set should be called for speed limits")
		assert.True(t, sawStart, "torrent-start should be called when not paused")
	})

	t.Run("duplicate", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-add": torrentAddResponse{
			TorrentDuplicate: &torrentInfo{ID: 5, HashString: "dd"},
		}})
		defer srv.Close()

		res, err := covClient(srv.URL).AddTorrentFileEx([]byte("data"), downloader.AddTorrentOptions{})
		require.NoError(t, err)
		assert.True(t, res.Success)
		assert.Equal(t, "dd", res.Hash)
	})

	t.Run("rpc error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "invalid"})
		}))
		defer srv.Close()

		res, err := covClient(srv.URL).AddTorrentFileEx([]byte("data"), downloader.AddTorrentOptions{})
		require.Error(t, err)
		assert.False(t, res.Success)
	})
}

func TestTrPauseResumeRemove(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		methods = append(methods, req.Method)
		_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success"})
	}))
	defer srv.Close()
	c := covClient(srv.URL)

	require.NoError(t, c.PauseTorrent("1"))
	require.NoError(t, c.ResumeTorrent("2"))
	require.NoError(t, c.RemoveTorrent("3", true))
	require.NoError(t, c.PauseTorrents([]string{"1", "abc"}))
	require.NoError(t, c.ResumeTorrents([]string{"2"}))
	require.NoError(t, c.RemoveTorrents([]string{"3"}, false))

	assert.Contains(t, methods, "torrent-stop")
	assert.Contains(t, methods, "torrent-start")
	assert.Contains(t, methods, "torrent-remove")
}

func TestTrPauseResume_EmptyIDs(t *testing.T) {
	c := covClient("http://unused")
	assert.NoError(t, c.PauseTorrents(nil))
	assert.NoError(t, c.ResumeTorrents(nil))
	assert.NoError(t, c.RemoveTorrents(nil, true))
	assert.NoError(t, c.SetTorrentCategory(" ", "x"))
	assert.NoError(t, c.SetTorrentTags(" ", "x"))
	assert.NoError(t, c.SetTorrentSavePath(" ", "x"))
	assert.NoError(t, c.RecheckTorrent(" "))
}

func TestTrSetters(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		methods = append(methods, req.Method)
		_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success"})
	}))
	defer srv.Close()
	c := covClient(srv.URL)

	require.NoError(t, c.SetTorrentCategory("1", "movies"))
	require.NoError(t, c.SetTorrentTags("1", "a,b"))
	require.NoError(t, c.SetTorrentSavePath("1", "/new"))
	require.NoError(t, c.RecheckTorrent("1"))

	assert.Contains(t, methods, "torrent-set")
	assert.Contains(t, methods, "torrent-set-location")
	assert.Contains(t, methods, "torrent-verify")
}

func TestTrGetTorrentFiles(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{
			"torrents": []map[string]any{{
				"files": []map[string]any{
					{"name": "a.mkv", "length": 1000, "bytesCompleted": 500},
					{"name": "b.mkv", "length": 2000, "bytesCompleted": 2000},
				},
				"fileStats": []map[string]any{
					{"wanted": true, "priority": 1},
					{"wanted": false, "priority": 0},
				},
			}},
		}})
		defer srv.Close()

		files, err := covClient(srv.URL).GetTorrentFiles("1")
		require.NoError(t, err)
		require.Len(t, files, 2)
		assert.Equal(t, "a.mkv", files[0].Name)
		assert.Equal(t, int64(1000), files[0].Size)
		assert.Equal(t, 0.5, files[0].Progress)
		assert.Equal(t, 6, files[0].Priority) // wanted + priority>0
		assert.Equal(t, 0, files[1].Priority) // not wanted
	})

	t.Run("no torrents", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{"torrents": []map[string]any{}}})
		defer srv.Close()

		_, err := covClient(srv.URL).GetTorrentFiles("1")
		require.ErrorIs(t, err, downloader.ErrTorrentNotFound)
	})
}

func TestTrGetTorrentTrackers(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{
			"torrents": []map[string]any{{
				"trackerStats": []map[string]any{
					{
						"announce": "http://tr1", "leecherCount": 3, "seederCount": 5,
						"downloadCount": 10, "announceState": 3, "lastAnnounceSucceeded": true,
					},
					{
						"host": "http://tr2", "lastAnnounceResult": "failed", "lastAnnounceSucceeded": false,
					},
				},
			}},
		}})
		defer srv.Close()

		trackers, err := covClient(srv.URL).GetTorrentTrackers("1")
		require.NoError(t, err)
		require.Len(t, trackers, 2)
		assert.Equal(t, "http://tr1", trackers[0].URL)
		assert.Equal(t, 5, trackers[0].Seeds)
		assert.Equal(t, 3, trackers[0].Leeches)
		assert.Equal(t, "http://tr2", trackers[1].URL) // falls back to host
		assert.Equal(t, 4, trackers[1].Status)         // failed announce
	})

	t.Run("no torrents", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{"torrents": []map[string]any{}}})
		defer srv.Close()

		_, err := covClient(srv.URL).GetTorrentTrackers("1")
		require.ErrorIs(t, err, downloader.ErrTorrentNotFound)
	})
}

func TestTrGetDiskInfo(t *testing.T) {
	srv := rpcServer(t, map[string]any{
		"session-get": map[string]any{"download-dir": "/downloads"},
		"free-space":  freeSpaceResponse{Path: "/downloads", SizeBytes: 8192},
	})
	defer srv.Close()

	info, err := covClient(srv.URL).GetDiskInfo()
	require.NoError(t, err)
	assert.Equal(t, "/downloads", info.Path)
	assert.Equal(t, int64(8192), info.FreeSpace)
}

func TestTrGetDiskInfo_DefaultPath(t *testing.T) {
	srv := rpcServer(t, map[string]any{
		"session-get": map[string]any{"download-dir": ""},
		"free-space":  freeSpaceResponse{Path: "/downloads", SizeBytes: 1},
	})
	defer srv.Close()

	info, err := covClient(srv.URL).GetDiskInfo()
	require.NoError(t, err)
	assert.Equal(t, "/downloads", info.Path)
}

func TestTrGetSpeedLimit(t *testing.T) {
	srv := rpcServer(t, map[string]any{"session-get": map[string]any{
		"speed-limit-down-enabled": true,
		"speed-limit-down":         100,
		"speed-limit-up-enabled":   false,
		"speed-limit-up":           200,
	}})
	defer srv.Close()

	lim, err := covClient(srv.URL).GetSpeedLimit()
	require.NoError(t, err)
	assert.Equal(t, int64(100*1024), lim.DownloadLimit)
	assert.Equal(t, int64(200*1024), lim.UploadLimit)
	assert.True(t, lim.LimitEnabled)
}

func TestTrSetSpeedLimit(t *testing.T) {
	var sawSet bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "session-set" {
			sawSet = true
		}
		_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success"})
	}))
	defer srv.Close()

	err := covClient(srv.URL).SetSpeedLimit(downloader.SpeedLimit{
		DownloadLimit: 1024 * 100, UploadLimit: 1024 * 200, LimitEnabled: true,
	})
	require.NoError(t, err)
	assert.True(t, sawSet)
}

func TestTrGetClientPaths(t *testing.T) {
	srv := rpcServer(t, map[string]any{"session-get": map[string]any{"download-dir": "/downloads"}})
	defer srv.Close()

	paths, err := covClient(srv.URL).GetClientPaths()
	require.NoError(t, err)
	assert.Equal(t, []string{"/downloads"}, paths)
}

func TestTrGetClientLabels(t *testing.T) {
	labels, err := covClient("http://unused").GetClientLabels()
	require.NoError(t, err)
	assert.Empty(t, labels)
}

func TestTrEnsureTorrentStarted(t *testing.T) {
	t.Run("no autostart", func(t *testing.T) {
		c := covClient("http://unused")
		c.autoStart = false
		require.NoError(t, c.EnsureTorrentStarted("h1"))
	})

	t.Run("starts stopped torrent", func(t *testing.T) {
		var started bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req rpcRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			resp := rpcResponse{Result: "success"}
			switch req.Method {
			case "torrent-get":
				raw, _ := json.Marshal(map[string]any{
					"torrents": []map[string]any{{"id": 1, "name": "t", "hashString": "h1", "status": 0}},
				})
				resp.Arguments = raw
			case "torrent-start":
				started = true
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer srv.Close()

		c := covClient(srv.URL)
		c.autoStart = true
		require.NoError(t, c.EnsureTorrentStarted("h1"))
		assert.True(t, started)
	})

	t.Run("already running", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{
			"torrents": []map[string]any{{"id": 1, "name": "t", "hashString": "h1", "status": 4}},
		}})
		defer srv.Close()

		c := covClient(srv.URL)
		c.autoStart = true
		require.NoError(t, c.EnsureTorrentStarted("h1"))
	})

	t.Run("not found", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{"torrents": []map[string]any{}}})
		defer srv.Close()

		c := covClient(srv.URL)
		c.autoStart = true
		require.Error(t, c.EnsureTorrentStarted("h1"))
	})
}

func TestTrWrapConnectionError(t *testing.T) {
	c := covClient("http://x")
	cases := []struct{ in, want string }{
		{"connection refused", "连接被拒绝"},
		{"no such host", "无法解析主机名"},
		{"timeout", "连接超时"},
		{"deadline exceeded", "连接超时"},
		{"x509 certificate", "SSL 证书错误"},
		{"weird", "连接失败"},
	}
	for _, tc := range cases {
		err := c.wrapConnectionError(errString(tc.in))
		require.Error(t, err)
		assert.Contains(t, err.Error(), tc.want, tc.in)
	}
}

func TestTrWrapStatusCodeError(t *testing.T) {
	c := covClient("http://x")
	assert.Contains(t, c.wrapStatusCodeError(http.StatusForbidden).Error(), "403")
	assert.Contains(t, c.wrapStatusCodeError(http.StatusNotFound).Error(), "404")
	assert.Contains(t, c.wrapStatusCodeError(http.StatusTeapot).Error(), "418")
}

func TestTrNormalizeAndSplitLabels(t *testing.T) {
	ids := normalizeTransmissionIDs([]string{"1", " 2 ", "", "abc"})
	require.Len(t, ids, 3)
	assert.Equal(t, 1, ids[0])
	assert.Equal(t, 2, ids[1])
	assert.Equal(t, "abc", ids[2])

	assert.Nil(t, splitLabels(""))
	assert.Equal(t, []string{"a", "b"}, splitLabels("a, b ,"))
}

func TestTrProcessSingleTorrentFile(t *testing.T) {
	// AddTorrent sleeps 500ms after adding; keep a single new-torrent flow.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := rpcResponse{Result: "success"}
		switch req.Method {
		case "session-get":
			resp.Arguments, _ = json.Marshal(map[string]any{"download-dir": "/dl"})
		case "free-space":
			resp.Arguments, _ = json.Marshal(freeSpaceResponse{Path: "/dl", SizeBytes: 1 << 40})
		case "torrent-get":
			resp.Arguments, _ = json.Marshal(map[string]any{"torrents": []map[string]any{}})
		case "torrent-add":
			resp.Arguments, _ = json.Marshal(torrentAddResponse{TorrentAdded: &torrentInfo{ID: 1, HashString: "hh"}})
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()
	c := covClient(srv.URL)
	c.autoStart = false

	dir := t.TempDir()
	path := filepath.Join(dir, "x.torrent")
	require.NoError(t, os.WriteFile(path, makeTorrentBytes(t), 0o644))

	require.NoError(t, c.ProcessSingleTorrentFile(context.Background(), path, "cat", "tag"))
}

func TestTrProcessTorrentFile_Errors(t *testing.T) {
	c := covClient("http://unused")
	require.Error(t, c.processTorrentFile(context.Background(), "/nonexistent.torrent", "", ""))

	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.torrent")
	require.NoError(t, os.WriteFile(bad, []byte("not bencode"), 0o644))
	require.Error(t, c.processTorrentFile(context.Background(), bad, "", ""))
}

func TestTrDoRequest_ErrorPaths(t *testing.T) {
	t.Run("client closed", func(t *testing.T) {
		c := covClient("http://unused")
		c.client = nil
		_, err := c.doRequest("session-get", nil)
		require.Error(t, err)
	})

	t.Run("non-200 status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).doRequest("session-get", nil)
		require.Error(t, err)
	})

	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("{bad"))
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).doRequest("session-get", nil)
		require.Error(t, err)
	})

	t.Run("409 refreshes session and retries", func(t *testing.T) {
		var calls int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 1 {
				w.Header().Set("X-Transmission-Session-Id", "new-sess")
				w.WriteHeader(http.StatusConflict)
				return
			}
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success"})
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).doRequest("session-get", nil)
		require.NoError(t, err)
		assert.Equal(t, 2, calls)
	})
}

func TestTrGetClientStatus_Errors(t *testing.T) {
	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success", Arguments: json.RawMessage("not json")})
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).GetClientStatus()
		require.Error(t, err)
	})

	t.Run("rpc error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "failure"})
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).GetClientStatus()
		require.Error(t, err)
	})
}

func TestTrGetDiskInfo_Errors(t *testing.T) {
	t.Run("session-get rpc error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "failure"})
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).GetDiskInfo()
		require.Error(t, err)
	})

	t.Run("free-space rpc error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req rpcRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Method == "free-space" {
				_ = json.NewEncoder(w).Encode(rpcResponse{Result: "failure"})
				return
			}
			raw, _ := json.Marshal(map[string]any{"download-dir": "/dl"})
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success", Arguments: raw})
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).GetDiskInfo()
		require.Error(t, err)
	})
}

func TestTrGetSpeedLimit_Errors(t *testing.T) {
	t.Run("rpc error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "failure"})
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).GetSpeedLimit()
		require.Error(t, err)
	})

	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success", Arguments: json.RawMessage("bad")})
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).GetSpeedLimit()
		require.Error(t, err)
	})
}

func TestTrSetSpeedLimit_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(rpcResponse{Result: "failure"})
	}))
	defer srv.Close()

	err := covClient(srv.URL).SetSpeedLimit(downloader.SpeedLimit{})
	require.Error(t, err)
}

func TestTrGetClientPaths_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(rpcResponse{Result: "failure"})
	}))
	defer srv.Close()

	_, err := covClient(srv.URL).GetClientPaths()
	require.Error(t, err)
}

func TestTrMapTransmissionTrackerStatus(t *testing.T) {
	assert.Equal(t, 4, mapTransmissionTrackerStatus(transmissionTrackerStat{
		LastAnnounceSucceeded: false, LastAnnounceResult: "err",
	}))
	assert.Equal(t, 2, mapTransmissionTrackerStatus(transmissionTrackerStat{
		LastAnnounceSucceeded: true, AnnounceState: 3,
	}))
	assert.Equal(t, 3, mapTransmissionTrackerStatus(transmissionTrackerStat{
		LastAnnounceSucceeded: true, AnnounceState: 1,
	}))
	assert.Equal(t, 1, mapTransmissionTrackerStatus(transmissionTrackerStat{
		LastAnnounceSucceeded: true, AnnounceState: 0,
	}))
}

func TestTrMapTransmissionPriority(t *testing.T) {
	assert.Equal(t, 0, mapTransmissionPriority(false, 5))
	assert.Equal(t, 6, mapTransmissionPriority(true, 1))
	assert.Equal(t, 1, mapTransmissionPriority(true, 0))
	assert.Equal(t, 1, mapTransmissionPriority(true, -1))
}

type errString string

func (e errString) Error() string { return string(e) }

func makeTorrentBytes(t *testing.T) []byte {
	t.Helper()
	return []byte("d8:announce35:http://tracker.example.com/announce4:infod6:lengthi1024e4:name8:test.txt12:piece lengthi16384e6:pieces20:01234567890123456789ee")
}
