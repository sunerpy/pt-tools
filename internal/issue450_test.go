// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	sm "github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

const issue450Site = models.SiteGroup("springsunday")

func newMockDl(t *testing.T) *sm.MockDownloader {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	return mockDl
}

func insertTorrentInfo(t *testing.T, hash, torrentID string, size int64, pushed bool, retry int) {
	t.Helper()
	p := pushed
	future := time.Now().Add(1 * time.Hour)
	ti := &models.TorrentInfo{
		SiteName:     string(issue450Site),
		TorrentID:    torrentID,
		TorrentHash:  &hash,
		IsPushed:     &p,
		FreeEndTime:  &future,
		TorrentSize:  size,
		RetryCount:   retry,
		IsDownloaded: true,
	}
	require.NoError(t, global.GlobalDB.UpsertTorrent(ti))
}

func getTorrent(t *testing.T, hash string) *models.TorrentInfo {
	t.Helper()
	ti, err := global.GlobalDB.GetTorrentBySiteAndHash(string(issue450Site), hash)
	require.NoError(t, err)
	return ti
}

// T1: free=102, min=20, size=95 → 102-95=7<20 → ErrTorrentTooLarge（不是 insufficient）。
func TestDiskProtect_TooLargeReturnsTorrentTooLarge(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	insertTorrentInfo(t, hash, "t1", 95*gb, false, 0)

	mockDl := newMockDl(t)
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(102*gb, nil)
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil)

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", issue450Site, false)
	require.ErrorIs(t, err, downloader.ErrTorrentTooLarge)
	require.NotErrorIs(t, err, downloader.ErrInsufficientSpace)
	assert.Equal(t, int64(0), GetDiskBudget().Reserved())
	ti := getTorrent(t, hash)
	assert.Equal(t, 0, ti.RetryCount, "too-large 不应累加 retry_count")
	assert.NotEmpty(t, ti.LastError, "应写入 last_error")
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr, "too-large 不应删除 .torrent")
}

// T2: free=4, min=20 → effectiveFree(4)<=min(20) → ErrInsufficientSpace。
func TestDiskProtect_DiskFullReturnsInsufficientSpace(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	insertTorrentInfo(t, hash, "t2", 1*gb, false, 0)

	mockDl := newMockDl(t)
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(4*gb, nil)
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil)

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", issue450Site, false)
	require.ErrorIs(t, err, downloader.ErrInsufficientSpace)
}

// T2a: free=20, min=20, size=1 → effectiveFree(20)<=min(20) → ErrInsufficientSpace（验证 <=）。
func TestDiskProtect_ExactlyAtMinStops(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	insertTorrentInfo(t, hash, "t2a", 1*gb, false, 0)

	mockDl := newMockDl(t)
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(20*gb, nil)
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil)

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", issue450Site, false)
	require.ErrorIs(t, err, downloader.ErrInsufficientSpace)
}

// T2b: free=80, min=20, size=0（DB=0 且文件无 length）→ 放行推送，不 Reserve。
func TestDiskProtect_ZeroSizePushesWhenAboveMin(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	insertTorrentInfo(t, hash, "t2b", 0, false, 0)

	mockDl := newMockDl(t)
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(80*gb, nil)
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil)
	mockDl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).
		Return(downloader.AddTorrentResult{Success: true, Hash: hash}, nil)

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", issue450Site, false)
	require.NoError(t, err)
	assert.Equal(t, int64(0), GetDiskBudget().Reserved(), "size 未知不 Reserve")
}

// T3: GetClientFreeSpace 报错 → ErrInsufficientSpace（fail-closed）。
func TestDiskProtect_FailClosedReturnsInsufficientSpace(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	insertTorrentInfo(t, hash, "t3", 30*gb, false, 0)

	mockDl := newMockDl(t)
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(int64(0), errors.New("qbit unreachable"))

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", issue450Site, false)
	require.ErrorIs(t, err, downloader.ErrInsufficientSpace)
}

// T4: free=80, min=20, size=30 → push, Reserved==30。
func TestDiskProtect_FitsStillPushes(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	insertTorrentInfo(t, hash, "t4", 30*gb, false, 0)

	mockDl := newMockDl(t)
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(80*gb, nil)
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil)
	mockDl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).
		Return(downloader.AddTorrentResult{Success: true, Hash: hash}, nil)

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", issue450Site, false)
	require.NoError(t, err)
	assert.Equal(t, 30*gb, GetDiskBudget().Reserved())
}

