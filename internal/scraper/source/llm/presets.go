package llm

// Preset OpenAI-compat 兼容 provider 的预设配置。
type Preset struct {
	BaseURL              string
	DefaultModels        []string
	SupportsStrictSchema bool
	Notes                string
}

// Presets 内置 10+ LLM provider 目录。
var Presets = map[string]Preset{
	"openai": {
		BaseURL:              "https://api.openai.com/v1",
		DefaultModels:        []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "gpt-3.5-turbo"},
		SupportsStrictSchema: true,
	},
	"kimi": {
		BaseURL:              "https://api.moonshot.cn/v1",
		DefaultModels:        []string{"moonshot-v1-8k", "moonshot-v1-32k", "moonshot-v1-128k"},
		SupportsStrictSchema: false,
		Notes:                "月之暗面（Moonshot）",
	},
	"glm": {
		BaseURL:              "https://open.bigmodel.cn/api/paas/v4",
		DefaultModels:        []string{"glm-4-plus", "glm-4-air", "glm-4-flash"},
		SupportsStrictSchema: false,
		Notes:                "智谱 GLM",
	},
	"qwen": {
		BaseURL:              "https://dashscope.aliyuncs.com/compatible-mode/v1",
		DefaultModels:        []string{"qwen-max", "qwen-plus", "qwen-turbo", "qwen2.5-72b-instruct"},
		SupportsStrictSchema: false,
		Notes:                "通义千问",
	},
	"deepseek": {
		BaseURL:              "https://api.deepseek.com/v1",
		DefaultModels:        []string{"deepseek-chat", "deepseek-reasoner"},
		SupportsStrictSchema: false,
	},
	"doubao": {
		BaseURL:              "https://ark.cn-beijing.volces.com/api/v3",
		DefaultModels:        []string{"doubao-pro-128k", "doubao-lite"},
		SupportsStrictSchema: false,
		Notes:                "字节豆包",
	},
	"yi": {
		BaseURL:              "https://api.lingyiwanwu.com/v1",
		DefaultModels:        []string{"yi-large", "yi-medium"},
		SupportsStrictSchema: false,
		Notes:                "零一万物",
	},
	"baichuan": {
		BaseURL:              "https://api.baichuan-ai.com/v1",
		DefaultModels:        []string{"Baichuan4", "Baichuan3-Turbo"},
		SupportsStrictSchema: false,
	},
	"groq": {
		BaseURL:              "https://api.groq.com/openai/v1",
		DefaultModels:        []string{"llama-3.3-70b-versatile", "mixtral-8x7b-32768"},
		SupportsStrictSchema: true,
	},
	"ollama": {
		BaseURL:              "http://localhost:11434/v1",
		DefaultModels:        []string{"qwen2.5:7b", "llama3.3", "gpt-oss:20b"},
		SupportsStrictSchema: false,
		Notes:                "本地 Ollama（OpenAI-compat 模式；推荐用 OllamaNativeProvider 获得原生 Format 支持）",
	},
}
