package assistant

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// maxListedModels 是模型列表最多回几个（有的服务商列出上千个）
const maxListedModels = 500

// ModelList 是服务商 GET /models 列出的模型去掉嵌入、语音、图像这类以后剩下的，给设置页的模型下拉框。
// 按名字判断，不保证每个都支持 Chat Completions 与工具调用（选好以后测试连接）。
type ModelList struct {
	Models []string `json:"models"`
	// Total 是服务商列出的模型数（去掉之前）
	Total int `json:"total"`
}

// ListModels 按输入（还没保存的也行）读服务商的模型列表。Key 的规则同 TestSettings：输入没给 Key 时用存着的，
// 但接口地址变了就不带。模型可以还没选，接口地址要有。
func (s *Service) ListModels(ctx context.Context, in SettingsInput) (ModelList, error) {
	stored, _, err := s.loadSettings(ctx)
	if err != nil {
		return ModelList{}, err
	}
	in.Enabled = false
	row, err := s.merged(stored, in)
	if err != nil {
		return ModelList{}, err
	}
	if row.BaseURL == "" {
		return ModelList{}, fmt.Errorf("%w: 先填接口地址", ErrInvalid)
	}
	c, err := s.decrypted(row)
	if err != nil {
		return ModelList{}, err
	}
	svc, err := s.modelService(c)
	if err != nil {
		return ModelList{}, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	page, err := svc.List(ctx)
	if err != nil {
		return ModelList{}, &TestError{Message: describeError(err, c.APIKey)}
	}
	ids := make([]string, 0, len(page.Data))
	for _, m := range page.Data {
		ids = append(ids, m.ID)
	}
	return ModelList{Models: chatModels(c.BaseURL, ids, c.APIKey), Total: len(ids)}, nil
}

// notChat 是模型名里出现就不能拿来对话的词：嵌入、重排、语音识别与合成、只走 Realtime 接口的、图像、视频、审核、
// 提示词防护，以及只有旧 Completions 接口的 babbage、davinci。gpt-audio 这类音频对话模型支持 Chat Completions，不去掉。
var notChat = []string{
	"embed", "rerank", "whisper", "tts", "transcribe", "speech", "orpheus", "realtime",
	"dall-e", "image", "imagen", "veo", "sora", "lyria", "moderation", "guard", "babbage", "davinci",
}

// googleNotText 是 Google AI Studio 的 Gemini 与 Gemma 里不是文本对话的（同 Voltip 的 usable_models；Nano Banana 是生成图像的）。
var googleNotText = []string{
	"embedding", "-tts", "-image", "-live", "native-audio", "transcribe", "robotics", "computer-use",
	"imagen", "veo", "lyria", "omni", "aqa", "banana",
}

// chatModels 整理服务商列出的模型编号：Google AI Studio 写成 models/gemini-…，去掉前缀，只留 Gemini 与 Gemma 的文本模型；
// 别的服务商去掉名字里有 notChat 的。设置里存不下的（有空白、太长）与回显了 Key 的不要；去重，按名字排，最多 maxListedModels 个。
func chatModels(baseURL string, ids []string, key string) []string {
	google := isGoogleAIStudio(baseURL)
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if google {
			id = strings.TrimPrefix(id, "models/")
		}
		lower := strings.ToLower(id)
		switch {
		case id == "" || len(id) > maxModelLen || hasSpace(id) || seen[id]:
			continue
		case key != "" && strings.Contains(id, key):
			continue
		case google && (!strings.HasPrefix(lower, "gemini-") && !strings.HasPrefix(lower, "gemma-") || containsAny(lower, googleNotText)):
			continue
		case !google && containsAny(lower, notChat):
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	slices.Sort(out)
	if len(out) > maxListedModels {
		out = out[:maxListedModels]
	}
	return out
}

// isGoogleAIStudio 是 Gemini API 的 OpenAI 兼容地址（generativelanguage.googleapis.com）。
func isGoogleAIStudio(baseURL string) bool {
	u, err := url.Parse(baseURL)
	return err == nil && strings.EqualFold(u.Hostname(), "generativelanguage.googleapis.com")
}

func containsAny(s string, words []string) bool {
	return slices.ContainsFunc(words, func(w string) bool { return strings.Contains(s, w) })
}
