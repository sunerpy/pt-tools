package version

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/requests"
)

var (
	ErrDockerEnvironment    = errors.New("自动升级不支持 Docker 环境，请更新容器镜像")
	ErrUnsupportedPlatform  = errors.New("当前平台不支持自动升级")
	ErrNoAssetFound         = errors.New("未找到适用于当前平台的安装包")
	ErrUpgradeInProgress    = errors.New("升级正在进行中")
	ErrDownloadFailed       = errors.New("下载安装包失败")
	ErrExtractionFailed     = errors.New("解压安装包失败")
	ErrReplacementFailed    = errors.New("替换可执行文件失败")
	ErrPermissionDenied     = errors.New("权限不足，无法替换可执行文件")
	ErrVersionAlreadyLatest = errors.New("当前已是最新版本")
)

type UpgradeStatus string

const (
	UpgradeStatusIdle        UpgradeStatus = "idle"
	UpgradeStatusDownloading UpgradeStatus = "downloading"
	UpgradeStatusExtracting  UpgradeStatus = "extracting"
	UpgradeStatusReplacing   UpgradeStatus = "replacing"
	UpgradeStatusCompleted   UpgradeStatus = "completed"
	UpgradeStatusFailed      UpgradeStatus = "failed"
)

type UpgradeProgress struct {
	Status          UpgradeStatus `json:"status"`
	TargetVersion   string        `json:"target_version,omitempty"`
	Progress        float64       `json:"progress"`
	BytesDownloaded int64         `json:"bytes_downloaded"`
	TotalBytes      int64         `json:"total_bytes"`
	Error           string        `json:"error,omitempty"`
	StartedAt       int64         `json:"started_at,omitempty"`
	CompletedAt     int64         `json:"completed_at,omitempty"`
}

type Upgrader struct {
	mu             sync.RWMutex
	progress       UpgradeProgress
	downloadCancel context.CancelFunc
}

var (
	defaultUpgrader *Upgrader
	upgraderOnce    sync.Once
)

func GetUpgrader() *Upgrader {
	upgraderOnce.Do(func() {
		defaultUpgrader = NewUpgrader()
	})
	return defaultUpgrader
}

func NewUpgrader() *Upgrader {
	return &Upgrader{
		progress: UpgradeProgress{Status: UpgradeStatusIdle},
	}
}

func (u *Upgrader) GetProgress() UpgradeProgress {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.progress
}

func (u *Upgrader) updateProgress(fn func(*UpgradeProgress)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	fn(&u.progress)
}

func (u *Upgrader) Cancel() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.downloadCancel != nil {
		u.downloadCancel()
		u.downloadCancel = nil
	}
	if u.progress.Status == UpgradeStatusDownloading || u.progress.Status == UpgradeStatusExtracting {
		u.progress.Status = UpgradeStatusFailed
		u.progress.Error = "升级已取消"
	}
}

func (u *Upgrader) CanUpgrade() error {
	env := DetectEnvironment()
	if env.IsDocker {
		return ErrDockerEnvironment
	}
	if !env.CanSelfUpgrade {
		return ErrUnsupportedPlatform
	}
	return nil
}

func (u *Upgrader) Upgrade(ctx context.Context, release *ReleaseInfo, proxyURL string) error {
	if err := u.CanUpgrade(); err != nil {
		return err
	}

	u.mu.Lock()
	if u.progress.Status == UpgradeStatusDownloading ||
		u.progress.Status == UpgradeStatusExtracting ||
		u.progress.Status == UpgradeStatusReplacing {
		u.mu.Unlock()
		return ErrUpgradeInProgress
	}

	downloadCtx, cancel := context.WithCancel(ctx)
	u.downloadCancel = cancel
	u.progress = UpgradeProgress{
		Status:        UpgradeStatusDownloading,
		TargetVersion: release.Version,
		StartedAt:     time.Now().Unix(),
	}
	u.mu.Unlock()

	go u.performUpgrade(downloadCtx, release, proxyURL)
	return nil
}

