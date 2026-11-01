package internal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/models"
)

// 同标签目录有两条 RSS、暂存记录归其中一条（SourceRSSID）时整站保存（网页改了另一条 RSS 的间隔）：
// 保存按编号原地更新 RSS，暂存记录仍归原来那条，另一条照旧不碰它，原来那条下一轮照常推送。
// 以前保存时 RSS 删掉重建、编号全变，记录的所有者不在了，同目录又有两条 RSS，这个记录没有 RSS 能接管。
func TestStagedOwnerSurvivesSiteSave(t *testing.T) {
	r := newRSSRig(t, "qb-a", "qb-b")
	idA, idB := r.dlID("qb-a"), r.dlID("qb-b")
	feed := newFeedServer(t, "z1")
	rs := r.addRSS(feed,
		models.RSSConfig{Name: "A", Tag: "movie", IntervalMinutes: 10, DownloaderID: &idA},
		models.RSSConfig{Name: "B", Tag: "movie", IntervalMinutes: 10, DownloaderID: &idB},
	)
	a := rs[0]
	r.site.set("z1", freeUntil(48*time.Hour))
	ctxA := r.fetch(a)
	require.True(t, r.staged(a, "z1"))
	z := r.record("z1")
	require.NotNil(t, z.SourceRSSID)
	require.Equal(t, a.ID, *z.SourceRSSID, "z1 归 A")

	store := core.NewConfigStore(r.db)
	sc, err := store.GetSiteConf(models.SiteGroup("springsunday"))
	require.NoError(t, err)
	for i := range sc.RSS {
		if sc.RSS[i].Name == "B" {
			sc.RSS[i].IntervalMinutes = 20
		}
	}
	require.NoError(t, store.UpsertSiteWithRSS(models.SiteGroup("springsunday"), sc))

	// 调度器重载时按库里的配置重建任务
	after, err := store.GetSiteConf(models.SiteGroup("springsunday"))
	require.NoError(t, err)
	byName := map[string]models.RSSConfig{}
	for _, rc := range after.RSS {
		byName[rc.Name] = rc
	}
	assert.Equal(t, a.ID, byName["A"].ID, "A 的编号不变")
	rec := r.record("z1")
	ownsA, _ := (&stagedPush{rss: byName["A"], siteName: "springsunday"}).owns(&rec)
	assert.True(t, ownsA, "暂存记录仍归 A")
	ownsB, reason := (&stagedPush{rss: byName["B"], siteName: "springsunday"}).owns(&rec)
	assert.False(t, ownsB, "B 不碰 A 的暂存文件")
	assert.Empty(t, reason)

	r.push(ctxA, byName["A"])
	require.Len(t, r.dls["qb-a"].addCalls(), 1, "A 照常推送自己的暂存文件")
	assert.Empty(t, r.dls["qb-b"].addCalls())
	assert.True(t, pushedOf(r.record("z1")))
	assert.False(t, r.staged(a, "z1"), "推送以后暂存文件清掉")
}
