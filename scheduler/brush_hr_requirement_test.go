package scheduler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// #727：站点定义里没有 H&R 做种时长时，不排除 H&R 的刷流任务也收不到有 H&R 的种子；这一轮的结果要写明原因。
func TestBrushMonitor_RejectReasonsInSummary(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.ExcludeHR = false })
	r.addItem(t, "h1", func(i *v2.TorrentItem) { i.HasHR = true })
	r.addItem(t, "h2", func(i *v2.TorrentItem) { i.HasHR = true })

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Eligible)
	assert.Contains(t, res.Summary(), "H&R 要求的做种时长未知 2 个", "写明为什么一个都不符合")
}

// #727：站点设置里填了「H&R 做种要求」时，按它收有 H&R 的种子，并记下做种要求（删种按它保护）。
func TestBrushMonitor_HRSeedTimeFromSiteSetting(t *testing.T) {
	r := newBrushRig(t)
	require.NoError(t, r.db.DB.Table("site_settings").Create(map[string]any{
		"name": "hdsky", "auth_method": "cookie", "enabled": true, "hr_seed_time_h": 72,
	}).Error)
	task := r.task(t, func(bt *models.BrushTask) { bt.ExcludeHR = false })
	r.addItem(t, "h1", func(i *v2.TorrentItem) { i.HasHR = true })

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Eligible)
	assert.Equal(t, 1, res.Added)
	var bt models.BrushTorrent
	require.NoError(t, r.db.DB.Where("task_id = ?", task.ID).First(&bt).Error)
	assert.True(t, bt.HasHR)
	assert.Equal(t, 72, bt.HRSeedTimeH, "按站点设置记下做种要求")
}
