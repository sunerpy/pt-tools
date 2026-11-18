package organize

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

// ErrForbidden 表示要浏览的目录不在能浏览的根目录里（见 FSRoots）。
var ErrForbidden = errors.New("只能浏览媒体库、监控目录、路径映射与下载器的下载目录")

// maxFSEntries 是浏览目录时一层最多列几项。
const maxFSEntries = 2000

// fsRootTimeout 是列根目录时等下载器回默认保存路径的时长：等不到的不列（测试里改短）。
var fsRootTimeout = 3 * time.Second

// 根目录从哪来（FSRoot.Kinds）。
const (
	fsRootLibrary    = "library"
	fsRootWatch      = "watch"
	fsRootPathMap    = "map"
	fsRootDownloader = "downloader"
)

// FSRoot 是「手动整理」浏览文件时能进的一个根目录。
type FSRoot struct {
	Path string `json:"path"`
	// Kinds 是它从哪来：library（媒体库）、watch（监控目录）、map（路径映射的本地路径）、downloader（下载器的下载目录换算成的本地路径）
	Kinds []string `json:"kinds"`
	// Labels 是它的名字（媒体库的名字、下载器的名字与目录别名）
	Labels []string `json:"labels,omitempty"`
}

// FSEntry 是目录里的一项。
type FSEntry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Dir     bool      `json:"dir"`
	Size    int64     `json:"size,omitempty"`
	ModTime time.Time `json:"mod_time"`
}

// FSList 是一层目录的内容：目录在前，按名字排。
type FSList struct {
	Path string `json:"path"`
	// Root 是它所在的根目录；Parent 是上一级（已经是根目录时为空）
	Root      string    `json:"root"`
	Parent    string    `json:"parent,omitempty"`
	Entries   []FSEntry `json:"entries"`
	Truncated bool      `json:"truncated,omitempty"`
}

// FSRoots 是能浏览的根目录：媒体库、监控目录、路径映射的本地路径、下载器的默认保存路径与 pt-tools 里设的下载目录
// （都按路径映射换成本地路径）。只回 pt-tools 里存在的目录，同一个目录合成一项，按路径排。
func (s *Service) FSRoots(ctx context.Context) ([]FSRoot, error) {
	db := s.cfg.DB.WithContext(ctx)
	byPath := map[string]*FSRoot{}
	add := func(p, kind, label string) {
		p = strings.TrimSpace(p)
		if p == "" || !filepath.IsAbs(p) {
			return
		}
		p = filepath.Clean(p)
		r := byPath[p]
		if r == nil {
			r = &FSRoot{Path: p}
			byPath[p] = r
		}
		if !slices.Contains(r.Kinds, kind) {
			r.Kinds = append(r.Kinds, kind)
		}
		if label != "" && !slices.Contains(r.Labels, label) {
			r.Labels = append(r.Labels, label)
		}
	}
	var libs []models.MediaLibrary
	if err := db.Order("id").Find(&libs).Error; err != nil {
		return nil, fmt.Errorf("读取媒体库失败: %w", err)
	}
	for _, l := range libs {
		add(l.Path, fsRootLibrary, l.Name)
	}
	var dirs []models.MediaWatchDir
	if err := db.Order("id").Find(&dirs).Error; err != nil {
		return nil, fmt.Errorf("读取监控目录失败: %w", err)
	}
	for _, d := range dirs {
		add(d.Path, fsRootWatch, "")
	}
	var maps []models.MediaPathMap
	if err := db.Order("id").Find(&maps).Error; err != nil {
		return nil, fmt.Errorf("读取路径映射失败: %w", err)
	}
	for _, m := range maps {
		add(m.LocalPrefix, fsRootPathMap, "")
	}
	settings, err := s.cfg.Downloaders.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("读取下载器失败: %w", err)
	}
	names := map[uint]string{}
	for _, d := range settings {
		names[d.ID] = d.Name
	}
	for id, p := range s.downloaderSavePaths(ctx, settings) {
		local, _, err := s.localPath(ctx, id, p)
		if err != nil {
			return nil, err
		}
		add(local, fsRootDownloader, names[id]+" 默认")
	}
	var dlDirs []models.DownloaderDirectory
	if err := db.Order("downloader_id, id").Find(&dlDirs).Error; err != nil {
		return nil, fmt.Errorf("读取下载器的下载目录失败: %w", err)
	}
	for _, d := range dlDirs {
		name, ok := names[d.DownloaderID]
		if !ok {
			continue
		}
		local, _, err := s.localPath(ctx, d.DownloaderID, d.Path)
		if err != nil {
			return nil, err
		}
		add(local, fsRootDownloader, strings.TrimSpace(strings.Join([]string{name, d.Alias}, " ")))
	}
	out := make([]FSRoot, 0, len(byPath))
	for _, r := range byPath {
		if st, err := os.Stat(r.Path); err == nil && st.IsDir() {
			out = append(out, *r)
		}
	}
	slices.SortFunc(out, func(a, b FSRoot) int { return strings.Compare(a.Path, b.Path) })
	return out, nil
}

