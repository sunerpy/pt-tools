package transfer

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sunerpy/pt-tools/models"
)

var (
	// ErrCrossDevice 表示源文件与媒体库不在同一个文件系统（硬链接做不了）。
	ErrCrossDevice = errors.New("源文件与媒体库不在同一个文件系统，不能硬链接：把下载目录和媒体库放在同一个分区（Docker 里挂成同一个卷），或改用复制")
	// ErrTargetExists 表示目标位置已经有一个不同的文件（洗版之外不替换）。
	ErrTargetExists = errors.New("目标位置已经有同名文件（不是同一个文件），没有覆盖")
	// ErrSourceMissing 表示源文件不存在。
	ErrSourceMissing = errors.New("源文件不存在")
	// ErrBadMode 表示整理方式不认识。
	ErrBadMode = errors.New("整理方式无效（可选 hardlink、copy、symlink、move）")
)

// Outcome 是一次整理的结果。
type Outcome int

const (
	// Created 表示这次新放进了库里。
	Created Outcome = iota
	// AlreadyDone 表示库里已经是这个文件（同一个 inode、指向源文件的软链接，或之前复制、移动过去的）。
	AlreadyDone
)

// NormalizeMode 把空串换成硬链接，并检查整理方式。
func NormalizeMode(mode string) (string, error) {
	switch mode {
	case "", models.MediaModeHardlink:
		return models.MediaModeHardlink, nil
	case models.MediaModeCopy, models.MediaModeSymlink, models.MediaModeMove:
		return mode, nil
	}
	return "", fmt.Errorf("%w: %q", ErrBadMode, mode)
}

// link 与 rename 是 os.Link、os.Rename，测试里换成假的以覆盖跨文件系统的情况。
var (
	link   = os.Link
	rename = os.Rename
)