func (u *Upgrader) performUpgrade(ctx context.Context, release *ReleaseInfo, proxyURL string) {
	var err error
	defer func() {
		if err != nil {
			u.updateProgress(func(p *UpgradeProgress) {
				p.Status = UpgradeStatusFailed
				p.Error = err.Error()
				p.CompletedAt = time.Now().Unix()
			})
		}
	}()

	env := DetectEnvironment()
	assetName := GetAssetNameForPlatform(env.OS, env.Arch)
	if assetName == "" {
		err = ErrUnsupportedPlatform
		return
	}

	downloadURL := u.findAssetURL(release, assetName)
	if downloadURL == "" {
		err = fmt.Errorf("%w: %s", ErrNoAssetFound, assetName)
		return
	}

	tempDir, err := os.MkdirTemp("", "pt-tools-upgrade-*")
	if err != nil {
		err = fmt.Errorf("创建临时目录失败: %w", err)
		return
	}
	defer os.RemoveAll(tempDir)

	archivePath := filepath.Join(tempDir, assetName)
	if err = u.downloadFile(ctx, downloadURL, archivePath, proxyURL); err != nil {
		return
	}

	u.updateProgress(func(p *UpgradeProgress) {
		p.Status = UpgradeStatusExtracting
	})

	binaryName := GetBinaryName(env.OS)
	extractedBinary := filepath.Join(tempDir, binaryName)
	if err = u.extractBinary(archivePath, extractedBinary, binaryName); err != nil {
		return
	}

	u.updateProgress(func(p *UpgradeProgress) {
		p.Status = UpgradeStatusReplacing
	})

	if err = u.replaceBinary(env.Executable, extractedBinary); err != nil {
		return
	}

	u.updateProgress(func(p *UpgradeProgress) {
		p.Status = UpgradeStatusCompleted
		p.Progress = 100
		p.CompletedAt = time.Now().Unix()
	})
}

func (u *Upgrader) findAssetURL(release *ReleaseInfo, assetName string) string {
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			return asset.DownloadURL
		}
	}
	return ""
}

func (u *Upgrader) downloadFile(ctx context.Context, downloadURL, destPath, proxyURL string) error {
	session := requests.NewSession().WithTimeout(10 * time.Minute)
	if proxyURL != "" {
		session = session.WithProxy(proxyURL)
	}
	defer session.Close()

	req, err := requests.NewGet(downloadURL).
		WithContext(ctx).
		WithHeader("Accept", "application/octet-stream").
		WithHeader("User-Agent", "pt-tools/"+Version).
		Build()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDownloadFailed, err)
	}

	resp, err := session.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDownloadFailed, err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("%w: HTTP %d", ErrDownloadFailed, resp.StatusCode)
	}

	body := resp.Bytes()
	totalBytes := int64(len(body))
	u.updateProgress(func(p *UpgradeProgress) {
		p.TotalBytes = totalBytes
		p.BytesDownloaded = totalBytes
		p.Progress = 80
	})

	if err := os.WriteFile(destPath, body, 0o644); err != nil {
		return fmt.Errorf("%w: %v", ErrDownloadFailed, err)
	}

	return nil
}

func (u *Upgrader) extractBinary(archivePath, destPath, binaryName string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return u.extractZip(archivePath, destPath, binaryName)
	}
	if strings.HasSuffix(archivePath, ".tar.gz") {
		return u.extractTarGz(archivePath, destPath, binaryName)
	}
	return fmt.Errorf("%w: 不支持的压缩格式", ErrExtractionFailed)
}

func (u *Upgrader) extractZip(archivePath, destPath, binaryName string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrExtractionFailed, err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		if filepath.Base(file.Name) == binaryName && !file.FileInfo().IsDir() {
			src, err := file.Open()
			if err != nil {
				return fmt.Errorf("%w: %v", ErrExtractionFailed, err)
			}
			defer src.Close()

			dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrExtractionFailed, err)
			}
			defer dst.Close()

			if _, err = io.Copy(dst, src); err != nil {
				return fmt.Errorf("%w: %v", ErrExtractionFailed, err)
			}

			u.updateProgress(func(p *UpgradeProgress) {
				p.Progress = 90
			})
			return nil
		}
	}

	return fmt.Errorf("%w: 未找到 %s", ErrExtractionFailed, binaryName)
}

