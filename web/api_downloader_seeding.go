package web

import (
	"context"
	"fmt"
	"net/http"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 用户统计页的「下载器做种」卡：每台启用的下载器现在正在做种的种子数与体积。
//
// 体积是种子体积相加：辅种和原种子共用一份数据，这里各算一次，和站点上报的做种体积是同一个口径
// （站点只看自己的种子），所以几个站的做种体积加起来和这里的合计能对上，比硬盘上占的多。

// DownloaderSeedingItem 是一台下载器的做种情况。
type DownloaderSeedingItem struct {
	DownloaderID   uint   `json:"downloader_id"`
	DownloaderName string `json:"downloader_name"`
	DownloaderType string `json:"downloader_type"`
	// Reachable 为假时这一台没读到种子列表，下面的数都是 0，原因在 Error
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
	// SeedingCount、SeedingSize 是正在做种的种子：状态是做种（qBittorrent 的 uploading、stalledUP、forcedUP，
	// Transmission 的 seed）
	SeedingCount int   `json:"seeding_count"`
	SeedingSize  int64 `json:"seeding_size"`
	// IdleCount、IdleSize 是下载完了、没在做种的种子（暂停、停止、排队、校验中、出错）
	IdleCount int   `json:"idle_count"`
	IdleSize  int64 `json:"idle_size"`
}

// DownloaderSeedingResponse 是各启用下载器的做种情况与合计（合计只算读到了的那几台）。
type DownloaderSeedingResponse struct {
	TotalSeedingCount int                     `json:"total_seeding_count"`
	TotalSeedingSize  int64                   `json:"total_seeding_size"`
	Downloaders       []DownloaderSeedingItem `json:"downloaders"`
}

// apiDownloaderSeedingStats 是 GET /api/downloader-torrents/seeding-stats。
func (s *Server) apiDownloaderSeedingStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	records, err := s.listEnabledDownloaderRecords(nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dm := s.getDownloaderManager()
	if dm == nil {
		http.Error(w, "下载器管理器未初始化", http.StatusInternalServerError)
		return
	}
	writeJSON(w, collectSeedingStats(r.Context(), dm, records))
}

// collectSeedingStats 读这些下载器的种子列表、按状态数做种的种子，几台同时读（同 transferProbeParallel）。
// 取不到实例或读不到列表的那台照样列出，Reachable 为假、写明原因；ctx 结束时没回来的那几台按没读到算
// （它们的请求有各自的超时）。顺序和 records 一样。
func collectSeedingStats(ctx context.Context, dm *downloader.DownloaderManager, records []downloaderRecord) DownloaderSeedingResponse {
	type result struct {
		i    int
		item DownloaderSeedingItem
	}
	results := make(chan result, len(records))
	sem := make(chan struct{}, transferProbeParallel)
	for i, rec := range records {
		go func() {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results <- result{i: i, item: seedingTimedOut(rec, ctx.Err())}
				return
			}
			defer func() { <-sem }()
			results <- result{i: i, item: seedingOf(ctx, dm, rec)}
		}()
	}
	items := make([]DownloaderSeedingItem, len(records))
	done := make([]bool, len(records))
collect:
	for range records {
		select {
		case res := <-results:
			items[res.i], done[res.i] = res.item, true
		case <-ctx.Done():
			break collect
		}
	}
	resp := DownloaderSeedingResponse{Downloaders: make([]DownloaderSeedingItem, 0, len(records))}
	for i, rec := range records {
		item := items[i]
		if !done[i] {
			item = seedingTimedOut(rec, ctx.Err())
		}
		resp.TotalSeedingCount += item.SeedingCount
		resp.TotalSeedingSize += item.SeedingSize
		resp.Downloaders = append(resp.Downloaders, item)
	}
	return resp
}

// seedingOf 读一台下载器的种子列表，数正在做种的与下载完了没在做种的。
func seedingOf(ctx context.Context, dm *downloader.DownloaderManager, rec downloaderRecord) DownloaderSeedingItem {
	dl, err := acquireDownloader(ctx, dm, rec.Name)
	if err != nil {
		return unreachableSeeding(rec, err)
	}
	torrents, err := dl.GetAllTorrents()
	if err != nil {
		return unreachableSeeding(rec, err)
	}
	item := DownloaderSeedingItem{DownloaderID: rec.ID, DownloaderName: rec.Name, DownloaderType: rec.Type, Reachable: true}
	for _, t := range torrents {
		switch {
		case t.State == downloader.TorrentSeeding:
			item.SeedingCount++
			item.SeedingSize += t.TotalSize
		case t.IsCompleted || t.Progress >= 1:
			item.IdleCount++
			item.IdleSize += t.TotalSize
		}
	}
	return item
}

// seedingTimedOut 是请求结束（App API 的时限到了，或网页那边断开）时还没读完的那一台。
func seedingTimedOut(rec downloaderRecord, err error) DownloaderSeedingItem {
	return unreachableSeeding(rec, fmt.Errorf("没有在时限内读到种子列表: %w", err))
}

func unreachableSeeding(rec downloaderRecord, err error) DownloaderSeedingItem {
	item := DownloaderSeedingItem{DownloaderID: rec.ID, DownloaderName: rec.Name, DownloaderType: rec.Type}
	if err != nil {
		item.Error = err.Error()
	}
	return item
}
