package version

import (
	"encoding/json"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectEnvironment(t *testing.T) {
	env := DetectEnvironment()

	assert.Equal(t, runtime.GOOS, env.OS)
	assert.Equal(t, runtime.GOARCH, env.Arch)
	assert.NotEmpty(t, env.Executable)
}

func TestGetAssetNameForPlatform(t *testing.T) {
	tests := []struct {
		name     string
		os       string
		arch     string
		expected string
	}{
		{
			name:     "Linux AMD64",
			os:       "linux",
			arch:     "amd64",
			expected: "pt-tools-linux-amd64.tar.gz",
		},
		{
			name:     "Linux ARM64",
			os:       "linux",
			arch:     "arm64",
			expected: "pt-tools-linux-arm64.tar.gz",
		},
		{
			name:     "Windows AMD64",
			os:       "windows",
			arch:     "amd64",
			expected: "pt-tools-windows-amd64.exe.zip",
		},
		{
			name:     "Windows ARM64",
			os:       "windows",
			arch:     "arm64",
			expected: "pt-tools-windows-arm64.exe.zip",
		},
		{
			name:     "Darwin AMD64",
			os:       "darwin",
			arch:     "amd64",
			expected: "pt-tools-darwin-amd64.tar.gz",
		},
		{
			name:     "Unsupported OS",
			os:       "freebsd",
			arch:     "amd64",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetAssetNameForPlatform(tt.os, tt.arch)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetBinaryName(t *testing.T) {
	tests := []struct {
		os       string
		expected string
	}{
		{"linux", "pt-tools"},
		{"darwin", "pt-tools"},
		{"windows", "pt-tools.exe"},
	}

	for _, tt := range tests {
		t.Run(tt.os, func(t *testing.T) {
			result := GetBinaryName(tt.os)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetAssetName(t *testing.T) {
	result := GetAssetName()
	expected := GetAssetNameForPlatform(runtime.GOOS, runtime.GOARCH)
	assert.Equal(t, expected, result)
}

func TestDetectEnvironmentBuildOverrides(t *testing.T) {
	oldOS, oldArch := BuildOS, BuildArch
	defer func() { BuildOS, BuildArch = oldOS, oldArch }()
	BuildOS = "linux"
	BuildArch = "arm64"

	env := DetectEnvironment()
	assert.Equal(t, "linux", env.OS)
	assert.Equal(t, "arm64", env.Arch)
	assert.True(t, env.CanSelfUpgrade)
}

func TestIsInAppDir(t *testing.T) {
	assert.False(t, isInAppDir(), "test binary is not under /app")
}

func TestDetectDocker(t *testing.T) {
	assert.False(t, detectDocker(), "test host is not the pt-tools docker image")
}

// 飞牛应用的启动脚本设 PT_TOOLS_MANAGED_BY=fnos：运行环境要报出来，网页不提供应用内升级；别的值当作没设置。
func TestDetectEnvironmentManagedBy(t *testing.T) {
	oldOS, oldArch := BuildOS, BuildArch
	t.Cleanup(func() { BuildOS, BuildArch = oldOS, oldArch })
	BuildOS, BuildArch = "linux", "amd64"

	tests := []struct {
		name  string
		value *string
		want  string
	}{
		{name: "fnOS app", value: ptr("fnos"), want: ManagedByFnOS},
		{name: "surrounding spaces", value: ptr(" fnos "), want: ManagedByFnOS},
		{name: "unknown value", value: ptr("synology"), want: ""},
		{name: "empty", value: ptr(""), want: ""},
		{name: "not set", value: nil, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(managedByEnv, "")
			if tt.value == nil {
				require.NoError(t, os.Unsetenv(managedByEnv))
			} else {
				t.Setenv(managedByEnv, *tt.value)
			}
			env := DetectEnvironment()
			assert.Equal(t, tt.want, env.ManagedBy)
			raw, err := json.Marshal(env)
			require.NoError(t, err)
			if tt.want == "" {
				assert.Equal(t, !env.IsDocker, env.CanSelfUpgrade)
				assert.NotContains(t, string(raw), "managed_by")
			} else {
				assert.False(t, env.CanSelfUpgrade)
				assert.Contains(t, string(raw), `"managed_by":"fnos"`)
			}
		})
	}
}

func ptr(s string) *string { return &s }
