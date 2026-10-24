package subscribe

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/internal/reseed"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 一个种子里有几集（整季包、多集）时只下缺的集：库里已有、在别处下载、不追的集在下载器里设成不下载
// （2026-10-25 用户：「我只要后四集 却下载了一整个合集」）。整理时下载器里没选的文件本来就跳过。
// 洗版要整个包（按每一集比版本），不走这里。

// noWantedPrefix 是种子里没有缺的集、没有推送时的结果前缀（不算推送失败）。
const noWantedPrefix = "没有下载 "

// fileListWait 是推送后等下载器列出种子文件的最长时间（qBittorrent 加种子是异步的）。
var fileListWait = 15 * time.Second

// filePlan 是按种子文件算出的：视频里有哪些集、哪些是要的、哪些不要（那个文件里没有一集是要的），以及对不出集的视频个数。
type filePlan struct {
	mp       *episodeMapper
	all      []int
	wanted   []int
	unwanted []int
	unmapped int
}

// none 是种子里的视频都对得出集、却没有一集是要的。
func (p *filePlan) none() bool {
	return len(p.all) > 0 && len(p.wanted) == 0 && p.unmapped == 0
}

// fileMapper 是按订阅的集号规则把文件名里的集号对到 TMDB 的集；读不到这一季时为 nil（按文件名里的集号）。
func (s *Service) fileMapper(ctx context.Context, sub *models.MediaSubscription) *episodeMapper {
	if season, err := s.seasonFor(ctx, sub, false); err == nil {
		return s.mapperFor(ctx, sub, season)
	}
	return nil
}

// planFiles 按种子文件算这次要下的集；单文件的种子没得选，返回 nil。
func (s *Service) planFiles(ctx context.Context, sub *models.MediaSubscription, data []byte, want map[int]bool) *filePlan {
	files, err := reseed.TorrentFiles(data)
	if err != nil || len(files) < 2 {
		return nil
	}
	p := &filePlan{mp: s.fileMapper(ctx, sub)}
	all, wanted, unwanted := map[int]bool{}, map[int]bool{}, map[int]bool{}
	for _, f := range files {
		eps, video := s.videoEpisodes(ctx, p.mp, f.Path)
		if !video {
			continue
		}
		if len(eps) == 0 {
			p.unmapped++
			continue
		}
		hit := false
		for _, n := range eps {
			all[n] = true
			if want[n] {
				wanted[n], hit = true, true
			}
		}
		if !hit {
			for _, n := range eps {
				unwanted[n] = true
			}
		}
	}
	p.all, p.wanted, p.unwanted = slices.Sorted(maps.Keys(all)), slices.Sorted(maps.Keys(wanted)), slices.Sorted(maps.Keys(unwanted))
	return p
}

// videoEpisodes 是一个视频文件对应的集（按文件名解析，含集号映射）。video 为假是不是视频；是视频而对不出集时 eps 为空。
func (s *Service) videoEpisodes(ctx context.Context, mp *episodeMapper, p string) (eps []int, video bool) {
	if !transfer.IsVideo(p) {
		return nil, false
	}
	name := path.Base(p)
	m := s.parse(ctx, name, "")
	sp := span{Start: m.Episode, End: max(m.EpisodeEnd, m.Episode), Whole: m.Episode == 0}
	if mp != nil {
		mapped, ok, _ := mp.mapEpisodes(m, name, "", sp)
		if !ok {
			return nil, true
		}
		sp = mapped
	}
	if sp.Whole || sp.Start <= 0 {
		return nil, true
	}
	for n := sp.Start; n <= max(sp.End, sp.Start); n++ {
		eps = append(eps, n)
	}
	return eps, true
}

// selectFiles 在下载器里把种子中没有一集是要的视频文件设成不下载，返回实际要下的集与设成不下载的文件个数。
// 下载器不支持时返回 downloader.ErrCapabilityUnsupported；下载器的文件列表里对不出要的集时不动它（整个种子照常下）。
func (s *Service) selectFiles(ctx context.Context, dlID uint, hash string, mp *episodeMapper, want map[int]bool) ([]int, int, error) {
	if s.cfg.Downloaders == nil {
		return nil, 0, downloader.ErrCapabilityUnsupported
	}
	dl, _, err := s.cfg.Downloaders.Get(ctx, dlID)
	if err != nil {
		return nil, 0, err
	}
	fs, ok := dl.(downloader.FileSelector)
	if !ok {
		return nil, 0, downloader.ErrCapabilityUnsupported
	}
	files, err := waitFiles(ctx, dl, hash)
	if err != nil {
		return nil, 0, err
	}
	kept := map[int]bool{}
	var off []int
	for _, f := range files {
		eps, _ := s.videoEpisodes(ctx, mp, f.Name)
		if len(eps) == 0 {
			continue
		}
		hit := false
		for _, n := range eps {
			if want[n] {
				kept[n], hit = true, true
			}
		}
		if !hit && f.Priority != 0 {
			off = append(off, f.Index)
		}
	}
	if len(kept) == 0 {
		return nil, 0, errors.New("下载器的文件列表里对不出缺的集")
	}
	if len(off) > 0 {
		if err := fs.SetFilePriority(ctx, hash, off, 0); err != nil {
			return nil, 0, err
		}
	}
	return slices.Sorted(maps.Keys(kept)), len(off), nil
}

// waitFiles 读下载器里种子的文件列表：刚推送的种子可能还没加进去，有限次重试。
func waitFiles(ctx context.Context, dl downloader.Downloader, hash string) ([]downloader.TorrentFile, error) {
	deadline := time.Now().Add(fileListWait)
	delay := 200 * time.Millisecond
	for {
		files, err := dl.GetTorrentFiles(hash)
		if err == nil && len(files) > 0 {
			return files, nil
		}
		left := time.Until(deadline)
		if left <= 0 {
			if err == nil {
				err = errors.New("下载器里还没有这个种子的文件列表")
			}
			return nil, err
		}
		t := time.NewTimer(min(delay, left))
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		case <-t.C:
		}
		delay = min(delay*2, 2*time.Second)
	}
}

// episodesText 是通知里写的集：连着的写成「第 3–4 集」，不连着的写成「第 3、5 集」。
func episodesText(eps []int) string {
	switch {
	case len(eps) == 1:
		return fmt.Sprintf("第 %d 集", eps[0])
	case eps[len(eps)-1]-eps[0]+1 == len(eps):
		return fmt.Sprintf("第 %d–%d 集", eps[0], eps[len(eps)-1])
	}
	parts := make([]string, 0, len(eps))
	for _, n := range eps {
		parts = append(parts, strconv.Itoa(n))
	}
	return "第 " + strings.Join(parts, "、") + " 集"
}
