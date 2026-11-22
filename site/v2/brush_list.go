package v2

import (
	"fmt"
	"strings"
	"time"
)

// BrushListConfig 是站点核实过的刷流读列表能力，写在站点定义的 BrushList 里（刷流见 scheduler 的 listBrushItems）。
// 没写的站点照旧：只勾「免费」时读免费列表，否则读全部种子列表的第 1 页、在本地按优惠类型挑，不补页。
type BrushListConfig struct {
	// DiscountFilters 是按优惠类型筛选种子列表的参数值，NexusPHP 是 torrents.php 的 spstate（HDSky：FREE 是 2、2XFREE 是 4）。
	// 任务允许的每一种优惠都在这里时，刷流按优惠类型各读一张列表；有一种不在时读全部种子列表、在本地挑。
	// 只有 NexusPHP 驱动认它（SearchQuery.BrushDiscount）。
	DiscountFilters map[DiscountLevel]string `json:"discountFilters,omitempty"`
	// Paging 表示核实过这个站点的种子列表能翻页：第 2 页（SearchQuery.Page 为 2，NexusPHP 请求 page=1）接着第 1 页往下列，
	// 不是又一份第 1 页。写了的站点在候选不够时补读第 2 页。
	Paging bool `json:"paging,omitempty"`
	// MinInterval 是同一轮里两次读种子列表至少隔多久（HDSky 限制 60 秒一次，写 61 秒）；0 是不限。
	MinInterval time.Duration `json:"-"`
}

// DiscountFilter 是这一种优惠的列表筛选值；没写这一类（或者 c 为 nil）时 ok 为假。
func (c *BrushListConfig) DiscountFilter(lv DiscountLevel) (string, bool) {
	if c == nil || lv == "" {
		return "", false
	}
	v, ok := c.DiscountFilters[lv]
	return v, ok && v != ""
}

// BrushListSource 是能给出刷流读列表能力的站点实例（BaseSite：驱动带着站点定义时）。
type BrushListSource interface {
	BrushList() *BrushListConfig
}

// BrushListOf 是站点实例的刷流读列表能力（驱动的站点定义里的 BrushList）；站点不带站点定义、或者定义里没写时为 nil。
// 刷流按它决定读哪几张列表、补不补页，与驱动换算筛选参数用的是同一份定义。
func BrushListOf(site Site) *BrushListConfig {
	if s, ok := site.(BrushListSource); ok {
		return s.BrushList()
	}
	return nil
}

// brushListLevels 是 DiscountFilters 能写的优惠类型（刷流任务能勾的那些）。
var brushListLevels = map[DiscountLevel]bool{
	DiscountFree: true, Discount2xFree: true, DiscountPercent30: true, DiscountPercent50: true,
	DiscountPercent70: true, Discount2xUp: true, Discount2x50: true,
}

// validateBrushList 检查 BrushList：优惠筛选只给 NexusPHP 站点写、优惠类型认得、值不为空，间隔不为负。
func (d *SiteDefinition) validateBrushList(addErr func(field, rule, detail string)) {
	b := d.BrushList
	if len(b.DiscountFilters) > 0 && d.Schema != SchemaNexusPHP {
		addErr("BrushList.DiscountFilters", "SchemaUnsupported", "only the NexusPHP driver maps discount filters to list parameters")
	}
	for lv, v := range b.DiscountFilters {
		if !brushListLevels[lv] {
			addErr("BrushList.DiscountFilters", "InvalidValue", fmt.Sprintf("%q is not a discount level a brush task can allow", lv))
			continue
		}
		if strings.TrimSpace(v) == "" {
			addErr(fmt.Sprintf("BrushList.DiscountFilters[%q]", lv), "Required", "must not be empty")
		}
	}
	if b.MinInterval < 0 {
		addErr("BrushList.MinInterval", "InvalidValue", "must be ≥ 0")
	}
}
