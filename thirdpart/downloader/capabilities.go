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

// TrackerReader 按 ctx 读取一个种子的 tracker 列表：ctx 取消时底层请求也一起取消。
// 不实现它的下载器只能用不可取消的 GetTorrentTrackers。
type TrackerReader interface {
	GetTorrentTrackersContext(ctx context.Context, id string) ([]TorrentTracker, error)
}

// TagRemover 从种子上去掉一个标签，只用于 pt-tools 自己加的、每个任务独有的标签。
// qBittorrent 用 torrents/deleteTags：标签从所有种子和标签列表里一起删掉（id 不用），不会改到别的标签。
// Transmission 只能整份改写一个种子的 labels，和别处同时改标签时会盖掉别人的修改，所以不实现：任务标签留在种子上，没有全局标签列表，不碍事。
type TagRemover interface {
	RemoveTag(id, tag string) error
}

// TorrentTagRemover 从指定的种子上去掉标签（逗号分隔；为空时去掉这些种子的全部标签），别的种子不动。
// qBittorrent 用 torrents/removeTags；Transmission 只能整份改写 labels，调用方改用 SetTorrentTags。
type TorrentTagRemover interface {
	RemoveTorrentTags(ids []string, tags string) error
}

// CategoryCreator 在下载器里建一个分类（带保存目录）。qBittorrent 用 torrents/createCategory，
// 已经有这个分类时不算失败；Transmission 没有分类，不实现。
type CategoryCreator interface {
	CreateCategory(name, savePath string) error
}

// BulkTrackerReader 一次读出全部种子的 tracker 列表（键是小写 info hash）。
// Transmission 的 torrent-get 能一次带回 trackerStats；qBittorrent 只能逐个读，不实现。
type BulkTrackerReader interface {
	GetAllTorrentTrackers(ctx context.Context) (map[string][]TorrentTracker, error)
}