// T5: [big, small, small], free=102, min=20 → big 跳过、两个 small 推送；不 break。
func TestRunPushLoop_TooLargeSkippedSmallPushed(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	bigPath, bigHash := makeUniqueTorrentFile(t, dir, "big")
	s1Path, s1Hash := makeUniqueTorrentFile(t, dir, "s1")
	s2Path, s2Hash := makeUniqueTorrentFile(t, dir, "s2")
	insertTorrentInfo(t, bigHash, "big", 95*gb, false, 0)
	insertTorrentInfo(t, s1Hash, "s1", 5*gb, false, 0)
	insertTorrentInfo(t, s2Hash, "s2", 5*gb, false, 0)

	mockDl := newMockDl(t)
	mockDl.EXPECT().CheckTorrentExists(gomock.Any()).Return(false, nil).AnyTimes()
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(102*gb, nil).AnyTimes()
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil).AnyTimes()
	mockDl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).
		Return(downloader.AddTorrentResult{Success: true}, nil).Times(2)

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	result := runPushLoop(context.Background(), mockDl, dlInfo, dir,
		[]string{bigPath, s1Path, s2Path}, "cat", "tag", "", issue450Site, false, 0)

	assert.Equal(t, 2, result.success)
	assert.Equal(t, 1, result.skippedTooLarge)
	assert.False(t, result.diskFull)
	_, statErr := os.Stat(bigPath)
	assert.NoError(t, statErr, "过大种子 .torrent 应保留")
}

// T6: [small,small,small], free=4<=min=20 → 第一个即 insufficient，break；零推送。
func TestRunPushLoop_DiskFullStops(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	p1, h1 := makeUniqueTorrentFile(t, dir, "a")
	p2, h2 := makeUniqueTorrentFile(t, dir, "b")
	p3, h3 := makeUniqueTorrentFile(t, dir, "c")
	insertTorrentInfo(t, h1, "a", 3*gb, false, 0)
	insertTorrentInfo(t, h2, "b", 3*gb, false, 0)
	insertTorrentInfo(t, h3, "c", 3*gb, false, 0)

	mockDl := newMockDl(t)
	mockDl.EXPECT().CheckTorrentExists(gomock.Any()).Return(false, nil).AnyTimes()
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(4*gb, nil).AnyTimes()
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil).AnyTimes()

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	result := runPushLoop(context.Background(), mockDl, dlInfo, dir,
		[]string{p1, p2, p3}, "cat", "tag", "", issue450Site, false, 0)

	assert.True(t, result.diskFull)
	assert.Equal(t, 0, result.success)
	for _, p := range []string{p1, p2, p3} {
		_, statErr := os.Stat(p)
		assert.NoError(t, statErr, "磁盘满停止，所有 .torrent 保留")
	}
}

// T7: [big, small, small]，big→too-large(continue) 然后 small→insufficient(break)。
func TestRunPushLoop_TooLargeThenDiskFull(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	bigPath, bigHash := makeUniqueTorrentFile(t, dir, "big")
	s1Path, s1Hash := makeUniqueTorrentFile(t, dir, "s1")
	s2Path, s2Hash := makeUniqueTorrentFile(t, dir, "s2")
	insertTorrentInfo(t, bigHash, "big", 95*gb, false, 0)
	insertTorrentInfo(t, s1Hash, "s1", 3*gb, false, 0)
	insertTorrentInfo(t, s2Hash, "s2", 3*gb, false, 0)

	mockDl := newMockDl(t)
	mockDl.EXPECT().CheckTorrentExists(gomock.Any()).Return(false, nil).AnyTimes()
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil).AnyTimes()
	gomock.InOrder(
		mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(102*gb, nil),
		mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(4*gb, nil),
	)

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	result := runPushLoop(context.Background(), mockDl, dlInfo, dir,
		[]string{bigPath, s1Path, s2Path}, "cat", "tag", "", issue450Site, false, 0)

	assert.Equal(t, 1, result.skippedTooLarge)
	assert.True(t, result.diskFull)
	assert.Equal(t, 0, result.success)
	_, statErr := os.Stat(s2Path)
	assert.NoError(t, statErr, "第三个未被处理，.torrent 保留")
}

