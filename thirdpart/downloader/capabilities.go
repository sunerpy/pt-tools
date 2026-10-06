package downloader

import (
	"context"
	"errors"
)

// 可选能力：不是每种下载器（或每个版本）都支持，所以不放进 Downloader 接口。
// 调用方用类型断言取用，断言失败或返回 ErrCapabilityUnsupported 时如实告诉用户。

// ErrCapabilityUnsupported 表示下载器或它当前的版本不支持这项操作。
var ErrCapabilityUnsupported = errors.New("下载器不支持这项操作")

// ErrTrackerNotFound 表示种子里没有要修改的那个 tracker 地址。
var ErrTrackerNotFound = errors.New("种子里没有这个 tracker 地址")

// TorrentExporter 把下载器里的种子导出成 .torrent 文件内容。
// qBittorrent 4.5 起提供 torrents/export；Transmission 没有对应接口，不实现。
type TorrentExporter interface {
	ExportTorrent(ctx context.Context, hash string) ([]byte, error)
}

// TrackerEditor 把种子的某个 tracker 地址改成新地址。
// qBittorrent 用 torrents/editTracker；Transmission 4.0（RPC 17）起用 torrent-set 的 trackerList，更早的版本用 trackerReplace。
type TrackerEditor interface {
	EditTracker(ctx context.Context, hash, oldURL, newURL string) error
}
