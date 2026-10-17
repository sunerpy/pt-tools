package definitions

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 分享率不到 1 时，上游 NexusPHP 把数值包在带颜色的 <font> 里（include/functions.php 的 get_ratio、
// public/userdetails.php 的「传输」格，颜色来自 get_ratio_color）。分享率正则不能停在颜色值上：
// 以前 25 个站点的 [^\d∞]* 会把 0.750 取成 880000 / 990000 这类数。
// 每个 NexusPHP 站的 ratio 选择器要么取到 0.75，要么取不到（取不到时按上传量 / 下载量计算）。
func TestAllSites_RatioIgnoresFontColor(t *testing.T) {
	colors := []string{"#ff0000", "#ee0000", "#dd0000", "#cc0000", "#bb0000", "#aa0000", "#990000", "#880000", "#770000", "#660000"}
	for _, color := range colors {
		ratio := `<font color="` + color + `">0.750</font>`
		page := `<html><body><table id="info_block"><tr><td><span class="medium">` +
			`<font class="color_ratio">分享率:</font> ` + ratio + ` <font class='color_uploaded'>上传量:</font> 750.00 GB ` +
			`<font class='color_downloaded'>下载量:</font> 1.00 TB</span></td></tr></table>` +
			`<table><tr><td class="rowhead nowrap">传输</td><td class="rowfollow"><table>` +
			`<tr><td class="embedded"><strong>分享率</strong>:  ` + ratio + `（<strong>实际分享率</strong>：0.750）</td></tr>` +
			`<tr><td class="embedded"><strong>上传量</strong>:  750.00 GB</td><td class="embedded"><strong>下载量</strong>:  1.00 TB</td></tr>` +
			`</table></td></tr><tr><td class="rowhead">分享率</td><td class="rowfollow">` + ratio + `</td></tr></table></body></html>`
		doc := FixtureDoc(t, "ratio_font_color", page)
		for _, def := range v2.GetDefinitionRegistry().GetAll() {
			if def.Schema != v2.SchemaNexusPHP || def.UserInfo == nil {
				continue
			}
			sel, ok := def.UserInfo.Selectors["ratio"]
			if !ok {
				continue
			}
			got := newTestNexusPHPDriver(def).ExtractFieldValuePublic(doc, sel)
			if got == "" || got == "0" {
				continue
			}
			v, err := strconv.ParseFloat(got, 64)
			if assert.NoError(t, err, "%s color=%s ratio=%q", def.ID, color, got) {
				assert.InDelta(t, 0.75, v, 1e-9, "%s color=%s ratio=%q", def.ID, color, got)
			}
		}
	}
}