// T7b: 磁盘满 break 后 defer sweep 仍执行——删除陈旧的 is_pushed .torrent。
func TestRunPushLoop_SweepRunsOnDiskFullBreak(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	stalePath, staleHash := makeUniqueTorrentFile(t, dir, "stale")
	smallPath, smallHash := makeUniqueTorrentFile(t, dir, "small")
	insertTorrentInfo(t, staleHash, "stale", 1*gb, true, 0) // is_pushed=true → 应被 sweep
	insertTorrentInfo(t, smallHash, "small", 3*gb, false, 0)

	mockDl := newMockDl(t)
	mockDl.EXPECT().CheckTorrentExists(gomock.Any()).Return(false, nil).AnyTimes()
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(4*gb, nil).AnyTimes()
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil).AnyTimes()

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	result := runPushLoop(context.Background(), mockDl, dlInfo, dir,
		[]string{smallPath, stalePath}, "cat", "tag", "", issue450Site, false, 24)

	assert.True(t, result.diskFull)
	_, statErr := os.Stat(stalePath)
	assert.True(t, os.IsNotExist(statErr), "defer sweep 应删除已推送的陈旧 .torrent")
}

// T8: too-large 后 last_error 含"超过"，retry_count 未增。
func TestDiskProtect_TooLargeRecordsLastError(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	insertTorrentInfo(t, hash, "t8", 95*gb, false, 0)

	mockDl := newMockDl(t)
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(102*gb, nil)
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil)

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", issue450Site, false)
	require.ErrorIs(t, err, downloader.ErrTorrentTooLarge)
	ti := getTorrent(t, hash)
	assert.Contains(t, ti.LastError, "超过")
	assert.Equal(t, 0, ti.RetryCount)
}

// T9: insufficient 后 last_error 含"磁盘空间不足"。
func TestDiskProtect_InsufficientRecordsLastError(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	insertTorrentInfo(t, hash, "t9", 1*gb, false, 0)

	mockDl := newMockDl(t)
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(4*gb, nil)
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil)

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", issue450Site, false)
	require.ErrorIs(t, err, downloader.ErrInsufficientSpace)
	ti := getTorrent(t, hash)
	assert.Contains(t, ti.LastError, "磁盘空间不足")
}

// T10: is_pushed=true → 删。
func TestSweep_RemovesPushedLeftover(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeUniqueTorrentFile(t, dir, "pushed")
	insertTorrentInfo(t, hash, "pushed", 1*gb, true, 0)

	sweepStagingDir(dir, issue450Site, 24)
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

// T11: 无 DB 记录（孤立）→ 删。
func TestSweep_RemovesOrphan(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, _ := makeUniqueTorrentFile(t, dir, "orphan")

	sweepStagingDir(dir, issue450Site, 24)
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

// T12: mtime 超 RetainHours 且未推送 → 删。
func TestSweep_RemovesAgedUnpushed(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeUniqueTorrentFile(t, dir, "aged")
	insertTorrentInfo(t, hash, "aged", 1*gb, false, 0)
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(path, old, old))

	sweepStagingDir(dir, issue450Site, 24)
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

// T13: 新鲜 mtime 且未推送 → 保留。
func TestSweep_KeepsFreshUnpushed(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeUniqueTorrentFile(t, dir, "fresh")
	insertTorrentInfo(t, hash, "fresh", 1*gb, false, 0)

	sweepStagingDir(dir, issue450Site, 24)
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr)
}

// T14: RetainHours=0 → 不删任何文件。
func TestSweep_DisabledWhenRetainZero(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeUniqueTorrentFile(t, dir, "disabled")
	insertTorrentInfo(t, hash, "disabled", 1*gb, true, 0)

	sweepStagingDir(dir, issue450Site, 0)
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr, "RetainHours=0 关闭 sweep")
}

// T15: retry_count < MaxRetry 且未老化 → 保留。
func TestSweep_KeepsMaxRetryNotReached(t *testing.T) {
	setUpDiskProtectTest(t)
	store := core.NewConfigStore(global.GlobalDB)
	require.NoError(t, store.SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir:           t.TempDir(),
		MaxRetry:              3,
		CleanupDiskProtect:    true,
		CleanupMinDiskSpaceGB: 20,
		CleanupEnabled:        false,
		RetainHours:           24,
	}))
	dir := t.TempDir()
	path, hash := makeUniqueTorrentFile(t, dir, "retry")
	insertTorrentInfo(t, hash, "retry", 1*gb, false, 1)

	sweepStagingDir(dir, issue450Site, 24)
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr, "retry_count(1) < MaxRetry(3) 且新鲜 → 保留")
}
