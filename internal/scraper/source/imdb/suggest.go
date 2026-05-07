package imdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

// searchSuggest 调用 IMDb 公开的 suggest 端点（无鉴权 JSON API）。
// URL: https://v3.sg.media-imdb.com/suggestion/x/{query}.json
// 这个端点驱动 IMDb 主页搜索框的 autocomplete，不需要 API key。
// 返回至多 10 条建议，我们在调用方过滤 MediaType 和 Year。
func (s *Scraper) searchSuggest(ctx context.Context, query string, year int, kind core.MediaType) ([]core.MediaSearchCandidate, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("imdb suggest: %w", core.ErrInvalidID)
	}
	// IMDb suggest 以 query 首字母分桶；路径形如 /suggestion/x/query.json（x 为首字母）。
	first := strings.ToLower(string([]rune(q)[0]))
	endpoint := "https://v3.sg.media-imdb.com/suggestion/" + first + "/" + url.PathEscape(q) + ".json"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("imdb suggest build: %w", err)
	}
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", s.baseURL+"/")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("imdb suggest request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("imdb suggest: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("imdb suggest read: %w", err)
	}

	var payload struct {
		D []struct {
			ID   string          `json:"id"`
			L    string          `json:"l"` // label (title)
			Q    string          `json:"q"` // kind: "feature"/"TV series"/...
			Y    int             `json:"y"` // year
			I    json.RawMessage `json:"i"` // image (ignored)
			QID  string          `json:"qid"`
			RANK int             `json:"rank"`
		} `json:"d"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("imdb suggest decode: %w", err)
	}

	out := make([]core.MediaSearchCandidate, 0, len(payload.D))
	for _, item := range payload.D {
		if !strings.HasPrefix(item.ID, "tt") {
			continue
		}
		itemKind := classifyIMDbKind(item.Q, item.QID)
		if kind != core.MediaTypeUnknown && itemKind != core.MediaTypeUnknown && itemKind != kind {
			continue
		}
		if year > 0 && item.Y > 0 && item.Y != year {
			continue
		}
		out = append(out, core.MediaSearchCandidate{
			ID:        item.ID,
			Title:     item.L,
			Year:      item.Y,
			MediaType: itemKind,
			Provider:  providerInfo.Name,
		})
	}
	return out, nil
}

// classifyIMDbKind 将 IMDb suggest 的 q/qid 字段映射到 core.MediaType。
// 已知取值（来自 IMDb suggest 端点的实际响应）：
//   - "feature"       → movie
//   - "TV series"     → tvShow
//   - "TV mini series"→ tvShow
//   - "TV movie"      → movie
//   - "video"         → movie (direct-to-video)
//   - "short"         → movie (short film)
//   - "video game"    → Unknown（显式排除，避免被 "video" 误匹配为 movie）
//   - 其他（podcast / music video 等）→ Unknown
func classifyIMDbKind(q, qid string) core.MediaType {
	combined := strings.ToLower(q + " " + qid)
	// 显式排除 video game（在 "video" 泛匹配之前检查）。
	if strings.Contains(combined, "video game") {
		return core.MediaTypeUnknown
	}
	switch {
	case strings.Contains(combined, "feature"),
		strings.Contains(combined, "tv movie"),
		strings.Contains(combined, "movie"),
		strings.Contains(combined, "video"),
		strings.Contains(combined, "short"):
		return core.MediaTypeMovie
	case strings.Contains(combined, "tv series"),
		strings.Contains(combined, "tv mini"),
		strings.Contains(combined, "series"):
		return core.MediaTypeTvShow
	default:
		return core.MediaTypeUnknown
	}
}
