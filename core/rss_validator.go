package core

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/utils"
)

// validateAndNormalizeRSS checks an entry against existing RSS entries; callers handle whole-list error context.
func validateAndNormalizeRSS(existing []models.RSSConfig, entry models.RSSConfig) (models.RSSConfig, error) {
	if strings.TrimSpace(entry.Name) == "" {
		return models.RSSConfig{}, errors.New("RSS 的 name 不能为空")
	}
	if strings.TrimSpace(entry.URL) == "" {
		return models.RSSConfig{}, errors.New("RSS 的 url 不能为空")
	}
	normalized := strings.TrimSpace(strings.ToLower(entry.URL))
	for _, r := range existing {
		if strings.TrimSpace(strings.ToLower(r.URL)) == normalized {
			return models.RSSConfig{}, fmt.Errorf("RSS 的 URL 与已有订阅重复: %s", utils.SanitizeURL(entry.URL))
		}
	}
	if entry.IntervalMinutes < models.MinIntervalMinutes {
		entry.IntervalMinutes = models.MinIntervalMinutes
	}
	return entry, nil
}

// rssPlaceholderError 拒绝示例里的占位地址（hdsky.xxx 这类）：照着示例填、忘了换成真实链接的 RSS 永远取不到。
func rssPlaceholderError(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !models.IsPlaceholderRSSHost(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("RSS 地址是示例里的占位地址（%s）：填站点 RSS 页上给你的链接", u.Hostname())
}
