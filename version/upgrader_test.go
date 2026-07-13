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
	"runtime"
	"strings"
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

func u_replaceUnix(cur, newBin, backup string) error {
	return NewUpgrader().replaceUnixBinary(cur, newBin, backup)
}

func httptestNewOK(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
}

// writeZipEntry writes a zip archive containing a single named entry.
func writeZipEntry(t *testing.T, archivePath, name string, content []byte) {
	t.Helper()
	f, err := os.Create(archivePath)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	w, err := zw.Create(name)
	require.NoError(t, err)
	_, err = w.Write(content)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())
}

// writeTarGzEntry writes a tar.gz archive containing a single named regular file.
func writeTarGzEntry(t *testing.T, archivePath, name string, content []byte) {
	t.Helper()
	f, err := os.Create(archivePath)
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg,
	}))
	_, err = tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())
}

func clearProxyForUpgrade(t *testing.T) {
	t.Helper()
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(k, "")
	}
	t.Setenv("NO_PROXY", "*")
	t.Setenv("no_proxy", "*")
}

func buildTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "a.tar.gz")
	f, err := os.Create(p)
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}))
	_, err = tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return b
}

// TestCanUpgrade_UnsupportedPlatform drives the CanSelfUpgrade=false branch by
// overriding BuildOS to a platform that cannot self-upgrade (darwin).
func TestCanUpgrade_UnsupportedPlatform(t *testing.T) {
	oldOS, oldArch := BuildOS, BuildArch
	defer func() { BuildOS, BuildArch = oldOS, oldArch }()
	BuildOS = "darwin"
	BuildArch = "amd64"

	// Only meaningful when not running inside the pt-tools docker image.
	if DetectEnvironment().IsDocker {
		t.Skip("running in docker; docker branch takes precedence")
	}

	u := NewUpgrader()
	err := u.CanUpgrade()
	assert.ErrorIs(t, err, ErrUnsupportedPlatform)
}

// TestUpgrade_RejectsWhenCannotUpgrade covers Upgrade's early-return when
// CanUpgrade fails (line 116 branch).
func TestUpgrade_RejectsWhenCannotUpgrade(t *testing.T) {
	oldOS, oldArch := BuildOS, BuildArch
	defer func() { BuildOS, BuildArch = oldOS, oldArch }()
	BuildOS = "darwin"
	BuildArch = "amd64"

	if DetectEnvironment().IsDocker {
		t.Skip("running in docker; docker branch takes precedence")
	}

	u := NewUpgrader()
	err := u.Upgrade(context.Background(), &ReleaseInfo{Version: "v9.9.9"}, "")
	assert.ErrorIs(t, err, ErrUnsupportedPlatform)
}

// TestCopyFile_DstOpenError covers copyFile's dst OpenFile error branch:
// the source opens fine but the destination is inside a nonexistent dir.
func TestCopyFile_DstOpenError(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	require.NoError(t, os.WriteFile(src, []byte("payload"), 0o644))

	dst := filepath.Join(dir, "no-such-dir", "out")
	err := copyFile(src, dst)
	assert.Error(t, err)
}

// TestExtractZip_DstOpenError covers extractZip's dst OpenFile failure: the
// archive DOES contain the target binary, but the destination path is
// unwritable (parent dir does not exist).
func TestExtractZip_DstOpenError(t *testing.T) {
	u := NewUpgrader()
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.zip")
	writeZipEntry(t, archive, "pt-tools", []byte("some binary content here"))

	err := u.extractZip(archive, filepath.Join(dir, "no-such-dir", "out"), "pt-tools")
	assert.ErrorIs(t, err, ErrExtractionFailed)
}

