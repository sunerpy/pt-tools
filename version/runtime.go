package version

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	dockerMarkerPath    = "/app/.pt-tools-docker"
	dockerMarkerContent = "pt-tools-docker-build"

	// managedByEnv 由安装包的启动脚本设置，说明 pt-tools 由哪个应用中心管理（deploy/fnos/native/cmd/main 设为 fnos）。
	// 这时二进制在应用中心的目录里，版本也由应用中心记录，网页里不提供升级，升级走应用中心。
	managedByEnv = "PT_TOOLS_MANAGED_BY"
	// ManagedByFnOS 表示 pt-tools 是飞牛应用中心安装的应用（.fpk）。
	ManagedByFnOS = "fnos"
)

type RuntimeEnvironment struct {
	IsDocker       bool   `json:"is_docker"`
	ManagedBy      string `json:"managed_by,omitempty"`
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	Executable     string `json:"executable"`
	CanSelfUpgrade bool   `json:"can_self_upgrade"`
}

func DetectEnvironment() RuntimeEnvironment {
	isDocker := detectDocker()
	execPath, _ := os.Executable()
	execPath, _ = filepath.EvalSymlinks(execPath)

	osName := runtime.GOOS
	archName := runtime.GOARCH

	if BuildOS != "" {
		osName = BuildOS
	}
	if BuildArch != "" {
		archName = BuildArch
	}

	env := RuntimeEnvironment{
		IsDocker:   isDocker,
		ManagedBy:  detectManagedBy(),
		OS:         osName,
		Arch:       archName,
		Executable: execPath,
	}

	env.CanSelfUpgrade = !isDocker && env.ManagedBy == "" && (osName == "linux" || osName == "windows")

	return env
}

// detectManagedBy 只认得已知的应用中心；别的值当作没设置。
func detectManagedBy() string {
	if strings.TrimSpace(os.Getenv(managedByEnv)) == ManagedByFnOS {
		return ManagedByFnOS
	}
	return ""
}

func detectDocker() bool {
	if data, err := os.ReadFile(dockerMarkerPath); err == nil {
		if string(data) == dockerMarkerContent {
			return true
		}
	}

	if _, err := os.Stat("/.dockerenv"); err == nil {
		if isInAppDir() {
			return true
		}
	}

	return false
}

func isInAppDir() bool {
	execPath, err := os.Executable()
	if err != nil {
		return false
	}
	execPath, _ = filepath.EvalSymlinks(execPath)
	dir := filepath.Dir(execPath)

	return dir == "/app/bin" || dir == "/app"
}

func GetAssetName() string {
	env := DetectEnvironment()
	return GetAssetNameForPlatform(env.OS, env.Arch)
}

// GetAssetNameForPlatform returns the asset name matching release workflow:
// Linux: pt-tools-linux-{arch}.tar.gz, Windows: pt-tools-windows-{arch}.exe.zip
func GetAssetNameForPlatform(osName, arch string) string {
	switch osName {
	case "windows":
		return "pt-tools-windows-" + arch + ".exe.zip"
	case "linux":
		return "pt-tools-linux-" + arch + ".tar.gz"
	case "darwin":
		return "pt-tools-darwin-" + arch + ".tar.gz"
	default:
		return ""
	}
}

func GetBinaryName(osName string) string {
	if osName == "windows" {
		return "pt-tools.exe"
	}
	return "pt-tools"
}
