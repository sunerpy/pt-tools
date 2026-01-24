package version

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUpgrader(t *testing.T) {
	u := NewUpgrader()
	assert.NotNil(t, u)

	progress := u.GetProgress()
	assert.Equal(t, UpgradeStatusIdle, progress.Status)
}

func TestGetUpgrader(t *testing.T) {
	u1 := GetUpgrader()
	u2 := GetUpgrader()
	assert.Same(t, u1, u2)
}

func TestUpgraderCanUpgrade(t *testing.T) {
	u := NewUpgrader()
	err := u.CanUpgrade()

	env := DetectEnvironment()
	if env.IsDocker {
		assert.ErrorIs(t, err, ErrDockerEnvironment)
	} else if !env.CanSelfUpgrade {
		assert.ErrorIs(t, err, ErrUnsupportedPlatform)
	} else {
		assert.NoError(t, err)
	}
}

func TestUpgraderFindAssetURL(t *testing.T) {
	u := NewUpgrader()

	release := &ReleaseInfo{
		Version: "v1.0.0",
		Assets: []ReleaseAsset{
			{Name: "pt-tools-linux-amd64.tar.gz", DownloadURL: "https://example.com/linux.tar.gz"},
			{Name: "pt-tools-windows-amd64.exe.zip", DownloadURL: "https://example.com/windows.zip"},
		},
	}

	tests := []struct {
		assetName string
		expected  string
	}{
		{"pt-tools-linux-amd64.tar.gz", "https://example.com/linux.tar.gz"},
		{"pt-tools-windows-amd64.exe.zip", "https://example.com/windows.zip"},
		{"pt-tools-darwin-amd64.tar.gz", ""},
	}

	for _, tt := range tests {
		t.Run(tt.assetName, func(t *testing.T) {
			result := u.findAssetURL(release, tt.assetName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUpgraderExtractTarGz(t *testing.T) {
	u := NewUpgrader()
	tempDir := t.TempDir()

	archivePath := filepath.Join(tempDir, "test.tar.gz")
	createTestTarGz(t, archivePath, "pt-tools", []byte("test binary content here, make it long enough to pass verification"))

	destPath := filepath.Join(tempDir, "extracted")
	err := u.extractTarGz(archivePath, destPath, "pt-tools")
	require.NoError(t, err)

	content, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "test binary content")
}

func TestUpgraderExtractZip(t *testing.T) {
	u := NewUpgrader()
	tempDir := t.TempDir()

	archivePath := filepath.Join(tempDir, "test.zip")
	createTestZip(t, archivePath, "pt-tools.exe", []byte("test binary content here, make it long enough"))

	destPath := filepath.Join(tempDir, "extracted.exe")
	err := u.extractZip(archivePath, destPath, "pt-tools.exe")
	require.NoError(t, err)

	content, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "test binary content")
}

func TestUpgraderExtractBinaryUnsupportedFormat(t *testing.T) {
	u := NewUpgrader()
	err := u.extractBinary("/tmp/test.rar", "/tmp/out", "binary")
	assert.ErrorIs(t, err, ErrExtractionFailed)
}

func TestUpgraderDownloadFile(t *testing.T) {
	testContent := []byte("test download content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "21")
		_, _ = w.Write(testContent)
	}))
	defer server.Close()

	u := NewUpgrader()
	tempDir := t.TempDir()
	destPath := filepath.Join(tempDir, "downloaded")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := u.downloadFile(ctx, server.URL, destPath, "")
	require.NoError(t, err)

	content, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, testContent, content)
}

func TestUpgraderDownloadFileWithProxy(t *testing.T) {
	testContent := []byte("proxied content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(testContent)
	}))
	defer server.Close()

	u := NewUpgrader()
	tempDir := t.TempDir()
	destPath := filepath.Join(tempDir, "downloaded")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := u.downloadFile(ctx, server.URL, destPath, "http://invalid-proxy:8080")
	assert.Error(t, err)
}

func TestUpgraderCancel(t *testing.T) {
	u := NewUpgrader()
	u.updateProgress(func(p *UpgradeProgress) {
		p.Status = UpgradeStatusDownloading
	})

	u.Cancel()

	progress := u.GetProgress()
	assert.Equal(t, UpgradeStatusFailed, progress.Status)
	assert.Contains(t, progress.Error, "取消")
}

func TestUpgradeProgressConcurrency(t *testing.T) {
	u := NewUpgrader()

	done := make(chan bool)
	for i := range 10 {
		go func(n int) {
			u.updateProgress(func(p *UpgradeProgress) {
				p.BytesDownloaded = int64(n * 100)
			})
			_ = u.GetProgress()
			done <- true
		}(i)
	}

	for range 10 {
		<-done
	}
}