// TestExtractTarGz_NotGzip covers the gzip.NewReader error branch when the
// archive file exists but is not gzip-compressed.
func TestExtractTarGz_NotGzip(t *testing.T) {
	u := NewUpgrader()
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.tar.gz")
	require.NoError(t, os.WriteFile(archive, []byte("not a gzip stream at all"), 0o644))

	err := u.extractTarGz(archive, filepath.Join(dir, "out"), "pt-tools")
	assert.ErrorIs(t, err, ErrExtractionFailed)
}

// TestExtractTarGz_DstOpenError covers extractTarGz's dst OpenFile failure:
// the archive contains the target binary but destination dir is missing.
func TestExtractTarGz_DstOpenError(t *testing.T) {
	u := NewUpgrader()
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.tar.gz")
	writeTarGzEntry(t, archive, "pt-tools", []byte("tar payload content"))

	err := u.extractTarGz(archive, filepath.Join(dir, "no-such-dir", "out"), "pt-tools")
	assert.ErrorIs(t, err, ErrExtractionFailed)
}

// TestValidateReplacementPath_StatError covers the os.Stat error branch: the
// filename has the required pt-tools prefix but the file does not exist.
func TestValidateReplacementPath_StatError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "pt-tools-missing")
	err := validateReplacementPath(missing)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "无法访问目标文件")
}

// TestValidateReplacementPath_IsDir covers the "target is a directory" branch.
func TestValidateReplacementPath_IsDir(t *testing.T) {
	dir := t.TempDir()
	ptDir := filepath.Join(dir, "pt-tools-dir")
	require.NoError(t, os.MkdirAll(ptDir, 0o755))
	err := validateReplacementPath(ptDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "目录")
}

// TestReplaceUnixBinary_PermissionDenied drives the os.IsPermission backup
// branch by making the parent directory read-only so the rename fails with
// EACCES. Skipped when running as root (root ignores mode bits).
func TestReplaceUnixBinary_PermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission bits are bypassed")
	}
	dir := t.TempDir()
	cur := filepath.Join(dir, "pt-tools")
	require.NoError(t, os.WriteFile(cur, []byte("old"), 0o755))
	newBin := filepath.Join(dir, "pt-tools-new")
	require.NoError(t, os.WriteFile(newBin, []byte("new"), 0o755))

	// Make the parent dir read+execute only (no write) so renaming cur fails.
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := u_replaceUnix(cur, newBin, cur+".pt-tools-backup")
	if err == nil {
		t.Skip("filesystem permitted rename despite read-only parent; skip")
	}
	// Either explicit permission-denied or a wrapped replacement failure is acceptable.
	assert.Error(t, err)
}

// TestDownloadFile_WriteError covers downloadFile's os.WriteFile failure branch:
// the HTTP response is 200 but the destination lives in a nonexistent directory.
func TestDownloadFile_WriteError(t *testing.T) {
	clearProxyForUpgrade(t)
	srv := httptestNewOK(t, []byte("some body content"))
	defer srv.Close()

	u := NewUpgrader()
	dest := filepath.Join(t.TempDir(), "no-such-dir", "out")
	err := u.downloadFile(context.Background(), srv.URL, dest, "")
	assert.ErrorIs(t, err, ErrDownloadFailed)
}

// TestExtractTarGz_CorruptTarStream covers extractTarGz's tarReader.Next error
// branch: a valid gzip wrapper around a truncated/garbage tar body.
func TestExtractTarGz_CorruptTarStream(t *testing.T) {
	u := NewUpgrader()
	dir := t.TempDir()
	archive := filepath.Join(dir, "bad.tar.gz")

	f, err := os.Create(archive)
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	_, err = gz.Write([]byte("this is not a valid tar header stream, just noise bytes"))
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())

	err = u.extractTarGz(archive, filepath.Join(dir, "out"), "pt-tools")
	assert.ErrorIs(t, err, ErrExtractionFailed)
}

