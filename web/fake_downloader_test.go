package web

import (
	"context"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// fakeDownloader is a full in-memory implementation of downloader.Downloader
// used by web tests to exercise handlers that iterate real torrents without a
// live client. Behaviour is configurable via the exported fields.
type fakeDownloader struct {
	name     string
	dlType   downloader.DownloaderType
	torrents []downloader.Torrent
	status   downloader.ClientStatus
	freSpace int64
	files    []downloader.TorrentFile
	trackers []downloader.TorrentTracker

	listErr        error
	getErr         error
	pauseErr       error
	resumeErr      error
	removeErr      error
	batchPauseErr  error
	batchResumeErr error
	batchRemoveErr error
	filesErr       error
	trackerErr     error
	addResult      downloader.AddTorrentResult
	addErr         error
}

func (f *fakeDownloader) Authenticate() error               { return nil }
func (f *fakeDownloader) Ping() (bool, error)               { return true, nil }
func (f *fakeDownloader) GetClientVersion() (string, error) { return "test", nil }
func (f *fakeDownloader) GetClientStatus() (downloader.ClientStatus, error) {
	return f.status, nil
}

func (f *fakeDownloader) GetClientFreeSpace(_ context.Context) (int64, error) {
	return f.freSpace, nil
}

func (f *fakeDownloader) GetIncompletePendingBytes(_ context.Context) (int64, error) {
	return 0, nil
}

func (f *fakeDownloader) GetAllTorrents() ([]downloader.Torrent, error) {
	return f.torrents, f.listErr
}

func (f *fakeDownloader) GetTorrentsBy(_ downloader.TorrentFilter) ([]downloader.Torrent, error) {
	return f.torrents, f.listErr
}

func (f *fakeDownloader) GetTorrent(id string) (downloader.Torrent, error) {
	if f.getErr != nil {
		return downloader.Torrent{}, f.getErr
	}
	for _, t := range f.torrents {
		if t.ID == id {
			return t, nil
		}
	}
	return downloader.Torrent{ID: id, Name: "fake"}, nil
}

func (f *fakeDownloader) AddTorrentEx(_ string, _ downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
	return f.addResult, f.addErr
}

func (f *fakeDownloader) AddTorrentFileEx(_ []byte, _ downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
	return f.addResult, f.addErr
}
func (f *fakeDownloader) PauseTorrent(_ string) error          { return f.pauseErr }
func (f *fakeDownloader) ResumeTorrent(_ string) error         { return f.resumeErr }
func (f *fakeDownloader) RemoveTorrent(_ string, _ bool) error { return f.removeErr }
func (f *fakeDownloader) PauseTorrents(_ []string) error {
	if f.batchPauseErr != nil {
		return f.batchPauseErr
	}
	return f.pauseErr
}

func (f *fakeDownloader) ResumeTorrents(_ []string) error {
	if f.batchResumeErr != nil {
		return f.batchResumeErr
	}
	return f.resumeErr
}

func (f *fakeDownloader) RemoveTorrents(_ []string, _ bool) error {
	if f.batchRemoveErr != nil {
		return f.batchRemoveErr
	}
	return f.removeErr
}
func (f *fakeDownloader) SetTorrentCategory(_, _ string) error { return nil }
func (f *fakeDownloader) SetTorrentTags(_, _ string) error     { return nil }
func (f *fakeDownloader) SetTorrentSavePath(_, _ string) error { return nil }
func (f *fakeDownloader) RecheckTorrent(_ string) error        { return nil }

func (f *fakeDownloader) GetTorrentFiles(_ string) ([]downloader.TorrentFile, error) {
	return f.files, f.filesErr
}

func (f *fakeDownloader) GetTorrentTrackers(_ string) ([]downloader.TorrentTracker, error) {
	return f.trackers, f.trackerErr
}

func (f *fakeDownloader) GetDiskInfo() (downloader.DiskInfo, error) {
	return downloader.DiskInfo{}, nil
}

func (f *fakeDownloader) GetSpeedLimit() (downloader.SpeedLimit, error) {
	return downloader.SpeedLimit{}, nil
}
func (f *fakeDownloader) SetSpeedLimit(_ downloader.SpeedLimit) error { return nil }
func (f *fakeDownloader) GetClientPaths() ([]string, error)           { return nil, nil }
func (f *fakeDownloader) GetClientLabels() ([]string, error)          { return nil, nil }
func (f *fakeDownloader) GetType() downloader.DownloaderType          { return f.dlType }
func (f *fakeDownloader) GetName() string                             { return f.name }
func (f *fakeDownloader) IsHealthy() bool                             { return true }
func (f *fakeDownloader) Close() error                                { return nil }
func (f *fakeDownloader) AddTorrent(_ []byte, _, _ string) error      { return nil }
func (f *fakeDownloader) AddTorrentWithPath(_ []byte, _, _, _ string) error {
	return nil
}
func (f *fakeDownloader) CheckTorrentExists(_ string) (bool, error) { return false, nil }
func (f *fakeDownloader) GetDiskSpace(_ context.Context) (int64, error) {
	return f.freSpace, nil
}

func (f *fakeDownloader) CanAddTorrent(_ context.Context, _ int64) (bool, error) {
	return true, nil
}

func (f *fakeDownloader) ProcessSingleTorrentFile(_ context.Context, _, _, _ string) error {
	return nil
}
