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

// CMCTParser 实现了 SiteParser 接口
type CMCTParser struct {
	Config ParserConfig
}

// ParseTitleAndID 解析标题和种子 ID
func (p *CMCTParser) ParseTitleAndID(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
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
func (p *CMCTParser) ParseDiscount(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	discountMapping := map[string]models.DiscountType{
		"free":          models.DISCOUNT_FREE,
		"twoup":         models.DISCOUNT_TWO_X,
		"twoupfree":     models.DISCOUNT_TWO_X_FREE,
		"thirtypercent": models.DISCOUNT_THIRTY,
		"halfdown":      models.DISCOUNT_FIFTY,
		"twouphalfdown": models.DISCOUNT_TWO_X_FIFTY,
		"pro_custom":    models.DISCOUNT_CUSTOM,
	}
	// 优惠类型解析
	fontSelection := e.DOM.Find("h1 font, h1 b font, h1 > font")
	found := false
	fontSelection.EachWithBreak(func(_ int, font *goquery.Selection) bool {
		class, exists := font.Attr("class")
		if exists {
			if discountType, ok := discountMapping[class]; ok {
				info.Discount = discountType
				found = true
				return false
			}
		}
		return true
	})
	if !found {
		info.Discount = models.DISCOUNT_NONE
		return
	}
	// 优惠结束时间解析
	endTimeAttr := e.DOM.Find("h1 span[title], h1 > span[title]").First().AttrOr("title", "")
	if endTimeAttr != "" {
		t, err := time.ParseInLocation(p.Config.TimeLayout, endTimeAttr, time.Local)
		if err == nil {
			info.EndTime = t
		} else {
			sLogger().Error("解析结束时间出错:", err, endTimeAttr)
		}
	} else {
		// 如果优惠类型是 "免费"，设置为长期默认值
		if info.Discount == models.DISCOUNT_FREE {
			info.EndTime = time.Now().AddDate(1, 0, 0) // 设置为一年后
		} else {
			info.EndTime = time.Time{}
		}
	}
}

// ParseHR 解析 HR 状态
func (p *CMCTParser) ParseHR(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	html, _ := e.DOM.Html()
	hrKeywords := []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"}
	for _, keyword := range hrKeywords {
		if strings.Contains(html, keyword) {
			info.HR = true
			break
		}
	}
}

func (p *CMCTParser) ParseTorrentSizeMB(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	// 查找 <span title="大小"> 并提取文本内容
	sizeText := e.DOM.Find("span[title='大小']").Text()
	if sizeText == "" {
		sLogger().Error("未找到大小信息")
		return
	}
	// 使用正则表达式提取大小和单位
	sizeRe := regexp.MustCompile(`([\d.]+)\s*(GB|MB|KB)`)
	matches := sizeRe.FindStringSubmatch(sizeText)
	if len(matches) < 3 {
		sLogger().Error("无法解析大小信息", sizeText)
		return
	}
	// 提取大小和单位
	sizeValue := matches[1]
	unit := matches[2]
	// 转换大小为浮点数
	size, err := strconv.ParseFloat(sizeValue, 64)
	if err != nil {
		sLogger().Error("无法解析大小值", sizeValue, err)
		return
	}
	// 根据单位换算为 MB
	switch strings.ToUpper(unit) {
	case "GB":
		size *= 1024
	case "KB":
		size /= 1024
	case "MB":
		// 原值直接是 MB，不需要处理
	default:
		sLogger().Warn("未知单位", unit)
	}
	// 存储大小到 TorrentInfo
	info.SizeMB = size
	// 检查是否成功解析大小
	if info.SizeMB == 0 {
		color.Red("无法解析种子大小")
		sLogger().Error("种子大小为 0", sizeText)
	}
}

func NewCMCTParser(options ...ParserOption) *CMCTParser {
	// 创建默认配置
	config := defaultParserConfig
	// 应用用户传入的配置
	for _, opt := range options {
		opt(&config)
	}
	// 返回带有配置的解析器
	return &CMCTParser{Config: config}
}