// TestPerformUpgrade_UnsupportedPlatform covers performUpgrade's assetName==""
// branch by overriding BuildOS to a platform with no release asset.
func TestPerformUpgrade_UnsupportedPlatform(t *testing.T) {
	oldOS, oldArch := BuildOS, BuildArch
	defer func() { BuildOS, BuildArch = oldOS, oldArch }()
	BuildOS = "freebsd"
	BuildArch = "amd64"

	u := NewUpgrader()
	u.performUpgrade(context.Background(), &ReleaseInfo{Version: "v9.9.9"}, "")
	p := u.GetProgress()
	assert.Equal(t, UpgradeStatusFailed, p.Status)
}

// TestExtractBinary_UnsupportedExtension covers the fallthrough error branch of
// extractBinary for an unrecognized archive extension.
func TestExtractBinary_UnsupportedExtension(t *testing.T) {
	u := NewUpgrader()
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.rar")
	require.NoError(t, os.WriteFile(archive, []byte("x"), 0o644))
	err := u.extractBinary(archive, filepath.Join(dir, "out"), "pt-tools")
	assert.ErrorIs(t, err, ErrExtractionFailed)
}

func TestUpgradeInProgressRejected(t *testing.T) {
	u := NewUpgrader()
	if err := u.CanUpgrade(); err != nil {
		t.Skipf("platform cannot self-upgrade: %v", err)
	}
	u.updateProgress(func(p *UpgradeProgress) { p.Status = UpgradeStatusReplacing })

	err := u.Upgrade(context.Background(), &ReleaseInfo{Version: "v9.9.9"}, "")
	assert.ErrorIs(t, err, ErrUpgradeInProgress)
}

func TestPerformUpgradeReachesReplaceAndFails(t *testing.T) {
	clearProxyForUpgrade(t)
	env := DetectEnvironment()
	assetName := GetAssetNameForPlatform(env.OS, env.Arch)
	if assetName == "" || !strings.HasSuffix(assetName, ".tar.gz") {
		t.Skipf("test host asset %q is not a tar.gz platform", assetName)
	}

	bin := make([]byte, 2*1024*1024)
	copy(bin, []byte("ELF-fake-binary"))
	archive := buildTarGz(t, GetBinaryName(env.OS), bin)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	u := NewUpgrader()
	release := &ReleaseInfo{
		Version: "v9.9.9",
		Assets:  []ReleaseAsset{{Name: assetName, DownloadURL: srv.URL}},
	}
	u.performUpgrade(context.Background(), release, "")

	p := u.GetProgress()
	assert.Equal(t, UpgradeStatusFailed, p.Status,
		"replace against non-pt-tools test binary must fail safely")
	assert.NotEmpty(t, p.Error)
}

func TestPerformUpgradeDownloadFailure(t *testing.T) {
	clearProxyForUpgrade(t)
	env := DetectEnvironment()
	assetName := GetAssetNameForPlatform(env.OS, env.Arch)
	if assetName == "" {
		t.Skip("unsupported platform")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	u := NewUpgrader()
	u.performUpgrade(context.Background(), &ReleaseInfo{
		Version: "v9.9.9",
		Assets:  []ReleaseAsset{{Name: assetName, DownloadURL: srv.URL}},
	}, "")
	p := u.GetProgress()
	assert.Equal(t, UpgradeStatusFailed, p.Status)
	assert.Contains(t, p.Error, "下载")
}

func TestPerformUpgradeExtractFailure(t *testing.T) {
	clearProxyForUpgrade(t)
	env := DetectEnvironment()
	assetName := GetAssetNameForPlatform(env.OS, env.Arch)
	if assetName == "" {
		t.Skip("unsupported platform")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("this is not a valid archive"))
	}))
	defer srv.Close()

	u := NewUpgrader()
	u.performUpgrade(context.Background(), &ReleaseInfo{
		Version: "v9.9.9",
		Assets:  []ReleaseAsset{{Name: assetName, DownloadURL: srv.URL}},
	}, "")
	p := u.GetProgress()
	assert.Equal(t, UpgradeStatusFailed, p.Status)
	assert.NotEmpty(t, p.Error, "invalid archive body must fail the upgrade")
}

