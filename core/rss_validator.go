package core

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/models"
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
			return models.RSSConfig{}, fmt.Errorf("RSS 的 URL 与已有订阅重复: %s", entry.URL)
		}
	}
	if entry.IntervalMinutes < models.MinIntervalMinutes {
		entry.IntervalMinutes = models.MinIntervalMinutes
	}
	return entry, nil
}
