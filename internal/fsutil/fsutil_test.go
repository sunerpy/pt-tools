package fsutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 临时目录所在的盘：总量大于 0、剩余不超过总量，同一块盘上的两个目录 Device 相同；不存在的路径报错。
func TestDiskUsage(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a")
	require.NoError(t, os.Mkdir(sub, 0o755))
	u, err := DiskUsage(dir)
	require.NoError(t, err)
	assert.Positive(t, u.Total)
	assert.LessOrEqual(t, u.Free, u.Total)
	assert.NotEmpty(t, u.Device)
	v, err := DiskUsage(sub)
	require.NoError(t, err)
	assert.Equal(t, u.Device, v.Device)

	_, err = DiskUsage(filepath.Join(dir, "none"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "读不到")
}

func TestLowSpace(t *testing.T) {
	const gb = 1 << 30
	for _, c := range []struct {
		free, total uint64
		low         bool
	}{
		{19 * gb, 0, true},
		{21 * gb, 0, false},
		{40 * gb, 1000 * gb, true},
		{60 * gb, 1000 * gb, false},
		{50 * gb, 1000 * gb, false},
		{19 * gb, 100 * gb, true},
	} {
		assert.Equal(t, c.low, LowSpace(c.free, c.total), "%d / %d", c.free/gb, c.total/gb)
	}
}
