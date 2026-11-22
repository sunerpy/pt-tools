package definitions

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 刷流读列表的能力只写核实过的站点（2026-11-22 在用户浏览器里核实）：
//   - HDSky：免费列表 spstate=2、2X 免费列表 spstate=4，免费列表第 2 页（page=1）接着第 1 页往下列；站点限制 60 秒读一次，隔 61 秒；
//   - PTTime：全部种子列表第 2 页（page=1）接着第 1 页往下列，只有置顶的重复；没有核实优惠筛选。
//
// 别的站点不写，刷流照旧只读第 1 页。要给新站点写，先拿两页样本核实，再改这里。
func TestBrushListOnlyForVerifiedSites(t *testing.T) {
	want := map[string]*v2.BrushListConfig{
		"hdsky": {
			DiscountFilters: map[v2.DiscountLevel]string{v2.DiscountFree: "2", v2.Discount2xFree: "4"},
			Paging:          true,
			MinInterval:     61 * time.Second,
		},
		"pttime": {Paging: true},
	}
	for _, def := range v2.GetDefinitionRegistry().GetAll() {
		w, ok := want[def.ID]
		if !ok {
			assert.Nil(t, def.BrushList, "%s 没有核实过刷流读列表的能力", def.ID)
			continue
		}
		require.NotNil(t, def.BrushList, def.ID)
		assert.Equal(t, w, def.BrushList, def.ID)
	}
	for id := range want {
		_, ok := v2.GetDefinitionRegistry().Get(id)
		assert.True(t, ok, id)
	}
}
