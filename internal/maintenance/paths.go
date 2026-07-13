// MIT License
// Copyright (c) 2025 pt-tools

package maintenance

import (
	"os"
	"path/filepath"
)

// evalSymlinksExisting 解析路径的软链。若路径本身不存在，则回退到解析其最近的
// 已存在祖先目录，再拼回剩余的不存在部分——这样对"尚不存在的候选文件"也能给出
// 一个可用于边界判定的解析路径（与 §2.4 步骤 3"不存在则按其已存在父目录判定"一致）。
func evalSymlinksExisting(path string) (string, error) {
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved, nil
	}
	// 逐级向上找到已存在的祖先，解析它，再把不存在的尾部拼回。
	parent := filepath.Dir(path)
	tail := filepath.Base(path)
	for parent != path {
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			return filepath.Join(resolved, tail), nil
		}
		tail = filepath.Join(filepath.Base(parent), tail)
		path = parent
		parent = filepath.Dir(path)
	}
	return "", os.ErrNotExist
}

// fileSize 返回文件字节数，出错返回 0（不影响删除判定，仅用于统计释放空间）。
func fileSize(path string) int64 {
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return info.Size()
	}
	return 0
}
