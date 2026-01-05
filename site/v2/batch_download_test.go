package v2

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBatchDownloadManifest tests manifest creation
func TestBatchDownloadManifest(t *testing.T) {
	t.Run("create manifest with torrents", func(t *testing.T) {
		torrents := []TorrentItem{
			{ID: "1", Title: "Torrent 1", SizeBytes: 1024, DiscountLevel: DiscountFree},
			{ID: "2", Title: "Torrent 2", SizeBytes: 2048, DiscountLevel: Discount2xFree},
		}

		manifest := make([]TorrentManifest, len(torrents))
		for i, t := range torrents {
			manifest[i] = TorrentManifest{
				ID:            t.ID,
				Title:         t.Title,
				SizeBytes:     t.SizeBytes,
				DiscountLevel: t.DiscountLevel,
			}
		}

		assert.Len(t, manifest, 2)
		assert.Equal(t, "1", manifest[0].ID)
		assert.Equal(t, "Torrent 1", manifest[0].Title)
	})

	t.Run("manifest JSON serialization", func(t *testing.T) {
		manifest := []TorrentManifest{
			{ID: "123", Title: "Test Torrent", SizeBytes: 1024, DiscountLevel: DiscountFree},
		}

		data, err := json.Marshal(manifest)
		require.NoError(t, err)

		var decoded []TorrentManifest
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Len(t, decoded, 1)
		assert.Equal(t, manifest[0].ID, decoded[0].ID)
		assert.Equal(t, manifest[0].Title, decoded[0].Title)
	})
}

// TestFreeTorrentFiltering tests the free torrent filtering logic
func TestFreeTorrentFiltering(t *testing.T) {
	t.Run("filter free torrents", func(t *testing.T) {
		torrents := []TorrentItem{
			{ID: "1", Title: "Free Torrent", DiscountLevel: DiscountFree},
			{ID: "2", Title: "2x Free Torrent", DiscountLevel: Discount2xFree},
			{ID: "3", Title: "Normal Torrent", DiscountLevel: DiscountNone},
			{ID: "4", Title: "50% Torrent", DiscountLevel: DiscountPercent50},
		}

		var freeTorrents []TorrentItem
		for _, t := range torrents {
			if t.DiscountLevel == DiscountFree || t.DiscountLevel == Discount2xFree {
				freeTorrents = append(freeTorrents, t)
			}
		}

		assert.Len(t, freeTorrents, 2)
		assert.Equal(t, "1", freeTorrents[0].ID)
		assert.Equal(t, "2", freeTorrents[1].ID)
	})

	t.Run("no free torrents", func(t *testing.T) {
		torrents := []TorrentItem{
			{ID: "1", Title: "Normal Torrent", DiscountLevel: DiscountNone},
			{ID: "2", Title: "50% Torrent", DiscountLevel: DiscountPercent50},
		}

		var freeTorrents []TorrentItem
		for _, t := range torrents {
			if t.DiscountLevel == DiscountFree || t.DiscountLevel == Discount2xFree {
				freeTorrents = append(freeTorrents, t)
			}
		}

		assert.Len(t, freeTorrents, 0)
	})
}

// TestArchiveCreation tests archive creation functionality
func TestArchiveCreation(t *testing.T) {
	t.Run("create tar.gz archive", func(t *testing.T) {
		files := map[string][]byte{
			"test1.torrent": []byte("torrent content 1"),
			"test2.torrent": []byte("torrent content 2"),
			"manifest.json": []byte(`{"siteName":"test","torrentCount":2}`),
		}

		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)

		for name, content := range files {
			hdr := &tar.Header{
				Name: name,
				Mode: 0o644,
				Size: int64(len(content)),
			}
			err := tw.WriteHeader(hdr)
			require.NoError(t, err)
			_, err = tw.Write(content)
			require.NoError(t, err)
		}

		err := tw.Close()
		require.NoError(t, err)
		err = gw.Close()
		require.NoError(t, err)

		// Verify archive can be read
		gr, err := gzip.NewReader(&buf)
		require.NoError(t, err)
		tr := tar.NewReader(gr)

		fileCount := 0
		for {
			_, err := tr.Next()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			fileCount++
		}

		assert.Equal(t, 3, fileCount)
	})

	t.Run("create zip archive", func(t *testing.T) {
		files := map[string][]byte{
			"test1.torrent": []byte("torrent content 1"),
			"test2.torrent": []byte("torrent content 2"),
			"manifest.json": []byte(`{"siteName":"test","torrentCount":2}`),
		}

		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)

		for name, content := range files {
			w, err := zw.Create(name)
			require.NoError(t, err)
			_, err = w.Write(content)
			require.NoError(t, err)
		}

		err := zw.Close()
		require.NoError(t, err)

		// Verify archive can be read
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		require.NoError(t, err)

		assert.Len(t, zr.File, 3)
	})

	t.Run("archive round-trip integrity", func(t *testing.T) {
		originalContent := []byte("original torrent content with special chars: 中文测试")

		// Create archive
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)

		hdr := &tar.Header{
			Name: "test.torrent",
			Mode: 0o644,
			Size: int64(len(originalContent)),
		}
		err := tw.WriteHeader(hdr)
		require.NoError(t, err)
		_, err = tw.Write(originalContent)
		require.NoError(t, err)

		err = tw.Close()
		require.NoError(t, err)
		err = gw.Close()
		require.NoError(t, err)

		// Read archive
		gr, err := gzip.NewReader(&buf)
		require.NoError(t, err)
		tr := tar.NewReader(gr)

		_, err = tr.Next()
		require.NoError(t, err)

		extractedContent, err := io.ReadAll(tr)
		require.NoError(t, err)

		assert.Equal(t, originalContent, extractedContent)
	})
}
