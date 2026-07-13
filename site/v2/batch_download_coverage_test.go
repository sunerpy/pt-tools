package v2

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// fakeBatchSite implements the Site interface for batch download tests.
type fakeBatchSite struct {
	id          string
	items       []TorrentItem
	searchErr   error
	downloadErr error
	data        []byte
	downloadMap map[string]error // per-torrent download errors
}

func (f *fakeBatchSite) ID() string     { return f.id }
func (f *fakeBatchSite) Name() string   { return f.id }
func (f *fakeBatchSite) Kind() SiteKind { return SiteNexusPHP }
func (f *fakeBatchSite) Login(ctx context.Context, creds Credentials) error {
	return nil
}

func (f *fakeBatchSite) GetUserInfo(ctx context.Context) (UserInfo, error) {
	return UserInfo{}, nil
}
func (f *fakeBatchSite) Close() error { return nil }

func (f *fakeBatchSite) Search(ctx context.Context, query SearchQuery) ([]TorrentItem, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.items, nil
}

func (f *fakeBatchSite) Download(ctx context.Context, torrentID string) ([]byte, error) {
	if f.downloadMap != nil {
		if err, ok := f.downloadMap[torrentID]; ok && err != nil {
			return nil, err
		}
	}
	if f.downloadErr != nil {
		return nil, f.downloadErr
	}
	if f.data != nil {
		return f.data, nil
	}
	return []byte("d4:infod6:lengthi1eee"), nil
}

func TestNewBatchDownloadService_NilLogger(t *testing.T) {
	svc := NewBatchDownloadService(&fakeBatchSite{id: "x"}, nil)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.logger)
}

func TestBatchService_FetchFreeTorrents(t *testing.T) {
	site := &fakeBatchSite{
		id: "test",
		items: []TorrentItem{
			{ID: "1", Title: "Free", DiscountLevel: DiscountFree},
			{ID: "2", Title: "Normal", DiscountLevel: DiscountNone},
			{ID: "3", Title: "2xFree", DiscountLevel: Discount2xFree},
		},
	}
	svc := NewBatchDownloadService(site, zap.NewNop())
	free, err := svc.FetchFreeTorrents(context.Background())
	require.NoError(t, err)
	assert.Len(t, free, 2)
}

func TestBatchService_FetchFreeTorrents_Error(t *testing.T) {
	site := &fakeBatchSite{id: "e", searchErr: errors.New("boom")}
	svc := NewBatchDownloadService(site, zap.NewNop())
	_, err := svc.FetchFreeTorrents(context.Background())
	assert.Error(t, err)
}

func TestBatchService_DownloadFreeTorrents_NoFree(t *testing.T) {
	site := &fakeBatchSite{
		id:    "nf",
		items: []TorrentItem{{ID: "1", Title: "Normal", DiscountLevel: DiscountNone}},
	}
	svc := NewBatchDownloadService(site, zap.NewNop())
	_, err := svc.DownloadFreeTorrents(context.Background(), "zip", t.TempDir())
	assert.ErrorIs(t, err, ErrNoFreeTorrents)
}

func TestBatchService_DownloadFreeTorrents_TarGz(t *testing.T) {
	site := &fakeBatchSite{
		id: "site",
		items: []TorrentItem{
			{ID: "1", Title: "Free A", DiscountLevel: DiscountFree, SizeBytes: 100},
			{ID: "2", Title: "Free B", DiscountLevel: Discount2xFree, SizeBytes: 200},
		},
		data: []byte("d4:infod6:lengthi1eee"),
	}
	svc := NewBatchDownloadService(site, zap.NewNop())
	outDir := t.TempDir()
	res, err := svc.DownloadFreeTorrents(context.Background(), "tar.gz", outDir)
	require.NoError(t, err)
	assert.Equal(t, "tar.gz", res.ArchiveType)
	assert.Equal(t, 2, res.TorrentCount)
	assert.Equal(t, int64(300), res.TotalSize)
	assert.FileExists(t, res.ArchivePath)
	assert.True(t, strings.HasSuffix(res.ArchivePath, ".tar.gz"))

	// Verify archive contents.
	f, err := os.Open(res.ArchivePath)
	require.NoError(t, err)
	defer f.Close()
	gr, err := gzip.NewReader(f)
	require.NoError(t, err)
	tr := tar.NewReader(gr)
	names := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		names[hdr.Name] = true
	}
	assert.True(t, names["manifest.json"])
}

