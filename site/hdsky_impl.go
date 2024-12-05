package site

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/fatih/color"
	"github.com/gocolly/colly"
	"github.com/sunerpy/pt-tools/models"
)

// ParserConfig 定义解析器的配置
type ParserConfig struct {
	TimeLayout string // 时间格式
}

// HDSkyParser 实现了 SiteParser 接口
type HDSkyParser struct {
	Config ParserConfig
}

// ParseTitleAndID 解析标题和种子 ID
func (p *HDSkyParser) ParseTitleAndID(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	title, exists := e.DOM.Find("input[name='torrent_name']").Attr("value")
	if exists {
		info.Title = title
	}
	torrentID, exists := e.DOM.Find("input[name='detail_torrent_id']").Attr("value")
	if exists {
		info.TorrentID = torrentID
	}
}

// ParseDiscount 解析优惠类型和结束时间
func (p *HDSkyParser) ParseDiscount(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	// 定义优惠类型的映射
	discountMapping := map[string]models.DiscountType{
		"free":          models.DISCOUNT_FREE,
		"twoup":         models.DISCOUNT_TWO_X,
		"twoupfree":     models.DISCOUNT_TWO_X_FREE,
		"thirtypercent": models.DISCOUNT_THIRTY,
		"halfdown":      models.DISCOUNT_FIFTY,
		"twouphalfdown": models.DISCOUNT_TWO_X_FIFTY,
		"pro_custom":    models.DISCOUNT_CUSTOM,
	}
	// 查找优惠类型
	fontSelection := e.DOM.Find("h1 font")
	found := false
	fontSelection.EachWithBreak(func(_ int, font *goquery.Selection) bool {
		class, exists := font.Attr("class")
		if exists {
			if discountType, ok := discountMapping[class]; ok {
				info.Discount = discountType
				found = true
				return false // 找到后退出循环
			}
		}
		return true // 继续循环
	})
	// 如果未匹配到优惠类型，设置为 "none"
	if !found {
		info.Discount = models.DISCOUNT_NONE
	}
	// 解析优惠结束时间
	endTimeAttr := e.DOM.Find("h1 span[title]").First().AttrOr("title", "")
	if endTimeAttr != "" {
		t, err := time.ParseInLocation(p.Config.TimeLayout, endTimeAttr, time.Local)
		if err == nil {
			info.EndTime = t
		} else {
			sLogger().Error("解析结束时间出错:", err)
		}
	}
}

// ParseHR 解析 HR 状态
func (p *HDSkyParser) ParseHR(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	html, _ := e.DOM.Html()
	hrKeywords := []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"}
	for _, keyword := range hrKeywords {
		if strings.Contains(html, keyword) {
			info.HR = true
			break
		}
	}
}

func (p *HDSkyParser) ParseTorrentSizeMB(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	// 查找包含“基本信息”行的表格
	e.DOM.Find("td.rowhead:contains('基本信息')").Each(func(_ int, s *goquery.Selection) {
		// 获取“基本信息”右侧的内容
		rowFollow := s.Next()
		text := rowFollow.Text()
		// 使用正则表达式提取大小和单位
		sizeRe := regexp.MustCompile(`大小：[^\d]*([\d.]+)\s*(GB|MB|KB)`)
		matches := sizeRe.FindStringSubmatch(text)
		if len(matches) < 3 {
			sLogger().Error("无法解析大小信息")
			return
		}
		// 提取大小和单位
		sizeValue := matches[1]
		unit := matches[2]
		// 转换大小为浮点数
		size, err := strconv.ParseFloat(sizeValue, 64)
		if err != nil {
			sLogger().Error("无法解析大小", err)
			return
		}
		// 根据单位换算为 MB
		switch strings.ToUpper(unit) {
		case "GB":
			size *= 1024
		case "KB":
			size /= 1024
		}
		// 存储大小到 TorrentInfo
		info.SizeMB = size
	})
	// 检查是否成功解析大小
	if info.SizeMB == 0 {
		color.Red("无法解析种子大小")
	}
}

type ParserOption func(*ParserConfig)

// 默认配置
var defaultParserConfig = ParserConfig{
	TimeLayout: "2006-01-02 15:04:05",
}

func WithTimeLayout(layout string) ParserOption {
	return func(cfg *ParserConfig) {
		cfg.TimeLayout = layout
	}
}

func NewHDSkyParser(options ...ParserOption) *HDSkyParser {
	// 创建默认配置
	config := defaultParserConfig
	// 应用用户传入的配置
	for _, opt := range options {
		opt(&config)
	}
	// 返回带有配置的解析器
	return &HDSkyParser{Config: config}
}
