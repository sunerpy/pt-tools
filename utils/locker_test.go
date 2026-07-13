package utils

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocker_LockUnlock(t *testing.T) {
	f := filepath.Join(t.TempDir(), "lockfile")
	l, err := NewLocker(f)
	require.NoError(t, err)
	require.NoError(t, l.Lock())
	require.NoError(t, l.Unlock())
	require.NotNil(t, l.File())
}

// TestNewLocker_Error covers the OpenFile error branch (path in a nonexistent dir).
func TestNewLocker_Error(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "no-such-dir", "lock")
	_, err := NewLocker(bad)
	require.Error(t, err)
}
