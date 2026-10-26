package v2

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 自定义优惠映射按子串匹配：长的关键词先比，结果不随 map 的遍历顺序变（twoupfree 同时包含 free 与 twoup）。
func TestParseDiscountFromElementPrefersLongestKeyword(t *testing.T) {
	mapping := map[string]DiscountLevel{
		"free":          DiscountFree,
		"twoup":         Discount2xUp,
		"twoupfree":     Discount2xFree,
		"halfdown":      DiscountPercent50,
		"twouphalfdown": Discount2x50,
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<font id="a" class="promotion twoupfree">2X免费</font><font id="b" class="promotion twouphalfdown">2X 50%</font><font id="c" class="promotion free">免费</font>`,
	))
	require.NoError(t, err)
	for range 200 {
		assert.Equal(t, Discount2xFree, parseDiscountFromElement(doc.Find("#a"), mapping))
		assert.Equal(t, Discount2x50, parseDiscountFromElement(doc.Find("#b"), mapping))
		assert.Equal(t, DiscountFree, parseDiscountFromElement(doc.Find("#c"), mapping))
	}
}