func TestVerifyBinary(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("valid binary", func(t *testing.T) {
		path := filepath.Join(tempDir, "valid")
		data := make([]byte, 2*1024*1024)
		require.NoError(t, os.WriteFile(path, data, 0o644))

		err := verifyBinary(path)
		assert.NoError(t, err)
	})

	t.Run("too small", func(t *testing.T) {
		path := filepath.Join(tempDir, "small")
		data := make([]byte, 100)
		require.NoError(t, os.WriteFile(path, data, 0o644))

		err := verifyBinary(path)
		assert.Error(t, err)
	})

	t.Run("not exists", func(t *testing.T) {
		err := verifyBinary(filepath.Join(tempDir, "notexists"))
		assert.Error(t, err)
	})
}

func TestCopyFile(t *testing.T) {
	tempDir := t.TempDir()
	srcPath := filepath.Join(tempDir, "src")
	dstPath := filepath.Join(tempDir, "dst")

	content := []byte("test content")
	require.NoError(t, os.WriteFile(srcPath, content, 0o644))

	err := copyFile(srcPath, dstPath)
	require.NoError(t, err)

	result, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	assert.Equal(t, content, result)
}

func TestValidateReplacementPath(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("valid pt-tools path", func(t *testing.T) {
		path := filepath.Join(tempDir, "pt-tools")
		require.NoError(t, os.WriteFile(path, []byte("binary"), 0o755))

		err := validateReplacementPath(path)
		assert.NoError(t, err)
	})

	t.Run("valid pt-tools.exe path", func(t *testing.T) {
		path := filepath.Join(tempDir, "pt-tools.exe")
		require.NoError(t, os.WriteFile(path, []byte("binary"), 0o755))

		err := validateReplacementPath(path)
		assert.NoError(t, err)
	})

	t.Run("empty path", func(t *testing.T) {
		err := validateReplacementPath("")
		assert.Error(t, err)
	})

	t.Run("non pt-tools file", func(t *testing.T) {
		path := filepath.Join(tempDir, "some-other-binary")
		require.NoError(t, os.WriteFile(path, []byte("binary"), 0o755))

		err := validateReplacementPath(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pt-tools")
	})

	t.Run("directory instead of file", func(t *testing.T) {
		path := filepath.Join(tempDir, "pt-tools-dir")
		require.NoError(t, os.MkdirAll(path, 0o755))

		err := validateReplacementPath(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "目录")
	})
}

func TestSafeRemoveBackup(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("valid backup file", func(t *testing.T) {
		path := filepath.Join(tempDir, "pt-tools.pt-tools-backup")
		require.NoError(t, os.WriteFile(path, []byte("backup"), 0o644))

		err := safeRemoveBackup(path)
		assert.NoError(t, err)
		_, statErr := os.Stat(path)
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("valid old file", func(t *testing.T) {
		path := filepath.Join(tempDir, "pt-tools.exe.pt-tools-old")
		require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))

		err := safeRemoveBackup(path)
		assert.NoError(t, err)
	})

	t.Run("non pt-tools file rejected", func(t *testing.T) {
		path := filepath.Join(tempDir, "other-file.backup")
		require.NoError(t, os.WriteFile(path, []byte("data"), 0o644))

		err := safeRemoveBackup(path)
		assert.Error(t, err)
		_, statErr := os.Stat(path)
		assert.False(t, os.IsNotExist(statErr))
	})

	t.Run("non backup suffix rejected", func(t *testing.T) {
		path := filepath.Join(tempDir, "pt-tools-config.json")
		require.NoError(t, os.WriteFile(path, []byte("config"), 0o644))

		err := safeRemoveBackup(path)
		assert.Error(t, err)
		_, statErr := os.Stat(path)
		assert.False(t, os.IsNotExist(statErr))
	})

	t.Run("empty path", func(t *testing.T) {
		err := safeRemoveBackup("")
		assert.NoError(t, err)
	})
}

func createTestTarGz(t *testing.T, archivePath, fileName string, content []byte) {
	t.Helper()

	file, err := os.Create(archivePath)
	require.NoError(t, err)
	defer file.Close()

	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	header := &tar.Header{
		Name: fileName,
		Mode: 0o755,
		Size: int64(len(content)),
	}
	require.NoError(t, tarWriter.WriteHeader(header))
	_, err = tarWriter.Write(content)
	require.NoError(t, err)
}

func createTestZip(t *testing.T, archivePath, fileName string, content []byte) {
	t.Helper()

	file, err := os.Create(archivePath)
	require.NoError(t, err)
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	writer, err := zipWriter.Create(fileName)
	require.NoError(t, err)
	_, err = io.Copy(writer, io.NewSectionReader(io.NewSectionReader(
		&bytesReaderAt{content},
		0, int64(len(content)),
	), 0, int64(len(content))))
	require.NoError(t, err)
}

type bytesReaderAt struct {
	data []byte
}

func (b *bytesReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(b.data)) {
		return 0, io.EOF
	}
	n = copy(p, b.data[off:])
	return n, nil
}
