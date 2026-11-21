package v2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// PromotionAJAXConfig 是「优惠状态由页面脚本另取」的接口。HDWinG（HDChina 的延续）把优惠图标从种子列表、详情页的 HTML
// 里拿掉了，原处只留 <span class="sp_state_placeholder" id="种子ID"></span>；页面脚本收集页面上所有占位的种子 ID，
// 带上页面里的 csrf 令牌 POST 这个地址（表单 ids[]=…&csrf=…），返回
// {"status":200,"message":{"种子ID":{"sp_state":"优惠图标","timeout":"<span title=\"结束时间\">剩余时间</span>"}}}，
// 再把占位换成图标（种子列表在后面接上 timeout）。直接取页面时没有执行脚本，不补的话每个种子看着都没有优惠。
type PromotionAJAXConfig struct {
	// Path 是取优惠状态的地址，例如 "/ajax_promotion.php"
	Path string `json:"path"`
}

// promotionPlaceholder 是页面里等着页面脚本填优惠状态的占位，id 是种子 ID。
const promotionPlaceholder = "span.sp_state_placeholder[id]"

// promotionState 是一个种子的优惠状态：sp_state 是优惠图标（没有优惠时为空），timeout 是带结束时间 title 的 span。
type promotionState struct {
	SpState string `json:"sp_state"`
	Timeout string `json:"timeout"`
}

func (d *NexusPHPDriver) promotionAJAX() *PromotionAJAXConfig {
	if d.siteDefinition == nil {
		return nil
	}
	return d.siteDefinition.PromotionAJAX
}

// fillPromotions 照页面脚本补上优惠状态：一页的占位一次取完，每个占位换成优惠图标，后面接上结束时间（详情页的脚本
// 不接，这里也接上，详情解析按它读结束时间）。站点没回的种子占位留着（网页上也是这样，看着没有优惠）；页面上没有占位时
// 不发请求。请求失败、返回不对时报错：不能把整页的种子都当成没有优惠（刷流、RSS 会当成不是免费）。
func (d *NexusPHPDriver) fillPromotions(ctx context.Context, doc *goquery.Document, baseURL, pageURL string) error {
	cfg := d.promotionAJAX()
	if cfg == nil || doc == nil {
		return nil
	}
	placeholders := doc.Find(promotionPlaceholder)
	var ids []string
	seen := map[string]bool{}
	placeholders.Each(func(_ int, s *goquery.Selection) {
		id := strings.TrimSpace(s.AttrOr("id", ""))
		if numericID.MatchString(id) && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	})
	if len(ids) == 0 {
		return nil
	}
	token := pageCSRFToken(doc)
	if token == "" {
		return fmt.Errorf("读取优惠状态（%s）: 页面里没有 csrf 令牌", cfg.Path)
	}
	states, err := d.fetchPromotionStates(ctx, baseURL+cfg.Path, pageURL, ids, token)
	if err != nil {
		return fmt.Errorf("读取优惠状态（%s）: %w", cfg.Path, err)
	}
	placeholders.Each(func(_ int, s *goquery.Selection) {
		if st, ok := states[strings.TrimSpace(s.AttrOr("id", ""))]; ok {
			s.ReplaceWithHtml(st.SpState + st.Timeout)
		}
	})
	return nil
}

// fetchPromotionStates 像页面脚本（jQuery.post）一样取 ids 的优惠状态，按种子 ID 返回。令牌只在表单里，不写进错误。
func (d *NexusPHPDriver) fetchPromotionStates(ctx context.Context, endpoint, pageURL string, ids []string, token string) (map[string]promotionState, error) {
	form := url.Values{"ids[]": ids, "csrf": {token}}
	headers := map[string]string{
		"User-Agent":       d.userAgent,
		"Accept":           "application/json, text/javascript, */*; q=0.01",
		"Accept-Language":  "zh-CN,zh;q=0.9,en;q=0.8",
		"Content-Type":     "application/x-www-form-urlencoded; charset=UTF-8",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          pageURL,
	}
	resp, err := d.sendWithCookie(headers, func(h map[string]string) (*HTTPResponse, error) {
		return d.httpClient.Post(ctx, endpoint, []byte(form.Encode()), h)
	})
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflareChallenge() {
		return nil, fmt.Errorf("HTTP %d: %w", resp.StatusCode, ErrCloudflareChallenge)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("HTTP 429: %w", ErrRateLimited)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPStatusError{Code: resp.StatusCode}
	}

	var body struct {
		Status  int             `json:"status"`
		Message json.RawMessage `json:"message"`
	}
	if err := json.Unmarshal(bytes.TrimPrefix(resp.Body, []byte("\xef\xbb\xbf")), &body); err != nil {
		return nil, errors.New("返回的不是 JSON")
	}
	if body.Status != http.StatusOK {
		return nil, fmt.Errorf("站点回的状态是 %d", body.Status)
	}
	states := map[string]promotionState{}
	// PHP 的空数组编成 []：没有可填的
	switch msg := bytes.TrimSpace(body.Message); {
	case len(msg) == 0, bytes.Equal(msg, []byte("null")), bytes.Equal(msg, []byte("[]")):
		return states, nil
	default:
		if err := json.Unmarshal(msg, &states); err != nil {
			return nil, errors.New("返回的 message 不是按种子 ID 列的优惠状态")
		}
	}
	return states, nil
}

// pageCSRFToken 是页面里的 csrf 令牌：页面头部 <meta name="x-csrf"> 的值；没有时用表单里的隐藏域 csrf
// （HDWinG 搜索框表单里的隐藏域与 meta 是同一个值）。
func pageCSRFToken(doc *goquery.Document) string {
	if v := strings.TrimSpace(doc.Find("meta[name='x-csrf']").First().AttrOr("content", "")); v != "" {
		return v
	}
	return strings.TrimSpace(doc.Find("input[name='csrf']").First().AttrOr("value", ""))
}

// promotionPending 报告种子 torrentID 的优惠占位还在页面上：站点没回它的优惠状态。
func promotionPending(doc *goquery.Document, torrentID string) bool {
	pending := false
	doc.Find(promotionPlaceholder).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		pending = strings.TrimSpace(s.AttrOr("id", "")) == torrentID
		return !pending
	})
	return pending
}
