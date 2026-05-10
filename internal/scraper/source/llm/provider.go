package llm

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// ProviderKind 标识底层 LLM 接入方式。
type ProviderKind string

const (
	KindOpenAICompat ProviderKind = "openai_compat"
	KindAnthropic    ProviderKind = "anthropic"
	KindOllamaNative ProviderKind = "ollama_native"
	KindMCPSampling  ProviderKind = "mcp_sampling"
	KindChained      ProviderKind = "chained"
)

// Provider LLM 适配器统一接口。
type Provider interface {
	Name() string
	Kind() ProviderKind
	Extract(ctx context.Context, req ExtractRequest) (*NFOResult, error)
	Close() error
}

// ExtractRequest LLM 提取元数据的请求。
type ExtractRequest struct {
	RawTitle    string
	Filename    string
	FileSize    int64
	SiteHints   map[string]string
	UserContext string
	Language    string
	MediaType   string
}

// NFOResult LLM 提取结果（严格 schema）。
type NFOResult struct {
	Title         string   `json:"title" jsonschema:"description=Official title without release tags"`
	OriginalTitle string   `json:"original_title,omitempty"`
	Year          int      `json:"year,omitempty" jsonschema:"minimum=1900,maximum=2100"`
	Type          string   `json:"type,omitempty" jsonschema:"enum=movie,enum=tv,enum=unknown"`
	TMDBID        int      `json:"tmdb_id,omitempty" jsonschema:"description=Set ONLY if 100% certain, otherwise OMIT"`
	IMDBID        string   `json:"imdb_id,omitempty" jsonschema:"description=Format tt1234567, ONLY if explicit in input"`
	Season        int      `json:"season,omitempty"`
	Episode       int      `json:"episode,omitempty"`
	Genres        []string `json:"genres,omitempty"`
	Language      string   `json:"language,omitempty"`
	Plot          string   `json:"plot,omitempty"`
	Directors     []string `json:"directors,omitempty"`
	Cast          []string `json:"cast,omitempty"`
	Runtime       int      `json:"runtime,omitempty"`

	GeneratedBy string    `json:"-"`
	GeneratedAt time.Time `json:"-"`
}

func decodeResultPayload(payload string) (*NFOResult, error) {
	cleaned := strings.TrimSpace(payload)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var result NFOResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, err
	}
	return &result, nil
}
