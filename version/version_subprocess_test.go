package version_test

import (
	"archive/tar"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildPtToolsHelper compiles a small program named `pt-tools` into a temp dir.
// The binary drives version package APIs that only behave correctly when the
// running executable is named `pt-tools` (CleanupOldBinary + self-replace),
// which is impossible to exercise from the `version.test` binary directly.
func buildPtToolsHelper(t *testing.T, dir string) string {
	t.Helper()
	src := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(src, []byte(helperMainSource), 0o644))

	out := filepath.Join(dir, "pt-tools")
	cmd := exec.Command("go", "build", "-o", out, src)
	cmd.Env = os.Environ()
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build pt-tools helper: %v\n%s", err, combined)
	}
	return out
}

// TestCleanupOldBinary_SubprocessRemovesLeftovers builds a real `pt-tools`
// binary, drops .pt-tools-old / .pt-tools-backup leftovers next to it, then
// runs it so version.CleanupOldBinary executes with a pt-tools-named
// executable and removes them.
func TestCleanupOldBinary_SubprocessRemovesLeftovers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper build/exec is unix-focused here")
	}
	dir := t.TempDir()
	bin := buildPtToolsHelper(t, dir)

	for _, suffix := range []string{".pt-tools-old", ".pt-tools-backup"} {
		require.NoError(t, os.WriteFile(bin+suffix, []byte("leftover"), 0o644))
	}

	cmd := exec.Command(bin, "cleanup")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "helper output: %s", out)
	assert.Contains(t, string(out), "cleanup-done")

	for _, suffix := range []string{".pt-tools-old", ".pt-tools-backup"} {
		_, statErr := os.Stat(bin + suffix)
		assert.True(t, os.IsNotExist(statErr), "leftover %s must be removed", suffix)
	}
}

// TestPerformUpgrade_SubprocessSuccess builds a real `pt-tools` binary and asks
// it to self-upgrade from a locally served tar.gz containing a fake 2MB
// pt-tools binary. Because the helper's own executable is named pt-tools, the
// full performUpgrade success path (extract + replaceUnixBinary + completed)
// runs to completion, replacing the temp binary (never the real one).
func TestPerformUpgrade_SubprocessSuccess(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("self-upgrade success path is unix tar.gz only")
	}
	dir := t.TempDir()
	bin := buildPtToolsHelper(t, dir)

	fake := make([]byte, 2*1024*1024)
	copy(fake, []byte("#!/bin/sh\necho new\n"))
	archive := buildTarGzForHelper(t, "pt-tools", fake)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	cmd := exec.Command(bin, "upgrade", srv.URL)
	cmd.Env = append(
		os.Environ(),
		"HTTP_PROXY=", "http_proxy=", "HTTPS_PROXY=", "https_proxy=",
		"ALL_PROXY=", "all_proxy=", "NO_PROXY=*", "no_proxy=*",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "helper output: %s", out)
	assert.Contains(t, string(out), "status=completed", "helper output: %s", out)

	got, rerr := os.ReadFile(bin)
	require.NoError(t, rerr)
	assert.Equal(t, len(fake), len(got), "binary must be replaced with the 2MB fake")
}

func buildTarGzForHelper(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.tar.gz")
	f, err := os.Create(p)
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
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return b
}

const helperMainSource = `package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/sunerpy/pt-tools/version"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: helper <cleanup|upgrade URL>")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "cleanup":
		version.CleanupOldBinary()
		fmt.Println("cleanup-done")
	case "upgrade":
		u := version.NewUpgrader()
		asset := version.GetAssetName()
		rel := &version.ReleaseInfo{
			Version: "v9.9.9",
			Assets:  []version.ReleaseAsset{{Name: asset, DownloadURL: os.Args[2]}},
		}
		if err := u.Upgrade(context.Background(), rel, ""); err != nil {
			fmt.Printf("upgrade-error=%v\n", err)
			os.Exit(1)
		}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			p := u.GetProgress()
			if p.Status == version.UpgradeStatusCompleted || p.Status == version.UpgradeStatusFailed {
				fmt.Printf("status=%s err=%s\n", p.Status, p.Error)
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		fmt.Println("status=timeout")
		os.Exit(1)
	default:
		fmt.Println("unknown command")
		os.Exit(2)
	}
}
`