func TestBatchService_DownloadFreeTorrents_Zip(t *testing.T) {
	site := &fakeBatchSite{
		id:    "zsite",
		items: []TorrentItem{{ID: "1", Title: "Free", DiscountLevel: DiscountFree, SizeBytes: 50}},
	}
	svc := NewBatchDownloadService(site, zap.NewNop())
	res, err := svc.DownloadFreeTorrents(context.Background(), "zip", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "zip", res.ArchiveType)
	assert.FileExists(t, res.ArchivePath)

	zr, err := zip.OpenReader(res.ArchivePath)
	require.NoError(t, err)
	defer zr.Close()
	assert.GreaterOrEqual(t, len(zr.File), 2) // torrent + manifest
}

func TestBatchService_DownloadFreeTorrents_AllDownloadsFail(t *testing.T) {
	site := &fakeBatchSite{
		id:          "fail",
		items:       []TorrentItem{{ID: "1", Title: "Free", DiscountLevel: DiscountFree}},
		downloadErr: errors.New("download failed"),
	}
	svc := NewBatchDownloadService(site, zap.NewNop())
	_, err := svc.DownloadFreeTorrents(context.Background(), "zip", t.TempDir())
	assert.ErrorIs(t, err, ErrTorrentDownloadFailed)
}

func TestBatchService_DownloadFreeTorrents_PartialFailure(t *testing.T) {
	site := &fakeBatchSite{
		id: "partial",
		items: []TorrentItem{
			{ID: "1", Title: "OK", DiscountLevel: DiscountFree, SizeBytes: 10},
			{ID: "2", Title: "Bad", DiscountLevel: DiscountFree, SizeBytes: 20},
		},
		downloadMap: map[string]error{"2": errors.New("skip me")},
	}
	svc := NewBatchDownloadService(site, zap.NewNop())
	res, err := svc.DownloadFreeTorrents(context.Background(), "tar.gz", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, 1, res.TorrentCount)
}

func TestBatchService_DownloadFreeTorrents_ContextCanceled(t *testing.T) {
	site := &fakeBatchSite{
		id:    "cancel",
		items: []TorrentItem{{ID: "1", Title: "Free", DiscountLevel: DiscountFree}},
	}
	svc := NewBatchDownloadService(site, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.DownloadFreeTorrents(ctx, "zip", t.TempDir())
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSanitizeFilename(t *testing.T) {
	assert.Equal(t, "normal_name", sanitizeFilename("normal_name"))
	assert.Equal(t, "a_b_c_d", sanitizeFilename("a/b\\c:d"))
	assert.Equal(t, "no_stars_", sanitizeFilename("no*stars?"))
	assert.Equal(t, "unnamed", sanitizeFilename("   "))
	assert.Equal(t, "unnamed", sanitizeFilename("..."))
	long := strings.Repeat("x", 300)
	assert.Len(t, sanitizeFilename(long), 200)
	// leading/trailing dots and spaces trimmed
	assert.Equal(t, "trimmed", sanitizeFilename(" .trimmed. "))
}

func TestReplaceAll(t *testing.T) {
	assert.Equal(t, "a_b_c", replaceAll("a-b-c", "-", "_"))
	assert.Equal(t, "no change", replaceAll("no change", "x", "y"))
	assert.Equal(t, "", replaceAll("aaa", "a", ""))
}

func TestIndexOf(t *testing.T) {
	assert.Equal(t, 0, indexOf("hello", "he"))
	assert.Equal(t, 2, indexOf("hello", "ll"))
	assert.Equal(t, -1, indexOf("hello", "z"))
	assert.Equal(t, -1, indexOf("ab", "abc"))
}

func TestTrimChars(t *testing.T) {
	assert.Equal(t, "abc", trimChars("...abc...", "."))
	assert.Equal(t, "abc", trimChars("  abc  ", " "))
	assert.Equal(t, "", trimChars("....", "."))
	assert.Equal(t, "abc", trimChars("abc", "x"))
}

func TestContainsChar(t *testing.T) {
	assert.True(t, containsChar("abc", 'b'))
	assert.False(t, containsChar("abc", 'z'))
	assert.False(t, containsChar("", 'a'))
}

func TestAddFileToTar_Error(t *testing.T) {
	// Nonexistent file should error.
	var buf strings.Builder
	_ = buf
	err := createTarGzArchive(filepath.Join(t.TempDir(), "out.tar.gz"), t.TempDir(), []string{"/nonexistent/file.txt"})
	assert.Error(t, err)
}

func TestAddFileToZip_Error(t *testing.T) {
	err := createZipArchive(filepath.Join(t.TempDir(), "out.zip"), t.TempDir(), []string{"/nonexistent/file.txt"})
	assert.Error(t, err)
}
