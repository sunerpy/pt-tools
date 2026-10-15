package tmdb

import (
	"errors"
	"net/url"
	"strings"
)

// NormalizeAPIBaseURL 规整用户填的 TMDB 接口地址：只写域名时补上 https:// 与 /3，去掉末尾的 /。
// 空串表示用官方地址（DefaultBaseURL）。可以是镜像或自建的中转，带不带子路径都行。
func NormalizeAPIBaseURL(raw string) (string, error) {
	u, err := parseBaseURL(raw, "TMDB 接口地址")
	if err != nil || u == nil {
		return "", err
	}
	if u.Path == "" {
		u.Path = "/3"
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// NormalizeImageBaseURL 规整用户填的 TMDB 图片地址（到尺寸之前为止，例如 https://image.tmdb.org/t/p/）：
// 只写域名时补上 https:// 与 /t/p/，末尾总是带 /。空串表示用官方地址（ImageBaseURL）。
func NormalizeImageBaseURL(raw string) (string, error) {
	u, err := parseBaseURL(raw, "TMDB 图片地址")
	if err != nil || u == nil {
		return "", err
	}
	if u.Path == "" {
		u.Path = "/t/p"
	}
	return strings.TrimRight(u.String(), "/") + "/", nil
}

// parseBaseURL 解析 http(s) 地址：没写协议时当作 https；不能带用户名密码、查询串与片段。空串返回 nil。
func parseBaseURL(raw, what string) (*url.URL, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, nil
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return nil, errors.New(what + "格式不对")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New(what + "要以 http:// 或 https:// 开头")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.HasSuffix(s, "?") || strings.Contains(s, "#") {
		return nil, errors.New(what + "不能带用户名密码、问号或井号")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u, nil
}

// PosterURL 是这个客户端的图片地址下的海报地址（size 为空时用 w342）：用户换了图片地址时，通知里的海报也用它。
func (c *Client) PosterURL(path, size string) string {
	if path == "" {
		return ""
	}
	if size == "" {
		size = "w342"
	}
	return c.imageBase + size + "/" + strings.TrimPrefix(path, "/")
}