// Transfer 把源文件 src 按 mode 放到 dst（父目录不存在时建好）。dst 已经是这个文件时返回 AlreadyDone；
// 是别的文件时返回 ErrTargetExists，不覆盖。硬链接跨文件系统时返回 ErrCrossDevice。
func Transfer(src, dst, mode string) (Outcome, error) {
	mode, err := NormalizeMode(mode)
	if err != nil {
		return 0, err
	}
	srcInfo, srcErr := os.Stat(src)
	dstInfo, dstErr := os.Lstat(dst)
	if dstErr == nil {
		return existing(src, dst, mode, srcInfo, srcErr, dstInfo)
	}
	if !errors.Is(dstErr, os.ErrNotExist) {
		return 0, fmt.Errorf("检查目标失败: %w", dstErr)
	}
	if srcErr != nil {
		if errors.Is(srcErr, os.ErrNotExist) {
			return 0, ErrSourceMissing
		}
		return 0, fmt.Errorf("读取源文件失败: %w", srcErr)
	}
	if !srcInfo.Mode().IsRegular() {
		return 0, fmt.Errorf("源文件不是普通文件: %s", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, fmt.Errorf("建立目录失败: %w", err)
	}
	switch mode {
	case models.MediaModeHardlink:
		if err := link(src, dst); err != nil {
			if isCrossDevice(err) {
				return 0, ErrCrossDevice
			}
			return 0, fmt.Errorf("建立硬链接失败: %w", err)
		}
	case models.MediaModeSymlink:
		if err := os.Symlink(src, dst); err != nil {
			return 0, fmt.Errorf("建立软链接失败: %w", err)
		}
	case models.MediaModeCopy:
		if err := copyFile(src, dst, srcInfo); err != nil {
			return 0, err
		}
	case models.MediaModeMove:
		if err := rename(src, dst); err != nil {
			if !isCrossDevice(err) {
				return 0, fmt.Errorf("移动失败: %w", err)
			}
			if err := copyFile(src, dst, srcInfo); err != nil {
				return 0, err
			}
			if err := os.Remove(src); err != nil {
				return 0, fmt.Errorf("已复制到库里，但删除源文件失败: %w", err)
			}
		}
	}
	return Created, nil
}

// existing 判断已经存在的目标是不是这次要放的文件。
func existing(src, dst, mode string, srcInfo os.FileInfo, srcErr error, dstInfo os.FileInfo) (Outcome, error) {
	switch mode {
	case models.MediaModeSymlink:
		if dstInfo.Mode()&os.ModeSymlink != 0 {
			if to, err := os.Readlink(dst); err == nil && filepath.Clean(to) == filepath.Clean(src) {
				return AlreadyDone, nil
			}
		}
	case models.MediaModeMove:
		// 移动过去以后源文件就没了
		if errors.Is(srcErr, os.ErrNotExist) && dstInfo.Mode().IsRegular() {
			return AlreadyDone, nil
		}
		if srcErr == nil && os.SameFile(srcInfo, dstInfo) {
			return AlreadyDone, nil
		}
	case models.MediaModeCopy:
		// 之前复制过去的（复制完、记录没写上时进程退出了）：大小相同就当是同一份
		if srcErr == nil && dstInfo.Mode().IsRegular() && dstInfo.Size() == srcInfo.Size() {
			return AlreadyDone, nil
		}
	default:
		if srcErr == nil && os.SameFile(srcInfo, dstInfo) {
			return AlreadyDone, nil
		}
	}
	if srcErr != nil && errors.Is(srcErr, os.ErrNotExist) {
		return 0, ErrSourceMissing
	}
	return 0, ErrTargetExists
}

// copyFile 先复制到同目录的临时文件，写完、落盘后再改名，中途失败不会在库里留下半个文件。
// 修改时间跟源文件一样（媒体服务器按它排「最近添加」）。
func copyFile(src, dst string, srcInfo os.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %w", err)
	}
	defer in.Close()
	var rnd [6]byte
	_, _ = rand.Read(rnd[:])
	tmp := dst + ".pt-tools-" + hex.EncodeToString(rnd[:]) + ".part"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("建立临时文件失败: %w", err)
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("复制失败: %w", err)
	}
	_ = os.Chtimes(tmp, srcInfo.ModTime(), srcInfo.ModTime())
	// 用硬链接放到目标位置：目标在这期间出现了也不会被覆盖；文件系统不支持硬链接时再改名
	if err := os.Link(tmp, dst); err == nil {
		_ = os.Remove(tmp)
		return nil
	} else if errors.Is(err, os.ErrExist) {
		_ = os.Remove(tmp)
		return ErrTargetExists
	}
	if _, err := os.Lstat(dst); err == nil {
		_ = os.Remove(tmp)
		return ErrTargetExists
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("复制完改名失败: %w", err)
	}
	return nil
}

// RemoveIfOurs 删掉库里的 dst，前提是它还是当初整理出的那个文件：硬链接、复制、移动按文件编号（fileID）确认，
// 软链接按指向的源文件确认。已经不在了返回 nil；确认不了时不删、返回错误。
func RemoveIfOurs(dst, mode, fileID, src string) error {
	info, err := os.Lstat(dst)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("检查库里的文件失败: %w", err)
	}
	if mode == models.MediaModeSymlink {
		to, err := os.Readlink(dst)
		if err != nil || filepath.Clean(to) != filepath.Clean(src) {
			return fmt.Errorf("库里的 %s 已经不是指向源文件的软链接，没有删除", filepath.Base(dst))
		}
	} else {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("库里的 %s 不是普通文件，没有删除", filepath.Base(dst))
		}
		id, err := FileID(dst)
		if err != nil || fileID == "" || id != fileID {
			return fmt.Errorf("库里的 %s 已经换成了别的文件，没有删除", filepath.Base(dst))
		}
	}
	if err := os.Remove(dst); err != nil {
		return fmt.Errorf("删除库里的文件失败: %w", err)
	}
	return nil
}

// RemoveEmptyDirs 从 dir 往上删空目录，到 root 为止（root 本身不删）。
func RemoveEmptyDirs(dir, root string) {
	root = filepath.Clean(root)
	for d := filepath.Clean(dir); d != root && Within(root, d); d = filepath.Dir(d) {
		entries, err := os.ReadDir(d)
		if err != nil || len(entries) > 0 {
			return
		}
		if os.Remove(d) != nil {
			return
		}
	}
}

// Within 报告 p 是不是在 root 里面（不含 root 本身）。
func Within(root, p string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(p))
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
