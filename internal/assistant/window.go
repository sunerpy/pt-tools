package assistant

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/openai/openai-go/v3"
)

// 上下文窗口的来处（UsageView.WindowSource）。
const (
	windowFromSetting  = "setting"
	windowFromProvider = "provider"
	windowFromTable    = "table"
	windowFromDefault  = "default"
)

// 自动时的上下文窗口：按服务商的模型列表与内置的表，最多按 autoWindowCap 算（窗口很大的模型照填满再压缩太费钱，
// 要更大的在设置里填）；都不知道时按 defaultWindow。
const (
	defaultWindow = 128_000
	autoWindowCap = 128_000
)

// knownWindows 是常见模型的上下文窗口（按模型名的前缀认，小写）。只是自动时的估计：服务商的模型列表报了的以列表为准，
// 不准的在设置里填。先写更长的前缀。
var knownWindows = []struct {
	prefix string
	window int
}{
	{"gpt-4.1", 1_047_576},
	{"gpt-5", 400_000},
	{"gpt-4o", 128_000},
	{"o3", 200_000},
	{"o4", 200_000},
	{"claude", 200_000},
	{"gemini", 1_048_576},
	{"deepseek", 128_000},
	{"qwen", 131_072},
	{"kimi", 131_072},
	{"moonshot", 131_072},
	{"glm", 128_000},
	{"llama-3", 131_072},
	{"llama3", 131_072},
	{"mistral", 32_768},
	{"mixtral", 32_768},
}

// providerWindowFields 是服务商在 GET /models 的每一项里报上下文长度的字段：OpenRouter 的 context_length、
// Groq 的 context_window、vLLM 的 max_model_len、LM Studio 的 max_context_length。
var providerWindowFields = []string{"context_length", "context_window", "max_model_len", "max_context_length"}

// providerWindow 读模型列表里一项报的上下文长度，没有时为 0。
func providerWindow(m openai.Model) int {
	for _, key := range providerWindowFields {
		f, ok := m.JSON.ExtraFields[key]
		if !ok {
			continue
		}
		var n float64
		if json.Unmarshal([]byte(f.Raw()), &n) == nil && n >= minContextWindow && n <= maxContextWindow {
			return int(n)
		}
	}
	// OpenRouter 也写在 top_provider.context_length 里
	if f, ok := m.JSON.ExtraFields["top_provider"]; ok {
		var tp struct {
			ContextLength float64 `json:"context_length"`
		}
		if json.Unmarshal([]byte(f.Raw()), &tp) == nil && tp.ContextLength >= minContextWindow && tp.ContextLength <= maxContextWindow {
			return int(tp.ContextLength)
		}
	}
	return 0
}

// tableWindow 按模型名认内置表里的上下文窗口，认不出时为 0。名字里带厂商前缀的（deepseek/deepseek-chat、models/gemini-…）去掉前缀再认。
func tableWindow(model string) int {
	name := strings.ToLower(model)
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	for _, k := range knownWindows {
		if strings.HasPrefix(name, k.prefix) {
			return k.window
		}
	}
	return 0
}

// windowFor 是模型的上下文窗口与来处：设置里填了用设置的；没填按服务商的模型列表、内置的表（最多 autoWindowCap），
// 都没有时按 defaultWindow。
func (s *Service) windowFor(set effective, model string) (int, string) {
	if set.contextWindow > 0 {
		return set.contextWindow, windowFromSetting
	}
	s.modelsMu.Lock()
	w := s.models.windows[model]
	fresh := s.models.key == modelsKey(set.llm)
	s.modelsMu.Unlock()
	if w > 0 && fresh {
		return min(w, autoWindowCap), windowFromProvider
	}
	if w = tableWindow(model); w > 0 {
		return min(w, autoWindowCap), windowFromTable
	}
	return defaultWindow, windowFromDefault
}

// estimateTokens 粗算一段文字的 token：英文、数字、符号约 4 个字节一个，汉字等约一个字一个（偏多估一点，宁可早压缩）。
func estimateTokens(s string) int {
	ascii, other := 0, 0
	for _, r := range s {
		if r < utf8.RuneSelf {
			ascii++
		} else {
			other++
		}
	}
	return (ascii+3)/4 + other
}
