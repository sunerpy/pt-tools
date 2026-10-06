package transmission

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

var (
	_ downloader.TrackerEditor     = (*TransmissionClient)(nil)
	_ downloader.TrackerReader     = (*TransmissionClient)(nil)
	_ downloader.BulkTrackerReader = (*TransmissionClient)(nil)
)

// GetTorrentTrackersContext 读取一个种子的 tracker 列表；请求受 ctx 约束。
func (t *TransmissionClient) GetTorrentTrackersContext(ctx context.Context, id string) ([]downloader.TorrentTracker, error) {
	resp, err := t.doRequestContext(ctx, "torrent-get", torrentGetArgs{
		IDs:    normalizeTransmissionIDs([]string{id}),
		Fields: []string{"id", "trackerStats"},
	})
	if err != nil {
		return nil, fmt.Errorf("读取种子 tracker 失败: %w", err)
	}
	var got struct {
		Torrents []struct {
			TrackerStats []transmissionTrackerStat `json:"trackerStats"`
		} `json:"torrents"`
	}
	if err := json.Unmarshal(resp.Arguments, &got); err != nil {
		return nil, fmt.Errorf("解析种子 tracker 失败: %w", err)
	}
	if len(got.Torrents) == 0 {
		return nil, fmt.Errorf("%w：%s", downloader.ErrTorrentNotFound, id)
	}
	return mapTrackerStats(got.Torrents[0].TrackerStats), nil
}

// GetAllTorrentTrackers 一次 torrent-get 读出全部种子的 tracker 状态（键是小写 info hash）。
func (t *TransmissionClient) GetAllTorrentTrackers(ctx context.Context) (map[string][]downloader.TorrentTracker, error) {
	resp, err := t.doRequestContext(ctx, "torrent-get", torrentGetArgs{Fields: []string{"id", "hashString", "trackerStats"}})
	if err != nil {
		return nil, fmt.Errorf("读取全部种子的 tracker 失败: %w", err)
	}
	var got struct {
		Torrents []struct {
			HashString   string                    `json:"hashString"`
			TrackerStats []transmissionTrackerStat `json:"trackerStats"`
		} `json:"torrents"`
	}
	if err := json.Unmarshal(resp.Arguments, &got); err != nil {
		return nil, fmt.Errorf("解析种子 tracker 失败: %w", err)
	}
	out := make(map[string][]downloader.TorrentTracker, len(got.Torrents))
	for _, tor := range got.Torrents {
		out[strings.ToLower(tor.HashString)] = mapTrackerStats(tor.TrackerStats)
	}
	return out, nil
}

// EditTracker 把种子的 oldURL 改成 newURL。Transmission 4.0（RPC 17）起 torrent-get 带 trackerList，
// 整份列表替换（分层的空行保持不变）；更早的版本没有 trackerList，按 tracker id 用 trackerReplace。
func (t *TransmissionClient) EditTracker(ctx context.Context, hash, oldURL, newURL string) error {
	hash, oldURL, newURL = strings.TrimSpace(hash), strings.TrimSpace(oldURL), strings.TrimSpace(newURL)
	if hash == "" || oldURL == "" || newURL == "" {
		return fmt.Errorf("种子 hash、原地址和新地址都不能为空")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	resp, err := t.doRequestContext(ctx, "torrent-get", torrentGetArgs{
		IDs:    normalizeTransmissionIDs([]string{hash}),
		Fields: []string{"id", "hashString", "trackers", "trackerList"},
	})
	if err != nil {
		return fmt.Errorf("读取种子 tracker 失败: %w", err)
	}
	var got struct {
		Torrents []struct {
			ID          int     `json:"id"`
			TrackerList *string `json:"trackerList"`
			Trackers    []struct {
				ID       int    `json:"id"`
				Announce string `json:"announce"`
			} `json:"trackers"`
		} `json:"torrents"`
	}
	if err := json.Unmarshal(resp.Arguments, &got); err != nil {
		return fmt.Errorf("解析种子 tracker 失败: %w", err)
	}
	if len(got.Torrents) == 0 {
		return fmt.Errorf("%w：%s", downloader.ErrTorrentNotFound, hash)
	}
	tor := got.Torrents[0]
	if err := ctx.Err(); err != nil {
		return err
	}

	args := map[string]any{"ids": []int{tor.ID}}
	if tor.TrackerList != nil {
		lines := strings.Split(strings.ReplaceAll(*tor.TrackerList, "\r\n", "\n"), "\n")
		found := false
		for i, line := range lines {
			if strings.TrimSpace(line) == oldURL {
				lines[i] = newURL
				found = true
			}
		}
		if !found {
			return downloader.ErrTrackerNotFound
		}
		args["trackerList"] = strings.TrimRight(strings.Join(lines, "\n"), "\n")
	} else {
		trackerID := -1
		for _, tr := range tor.Trackers {
			if strings.TrimSpace(tr.Announce) == oldURL {
				trackerID = tr.ID
				break
			}
		}
		if trackerID < 0 {
			return downloader.ErrTrackerNotFound
		}
		args["trackerReplace"] = []any{trackerID, newURL}
	}
	if _, err := t.doRequestContext(ctx, "torrent-set", args); err != nil {
		return fmt.Errorf("修改 tracker 失败: %w", err)
	}
	return nil
}
