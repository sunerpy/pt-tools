//go:build linux

package organize

import (
	"errors"

	"golang.org/x/sys/unix"
)

// errRealtimeUnsupported 表示这个系统上不做实时监控（Linux 上用不到）。
var errRealtimeUnsupported = errors.New("这个系统上不支持实时监控")

// networkFS 报告目录在不在网络文件系统上（NFS、SMB、CephFS）：inotify 收不到别的机器写进来的改动。
func networkFS(path string) (string, bool) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return "", false
	}
	switch uint32(st.Type) { //nolint:gosec // 文件系统的类型号按 32 位比较（32 位平台上 Type 是 int32）
	case unix.NFS_SUPER_MAGIC:
		return "NFS", true
	case unix.SMB_SUPER_MAGIC, unix.SMB2_SUPER_MAGIC, unix.CIFS_SUPER_MAGIC:
		return "SMB", true
	case unix.CEPH_SUPER_MAGIC:
		return "CephFS", true
	}
	return "", false
}

// watchLimit 报告是不是 inotify 的监听数到了上限（inotify_add_watch 回 ENOSPC）。
func watchLimit(err error) bool { return errors.Is(err, unix.ENOSPC) }

// instanceLimit 报告是不是 inotify 的实例数到了上限（inotify_init1 回 EMFILE）。
func instanceLimit(err error) bool { return errors.Is(err, unix.EMFILE) }

// realtimeSupported 报告这个系统上做不做实时监控。
func realtimeSupported() bool { return true }