// downloaderSavePaths 是启用的下载器各自的默认保存路径（下载器里看到的），几台一起问，最多等 fsRootTimeout；
// 连不上、出错或等不到的不列。
func (s *Service) downloaderSavePaths(ctx context.Context, settings []models.DownloaderSetting) map[uint]string {
	type reply struct {
		id   uint
		path string
	}
	ch := make(chan reply, len(settings))
	n := 0
	for _, d := range settings {
		if !d.Enabled {
			continue
		}
		n++
		go func(id uint) {
			var p string
			if dl, _, err := s.cfg.Downloaders.Get(ctx, id); err == nil && dl != nil {
				if info, err := dl.GetDiskInfo(); err == nil {
					p = strings.TrimSpace(info.Path)
				}
			}
			ch <- reply{id, p}
		}(d.ID)
	}
	out := map[uint]string{}
	timer := time.NewTimer(fsRootTimeout)
	defer timer.Stop()
	for range n {
		select {
		case r := <-ch:
			if r.path != "" {
				out[r.id] = r.path
			}
		case <-timer.C:
			return out
		case <-ctx.Done():
			return out
		}
	}
	return out
}

// FSList 列出一个目录的一层（隐藏的、NAS 的系统目录与没下载完的文件不列；目录在前，按名字排，最多 maxFSEntries 项）。
// 目录解开软链接以后必须在某个根目录里（或就是根目录），否则返回 ErrForbidden；不是目录时 ErrInvalid，不存在时 ErrNotFound。
// 软链接按指向的算，指向根目录以外的不列。
func (s *Service) FSList(ctx context.Context, dir string) (FSList, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" || !filepath.IsAbs(dir) {
		return FSList{}, fmt.Errorf("%w: 要写绝对路径", ErrInvalid)
	}
	dir = filepath.Clean(dir)
	roots, err := s.FSRoots(ctx)
	if err != nil {
		return FSList{}, err
	}
	real, err := filepath.EvalSymlinks(dir)
	if errors.Is(err, os.ErrNotExist) {
		return FSList{}, fmt.Errorf("%w: %s 不存在", ErrNotFound, dir)
	}
	if err != nil {
		return FSList{}, fmt.Errorf("读取 %s 失败: %w", dir, err)
	}
	reals := make([]string, len(roots))
	for i, r := range roots {
		reals[i], _ = filepath.EvalSymlinks(r.Path)
	}
	// 套着的根目录（库在映射的目录里）取最近的一个
	at := -1
	for i, rr := range reals {
		if rr != "" && inOrAt(rr, real) && (at < 0 || len(rr) > len(reals[at])) {
			at = i
		}
	}
	if at < 0 {
		return FSList{}, ErrForbidden
	}
	if !isDir(real) {
		return FSList{}, fmt.Errorf("%w: %s 不是目录", ErrInvalid, dir)
	}
	entries, err := os.ReadDir(real)
	if err != nil {
		return FSList{}, fmt.Errorf("读取 %s 失败: %w", dir, err)
	}
	out := FSList{Path: dir, Root: roots[at].Path, Entries: []FSEntry{}}
	if real != reals[at] {
		out.Parent = filepath.Dir(dir)
	}
	inRoots := func(p string) bool {
		return slices.ContainsFunc(reals, func(rr string) bool { return rr != "" && inOrAt(rr, p) })
	}
	for _, e := range entries {
		if skipWatchEntry(e.Name()) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(p)
			if err != nil || !inRoots(target) {
				continue
			}
			if info, err = os.Stat(target); err != nil {
				continue
			}
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			continue
		}
		ent := FSEntry{Name: e.Name(), Path: p, Dir: info.IsDir(), ModTime: info.ModTime()}
		if !ent.Dir {
			ent.Size = info.Size()
		}
		out.Entries = append(out.Entries, ent)
	}
	slices.SortFunc(out.Entries, func(a, b FSEntry) int {
		if a.Dir != b.Dir {
			if a.Dir {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	if len(out.Entries) > maxFSEntries {
		out.Entries, out.Truncated = out.Entries[:maxFSEntries], true
	}
	return out, nil
}

// inOrAt 报告 p 是不是 root 或在 root 里面。
func inOrAt(root, p string) bool {
	return filepath.Clean(root) == filepath.Clean(p) || transfer.Within(root, p)
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
