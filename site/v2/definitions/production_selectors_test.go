package definitions

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 真正运行时站点由 SiteFactory 建（定义里的搜索选择器并进默认值），fixture 测试却直接拿定义里的选择器建驱动。
// 这里确认两边一致：定义写了优惠映射、SubtitleSelector 的，运行时的驱动也用得上（以前合并时被丢掉，
// 文字显示的 50%、2X 这类优惠读成没有优惠，副标题用的是默认取法）。
func TestNexusPHPDefinitions_ProductionDriverKeepsSearchSelectors(t *testing.T) {
	factory := v2.NewSiteFactory(nil)
	checked := 0
	for _, def := range v2.GetDefinitionRegistry().GetAll() {
		if def.Schema != v2.SchemaNexusPHP || def.CreateDriver != nil || def.Selectors == nil {
			continue
		}
		sel := def.Selectors
		if sel.DiscountMapping == nil && sel.SubtitleSelector == nil {
			continue
		}
		site, err := factory.CreateSite(v2.SiteConfig{
			ID: def.ID, BaseURL: def.URLs[0], Options: json.RawMessage(`{"cookie":"c_secure_uid=1"}`),
		})
		require.NoError(t, err, def.ID)
		base, ok := site.(*v2.BaseSite[v2.NexusPHPRequest, v2.NexusPHPResponse])
		require.True(t, ok, def.ID)
		driver, ok := base.GetDriver().(*v2.NexusPHPDriver)
		require.True(t, ok, def.ID)

		if sel.DiscountMapping != nil {
			assert.Equal(t, sel.DiscountMapping, driver.Selectors.DiscountMapping, "%s: 优惠映射", def.ID)
		}
		if sel.SubtitleSelector != nil {
			assert.Equal(t, sel.SubtitleSelector, driver.Selectors.SubtitleSelector, "%s: SubtitleSelector", def.ID)
			assert.Equal(t, sel.Subtitle, driver.Selectors.Subtitle, "%s: 只写了 SubtitleSelector 时不沿用默认的 Subtitle", def.ID)
		}
		checked++
	}
	assert.Positive(t, checked, "至少有一个站点定义写了优惠映射或 SubtitleSelector")
}
