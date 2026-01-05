package v2

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"
)

// createTestTorrent creates a minimal valid torrent file for testing
func createTestTorrent(name string) []byte {
	metainfo := map[string]interface{}{
		"announce": "http://tracker.example.com/announce",
		"info": map[string]interface{}{
			"name":         name,
			"piece length": int64(262144),
			"pieces":       string(make([]byte, 20)), // One piece hash
			"length":       int64(1024),
		},
	}

	data, _ := bencode.EncodeBytes(metainfo)
	return data
}

// createMultiFileTorrent creates a multi-file torrent for testing
func createMultiFileTorrent(name string, files []torrentFileInfo) []byte {
	metainfo := map[string]interface{}{
		"announce": "http://tracker.example.com/announce",
		"info": map[string]interface{}{
			"name":         name,
			"piece length": int64(262144),
			"pieces":       string(make([]byte, 20)),
			"files":        files,
		},
	}

	data, _ := bencode.EncodeBytes(metainfo)
	return data
}

func TestComputeTorrentHash(t *testing.T) {
	data := createTestTorrent("test-torrent")

	hash, err := ComputeTorrentHash(data)
	require.NoError(t, err)
	assert.Len(t, hash, 40)
	assert.True(t, IsTorrentHash(hash))
}

func TestComputeTorrentHash_Empty(t *testing.T) {
	_, err := ComputeTorrentHash(nil)
	assert.ErrorIs(t, err, ErrInvalidTorrent)

	_, err = ComputeTorrentHash([]byte{})
	assert.ErrorIs(t, err, ErrInvalidTorrent)
}

func TestComputeTorrentHash_Invalid(t *testing.T) {
	_, err := ComputeTorrentHash([]byte("not a torrent"))
	assert.Error(t, err)
}

func TestComputeTorrentHash_Idempotent(t *testing.T) {
	data := createTestTorrent("test-torrent")

	hash1, err := ComputeTorrentHash(data)
	require.NoError(t, err)

	hash2, err := ComputeTorrentHash(data)
	require.NoError(t, err)

	assert.Equal(t, hash1, hash2)
}

func TestParseTorrent(t *testing.T) {
	data := createTestTorrent("test-torrent")

	parsed, err := ParseTorrent(data)
	require.NoError(t, err)

	assert.Equal(t, "test-torrent", parsed.Name)
	assert.Len(t, parsed.InfoHash, 40)
	assert.Equal(t, int64(1024), parsed.Size)
	assert.Equal(t, int64(262144), parsed.PieceLength)
	assert.Equal(t, "http://tracker.example.com/announce", parsed.Announce)
	assert.Len(t, parsed.Files, 1)
	assert.Equal(t, "test-torrent", parsed.Files[0].Path)
}

func TestParseTorrent_MultiFile(t *testing.T) {
	files := []torrentFileInfo{
		{Length: 1000, Path: []string{"folder", "file1.txt"}},
		{Length: 2000, Path: []string{"folder", "file2.txt"}},
	}
	data := createMultiFileTorrent("multi-file-torrent", files)

	parsed, err := ParseTorrent(data)
	require.NoError(t, err)

	assert.Equal(t, "multi-file-torrent", parsed.Name)
	assert.Equal(t, int64(3000), parsed.Size)
	assert.Len(t, parsed.Files, 2)
	assert.Equal(t, "folder/file1.txt", parsed.Files[0].Path)
	assert.Equal(t, int64(1000), parsed.Files[0].Length)
}

func TestParseTorrent_Empty(t *testing.T) {
	_, err := ParseTorrent(nil)
	assert.ErrorIs(t, err, ErrInvalidTorrent)
}

func TestExtractMagnetHash(t *testing.T) {
	tests := []struct {
		name     string
		magnet   string
		expected string
		wantErr  bool
	}{
		{
			name:     "valid hex hash",
			magnet:   "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567",
			expected: "0123456789abcdef0123456789abcdef01234567",
			wantErr:  false,
		},
		{
			name:     "uppercase hex hash",
			magnet:   "magnet:?xt=urn:btih:0123456789ABCDEF0123456789ABCDEF01234567",
			expected: "0123456789abcdef0123456789abcdef01234567",
			wantErr:  false,
		},
		{
			name:     "with additional params",
			magnet:   "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=test&tr=http://tracker.com",
			expected: "0123456789abcdef0123456789abcdef01234567",
			wantErr:  false,
		},
		{
			name:    "invalid prefix",
			magnet:  "http://example.com",
			wantErr: true,
		},
		{
			name:    "missing xt param",
			magnet:  "magnet:?dn=test",
			wantErr: true,
		},
		{
			name:    "invalid xt format",
			magnet:  "magnet:?xt=invalid",
			wantErr: true,
		},
		{
			name:    "short hash",
			magnet:  "magnet:?xt=urn:btih:0123456789",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := ExtractMagnetHash(tt.magnet)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, hash)
			}
		})
	}
}

func TestExtractMagnetHash_Base32(t *testing.T) {
	// Base32 encoded hash (32 chars)
	magnet := "magnet:?xt=urn:btih:CQSQMCMG6ROQFUIG4RSLWOTFD5BGXNUP"

	hash, err := ExtractMagnetHash(magnet)
	require.NoError(t, err)
	assert.Len(t, hash, 40)
	assert.True(t, IsTorrentHash(hash))
}

func TestGetRemoteTorrent(t *testing.T) {
	torrentData := createTestTorrent("remote-torrent")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-bittorrent")
		w.Write(torrentData)
	}))
	defer server.Close()

	parsed, err := GetRemoteTorrent(server.URL, "")
	require.NoError(t, err)
	assert.Equal(t, "remote-torrent", parsed.Name)
}

