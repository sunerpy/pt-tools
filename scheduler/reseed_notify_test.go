package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/reseed"
	"github.com/sunerpy/pt-tools/models"
)

// 每轮结果推送：按设置（关闭、每一轮、只在有变化时）在轮次结束以后写进投递表，同一轮只写一次；
// 退出时被打断的轮次不推送；正文只有计数与状态。
func TestReseedWorker_NotifyRound(t *testing.T) {
	w, svc, db := newReseedWorkerForTest(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&models.NotificationConf{}, &models.MonitorNotificationLog{}))
	require.NoError(t, db.Create(&models.NotificationConf{ID: 1, ChannelType: "capture", Name: "tg", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.NotificationConf{ID: 2, ChannelType: "capture", Name: "qq", Enabled: true}).Error)
	w.cfg.Notifier = NewMonitorNotifier(db, &captureSender{}, nil, zap.NewNop().Sugar())

	setMode := func(mode string, channels ...uint) {
		t.Helper()
		_, err := svc.SaveSettings(ctx, reseed.SettingsUpdate{NotifyMode: &mode, NotifyChannelIDs: &channels})
		require.NoError(t, err)
	}
	round := func(res reseed.RunResult, runErr error) uint {
		t.Helper()
		id, err := w.start(models.ReseedTriggerManual, func(context.Context) (string, *uint, error) { return "", nil, nil },
			func(context.Context, reseed.RunOptions) (reseed.RunResult, error) { return res, runErr })
		require.NoError(t, err)
		require.Eventually(t, func() bool { return !w.Running() }, 5*time.Second, 10*time.Millisecond)
		return id
	}
	rows := func() []models.MonitorNotificationLog {
		t.Helper()
		var out []models.MonitorNotificationLog
		require.NoError(t, db.Where("source = ? AND kind = ?", "reseed", "round").Order("id").Find(&out).Error)
		return out
	}

	round(reseed.RunResult{Hashes: 3, Created: 1}, nil)
	assert.Empty(t, rows(), "默认关闭")

	setMode(models.ReseedNotifyAlways, 2)
	id := round(reseed.RunResult{Hashes: 3, Candidates: 2, Created: 1, IYUUCalls: 2, Skipped: map[string]int{reseed.SkipTried: 1}}, nil)
	got := rows()
	require.Len(t, got, 1)
	assert.Equal(t, uint(2), got[0].NotificationConfID, "只发选中的通道")
	assert.Equal(t, "*", got[0].Subject)
	payload := decodeReseedPayload(t, got[0])
	assert.Equal(t, fmt.Sprintf("辅种：第 %d 轮完成（立即运行）", id), payload.Title)
	assert.Contains(t, payload.Text, "查询 3 个种子，找到 2 个可辅种，加入 1 个，跳过 1 个。")
	assert.Contains(t, payload.Text, "请求 IYUU 2 次")
	require.NotNil(t, payload.Event)
	assert.Equal(t, notify.EventReseedRound, payload.Event.Type)
	assert.EqualValues(t, 1, payload.Event.Fields["created"])
	assert.Equal(t, "ok", payload.Event.Fields["status"])

	// 只在有变化时：跑完、没有加入也没有失败的不发；提前停下、没能开始的发
	setMode(models.ReseedNotifyChanges, 1)
	round(reseed.RunResult{Hashes: 5, Candidates: 1, Gone: 1}, nil)
	assert.Len(t, rows(), 1, "没有变化")
	round(reseed.RunResult{Hashes: 5, End: reseed.EndLimited, Stopped: "IYUU 限流：请求太频繁"}, nil)
	failedID := round(reseed.RunResult{}, errors.New("读取站点列表失败"))
	got = rows()
	require.Len(t, got, 3)
	payload = decodeReseedPayload(t, got[1])
	assert.Contains(t, payload.Title, "提前停下")
	assert.Contains(t, payload.Text, "里点「继续」")
	payload = decodeReseedPayload(t, got[2])
	assert.Equal(t, fmt.Sprintf("辅种：第 %d 轮没能开始（立即运行）", failedID), payload.Title)
	assert.Equal(t, "读取站点列表失败", payload.Event.Fields["stopped"])

	// 退出时被打断的不发
	round(reseed.RunResult{Hashes: 5, Failed: 1, End: reseed.EndShutdown, Stopped: "pt-tools 正在退出"}, nil)
	assert.Len(t, rows(), 3)

	// 选中的通道停用了：不改发给别的通道
	require.NoError(t, db.Model(&models.NotificationConf{}).Where("id = 1").Update("enabled", false).Error)
	round(reseed.RunResult{Created: 1}, nil)
	assert.Len(t, rows(), 3)

	// 同一轮重复写不会多一条
	n := len(rows())
	a := &reseedActive{id: id, trigger: models.ReseedTriggerManual, started: reseedRunRow(t, db, id).StartedAt}
	setMode(models.ReseedNotifyAlways, 2)
	w.notifyRound(ctx, a, reseed.RunResult{Created: 1}, models.ReseedRunOK, "", "加入 1 个", 0)
	assert.Len(t, rows(), n)

	// 没有投递器时什么都不做
	w.cfg.Notifier = nil
	round(reseed.RunResult{Created: 1}, nil)
	assert.Len(t, rows(), n)
}

type reseedPayload struct {
	Title string        `json:"title"`
	Text  string        `json:"text"`
	Event *notify.Event `json:"event"`
}

func decodeReseedPayload(t *testing.T, row models.MonitorNotificationLog) reseedPayload {
	t.Helper()
	var p reseedPayload
	require.NoError(t, json.Unmarshal([]byte(row.PayloadJSON), &p))
	require.NotNil(t, p.Event)
	return p
}