func TestPerformUpgradeNoAsset(t *testing.T) {
	env := DetectEnvironment()
	if GetAssetNameForPlatform(env.OS, env.Arch) == "" {
		t.Skip("unsupported platform")
	}
	u := NewUpgrader()
	u.performUpgrade(context.Background(), &ReleaseInfo{Version: "v9.9.9"}, "")
	p := u.GetProgress()
	assert.Equal(t, UpgradeStatusFailed, p.Status)
	assert.Contains(t, p.Error, "安装包")
}

func TestReplaceBinaryVerifyFailure(t *testing.T) {
	dir := t.TempDir()
	cur := filepath.Join(dir, "pt-tools")
	require.NoError(t, os.WriteFile(cur, make([]byte, 2*1024*1024), 0o755))
	newBin := filepath.Join(dir, "pt-tools-new")
	require.NoError(t, os.WriteFile(newBin, []byte("too small"), 0o755))

	u := NewUpgrader()
	err := u.replaceBinary(cur, newBin)
	assert.ErrorIs(t, err, ErrReplacementFailed)
}

func TestReplaceBinaryValidatePathFailure(t *testing.T) {
	dir := t.TempDir()
	cur := filepath.Join(dir, "not-pt")
	require.NoError(t, os.WriteFile(cur, []byte("x"), 0o755))
	newBin := filepath.Join(dir, "pt-tools-new")
	require.NoError(t, os.WriteFile(newBin, make([]byte, 2*1024*1024), 0o755))

	u := NewUpgrader()
	err := u.replaceBinary(cur, newBin)
	assert.ErrorIs(t, err, ErrReplacementFailed)
}

func TestReplaceUnixBinarySuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only")
	}
	dir := t.TempDir()
	cur := filepath.Join(dir, "pt-tools")
	require.NoError(t, os.WriteFile(cur, []byte("old"), 0o755))
	newBin := filepath.Join(dir, "pt-tools-new")
	require.NoError(t, os.WriteFile(newBin, []byte("new-content"), 0o755))

	u := NewUpgrader()
	err := u.replaceUnixBinary(cur, newBin, cur+".pt-tools-backup")
	require.NoError(t, err)

	got, err := os.ReadFile(cur)
	require.NoError(t, err)
	assert.Equal(t, "new-content", string(got))
	_, statErr := os.Stat(cur + ".pt-tools-backup")
	assert.True(t, os.IsNotExist(statErr), "backup must be removed on success")
}

func TestReplaceWindowsBinarySuccess(t *testing.T) {
	dir := t.TempDir()
	cur := filepath.Join(dir, "pt-tools.exe")
	require.NoError(t, os.WriteFile(cur, []byte("old"), 0o755))
	newBin := filepath.Join(dir, "pt-tools-new.exe")
	require.NoError(t, os.WriteFile(newBin, []byte("new-win"), 0o755))

	u := NewUpgrader()
	err := u.replaceWindowsBinary(cur, newBin, cur+".pt-tools-backup")
	require.NoError(t, err)

	got, err := os.ReadFile(cur)
	require.NoError(t, err)
	assert.Equal(t, "new-win", string(got))
}

func TestExtractZipBinaryNotFound(t *testing.T) {
	u := NewUpgrader()
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.zip")
	createTestZip(t, archive, "other-file", []byte("data"))
	err := u.extractZip(archive, filepath.Join(dir, "out"), "pt-tools")
	assert.ErrorIs(t, err, ErrExtractionFailed)
}

func TestExtractTarGzBinaryNotFound(t *testing.T) {
	u := NewUpgrader()
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.tar.gz")
	createTestTarGz(t, archive, "other-file", []byte("data"))
	err := u.extractTarGz(archive, filepath.Join(dir, "out"), "pt-tools")
	assert.ErrorIs(t, err, ErrExtractionFailed)
}

