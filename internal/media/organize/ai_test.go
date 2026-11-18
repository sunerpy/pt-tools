package organize

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 「让 AI 认」的输入：只在整理卡在没认出条目时有（种子名与站点标题）；认出了、别的问题时写明原因
func TestRecognizeInput(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	const mystery = "7777777777777777777777777777777777777777"
	e.addTorrent(mystery, "Mystery.Thing.2020.1080p.mkv", map[string]int{"Mystery.Thing.2020.1080p.mkv": 2}, nil)
	hash := mystery
	require.NoError(t, e.db.Create(&models.TorrentInfo{SiteName: "hdsky", TorrentID: "1", TorrentHash: &hash, Title: "神秘事件 2020"}).Error)

	in, err := e.svc.RecognizeInput(e.ctx, Request{DownloaderID: 1, Hash: mystery})
	require.NoError(t, err)
	assert.Equal(t, "Mystery.Thing.2020.1080p.mkv", in.Title)
	assert.Equal(t, "神秘事件 2020", in.Subtitle, "站点上的标题作副标题")

	e.oppenheimer()
	_, err = e.svc.RecognizeInput(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "已经认出了条目")
	// 认出了条目、卡在别的地方（没有启用的电影库）：不用 AI 认
	require.NoError(t, e.db.Model(&models.MediaLibrary{}).Where("kind = ?", models.MediaKindMovie).Update("enabled", false).Error)
	_, err = e.svc.RecognizeInput(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "不是因为没认出条目")
	require.NoError(t, e.db.Model(&models.MediaLibrary{}).Where("kind = ?", models.MediaKindMovie).Update("enabled", true).Error)

	e.addTorrent("1111111111111111111111111111111111111111", "Lost.2022.mkv", map[string]int{"Lost.2022.mkv": 2}, func(t *downloader.Torrent) { t.SavePath = "/elsewhere" })
	_, err = e.svc.RecognizeInput(e.ctx, Request{DownloaderID: 1, Hash: "1111111111111111111111111111111111111111"})
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "不是因为没认出条目")
	assert.Contains(t, err.Error(), "路径映射")

	_, err = e.svc.RecognizeInput(e.ctx, Request{DownloaderID: 1, Hash: "9999"})
	require.ErrorIs(t, err, ErrNotFound)

	// 整理历史里的一条：按记录的下载器与种子认，不带之前指定的条目
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: mystery})
	require.NoError(t, err)
	require.Equal(t, 1, res.Failed)
	var row models.MediaTransferHistory
	require.NoError(t, e.db.Where("info_hash = ?", mystery).First(&row).Error)
	require.NoError(t, e.db.Model(&row).Updates(map[string]any{"trigger": models.MediaTriggerManual, "media_type": "movie", "tmdb_id": 872585}).Error)
	req, err := e.svc.HistoryRequest(e.ctx, row.ID)
	require.NoError(t, err)
	assert.Equal(t, Request{DownloaderID: 1, Hash: mystery}, req)
	_, err = e.svc.HistoryRequest(e.ctx, 99999)
	require.ErrorIs(t, err, ErrNotFound)
}
