package internal

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/models"
)

// 计划暂停时刻已到的承诺暂停推送不开始（docs/design/free-expiry-safety.md B.3.2）。

// 承诺到期暂停、计划暂停时刻（结束时间减提前量）已到或一分钟内就到：暂停加入、不开始，交给监控器确认暂停
// （要求免费、余量刚够时也一样）；时刻还远时开始，而且开始在登记到期定时器之前——定时暂停不会抢在开始前执行、又被开始撤掉。
func TestPush_PauseDueKeepsPausedAndResumesBeforeSchedule(t *testing.T) {
	cases := []struct {
		name        string
		untilEnd    time.Duration
		requireFree bool
		wantResume  int
	}{
		{"计划暂停时刻已过", 5 * time.Minute, false, 0},
		{"计划暂停时刻一分钟内就到", 10*time.Minute + 30*time.Second, false, 0},
		{"要求免费、余量刚够", 10*time.Minute + 30*time.Second, true, 0},
		{"计划暂停时刻还远", 3 * time.Hour, false, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := pushRig(t)
			require.NoError(t, core.NewConfigStore(r.db).SaveGlobalSettings(models.SettingsGlobal{
				DownloadDir: r.base, DefaultIntervalMinutes: 10, FreeEndAdvanceMinutes: 10,
			}))
			resumedAtSchedule := -1
			RegisterTorrentScheduler(func(models.TorrentInfo) { resumedAtSchedule = len(r.dls["qb"].resumeCalls()) })
			t.Cleanup(func() { RegisterTorrentScheduler(nil) })
			now := time.Now()
			res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
				SiteID: "springsunday", TorrentID: "pd1", TorrentData: roundTorrentBytes("pd1"), DownloaderID: r.dlID("qb"),
				Policy: AdmissionPolicy{
					RequireFree: tc.requireFree, PauseOnFreeEnd: true, SafetyMargin: SafetyMarginFor(10),
					SourceKind: SourceKindRSSNotify, AttemptStart: now,
				},
				Snapshot: untilSnap(now, now.Add(tc.untilEnd)),
			})
			require.NoError(t, err)
			require.True(t, res.Success, res.Message)
			adds := r.dls["qb"].addCalls()
			require.Len(t, adds, 1)
			assert.True(t, adds[0].AddAtPaused, "承诺到期暂停的先暂停加入")
			assert.Len(t, r.dls["qb"].resumeCalls(), tc.wantResume)
			assert.Equal(t, tc.wantResume, resumedAtSchedule, "登记到期定时器时开始已经做完（或确定不开始）")
			if tc.wantResume == 0 {
				assert.Equal(t, PushStatusPushedPaused, res.Status)
			}
			ti := r.record("pd1")
			require.NotNil(t, ti.PausePlannedAt, "照样登记到期监控")
			assert.True(t, ti.PausePlannedAt.Equal(DBTime(now.Add(tc.untilEnd-10*time.Minute))))
		})
	}
}