func TestGetRemoteTorrent_WithCookie(t *testing.T) {
	torrentData := createTestTorrent("remote-torrent")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify cookie is sent
		assert.Contains(t, r.Header.Get("Cookie"), "test-cookie")
		w.Write(torrentData)
	}))
	defer server.Close()

	parsed, err := GetRemoteTorrent(server.URL, "test-cookie")
	require.NoError(t, err)
	assert.Equal(t, "remote-torrent", parsed.Name)
}

func TestGetRemoteTorrent_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := GetRemoteTorrent(server.URL, "")
	assert.Error(t, err)
}

func TestGetRemoteTorrent_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Empty body
	}))
	defer server.Close()

	_, err := GetRemoteTorrent(server.URL, "")
	assert.ErrorIs(t, err, ErrTorrentNotFound)
}

func TestValidateInfoHash(t *testing.T) {
	tests := []struct {
		hash  string
		valid bool
	}{
		{"0123456789abcdef0123456789abcdef01234567", true},
		{"0123456789ABCDEF0123456789ABCDEF01234567", true},
		{"0123456789abcdef0123456789abcdef0123456", false},   // Too short
		{"0123456789abcdef0123456789abcdef012345678", false}, // Too long
		{"0123456789abcdef0123456789abcdef0123456g", false},  // Invalid char
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.hash, func(t *testing.T) {
			assert.Equal(t, tt.valid, ValidateInfoHash(tt.hash))
		})
	}
}

func TestNormalizeInfoHash(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0123456789ABCDEF0123456789ABCDEF01234567", "0123456789abcdef0123456789abcdef01234567"},
		{"  0123456789abcdef0123456789abcdef01234567  ", "0123456789abcdef0123456789abcdef01234567"},
		{"0123456789abcdef0123456789abcdef01234567", "0123456789abcdef0123456789abcdef01234567"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, NormalizeInfoHash(tt.input))
		})
	}
}

func TestBuildMagnetLink(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef01234567"

	// Basic magnet
	magnet := BuildMagnetLink(hash, "", nil)
	assert.Contains(t, magnet, "magnet:?xt=urn:btih:")
	assert.Contains(t, magnet, hash)

	// With name
	magnet = BuildMagnetLink(hash, "Test Torrent", nil)
	assert.Contains(t, magnet, "&dn=Test+Torrent")

	// With trackers
	trackers := []string{"http://tracker1.com", "http://tracker2.com"}
	magnet = BuildMagnetLink(hash, "Test", trackers)
	assert.Contains(t, magnet, "&tr=http%3A%2F%2Ftracker1.com")
	assert.Contains(t, magnet, "&tr=http%3A%2F%2Ftracker2.com")
}

func TestIsTorrentHash(t *testing.T) {
	assert.True(t, IsTorrentHash("0123456789abcdef0123456789abcdef01234567"))
	assert.True(t, IsTorrentHash("0123456789ABCDEF0123456789ABCDEF01234567"))
	assert.False(t, IsTorrentHash("short"))
	assert.False(t, IsTorrentHash(""))
	assert.False(t, IsTorrentHash("0123456789abcdef0123456789abcdef0123456g"))
}

func TestBase32Decode(t *testing.T) {
	// Test known base32 value - "Hello" in base32 is JBSWY3DPEBLW64TMMQ
	// But for our purposes, we just need to verify it decodes without error
	// and produces consistent output
	decoded, err := base32Decode("JBSWY3DP")
	require.NoError(t, err)
	assert.Equal(t, "Hello", string(decoded))
}

func TestBase32Decode_Invalid(t *testing.T) {
	_, err := base32Decode("invalid!@#")
	assert.Error(t, err)
}

func TestIsValidTorrentContentType(t *testing.T) {
	assert.True(t, isValidTorrentContentType("application/x-bittorrent"))
	assert.True(t, isValidTorrentContentType("application/octet-stream"))
	assert.True(t, isValidTorrentContentType("Application/X-BitTorrent"))
	assert.False(t, isValidTorrentContentType("text/html"))
	assert.False(t, isValidTorrentContentType(""))
}

func TestInfoHashFromMagnet(t *testing.T) {
	// InfoHashFromMagnet is an alias for ExtractMagnetHash
	hash, err := InfoHashFromMagnet("magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567")
	require.NoError(t, err)
	assert.Equal(t, "0123456789abcdef0123456789abcdef01234567", hash)
}

func TestParsedTorrent_RawMetadata(t *testing.T) {
	data := createTestTorrent("test")

	parsed, err := ParseTorrent(data)
	require.NoError(t, err)

	// RawMetadata should contain the original bytes
	assert.Equal(t, data, parsed.RawMetadata)

	// Should be able to compute hash from raw metadata
	hash, err := ComputeTorrentHash(parsed.RawMetadata)
	require.NoError(t, err)
	assert.Equal(t, parsed.InfoHash, hash)
}

func TestComputeTorrentHash_DifferentTorrents(t *testing.T) {
	data1 := createTestTorrent("torrent1")
	data2 := createTestTorrent("torrent2")

	hash1, err := ComputeTorrentHash(data1)
	require.NoError(t, err)

	hash2, err := ComputeTorrentHash(data2)
	require.NoError(t, err)

	// Different torrents should have different hashes
	assert.NotEqual(t, hash1, hash2)
}

func TestHexDecodeString(t *testing.T) {
	// Valid hex
	_, err := hex.DecodeString("0123456789abcdef")
	assert.NoError(t, err)

	// Invalid hex
	_, err = hex.DecodeString("invalid")
	assert.Error(t, err)
}
