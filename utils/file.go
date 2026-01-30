package utils

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// CheckDirectory 检查目录是否存在且为空
// 返回值：
// - exists: 目录是否存在
// - empty: 目录是否为空（如果存在）
// - err: 错误信息（如果存在）
func CheckDirectory(path string) (exists, empty bool, err error) {
	// 判断目录是否存在
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, false, nil // 目录不存在
	}
	if err != nil {
		return false, false, err // 其他错误
	}
	if !info.IsDir() {
		return false, false, fmt.Errorf("路径不是目录: %s", path)
	}
	// 判断目录是否为空
	files, err := os.ReadDir(path)
	if err != nil {
		return true, false, err // 存在但读取目录失败
	}
	return true, len(files) == 0, nil // 返回是否存在以及是否为空
}

func DirectoryExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil // 目录不存在
	}
	if err != nil {
		return false, err // 其他错误
	}
	return info.IsDir(), nil // 检查是否为目录
}

func IsDirectoryEmpty(path string) (bool, error) {
	files, err := os.ReadDir(path)
	if err != nil {
		return false, err // 读取目录失败
	}
	return len(files) == 0, nil // 如果文件数为0，则目录为空
}

func ResolveDownloadBase(home, work, dir string) (string, error) {
	d := strings.TrimSpace(dir)
	if d == "" {
		return "", fmt.Errorf("下载目录不能为空")
	}
	var base string
	if filepath.IsAbs(d) {
		base = d
	} else {
		base = filepath.Join(home, work, d)
	}
	if _, err := os.Stat(base); os.IsNotExist(err) {
		if err = os.MkdirAll(base, 0o755); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	return base, nil
}

func SubPathFromTag(tag string) string {
	return strings.TrimSpace(tag)
}

// sensitiveParams 定义需要脱敏的 URL 查询参数名（小写匹配）
var sensitiveParams = map[string]bool{
	"passkey":      true,
	"sign":         true,
	"apikey":       true,
	"api_key":      true,
	"token":        true,
	"secret":       true,
	"key":          true,
	"authkey":      true,
	"auth":         true,
	"password":     true,
	"pwd":          true,
	"rsskey":       true,
	"torrent_pass": true,
}

// SanitizeURL 对 URL 中的敏感查询参数进行脱敏处理
// 返回脱敏后的 URL 字符串，敏感参数值会被替换为 "***"
// 如果解析失败，返回 "<invalid-url>"
func SanitizeURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "<invalid-url>"
	}

	query := parsed.Query()
	if len(query) == 0 {
		return rawURL
	}

	modified := false
	for key := range query {
		if sensitiveParams[strings.ToLower(key)] {
			query.Set(key, "***")
			modified = true
		}
	}

	if !modified {
		return rawURL
	}

	parsed.RawQuery = query.Encode()
	return parsed.String()
}
