// MIT License
// Copyright (c) 2025 pt-tools

// Tests for Issue #299 disk-protection helpers in qbit package:
//   - ComputeTorrentSize: parse content size from .torrent metadata
//   - GetIncompletePendingBytes: aggregate amount_left across active downloads

package qbit

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"
)

// makeSingleFileTorrent encodes a minimal single-file .torrent with given length.
func makeSingleFileTorrent(t *testing.T, length int64) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{
		"info": map[string]any{
			"name":         "single.bin",
			"length":       length,
			"piece length": int64(16384),
			"pieces":       "01234567890123456789",
		},
	}))
	return buf.Bytes()
}

// makeMultiFileTorrent encodes a minimal multi-file .torrent with given file lengths.
func makeMultiFileTorrent(t *testing.T, fileLengths ...int64) []byte {
	t.Helper()
	files := make([]any, 0, len(fileLengths))
	for i, l := range fileLengths {
		files = append(files, map[string]any{
			"length": l,
			"path":   []string{"f" + string(rune('0'+i))},
		})
	}
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{
		"info": map[string]any{
			"name":         "multi",
			"files":        files,
			"piece length": int64(16384),
			"pieces":       "01234567890123456789",
		},
	}))
	return buf.Bytes()
}

func TestComputeTorrentSize_SingleFile(t *testing.T) {
	data := makeSingleFileTorrent(t, 1234567890)
	size, err := ComputeTorrentSize(data)
	require.NoError(t, err)
	assert.Equal(t, int64(1234567890), size)
}

func TestComputeTorrentSize_MultiFile(t *testing.T) {
	data := makeMultiFileTorrent(t, 100, 200, 300)
	size, err := ComputeTorrentSize(data)
	require.NoError(t, err)
	assert.Equal(t, int64(600), size, "multi-file 大小应为 files[].length 之和")
}

func TestComputeTorrentSize_EmptyMultiFile(t *testing.T) {
	data := makeMultiFileTorrent(t)
	size, err := ComputeTorrentSize(data)
	require.NoError(t, err)
	assert.Equal(t, int64(0), size, "无文件 multi 应返回 0，不报错")
}

func TestComputeTorrentSize_InvalidData(t *testing.T) {
	_, err := ComputeTorrentSize([]byte("not bencode"))
	require.Error(t, err)
}

// TestGetIncompletePendingBytes_AggregatesActiveStates 验证 active 状态种子的
// amount_left 被正确聚合。
func TestGetIncompletePendingBytes_AggregatesActiveStates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/torrents/info", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"state": "downloading", "amount_left": 50000000000},
			{"state": "stalledDL",   "amount_left": 30000000000},
			{"state": "queuedDL",    "amount_left": 20000000000},
			{"state": "uploading",   "amount_left": 0},
			{"state": "pausedUP",    "amount_left": 0},
			{"state": "error",       "amount_left": 1000}
		]`))
	}))
	defer server.Close()

	q := NewQbitClientForTesting(server.Client(), server.URL)
	got, err := q.GetIncompletePendingBytes(t.Context())
	require.NoError(t, err)
	// 50G + 30G + 20G = 100G；其他状态（uploading/pausedUP/error）不计入
	assert.Equal(t, int64(100000000000), got)
}

// TestGetIncompletePendingBytes_IncludesPausedDL 验证 pausedDL 计入聚合。
// 与 qBit 的 pausedUP（已完成暂停做种）相反，pausedDL 仍持有未下载完的份额。
func TestGetIncompletePendingBytes_IncludesPausedDL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"state": "pausedDL", "amount_left": 5000000000},
			{"state": "stoppedDL", "amount_left": 3000000000}
		]`))
	}))
	defer server.Close()

	q := NewQbitClientForTesting(server.Client(), server.URL)
	got, err := q.GetIncompletePendingBytes(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(8000000000), got, "pausedDL + stoppedDL 应计入（用户可恢复）")
}

func TestGetIncompletePendingBytes_EmptyList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	q := NewQbitClientForTesting(server.Client(), server.URL)
	got, err := q.GetIncompletePendingBytes(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(0), got)
}

func TestGetIncompletePendingBytes_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	q := NewQbitClientForTesting(server.Client(), server.URL)
	_, err := q.GetIncompletePendingBytes(t.Context())
	require.Error(t, err)
}

func TestGetIncompletePendingBytes_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	q := NewQbitClientForTesting(server.Client(), server.URL)
	_, err := q.GetIncompletePendingBytes(t.Context())
	require.Error(t, err)
}

// TestGetIncompletePendingBytes_SkipsNegativeAmount 验证负数被忽略（不存在但保护性测试）。
func TestGetIncompletePendingBytes_SkipsNegativeAmount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"state": "downloading", "amount_left": -500}]`))
	}))
	defer server.Close()

	q := NewQbitClientForTesting(server.Client(), server.URL)
	got, err := q.GetIncompletePendingBytes(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(0), got, "amount_left<=0 应被忽略")
}