func TestExtractBinaryDispatchZip(t *testing.T) {
	u := NewUpgrader()
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.zip")
	createTestZip(t, archive, "pt-tools", []byte("zip binary content long enough"))
	out := filepath.Join(dir, "out")
	require.NoError(t, u.extractBinary(archive, out, "pt-tools"))
}

func TestExtractZipOpenFailure(t *testing.T) {
	u := NewUpgrader()
	err := u.extractZip(filepath.Join(t.TempDir(), "missing.zip"), "/tmp/out", "pt-tools")
	assert.ErrorIs(t, err, ErrExtractionFailed)
}

func TestExtractTarGzOpenFailure(t *testing.T) {
	u := NewUpgrader()
	err := u.extractTarGz(filepath.Join(t.TempDir(), "missing.tar.gz"), "/tmp/out", "pt-tools")
	assert.ErrorIs(t, err, ErrExtractionFailed)
}

func TestCleanupOldBinaryNoPanic(t *testing.T) {
	assert.NotPanics(t, func() { CleanupOldBinary() })
}

func TestCancelWhenIdleNoStatusChange(t *testing.T) {
	u := NewUpgrader()
	u.Cancel()
	assert.Equal(t, UpgradeStatusIdle, u.GetProgress().Status)
}

func TestReplaceBinaryFullSuccessUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only")
	}
	dir := t.TempDir()
	cur := filepath.Join(dir, "pt-tools")
	require.NoError(t, os.WriteFile(cur, []byte("old"), 0o755))
	newBin := filepath.Join(dir, "pt-tools-new")
	require.NoError(t, os.WriteFile(newBin, make([]byte, 2*1024*1024), 0o755))

	u := NewUpgrader()
	require.NoError(t, u.replaceBinary(cur, newBin))
	info, err := os.Stat(cur)
	require.NoError(t, err)
	assert.Equal(t, int64(2*1024*1024), info.Size())
}

func TestUpgradeStartsWhenIdle(t *testing.T) {
	u := NewUpgrader()
	if err := u.CanUpgrade(); err != nil {
		t.Skipf("platform cannot self-upgrade: %v", err)
	}
	release := &ReleaseInfo{Version: "v9.9.9"}
	err := u.Upgrade(context.Background(), release, "")
	require.NoError(t, err, "Upgrade must accept work when idle")

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if u.GetProgress().Status == UpgradeStatusFailed {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	assert.Equal(t, UpgradeStatusFailed, u.GetProgress().Status,
		"no asset for v9.9.9 → background upgrade fails")
}

func TestReplaceUnixBinaryBackupRenameFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only")
	}
	u := NewUpgrader()
	err := u.replaceUnixBinary(
		filepath.Join(t.TempDir(), "does-not-exist", "pt-tools"),
		filepath.Join(t.TempDir(), "new"),
		filepath.Join(t.TempDir(), "backup"),
	)
	assert.ErrorIs(t, err, ErrReplacementFailed)
}

func TestReplaceWindowsBinaryRenameFailure(t *testing.T) {
	u := NewUpgrader()
	err := u.replaceWindowsBinary(
		filepath.Join(t.TempDir(), "missing-dir", "pt-tools.exe"),
		filepath.Join(t.TempDir(), "new.exe"),
		filepath.Join(t.TempDir(), "pt-tools.exe.pt-tools-backup"),
	)
	assert.ErrorIs(t, err, ErrReplacementFailed)
}

func TestReplaceWindowsBinaryCopyFailure(t *testing.T) {
	dir := t.TempDir()
	cur := filepath.Join(dir, "pt-tools.exe")
	require.NoError(t, os.WriteFile(cur, []byte("old"), 0o755))

	u := NewUpgrader()
	err := u.replaceWindowsBinary(cur, filepath.Join(dir, "no-such-new.exe"), cur+".pt-tools-backup")
	require.Error(t, err)
	restored, rerr := os.ReadFile(cur)
	require.NoError(t, rerr)
	assert.Equal(t, "old", string(restored), "original must be restored after copy failure")
}

