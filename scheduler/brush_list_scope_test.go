package scheduler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestBrushListFreeOnly(t *testing.T) {
	cases := []struct {
		discounts string
		want      bool
	}{
		{"", false},                // 默认：免费 + 2X 免费
		{"FREE", true},             // 只勾免费：照旧读免费列表
		{" free ", true},           // 大小写与空白
		{"FREE,2XFREE", false},     // 和默认一样
		{"FREE,PERCENT_50", false}, // 勾了 50%
		{"2XFREE", false},          // 只勾 2X 免费
		{"FREE,FREE", true},        // 重复的只算一次
		{"PERCENT_50,2X50", false}, // 不含免费
		{"FREE, ,", true},          // 空项忽略
	}
	for _, c := range cases {
		t.Run(c.discounts, func(t *testing.T) {
			assert.Equal(t, c.want, brushListFreeOnly(models.BrushTask{Discounts: c.discounts}))
		})
	}
}

// 优惠类型只勾「免费」时读站点的免费列表；勾了别的类型（默认就勾着 2X 免费）时读不带筛选的第一页，交给入场判断在本地挑。
func TestBrushMonitor_ListFilterFollowsDiscounts(t *testing.T) {
	cases := []struct {
		name, discounts string
		wantFreeOnly    bool
		wantStep        string
	}{
		{"默认", "", false, "读取 hdsky 的全部种子列表第 1 页（本地按优惠类型挑）：2 个"},
		{"只勾免费", "FREE", true, "读取 hdsky 的免费种子列表第 1 页：2 个"},
		{"免费与 PERCENT_50", "FREE,PERCENT_50", false, "读取 hdsky 的全部种子列表第 1 页（本地按优惠类型挑）：2 个"},
		{"只勾 2X 免费", "2XFREE", false, "读取 hdsky 的全部种子列表第 1 页（本地按优惠类型挑）：2 个"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newBrushRig(t)
			task := r.task(t, func(bt *models.BrushTask) { bt.Discounts = c.discounts; bt.MaxDownloading = 5 })
			r.addItem(t, "1", nil)
			r.addItem(t, "2", func(i *v2.TorrentItem) { i.DiscountLevel = v2.Discount2xFree })

			_, err := r.mon.RunTask(context.Background(), task.ID)
			require.NoError(t, err)
			require.Len(t, r.site.searches, 1, "每轮只请求一次列表")
			assert.Equal(t, c.wantFreeOnly, r.site.searches[0].FreeOnly)
			assert.Equal(t, []string{c.wantStep}, stepTexts(onlyRun(t, r, task.ID), brushStepList))
		})
	}
}

// 默认的优惠类型（免费 + 2X 免费）下，2X 免费的种子能加进来：列表不再只读免费。
func TestBrushMonitor_DefaultDiscountsAdmit2xFree(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 5 })
	r.addItem(t, "1", func(i *v2.TorrentItem) { i.DiscountLevel = v2.Discount2xFree })
	r.addItem(t, "2", func(i *v2.TorrentItem) { i.DiscountLevel = v2.DiscountPercent50 })

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.False(t, r.site.searches[0].FreeOnly)
	require.Len(t, r.pushes, 1)
	assert.Equal(t, "1", r.pushes[0].TorrentID, "2X 免费的加进来，50% 的在本地按优惠类型拦下")
	assert.Equal(t, 1, res.Added)
}
