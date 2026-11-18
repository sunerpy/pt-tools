package web

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/sunerpy/pt-tools/global"
	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 删除刷流任务时，任务名下留在下载器里的种子怎么办：DELETE /api/brush/tasks/{id}?torrents=。
const (
	// BrushDeleteHandover（默认，不带参数也是它）交给自动删种：删掉任务和刷流记录以后，种子不再归刷流管，按自动删种的规则处理。
	BrushDeleteHandover = "handover"
	// BrushDeleteKeep 保留：先给种子加上保留标签（pt-tools-keep），自动删除都跳过它们，再删任务。
	BrushDeleteKeep = "keep"
)

// brushDeleteKeepReason 是删除任务时选了保留、交还刷流记录写的原因（有种子保留失败、任务没删时，任务的种子列表里看得到）。
const brushDeleteKeepReason = "删除刷流任务时选了保留，交还给用户"

// errBrushKeepIncomplete 表示删除任务时选了保留，有种子保留失败：任务没有删。
var errBrushKeepIncomplete = errors.New("有种子保留失败，任务没有删除")

// BrushKeepFailure 是删除任务时保留失败的一个种子。
type BrushKeepFailure struct {
	Title          string `json:"title"`
	InfoHash       string `json:"info_hash"`
	DownloaderID   uint   `json:"downloader_id"`
	DownloaderName string `json:"downloader_name,omitempty"`
	Message        string `json:"message"`
}

// BrushDeleteResponse 是删除刷流任务的结果。选了保留时，Kept 是加上保留标签（本来就带的也算）并交还给用户的种子数，
// Missing 是已经不在下载器里的。有种子保留失败时任务不删、接口回 502，Error 与 Failed 写明是哪几个；
// 已经加上标签的保持保留（它们已经交还给用户）。
type BrushDeleteResponse struct {
	Success bool               `json:"success"`
	Error   string             `json:"error,omitempty"`
	Kept    int                `json:"kept,omitempty"`
	Missing int                `json:"missing,omitempty"`
	Failed  []BrushKeepFailure `json:"failed,omitempty"`
}

// brushKeepConn 是一台要保留种子的下载器：实例与名字，取不到时是原因。
type brushKeepConn struct {
	dl   downloader.Downloader
	name string
	err  error
}

// keepBrushTorrentsAndDelete 是删除任务时选了保留：给任务名下进行中的种子逐个加上保留标签并交还刷流记录（与种子列表的
// 「保留」同一个做法，见 keepOne），全部成功（种子已经不在下载器里的不算失败）才删除任务。调用方持着任务锁（WithTaskLock），
// 这期间刷流不会往这个任务里加种。
//
// 种子按各自加入时的下载器处理（下载器编号为 0 的旧记录按任务的下载器），每台下载器读一次种子列表。有下载器连不上、
// 读不出种子列表或者标签加不上时不删任务，返回 errBrushKeepIncomplete，resp.Failed 写明是哪几个。先连上各台下载器，
// 再拿删除锁（见 internal.RemovalMutex）：读种子列表、加标签与删除任务都在锁里，与自动删除不会交错。
func (s *Server) keepBrushTorrentsAndDelete(ctx context.Context, repo *models.BrushRepository, id uint, resp *BrushDeleteResponse) error {
	task, err := repo.GetTask(id)
	if err != nil {
		return err
	}
	rows, err := repo.ActiveTorrents(id)
	if err != nil {
		return err
	}
	groups := map[uint][]models.BrushTorrent{}
	for _, bt := range rows {
		dlID := bt.DownloaderID
		if dlID == 0 {
			dlID = task.DownloaderID
		}
		groups[dlID] = append(groups[dlID], bt)
	}
	ids := slices.Sorted(maps.Keys(groups))
	conns, err := s.brushKeepConns(ctx, ids)
	if err != nil {
		return err
	}

	mu := ptinternal.RemovalMutex()
	mu.Lock()
	defer mu.Unlock()
	for _, dlID := range ids {
		c := conns[dlID]
		fail := func(bt models.BrushTorrent, msg string) {
			title := bt.Title
			if title == "" {
				title = bt.TorrentID
			}
			resp.Failed = append(resp.Failed, BrushKeepFailure{
				Title: title, InfoHash: bt.InfoHash, DownloaderID: dlID, DownloaderName: c.name, Message: msg,
			})
		}
		var all []downloader.Torrent
		if c.err == nil {
			if all, err = c.dl.GetAllTorrents(); err != nil {
				c.err = fmt.Errorf("读取种子列表失败: %w", err)
			}
		}
		if c.err != nil {
			for _, bt := range groups[dlID] {
				fail(bt, c.err.Error())
			}
			continue
		}
		labels := c.dl.GetType() == downloader.DownloaderTransmission
		for _, bt := range groups[dlID] {
			t, ok := findTorrent(all, bt.InfoHash)
			if !ok {
				resp.Missing++
				continue
			}
			if err := keepOne(c.dl, dlID, t, labels, repo, brushDeleteKeepReason); err != nil {
				fail(bt, err.Error())
				continue
			}
			resp.Kept++
		}
	}
	if len(resp.Failed) > 0 {
		return errBrushKeepIncomplete
	}
	return repo.DeleteTask(id)
}

// brushKeepConns 按设置里的编号取各台下载器的实例；已经删掉、没启用或者连不上的记下原因。读不出下载器设置时返回错误。
func (s *Server) brushKeepConns(ctx context.Context, ids []uint) (map[uint]brushKeepConn, error) {
	out := make(map[uint]brushKeepConn, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var settings []models.DownloaderSetting
	if err := global.GlobalDB.DB.Where("id IN ?", ids).Find(&settings).Error; err != nil {
		return nil, fmt.Errorf("读取下载器失败: %w", err)
	}
	byID := make(map[uint]models.DownloaderSetting, len(settings))
	for _, ds := range settings {
		byID[ds.ID] = ds
	}
	dm := s.getDownloaderManager()
	for _, id := range ids {
		ds, ok := byID[id]
		switch {
		case !ok:
			out[id] = brushKeepConn{err: fmt.Errorf("下载器 %d 已经不存在", id)}
		case !ds.Enabled:
			out[id] = brushKeepConn{name: ds.Name, err: fmt.Errorf("下载器 %s 未启用", ds.Name)}
		case dm == nil:
			out[id] = brushKeepConn{name: ds.Name, err: errors.New("下载器管理器未初始化")}
		default:
			dl, err := acquireDownloader(ctx, dm, ds.Name)
			if err != nil {
				err = fmt.Errorf("下载器不可用: %w", err)
			}
			out[id] = brushKeepConn{dl: dl, name: ds.Name, err: err}
		}
	}
	return out, nil
}