func TestCancelDuringExtracting(t *testing.T) {
	u := NewUpgrader()
	u.updateProgress(func(p *UpgradeProgress) { p.Status = UpgradeStatusExtracting })
	u.Cancel()
	assert.Equal(t, UpgradeStatusFailed, u.GetProgress().Status)
}

func TestCancelInvokesDownloadCancel(t *testing.T) {
	u := NewUpgrader()
	called := false
	u.mu.Lock()
	u.downloadCancel = func() { called = true }
	u.progress.Status = UpgradeStatusDownloading
	u.mu.Unlock()

	u.Cancel()
	assert.True(t, called, "Cancel must invoke the stored downloadCancel func")
	assert.Equal(t, UpgradeStatusFailed, u.GetProgress().Status)

	u.mu.RLock()
	assert.Nil(t, u.downloadCancel, "downloadCancel must be cleared after Cancel")
	u.mu.RUnlock()
}

func TestCanUpgradeMatchesEnvironment(t *testing.T) {
	u := NewUpgrader()
	err := u.CanUpgrade()
	env := DetectEnvironment()
	switch {
	case env.IsDocker:
		assert.ErrorIs(t, err, ErrDockerEnvironment)
	case !env.CanSelfUpgrade:
		assert.ErrorIs(t, err, ErrUnsupportedPlatform)
	default:
		assert.NoError(t, err)
	}
}

func TestReplaceUnixBinaryNewRenameFailureRestores(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only")
	}
	dir := t.TempDir()
	cur := filepath.Join(dir, "pt-tools")
	require.NoError(t, os.WriteFile(cur, []byte("original"), 0o755))
	missingNew := filepath.Join(dir, "nope", "new")

	u := NewUpgrader()
	err := u.replaceUnixBinary(cur, missingNew, cur+".pt-tools-backup")
	assert.ErrorIs(t, err, ErrReplacementFailed)
	restored, rerr := os.ReadFile(cur)
	require.NoError(t, rerr)
	assert.Equal(t, "original", string(restored), "backup must be restored when new rename fails")
}

func TestCleanupOldBinaryRemovesLeftovers(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip("cannot resolve executable")
	}
	exe, err = filepath.EvalSymlinks(exe)
	require.NoError(t, err)
	if !strings.HasPrefix(filepath.Base(exe), "pt-tools") {
		t.Skipf("test binary %q not pt-tools-prefixed; cleanup branch untestable here", filepath.Base(exe))
	}
	for _, suffix := range []string{".pt-tools-old", ".pt-tools-backup"} {
		require.NoError(t, os.WriteFile(exe+suffix, []byte("x"), 0o644))
	}
	CleanupOldBinary()
	for _, suffix := range []string{".pt-tools-old", ".pt-tools-backup"} {
		_, statErr := os.Stat(exe + suffix)
		assert.True(t, os.IsNotExist(statErr))
	}
}

func TestDownloadFileNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	u := NewUpgrader()
	err := u.downloadFile(context.Background(), srv.URL, filepath.Join(t.TempDir(), "out"), "")
	assert.ErrorIs(t, err, ErrDownloadFailed)
}

func TestDownloadFileBadURL(t *testing.T) {
	u := NewUpgrader()
	err := u.downloadFile(context.Background(), "://bad-url", filepath.Join(t.TempDir(), "out"), "")
	assert.ErrorIs(t, err, ErrDownloadFailed)
}

func TestExtractBinaryDispatchTarGz(t *testing.T) {
	u := NewUpgrader()
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.tar.gz")
	createTestTarGz(t, archive, "pt-tools", []byte("tar binary content long enough"))
	require.NoError(t, u.extractBinary(archive, filepath.Join(dir, "out"), "pt-tools"))
}
