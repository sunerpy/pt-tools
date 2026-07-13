package version

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func u_replaceUnix(cur, newBin, backup string) error {
	return NewUpgrader().replaceUnixBinary(cur, newBin, backup)
}

// TestFilterNewReleases_WithAssets covers the asset-copy loop inside
// filterNewReleases (previously-uncovered lines building ReleaseAsset slices).
func TestFilterNewReleases_WithAssets(t *testing.T) {
	c := NewChecker()
	oldVersion := Version
	defer func() { Version = oldVersion }()
	Version = "v1.0.0"

	releases := []GitHubRelease{
		{
			TagName: "v2.0.0", Name: "Release 2", HTMLURL: "u",
			Assets: []GitHubAsset{
				{Name: "pt-tools-linux-amd64.tar.gz", DownloadURL: "https://x/a", Size: 111},
				{Name: "pt-tools-windows-amd64.exe.zip", DownloadURL: "https://x/b", Size: 222},
			},
		},
	}
	got := c.filterNewReleases(releases, false)
	require.Len(t, got, 1)
	require.Len(t, got[0].Assets, 2)
	assert.Equal(t, "pt-tools-linux-amd64.tar.gz", got[0].Assets[0].Name)
	assert.Equal(t, int64(111), got[0].Assets[0].Size)
	assert.Equal(t, "https://x/b", got[0].Assets[1].DownloadURL)
}

// TestCheckForUpdates_ForceError drives CheckForUpdates with Force=true through
// the fetch path; the unreachable proxy guarantees an error is recorded,
// exercising the error-return branch after the cache short-circuit checks.
func TestCheckForUpdates_ForceError(t *testing.T) {
	c := NewChecker()
	c.lastResult = &VersionCheckResult{CurrentVersion: "v0.0.1"}
	c.lastCheck = time.Now()

	res, err := c.CheckForUpdates(context.Background(), CheckOptions{
		Force:    true,
		ProxyURL: "http://127.0.0.1:1",
	})
	require.Error(t, err)
	require.NotNil(t, res)
	assert.NotEmpty(t, res.Error)
	assert.Equal(t, Version, res.CurrentVersion)
}

// TestCheckForUpdates_PrereleaseChangeBypassesCache exercises the
// prerelChanged branch of the cache guard.
func TestCheckForUpdates_PrereleaseChangeBypassesCache(t *testing.T) {
	c := NewChecker()
	c.lastResult = &VersionCheckResult{CurrentVersion: "v0.0.1"}
	c.lastCheck = time.Now()
	c.lastProxy = "http://127.0.0.1:1"
	c.lastIncludePrerel = false

	res, _ := c.CheckForUpdates(context.Background(), CheckOptions{
		ProxyURL:          "http://127.0.0.1:1",
		IncludePrerelease: true,
	})
	require.NotNil(t, res)
	assert.NotEmpty(t, res.Error, "prerelease flag change must bypass cache and attempt fetch")
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
