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
