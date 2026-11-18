package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 刷流正在管的种子按记录认：记录上的下载器编号对得上；编号为 0 的旧记录按所属任务的下载器；结束了的不算。
func TestBrushRepository_ActiveHashesOn(t *testing.T) {
	db := newBrushTestDB(t)
	repo := NewBrushRepository(db)
	onOne := &BrushTask{Name: "a", SiteName: "s", DownloaderID: 1}
	onTwo := &BrushTask{Name: "b", SiteName: "s", DownloaderID: 2}
	require.NoError(t, repo.SaveTask(onOne))
	require.NoError(t, repo.SaveTask(onTwo))
	add := func(task *BrushTask, id, hash string, dlID uint, state string) *BrushTorrent {
		bt := &BrushTorrent{TaskID: task.ID, InfoHash: hash, SiteName: "s", TorrentID: id, DownloaderID: dlID, State: state, AddedAt: time.Now()}
		require.NoError(t, db.Create(bt).Error)
		return bt
	}
	add(onOne, "1", "AA", 1, BrushTorrentActive)
	add(onOne, "2", "bb", 0, BrushTorrentActive) // 旧记录：按任务的下载器 1
	add(onTwo, "3", "cc", 0, BrushTorrentActive) // 任务在下载器 2
	add(onTwo, "4", "dd", 1, BrushTorrentActive) // 任务换了下载器，种子还在下载器 1
	add(onOne, "5", "ee", 1, BrushTorrentRemoved)
	add(onOne, "6", "ff", 1, BrushTorrentGone)

	got, err := repo.ActiveHashesOn(1)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"aa": true, "bb": true, "dd": true}, got)
	got, err = repo.ActiveHashesOn(2)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"cc": true}, got)
}

// 点「保留」时交还这台下载器上这个种子的进行中记录：记成 gone，原因写清楚；重复交还不再改；别的下载器上的不动。
func TestBrushRepository_EndActiveByHash(t *testing.T) {
	db := newBrushTestDB(t)
	repo := NewBrushRepository(db)
	task := &BrushTask{Name: "a", SiteName: "s", DownloaderID: 1}
	require.NoError(t, repo.SaveTask(task))
	mine := &BrushTorrent{TaskID: task.ID, InfoHash: "AbC", SiteName: "s", TorrentID: "1", DownloaderID: 1, State: BrushTorrentActive, AddedAt: time.Now()}
	other := &BrushTorrent{TaskID: task.ID, InfoHash: "abc2", SiteName: "s", TorrentID: "2", DownloaderID: 2, State: BrushTorrentActive, AddedAt: time.Now()}
	require.NoError(t, db.Create(mine).Error)
	require.NoError(t, db.Create(other).Error)

	active, err := repo.IsActive(mine.ID)
	require.NoError(t, err)
	assert.True(t, active)

	at := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	n, err := repo.EndActiveByHash(1, "abc", "种子带保留标签（pt-tools-keep），交还给用户", at)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	var got BrushTorrent
	require.NoError(t, db.First(&got, mine.ID).Error)
	assert.Equal(t, BrushTorrentGone, got.State)
	assert.Equal(t, "种子带保留标签（pt-tools-keep），交还给用户", got.RemoveReason)
	active, err = repo.IsActive(mine.ID)
	require.NoError(t, err)
	assert.False(t, active)

	n, err = repo.EndActiveByHash(1, "abc", "又一次", at.Add(time.Hour))
	require.NoError(t, err)
	assert.Zero(t, n, "已经交还过")
	require.NoError(t, db.First(&got, mine.ID).Error)
	assert.Equal(t, "种子带保留标签（pt-tools-keep），交还给用户", got.RemoveReason)

	n, err = repo.EndActiveByHash(1, "abc2", "x", at)
	require.NoError(t, err)
	assert.Zero(t, n, "别的下载器上的不动")
}
