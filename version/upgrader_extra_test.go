package version

import (
	"archive/tar"
	"compress/gzip"
	"context"
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

func TestCheckForUpdatesReturnsCachedResult(t *testing.T) {
	c := NewChecker()
	cached := &VersionCheckResult{CurrentVersion: "v1.2.3", HasUpdate: false}
	c.lastResult = cached
	c.lastCheck = time.Now()
	c.lastProxy = ""
	c.lastIncludePrerel = false

	got, err := c.CheckForUpdates(context.Background(), CheckOptions{})
	require.NoError(t, err)
	assert.Same(t, cached, got, "fresh cache with matching opts must short-circuit without network")
}

func TestCheckForUpdatesProxyChangeBypassesCache(t *testing.T) {
	c := NewChecker()
	c.lastResult = &VersionCheckResult{CurrentVersion: "v1.0.0"}
	c.lastCheck = time.Now()
	c.lastProxy = ""

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res, _ := c.CheckForUpdates(ctx, CheckOptions{ProxyURL: "http://127.0.0.1:1"})
	require.NotNil(t, res)
	assert.Equal(t, Version, res.CurrentVersion)
	assert.NotEmpty(t, res.Error, "unreachable proxy must record an error, proving cache bypass")
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

func clearProxyForUpgrade(t *testing.T) {
	t.Helper()
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(k, "")
	}
	t.Setenv("NO_PROXY", "*")
	t.Setenv("no_proxy", "*")
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

func TestDetectEnvironmentBuildOverrides(t *testing.T) {
	oldOS, oldArch := BuildOS, BuildArch
	defer func() { BuildOS, BuildArch = oldOS, oldArch }()
	BuildOS = "linux"
	BuildArch = "arm64"

	env := DetectEnvironment()
	assert.Equal(t, "linux", env.OS)
	assert.Equal(t, "arm64", env.Arch)
	assert.True(t, env.CanSelfUpgrade)
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

func TestIsInAppDir(t *testing.T) {
	assert.False(t, isInAppDir(), "test binary is not under /app")
}

func TestDetectDocker(t *testing.T) {
	assert.False(t, detectDocker(), "test host is not the pt-tools docker image")
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
