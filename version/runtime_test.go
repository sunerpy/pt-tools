package version

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
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
