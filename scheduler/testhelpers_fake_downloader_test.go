package scheduler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// schedFakeDownloader is a configurable in-memory downloader.Downloader used by
// scheduler tests to drive processDownloader / runOnce / handleFreeEndedTorrent
// without a live client. Call counters and configurable errors let tests assert
// the exact side effects (removed IDs, pause calls, etc.).
type schedFakeDownloader struct {
	name     string
	dlType   downloader.DownloaderType
	healthy  bool
	torrents []downloader.Torrent
	trackers map[string][]downloader.TorrentTracker
	diskInfo downloader.DiskInfo

	getAllErr   error
	diskErr     error
	trackersErr error
	removeErr   error
	pauseErr    error
	getErr      error

	// per-torrent GetTorrent override keyed by id
	torrentByID map[string]downloader.Torrent

	// recorded side effects
	removedBatch    [][]string
	removedSingle   []string
	pausedIDs       []string
	removeDataFlags []bool
	closeCount      int
}

func newSchedFakeDownloader(name string) *schedFakeDownloader {
	return &schedFakeDownloader{
		name:        name,
		dlType:      downloader.DownloaderQBittorrent,
		healthy:     true,
		trackers:    map[string][]downloader.TorrentTracker{},
		torrentByID: map[string]downloader.Torrent{},
	}
}

func (f *schedFakeDownloader) Authenticate() error               { return nil }
func (f *schedFakeDownloader) Ping() (bool, error)               { return true, nil }
func (f *schedFakeDownloader) GetClientVersion() (string, error) { return "test", nil }
func (f *schedFakeDownloader) GetClientStatus() (downloader.ClientStatus, error) {
	return downloader.ClientStatus{}, nil
}

func (f *schedFakeDownloader) GetClientFreeSpace(_ context.Context) (int64, error) {
	return f.diskInfo.FreeSpace, nil
}

func (f *schedFakeDownloader) GetIncompletePendingBytes(_ context.Context) (int64, error) {
	return 0, nil
}

func (f *schedFakeDownloader) GetAllTorrents() ([]downloader.Torrent, error) {
	return f.torrents, f.getAllErr
}

func (f *schedFakeDownloader) GetTorrentsBy(_ downloader.TorrentFilter) ([]downloader.Torrent, error) {
	return f.torrents, f.getAllErr
}

func (f *schedFakeDownloader) GetTorrent(id string) (downloader.Torrent, error) {
	if f.getErr != nil {
		return downloader.Torrent{}, f.getErr
	}
	if t, ok := f.torrentByID[id]; ok {
		return t, nil
	}
	for _, t := range f.torrents {
		if t.ID == id || t.InfoHash == id {
			return t, nil
		}
	}
	return downloader.Torrent{}, downloader.ErrTorrentNotFound
}

func (f *schedFakeDownloader) AddTorrentEx(_ string, _ downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
	return downloader.AddTorrentResult{}, nil
}

func (f *schedFakeDownloader) AddTorrentFileEx(_ []byte, _ downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
	return downloader.AddTorrentResult{}, nil
}

func (f *schedFakeDownloader) PauseTorrent(id string) error {
	if f.pauseErr != nil {
		return f.pauseErr
	}
	f.pausedIDs = append(f.pausedIDs, id)
	return nil
}

func (f *schedFakeDownloader) ResumeTorrent(_ string) error { return nil }

func (f *schedFakeDownloader) RemoveTorrent(id string, removeData bool) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removedSingle = append(f.removedSingle, id)
	f.removeDataFlags = append(f.removeDataFlags, removeData)
	return nil
}

func (f *schedFakeDownloader) PauseTorrents(_ []string) error { return nil }
func (f *schedFakeDownloader) ResumeTorrents(_ []string) error {
	return nil
}

func (f *schedFakeDownloader) RemoveTorrents(ids []string, removeData bool) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removedBatch = append(f.removedBatch, ids)
	f.removeDataFlags = append(f.removeDataFlags, removeData)
	return nil
}

func (f *schedFakeDownloader) SetTorrentCategory(_, _ string) error { return nil }
func (f *schedFakeDownloader) SetTorrentTags(_, _ string) error     { return nil }
func (f *schedFakeDownloader) SetTorrentSavePath(_, _ string) error { return nil }
func (f *schedFakeDownloader) RecheckTorrent(_ string) error        { return nil }

func (f *schedFakeDownloader) GetTorrentFiles(_ string) ([]downloader.TorrentFile, error) {
	return nil, nil
}

func (f *schedFakeDownloader) GetTorrentTrackers(id string) ([]downloader.TorrentTracker, error) {
	if f.trackersErr != nil {
		return nil, f.trackersErr
	}
	return f.trackers[id], nil
}

func (f *schedFakeDownloader) GetDiskInfo() (downloader.DiskInfo, error) {
	return f.diskInfo, f.diskErr
}

func (f *schedFakeDownloader) GetSpeedLimit() (downloader.SpeedLimit, error) {
	return downloader.SpeedLimit{}, nil
}
func (f *schedFakeDownloader) SetSpeedLimit(_ downloader.SpeedLimit) error { return nil }
func (f *schedFakeDownloader) GetClientPaths() ([]string, error)           { return nil, nil }
func (f *schedFakeDownloader) GetClientLabels() ([]string, error)          { return nil, nil }
func (f *schedFakeDownloader) GetType() downloader.DownloaderType          { return f.dlType }
func (f *schedFakeDownloader) GetName() string                             { return f.name }
func (f *schedFakeDownloader) IsHealthy() bool                             { return f.healthy }
func (f *schedFakeDownloader) Close() error                                { f.closeCount++; return nil }
func (f *schedFakeDownloader) AddTorrent(_ []byte, _, _ string) error      { return nil }
func (f *schedFakeDownloader) AddTorrentWithPath(_ []byte, _, _, _ string) error {
	return nil
}
func (f *schedFakeDownloader) CheckTorrentExists(_ string) (bool, error) { return false, nil }
func (f *schedFakeDownloader) GetDiskSpace(_ context.Context) (int64, error) {
	return f.diskInfo.FreeSpace, nil
}

func (f *schedFakeDownloader) CanAddTorrent(_ context.Context, _ int64) (bool, error) {
	return true, nil
}

func (f *schedFakeDownloader) ProcessSingleTorrentFile(_ context.Context, _, _, _ string) error {
	return nil
}

// registerFakeDownloader wires a fake into a DownloaderManager through the real
// factory/config path so ListDownloaders()/GetDownloader() return it, mirroring
// production usage. isDefault makes it the default downloader.
func registerFakeDownloader(t *testing.T, dm *downloader.DownloaderManager, fake *schedFakeDownloader, isDefault bool) {
	t.Helper()
	dm.RegisterFactory(downloader.DownloaderQBittorrent, func(_ downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		fake.name = name
		return fake, nil
	})
	require.NoError(t, dm.RegisterConfig(fake.name, downloader.NewGenericConfig(
		downloader.DownloaderQBittorrent, "http://localhost:8080", "u", "p", true,
	), isDefault))
}
