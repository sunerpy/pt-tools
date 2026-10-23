package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// mteamRig 是站点名 mteam、按分区返回列表的刷流环境。ids 是各分区的种子编号（同一编号的种子文件只放一份）。
func mteamRig(t *testing.T, ids map[string][]string) *brushRig {
	t.Helper()
	r := newBrushRig(t)
	r.siteName = "mteam"
	r.site.kind = v2.SiteMTorrent
	r.site.sectionItems = map[string][]v2.TorrentItem{}
	for section, list := range ids {
		for _, id := range list {
			if _, ok := r.site.files[id]; !ok {
				r.addItem(t, id, nil)
			}
			for _, it := range r.site.items {
				if it.ID == id {
					r.site.sectionItems[section] = append(r.site.sectionItems[section], it)
				}
			}
		}
	}
	return r
}

func mteamTask(t *testing.T, r *brushRig, section string) *models.BrushTask {
	t.Helper()
	return r.task(t, func(task *models.BrushTask) {
		task.Name, task.SiteName, task.Section, task.MaxDownloading = "mteam 刷流", "mteam", section, 5
	})
}

func searchedSections(r *brushRig) []string {
	out := make([]string, 0, len(r.site.searches))
	for _, q := range r.site.searches {
		out = append(out, q.Section)
	}
	return out
}

func TestBrushMonitor_MTeamSections(t *testing.T) {
	cases := []struct {
		section   string
		wantRead  []string
		wantScope string
		wantAdded []string
	}{
		// 空值（老任务）等于综合区
		{"", []string{"normal"}, "M-Team 综合区第 1 页", []string{"1", "3"}},
		{"normal", []string{"normal"}, "M-Team 综合区第 1 页", []string{"1", "3"}},
		{"adult", []string{"adult"}, "M-Team 成人区第 1 页", []string{"2", "3"}},
		// 两个都读：先综合区、后成人区，两边都有的种子（编号 3）只算一次
		{"all", []string{"normal", "adult"}, "M-Team 综合区、成人区第 1 页", []string{"1", "2", "3"}},
	}
	for _, c := range cases {
		t.Run("section="+c.section, func(t *testing.T) {
			r := mteamRig(t, map[string][]string{"normal": {"1", "3"}, "adult": {"2", "3"}})
			task := mteamTask(t, r, c.section)
			res, err := r.mon.RunTask(context.Background(), task.ID)
			require.NoError(t, err)
			assert.Equal(t, c.wantRead, searchedSections(r))
			for _, q := range r.site.searches {
				assert.True(t, q.FreeOnly)
			}
			assert.Equal(t, c.wantScope, res.Scope)
			assert.Equal(t, len(c.wantAdded), res.Listed)
			var added []string
			for _, p := range r.pushes {
				added = append(added, p.TorrentID)
			}
			assert.ElementsMatch(t, c.wantAdded, added)
			assert.Contains(t, res.Summary(), c.wantScope+"：列表 ")

			got, err := models.NewBrushRepository(r.db.DB).GetTask(task.ID)
			require.NoError(t, err)
			assert.Equal(t, res.Summary(), got.LastResult)
		})
	}
}

func TestBrushMonitor_MTeamAllSharesOneAttemptStart(t *testing.T) {
	// 读两个分区之间时间在走：推送闸门的「本次尝试」要是读第一个分区之前的时间，两个分区的种子共用它
	r := mteamRig(t, map[string][]string{"normal": {"1"}, "adult": {"2"}})
	before := r.clock.Now()
	r.site.onSearch = func() { r.clock.Advance(time.Minute) }
	task := mteamTask(t, r, "all")
	_, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	require.Len(t, r.pushes, 2)
	for _, p := range r.pushes {
		assert.True(t, before.Equal(p.Policy.AttemptStart), "attempt start = %s", p.Policy.AttemptStart)
		require.NotNil(t, p.Snapshot)
		assert.False(t, p.Snapshot.VerifiedAt.Before(p.Policy.AttemptStart), "快照不能早于本次尝试")
	}
}

func TestBrushMonitor_MTeamOneSectionFails(t *testing.T) {
	r := mteamRig(t, map[string][]string{"normal": {"1"}})
	r.site.sectionErr = map[string]error{"adult": errors.New("HTTP 502")}
	task := mteamTask(t, r, "all")
	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err, "一个分区读失败不让整轮失败")
	assert.Equal(t, 1, res.Added)
	assert.Equal(t, "M-Team 综合区第 1 页", res.Scope, "只写读到的分区")
	require.Len(t, res.Errors, 1)
	assert.Contains(t, res.Errors[0], "成人区")
	assert.Contains(t, res.Errors[0], "HTTP 502")

	got, err := models.NewBrushRepository(r.db.DB).GetTask(task.ID)
	require.NoError(t, err)
	assert.Contains(t, got.LastError, "成人区")
}

func TestBrushMonitor_MTeamAllSectionsFail(t *testing.T) {
	r := mteamRig(t, nil)
	r.site.sectionErr = map[string]error{"normal": errors.New("HTTP 500"), "adult": errors.New("HTTP 502")}
	task := mteamTask(t, r, "all")
	_, err := r.mon.RunTask(context.Background(), task.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "综合区: HTTP 500")
	assert.Contains(t, err.Error(), "成人区: HTTP 502")
	assert.Empty(t, r.pushes)
}

func TestBrushMonitor_SectionIgnoredForOtherSites(t *testing.T) {
	// 别的站点库里留着旧的分区值：只读一次、不带分区，摘要不加前缀
	r := newBrushRig(t)
	r.addItem(t, "1", nil)
	task := r.task(t, func(task *models.BrushTask) { task.Section = "all" })
	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	require.Len(t, r.site.searches, 1)
	assert.Empty(t, r.site.searches[0].Section)
	assert.Empty(t, res.Scope)
	assert.Equal(t, "列表 1 个、符合条件 1 个、加入 1 个", res.Summary())
}
