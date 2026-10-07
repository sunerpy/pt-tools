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
	// ErrNotOurs 表示库里的文件已经不是当初整理出的那个（换成了别的文件、不是软链接），没有删除。
	ErrNotOurs = errors.New("已经不是当初整理出的文件")
)

// Outcome 是一次整理的结果。
type Outcome int

const (
	// Created 表示这次新放进了库里。
	Created Outcome = iota
	// AlreadyDone 表示库里已经是这个文件（同一个 inode，或指向源文件的软链接）。复制与移动出的文件和源文件
	// 不是同一个，认不出是不是自己放的，目标已存在时一律当作别人的文件。
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

// link、rename、linkTmp 与 noReplace 是实际的文件系统调用，测试里换成假的以覆盖跨文件系统、
// 不支持硬链接的文件系统等情况。
var (
	link      = os.Link
	linkTmp   = os.Link
	noReplace = renameNoReplace
	// removeSrc 删掉移动以后的源文件（测试里换成会失败的）
	removeSrc = os.Remove
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
		if err := move(src, dst, srcInfo); err != nil {
			return 0, err
		}
	}
	return Created, nil
}

// move 把 src 移到 dst，不覆盖 dst：同一个文件系统上用不覆盖的改名；不支持时用硬链接放过去再删源文件；
// 跨文件系统或不支持硬链接时复制过去（同样不覆盖）再删源文件。
func move(src, dst string, srcInfo os.FileInfo) error {
	err := noReplace(src, dst)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, os.ErrExist):
		return ErrTargetExists
	case errors.Is(err, errNoReplaceUnsupported):
		err = linkTmp(src, dst)
		if errors.Is(err, os.ErrExist) {
			return ErrTargetExists
		}
		if err == nil {
			return removeMoved(src, models.MediaModeHardlink)
		}
	case !isCrossDevice(err):
		return fmt.Errorf("移动失败: %w", err)
	}
	if err := copyFile(src, dst, srcInfo); err != nil {
		return err
	}
	return removeMoved(src, models.MediaModeCopy)
}

// SourceKeptError 表示移动时文件已经放进库里，但删不掉源文件（例如下载目录只读）：库里这份按 Mode 算
// （硬链接放过去的是硬链接，复制过去的是复制），源文件留在原处。调用方按 Mode 记下这份文件。
type SourceKeptError struct {
	Mode string
	Err  error
}

func (e *SourceKeptError) Error() string {
	how := "复制"
	if e.Mode == models.MediaModeHardlink {
		how = "硬链接"
	}
	return fmt.Sprintf("删不掉源文件，没有移动成：库里这份按%s放好了，源文件留在下载目录里（%v）", how, e.Err)
}

func (e *SourceKeptError) Unwrap() error { return e.Err }

// removeMoved 在目标放好以后删掉源文件；删不掉时返回 SourceKeptError，库里那份留着。
func removeMoved(src, kept string) error {
	if err := removeSrc(src); err != nil {
		return &SourceKeptError{Mode: kept, Err: err}
	}
	return nil
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
	case models.MediaModeCopy, models.MediaModeMove:
		// 复制、移动出的文件与源文件不是同一个：目标已存在时认不出是不是自己放的，当作别人的文件
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

// copyFile 先复制到同目录的临时文件，写完、落盘后再放到目标位置，中途失败不会在库里留下半个文件。
// 放到目标位置时都不覆盖已有的文件：先用硬链接；文件系统不支持硬链接时用不覆盖的改名（Linux 的
// RENAME_NOREPLACE）；也不支持时以独占方式新建目标再复制一遍。修改时间跟源文件一样（媒体服务器按它排「最近添加」）。
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
	defer os.Remove(tmp)
	err = linkTmp(tmp, dst)
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrExist) {
		return ErrTargetExists
	}
	switch err := noReplace(tmp, dst); {
	case err == nil:
		return nil
	case errors.Is(err, os.ErrExist):
		return ErrTargetExists
	case !errors.Is(err, errNoReplaceUnsupported):
		return fmt.Errorf("复制完放到库里失败: %w", err)
	}
	return exclusiveCopy(tmp, dst, srcInfo)
}

// exclusiveCopy 以独占方式新建目标（已存在时返回 ErrTargetExists），把 tmp 复制进去；中途失败删掉这个新建的目标。
func exclusiveCopy(tmp, dst string, srcInfo os.FileInfo) error {
	in, err := os.Open(tmp)
	if err != nil {
		return fmt.Errorf("打开临时文件失败: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return ErrTargetExists
	}
	if err != nil {
		return fmt.Errorf("建立目标文件失败: %w", err)
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("复制失败: %w", err)
	}
	_ = os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime())
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
			return fmt.Errorf("%w：库里的 %s 不再是指向源文件的软链接，没有删除", ErrNotOurs, filepath.Base(dst))
		}
	} else {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w：库里的 %s 不是普通文件，没有删除", ErrNotOurs, filepath.Base(dst))
		}
		id, err := FileID(dst)
		if err != nil || fileID == "" || id != fileID {
			return fmt.Errorf("%w：库里的 %s 已经换成了别的文件或改过，没有删除", ErrNotOurs, filepath.Base(dst))
		}
	}
	if err := os.Remove(dst); err != nil {
		return fmt.Errorf("删除库里的文件失败: %w", err)
	}
	return nil
}

// RemoveEmptyDirs 从 dir 往上删空目录，到 root 为止（root 本身不删）。目录的实际位置（解开软链接后）
// 不在 root 的实际位置里时停下：库里有指到别处的目录软链接时，不去删库外的空目录。
func RemoveEmptyDirs(dir, root string) {
	root = filepath.Clean(root)
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return
	}
	for d := filepath.Clean(dir); d != root && Within(root, d); d = filepath.Dir(d) {
		if real, err := filepath.EvalSymlinks(d); err != nil || !Within(realRoot, real) {
			return
		}
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
