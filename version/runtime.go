package version

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	dockerMarkerPath    = "/app/.pt-tools-docker"
	dockerMarkerContent = "pt-tools-docker-build"
)

type RuntimeEnvironment struct {
	IsDocker       bool   `json:"is_docker"`
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
		OS:         osName,
		Arch:       archName,
		Executable: execPath,
	}

	env.CanSelfUpgrade = !isDocker && (osName == "linux" || osName == "windows")

	return env
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
