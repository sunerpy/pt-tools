package transfer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestListJobs(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for _, st := range []string{models.TransferPending, models.TransferChecking, models.TransferDone, models.TransferFailed} {
		require.NoError(t, e.db.Create(&models.TorrentTransferJob{InfoHash: st, State: st, TorrentData: []byte("x")}).Error)
	}
	require.NoError(t, e.db.Create(&models.TorrentTransferJob{Kind: models.JobKindReseed, InfoHash: "r", State: models.TransferDone}).Error)
	reseed, err := e.svc.ListJobs(ctx, "", models.JobKindReseed)
	require.NoError(t, err)
	assert.Len(t, reseed, 1, "按种类分开列")
	all, err := e.svc.ListJobs(ctx, "", models.JobKindTransfer)
	require.NoError(t, err)
	require.Len(t, all, 4)
	assert.Equal(t, models.TransferFailed, all[0].State, "最新的在前")
	assert.Empty(t, all[0].TorrentData, "列表不带种子文件")
	active, err := e.svc.ListJobs(ctx, JobsActive, models.JobKindTransfer)
	require.NoError(t, err)
	assert.Len(t, active, 2)
	finished, err := e.svc.ListJobs(ctx, JobsFinished, models.JobKindTransfer)
	require.NoError(t, err)
	assert.Len(t, finished, 2)
}

func TestPathMapsStore(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for _, bad := range []struct {
		src, dst uint
		items    []PathMapEntry
	}{
		{0, dstID, nil},
		{srcID, srcID, nil},
		{srcID, dstID, []PathMapEntry{{SourcePrefix: "/a", TargetPrefix: " "}}},
		{srcID, dstID, []PathMapEntry{{SourcePrefix: "/a", TargetPrefix: "/b"}, {SourcePrefix: "/a/", TargetPrefix: "/c"}}},
		{srcID, dstID, make([]PathMapEntry, MaxPathMaps+1)},
	} {
		_, err := e.svc.SavePathMaps(ctx, bad.src, bad.dst, bad.items)
		assert.ErrorIs(t, err, ErrPathMapInvalid, "%+v", bad)
	}
	maps, err := e.svc.SavePathMaps(ctx, srcID, dstID, []PathMapEntry{{SourcePrefix: " /downloads ", TargetPrefix: "/data"}, {SourcePrefix: "/x", TargetPrefix: "/y"}})
	require.NoError(t, err)
	require.Len(t, maps, 2)
	assert.Equal(t, "/downloads", maps[0].SourcePrefix)
	_, err = e.svc.SavePathMaps(ctx, dstID, srcID, []PathMapEntry{{SourcePrefix: "/data", TargetPrefix: "/downloads"}})
	require.NoError(t, err)

	pair, err := e.svc.ListPathMaps(ctx, srcID, dstID)
	require.NoError(t, err)
	assert.Len(t, pair, 2)
	all, err := e.svc.ListPathMaps(ctx, 0, 0)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	maps, err = e.svc.SavePathMaps(ctx, srcID, dstID, nil)
	require.NoError(t, err)
	assert.Empty(t, maps, "整体替换：空列表清掉这一对")
	all, err = e.svc.ListPathMaps(ctx, 0, 0)
	require.NoError(t, err)
	assert.Len(t, all, 1, "别的下载器对不受影响")
}

func TestRulesStore(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, err := e.svc.SaveRule(ctx, models.TransferRule{Name: "", SourceDownloaderID: srcID, TargetDownloaderID: dstID})
	assert.ErrorIs(t, err, ErrRuleInvalid)

	r, err := e.svc.SaveRule(ctx, models.TransferRule{Name: "a", Enabled: true, SourceDownloaderID: srcID, TargetDownloaderID: dstID})
	require.NoError(t, err)
	assert.NotZero(t, r.ID)
	assert.Equal(t, models.TransferRuleDefaultMaxPerRun, r.MaxPerRun)
	_, err = e.svc.SaveRule(ctx, models.TransferRule{Name: "a", SourceDownloaderID: srcID, TargetDownloaderID: dstID})
	assert.ErrorIs(t, err, ErrRuleNameTaken)

	require.NoError(t, e.svc.RecordRuleRun(ctx, r.ID, e.now, "done"))
	r.Enabled = false
	r.LastResult = "不会被写进去"
	upd, err := e.svc.SaveRule(ctx, r)
	require.NoError(t, err)
	assert.False(t, upd.Enabled, "关掉的开关原样保存")
	assert.Equal(t, "done", upd.LastResult)
	require.NotNil(t, upd.LastRunAt)

	_, err = e.svc.SaveRule(ctx, models.TransferRule{ID: 999, Name: "z", SourceDownloaderID: srcID, TargetDownloaderID: dstID})
	assert.ErrorIs(t, err, ErrRuleNotFound)

	rules, err := e.svc.ListRules(ctx)
	require.NoError(t, err)
	assert.Len(t, rules, 1)
	_, err = e.svc.GetRule(ctx, 999)
	assert.ErrorIs(t, err, ErrRuleNotFound)
	require.NoError(t, e.svc.DeleteRule(ctx, r.ID))
	assert.ErrorIs(t, e.svc.DeleteRule(ctx, r.ID), ErrRuleNotFound)
	assert.Equal(t, e.now, e.svc.Now())
}
