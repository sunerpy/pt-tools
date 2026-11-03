//go:build linux

package organize

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

func TestNetworkKind(t *testing.T) {
	for _, tc := range []struct {
		magic uint32
		kind  string
		net   bool
	}{
		{unix.NFS_SUPER_MAGIC, "NFS", true},
		{unix.SMB_SUPER_MAGIC, "SMB", true},
		{unix.SMB2_SUPER_MAGIC, "SMB", true},
		{unix.CIFS_SUPER_MAGIC, "SMB", true},
		{unix.CEPH_SUPER_MAGIC, "CephFS", true},
		{unix.EXT4_SUPER_MAGIC, "", false},
		{unix.BTRFS_SUPER_MAGIC, "", false},
		{unix.TMPFS_MAGIC, "", false},
	} {
		kind, net := networkKind(tc.magic)
		assert.Equal(t, tc.kind, kind, "%#x", tc.magic)
		assert.Equal(t, tc.net, net, "%#x", tc.magic)
	}
	_, net := networkFS(t.TempDir())
	assert.False(t, net, "测试的临时目录不在网络文件系统上")
	_, net = networkFS("/no/such/dir")
	assert.False(t, net, "读不了时不当作网络文件系统")
}

func TestRealtimeReasonLimits(t *testing.T) {
	watches := fmt.Errorf("加监听: %w", unix.ENOSPC)
	assert.True(t, watchLimit(watches))
	assert.False(t, instanceLimit(watches))
	assert.Contains(t, realtimeReason(watches), "fs.inotify.max_user_watches")
	instances := fmt.Errorf("建监听: %w", unix.EMFILE)
	assert.True(t, instanceLimit(instances))
	assert.Contains(t, realtimeReason(instances), "fs.inotify.max_user_instances")
	assert.Equal(t, "加不了监听：权限不够", realtimeReason(errors.New("权限不够")))
}
