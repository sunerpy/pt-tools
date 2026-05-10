package douban

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

const (
	defaultBaseURL = "https://frodo.douban.com/api/v2"
	defaultHTMLURL = "https://movie.douban.com"
	apiKey         = "0dad551ec0f84ed02907ff5c42e8ec70"
	secretKey      = "bf7dddc7c9cfe6f7"
)

func signFrodo(method, path string, timestamp int64) string {
	pathEncoded := url.QueryEscape(path)
	signingString := fmt.Sprintf("%s&%s&%d", strings.ToUpper(method), pathEncoded, timestamp)
	mac := hmac.New(sha1.New, []byte(secretKey))
	_, _ = mac.Write([]byte(signingString))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
