package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 只收免费优惠的刷流任务：推送要求免费，带着这一轮列表读到的资格；允许非免费优惠的任务不要求免费。
func TestBrush_PushPolicyFollowsAllowedDiscounts(t *testing.T) {
	r := newBrushRig(t)
	r.addItem(t, "1", nil)
	task := r.task(t, nil) // 默认只收免费

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, 1, res.Added)
	require.Len(t, r.pushes, 1)
	p := r.pushes[0]
	assert.True(t, p.Policy.RequireFree, "只收免费的任务要求免费")
	assert.False(t, p.Policy.PauseOnFreeEnd, "刷流自己处理免费到期")
	assert.Equal(t, ptinternal.SourceKindBrush, p.Policy.SourceKind)
	require.NotNil(t, p.Snapshot)
	assert.Equal(t, v2.FreeUntil, p.Snapshot.Validity)
	assert.False(t, p.Snapshot.VerifiedAt.Before(p.Policy.AttemptStart), "资格是这一轮列表读到的")
	assert.Equal(t, r.clock.Now(), p.Policy.Now(), "门禁用刷流的时钟")

	r2 := newBrushRig(t)
	r2.addItem(t, "2", func(it *v2.TorrentItem) { it.DiscountLevel = v2.DiscountPercent50 })
	task2 := r2.task(t, func(bt *models.BrushTask) { bt.Discounts = "FREE,PERCENT_50" })
	res, err = r2.mon.RunTask(context.Background(), task2.ID)
	require.NoError(t, err)
	require.Equal(t, 1, res.Added)
	require.Len(t, r2.pushes, 1)
	assert.False(t, r2.pushes[0].Policy.RequireFree, "允许半价的任务不要求免费")
}

// 只收免费时，列表里免费但没有结束时间的不加（结束时间未知不能当长期免费）；资格被门禁拒绝的跳过、接着看下一个。
func TestBrush_FreeOnlySkipsUnknownEndAndContinuesAfterFreeRejection(t *testing.T) {
	r := newBrushRig(t)
	r.addItem(t, "u", func(it *v2.TorrentItem) { it.DiscountEndTime = time.Time{} })
	task := r.task(t, nil)
	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Zero(t, res.Added)
	assert.Empty(t, r.pushes, "结束时间未知的不推送")

	r2 := newBrushRig(t)
	r2.addItem(t, "a", nil)
	r2.addItem(t, "b", func(it *v2.TorrentItem) { it.Seeders = 1 })
	task2 := r2.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 5 })
	first := true
	r2.mon.cfg.Push = func(_ context.Context, req ptinternal.PushTorrentRequest) (*ptinternal.PushTorrentResult, error) {
		r2.pushes = append(r2.pushes, req)
		if first {
			first = false
			return &ptinternal.PushTorrentResult{Success: false, Status: ptinternal.PushStatusRejectedExpired, Message: "免费剩余时间不超过 5 分钟，不推送"}, nil
		}
		return &ptinternal.PushTorrentResult{Success: true, TorrentHash: "h-" + req.TorrentID}, nil
	}
	res, err = r2.mon.RunTask(context.Background(), task2.ID)
	require.NoError(t, err)
	assert.Len(t, r2.pushes, 2, "资格被拒的跳过，不停下这一轮")
	assert.Equal(t, 1, res.Added)
	assert.Empty(t, res.Stopped)
}