func (u *Upgrader) extractTarGz(archivePath, destPath, binaryName string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrExtractionFailed, err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrExtractionFailed, err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: %v", ErrExtractionFailed, err)
		}

		if filepath.Base(header.Name) == binaryName && header.Typeflag == tar.TypeReg {
			dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrExtractionFailed, err)
			}
			defer dst.Close()

			if _, err = io.Copy(dst, tarReader); err != nil {
				return fmt.Errorf("%w: %v", ErrExtractionFailed, err)
			}

			u.updateProgress(func(p *UpgradeProgress) {
				p.Progress = 90
			})
			return nil
		}
	}

	return fmt.Errorf("%w: 未找到 %s", ErrExtractionFailed, binaryName)
}

func (u *Upgrader) replaceBinary(currentPath, newPath string) error {
	if err := verifyBinary(newPath); err != nil {
		return fmt.Errorf("%w: 新版本验证失败: %v", ErrReplacementFailed, err)
	}

	if err := validateReplacementPath(currentPath); err != nil {
		return fmt.Errorf("%w: %v", ErrReplacementFailed, err)
	}

	backupPath := currentPath + ".pt-tools-backup"
	if runtime.GOOS == "windows" {
		return u.replaceWindowsBinary(currentPath, newPath, backupPath)
	}
	return u.replaceUnixBinary(currentPath, newPath, backupPath)
}

func validateReplacementPath(path string) error {
	if path == "" {
		return errors.New("路径不能为空")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("无法解析路径: %w", err)
	}

	base := filepath.Base(absPath)
	if !strings.HasPrefix(base, "pt-tools") {
		return fmt.Errorf("目标文件名必须以 pt-tools 开头: %s", base)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("无法访问目标文件: %w", err)
	}
	if info.IsDir() {
		return errors.New("目标路径是目录，不是文件")
	}

	return nil
}

func (u *Upgrader) replaceUnixBinary(currentPath, newPath, backupPath string) error {
	if err := os.Rename(currentPath, backupPath); err != nil {
		if os.IsPermission(err) {
			return ErrPermissionDenied
		}
		return fmt.Errorf("%w: 备份失败: %v", ErrReplacementFailed, err)
	}

	if err := os.Rename(newPath, currentPath); err != nil {
		_ = os.Rename(backupPath, currentPath)
		if os.IsPermission(err) {
			return ErrPermissionDenied
		}
		return fmt.Errorf("%w: 替换失败: %v", ErrReplacementFailed, err)
	}

	_ = safeRemoveBackup(backupPath)
	return nil
}

func (u *Upgrader) replaceWindowsBinary(currentPath, newPath, backupPath string) error {
	oldPath := currentPath + ".pt-tools-old"

	_ = safeRemoveBackup(oldPath)
	_ = safeRemoveBackup(backupPath)

	if err := os.Rename(currentPath, oldPath); err != nil {
		if os.IsPermission(err) {
			return ErrPermissionDenied
		}
		return fmt.Errorf("%w: 重命名当前文件失败: %v", ErrReplacementFailed, err)
	}

	if err := copyFile(newPath, currentPath); err != nil {
		_ = os.Rename(oldPath, currentPath)
		if os.IsPermission(err) {
			return ErrPermissionDenied
		}
		return fmt.Errorf("%w: 复制新文件失败: %v", ErrReplacementFailed, err)
	}

	return nil
}

func safeRemoveBackup(path string) error {
	if path == "" {
		return nil
	}

	base := filepath.Base(path)
	if !strings.Contains(base, "pt-tools") {
		return errors.New("拒绝删除非 pt-tools 相关文件")
	}
	if !strings.HasSuffix(base, ".pt-tools-backup") && !strings.HasSuffix(base, ".pt-tools-old") {
		return errors.New("拒绝删除非备份文件")
	}

	return os.Remove(path)
}

func verifyBinary(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() < 1024*1024 {
		return errors.New("文件大小异常")
	}
	return nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// CleanupOldBinary removes leftover .pt-tools-old and .pt-tools-backup files from previous upgrades.
// Call this at application startup.
func CleanupOldBinary() {
	exe, err := os.Executable()
	if err != nil {
		return
	}

	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return
	}

	base := filepath.Base(exe)
	if !strings.HasPrefix(base, "pt-tools") {
		return
	}

	oldPath := exe + ".pt-tools-old"
	if _, err := os.Stat(oldPath); err == nil {
		_ = safeRemoveBackup(oldPath)
	}

	backupPath := exe + ".pt-tools-backup"
	if _, err := os.Stat(backupPath); err == nil {
		_ = safeRemoveBackup(backupPath)
	}
}
