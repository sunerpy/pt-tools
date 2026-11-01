package web

import (
	"fmt"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/global"
	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// keepHandOverReason 是点「保留」时交还刷流记录写的原因。
const keepHandOverReason = "在种子列表点了「保留」，交还给用户"

// keepTorrents 给一台下载器里的一批种子加上（keep 为真）或去掉保留标签（pt-tools-keep，见 models.KeepTag），整批在删除锁里做
// （见 internal.RemovalMutex）：保留先完成时，之后的自动删除一定看得到标签；自动删除先完成时，这里发现种子已经不在了，报失败。
// 加保留时同时交还刷流任务正在管的这个种子（刷流记录记成 gone，终态）：刷流不再管它，以后取消保留也不回到刷流，按自动删种的规则处理。
func keepTorrents(dl downloader.Downloader, rec downloaderRecord, targets []TorrentActionTarget, keep bool, resp *BatchTorrentActionResponse) {
	mu := ptinternal.RemovalMutex()
	mu.Lock()
	defer mu.Unlock()

	result := func(target TorrentActionTarget, err error) {
		r := BatchTorrentActionResult{DownloaderID: target.DownloaderID, DownloaderName: rec.Name, TaskID: target.TaskID, Success: err == nil}
		if err != nil {
			r.Message = err.Error()
			resp.FailedCount++
		} else {
			resp.SuccessCount++
		}
		resp.Results = append(resp.Results, r)
	}
	all, err := dl.GetAllTorrents()
	if err != nil {
		for _, target := range targets {
			result(target, fmt.Errorf("读取种子列表失败: %w", err))
		}
		return
	}
	labels := dl.GetType() == downloader.DownloaderTransmission
	repo := models.NewBrushRepository(global.GlobalDB.DB)
	for _, target := range targets {
		t, ok := findTorrent(all, target.TaskID)
		switch {
		case !ok:
			result(target, fmt.Errorf("种子不在下载器里了"))
		case keep:
			result(target, keepOne(dl, rec.ID, t, labels, repo))
		default:
			result(target, unkeepOne(dl, t, labels))
		}
	}
}

// findTorrent 按任务编号或 info hash 找种子（qBittorrent 的任务编号就是 hash，Transmission 的是数字编号）。
func findTorrent(all []downloader.Torrent, id string) (downloader.Torrent, bool) {
	for _, t := range all {
		if t.ID == id || (t.InfoHash != "" && strings.EqualFold(t.InfoHash, id)) {
			return t, true
		}
	}
	return downloader.Torrent{}, false
}

// keepOne 给一个种子加上保留标签，再交还刷流记录。交还失败时撤回刚加的标签、报失败；撤回也失败时报部分失败
// （标签留着，刷流下一轮看到标签会交还）。种子本来就带保留标签时不改标签，只补交还。
func keepOne(dl downloader.Downloader, dlID uint, t downloader.Torrent, labels bool, repo *models.BrushRepository) error {
	had := hasKeepTag(t)
	if !had {
		if err := addKeepTag(dl, t, labels); err != nil {
			return fmt.Errorf("加保留标签失败: %w", err)
		}
	}
	if _, err := repo.EndActiveByHash(dlID, t.InfoHash, keepHandOverReason, time.Now()); err != nil {
		if had {
			return fmt.Errorf("刷流记录没改（%v）；刷流下一轮看到保留标签会交还", err)
		}
		if rerr := revertKeepTag(dl, t, labels); rerr != nil {
			return fmt.Errorf("已加保留标签，刷流记录没改（%v）；刷流下一轮看到标签会交还", err)
		}
		return fmt.Errorf("没能从刷流任务交还（%v），已撤回保留", err)
	}
	return nil
}

// unkeepOne 去掉种子的保留标签；没有这个标签时什么也不做。之后按自动删种的规则处理（交还过的刷流种子不回到刷流）。
func unkeepOne(dl downloader.Downloader, t downloader.Torrent, labels bool) error {
	if !hasKeepTag(t) {
		return nil
	}
	if labels {
		return dl.SetTorrentTags(t.ID, strings.Join(withoutKeepTag(t.Tags), ","))
	}
	tr, ok := dl.(downloader.TorrentTagRemover)
	if !ok {
		return downloader.ErrCapabilityUnsupported
	}
	return tr.RemoveTorrentTags([]string{t.ID}, models.KeepTag)
}

// addKeepTag 加保留标签。qBittorrent 的 SetTorrentTags 是 addTags（只新增）；Transmission 只能整份改写 labels，
// 写回原来的 labels 再加上它（分类是第一个 label，不动）。
func addKeepTag(dl downloader.Downloader, t downloader.Torrent, labels bool) error {
	if !labels {
		return dl.SetTorrentTags(t.ID, models.KeepTag)
	}
	return dl.SetTorrentTags(t.ID, strings.Join(append(splitLabelList(t.Tags), models.KeepTag), ","))
}

// revertKeepTag 撤回刚加的保留标签：qBittorrent 去掉这一个标签，Transmission 写回原来的 labels。
func revertKeepTag(dl downloader.Downloader, t downloader.Torrent, labels bool) error {
	if labels {
		return dl.SetTorrentTags(t.ID, strings.Join(splitLabelList(t.Tags), ","))
	}
	tr, ok := dl.(downloader.TorrentTagRemover)
	if !ok {
		return downloader.ErrCapabilityUnsupported
	}
	return tr.RemoveTorrentTags([]string{t.ID}, models.KeepTag)
}

// hasKeepTag 报告种子是不是带保留标签（大小写不敏感）。
func hasKeepTag(t downloader.Torrent) bool {
	for _, tag := range splitLabelList(t.Tags) {
		if strings.EqualFold(tag, models.KeepTag) {
			return true
		}
	}
	return false
}

// withoutKeepTag 是去掉保留标签以后的标签，顺序不变。
func withoutKeepTag(tags string) []string {
	var out []string
	for _, tag := range splitLabelList(tags) {
		if !strings.EqualFold(tag, models.KeepTag) {
			out = append(out, tag)
		}
	}
	return out
}

// splitLabelList 把逗号分隔的标签拆开，去掉空的，顺序不变。
func splitLabelList(s string) []string {
	var out []string
	for tag := range strings.SplitSeq(s, ",") {
		if tag = strings.TrimSpace(tag); tag != "" {
			out = append(out, tag)
		}
	}
	return out
}
