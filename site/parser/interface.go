package parser

import (
	"time"

	"github.com/gocolly/colly"
)

// ParsedTorrentInfo 解析后的种子信息
type ParsedTorrentInfo struct {
	ID           string
	Title        string
	Size         int64 // 字节
	IsFree       bool
	FreeEndTime  time.Time
	HasHR        bool
	DownloadURL  string
	DiscountType string
}

// SiteParser 站点解析器接口
// 定义从HTML页面解析种子信息的方法
type SiteParser interface {
	// ParseTorrentPage 解析种子详情页面
	// e: colly HTML元素
	// 返回解析后的种子信息
	ParseTorrentPage(e *colly.HTMLElement) (*ParsedTorrentInfo, error)

	// ParseRSSItem 解析RSS条目中的额外信息
	// 某些站点的RSS包含额外的种子信息
	ParseRSSItem(title, link, description string) (*ParsedTorrentInfo, error)

	// GetName 获取解析器名称
	GetName() string
}

// ParserConfig 可配置解析器的配置
type ParserConfig struct {
	// Name 解析器名称
	Name string `json:"name"`

	// TimeLayout 时间格式
	TimeLayout string `json:"time_layout"`

	// Selectors CSS选择器配置
	Selectors SelectorConfig `json:"selectors"`

	// Patterns 正则表达式配置
	Patterns PatternConfig `json:"patterns"`

	// DiscountMapping 优惠类型映射
	DiscountMapping map[string]string `json:"discount_mapping"`
}

// SelectorConfig CSS选择器配置
type SelectorConfig struct {
	// TitleSelector 标题选择器
	TitleSelector string `json:"title_selector"`

	// IDSelector 种子ID选择器
	IDSelector string `json:"id_selector"`

	// SizeSelector 大小选择器
	SizeSelector string `json:"size_selector"`

	// DiscountSelector 优惠类型选择器
	DiscountSelector string `json:"discount_selector"`

	// FreeEndTimeSelector 免费结束时间选择器
	FreeEndTimeSelector string `json:"free_end_time_selector"`

	// HRSelector HR标记选择器
	HRSelector string `json:"hr_selector"`

	// DownloadURLSelector 下载链接选择器
	DownloadURLSelector string `json:"download_url_selector"`
}

// PatternConfig 正则表达式配置
type PatternConfig struct {
	// SizePattern 大小提取正则
	// 应包含两个捕获组：数值和单位
	// 例如: `([\d.]+)\s*(GB|MB|KB|TB)`
	SizePattern string `json:"size_pattern"`

	// IDPattern 种子ID提取正则
	IDPattern string `json:"id_pattern"`

	// HRKeywords HR关键词列表
	HRKeywords []string `json:"hr_keywords"`
}

// DefaultParserConfig 返回默认解析器配置
func DefaultParserConfig() *ParserConfig {
	return &ParserConfig{
		Name:       "default",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors: SelectorConfig{
			TitleSelector:       "h1",
			IDSelector:          "input[name='id']",
			SizeSelector:        "td.rowhead:contains('大小')",
			DiscountSelector:    "h1 font",
			FreeEndTimeSelector: "h1 span[title]",
			HRSelector:          "img[alt*='HR']",
			DownloadURLSelector: "a[href*='download']",
		},
		Patterns: PatternConfig{
			SizePattern: `([\d.]+)\s*(GB|MB|KB|TB)`,
			IDPattern:   `id=(\d+)`,
			HRKeywords:  []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"},
		},
		DiscountMapping: map[string]string{
			"free":          "free",
			"twoup":         "2x",
			"twoupfree":     "2x_free",
			"thirtypercent": "30%",
			"halfdown":      "50%",
			"twouphalfdown": "2x_50%",
		},
	}
}
