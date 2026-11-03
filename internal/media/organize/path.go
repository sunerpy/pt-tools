package organize

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 手动指定一个路径整理（网页上的「手动整理」）：这个文件或文件夹当作一个种子，和目录监控的一项一样整理。
const (
	// pathSource 是这种整理的记录里「下载器」一栏写的名字
	pathSource = "指定路径"
	// pathHashPrefix 是这种整理的记录的 InfoHash 的前缀（见 pathHash）
	pathHashPrefix = "path:"
	// pathOtherFileNote 写在整理记录的消息前面：这个位置上的文件换过了（见 markOtherFiles）
	pathOtherFileNote = "这个位置上的文件换过了"
	// pathTorrentTimeout 是认下载器里的种子时连下载器最多等多久
	pathTorrentTimeout = 10 * time.Second
	// maxSourceBase 是源路径最后一段最长多少字节：整理记录的 torrent_name 列宽，requestOf 靠它还原路径
	maxSourceBase = 512
)

// cleanSource 检查网页填的源路径：不含 NUL；cleanDir 的规则（绝对路径、不超过 1024 字节、合法 UTF-8、规整后不是根目录）；
// 在这个系统上是绝对路径；最后一段不超过 512 字节（整理记录的 torrent_name 列宽，requestOf 靠它还原路径）。
func cleanSource(raw string) (string, error) {
	if strings.ContainsRune(raw, 0) {
		return "", fmt.Errorf("%w: 源路径里不能有 NUL 字符", ErrInvalid)
	}
	p, err := cleanDir(raw, "源路径")
	if err != nil {
		return "", err
	}
	// cleanDir 也认 Windows 的写法（盘符、\\ 开头）：在别的系统上它们是相对路径
	if runtime.GOOS != "windows" && !filepath.IsAbs(p) {
		return "", fmt.Errorf("%w: 源路径要填绝对路径", ErrInvalid)
	}
	if len(filepath.Base(p)) > maxSourceBase {
		return "", fmt.Errorf("%w: 源路径的最后一段太长（最多 %d 字节）", ErrInvalid, maxSourceBase)
	}
	return p, nil
}

// pathRow 报告整理记录是不是手动指定路径整理的。
func pathRow(r models.MediaTransferHistory) bool {
	return r.DownloaderID == 0 && r.WatchDirID == 0 && strings.HasPrefix(r.InfoHash, pathHashPrefix)
}

// errSourceMissing 是源路径在 pt-tools 里找不到时的错误。
func errSourceMissing(src string) error {
	return fmt.Errorf("%w: pt-tools 里找不到 %s：Docker 里要把它所在的目录挂进容器，这里填容器里的路径", ErrInvalid, src)
}

// buildPathPlan 是手动指定的一个路径（文件或文件夹，req.SourcePath 已经规整过）的整理计划：先看它是不是下载器里的种子——
// 种子的内容就是它时改成按种子整理，和种子部分重叠时不整理；否则当作一个种子，名字用来识别（同目录监控的一项）。
func (s *Service) buildPathPlan(ctx context.Context, req Request) (*Plan, error) {
	src := req.SourcePath
	info, err := os.Lstat(src)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, errSourceMissing(src)
	case err != nil:
		return nil, fmt.Errorf("读取 %s 失败: %w", src, err)
	case info.Mode()&os.ModeSymlink != 0:
		if to, lerr := filepath.EvalSymlinks(src); lerr == nil {
			return nil, fmt.Errorf("%w: %s 是软链接：填它指向的路径（%s）", ErrInvalid, src, to)
		}
		return nil, fmt.Errorf("%w: %s 是软链接：填它指向的路径", ErrInvalid, src)
	case !info.IsDir() && !info.Mode().IsRegular():
		return nil, fmt.Errorf("%w: %s 不是文件也不是文件夹", ErrInvalid, src)
	}
	// 库里的文件不能再整理一遍（移动整理时还会把库里的文件搬走）：启用与否的库都算
	var libs []models.MediaLibrary
	if err = s.cfg.DB.WithContext(ctx).Order("id").Find(&libs).Error; err != nil {
		return nil, fmt.Errorf("读取媒体库失败: %w", err)
	}
	for _, l := range libs {
		if overlaps(l.Path, src) {
			return nil, fmt.Errorf("%w: %s 是媒体库「%s」的目录、在它里面或包含它：库里的文件不能再整理一遍", ErrInvalid, src, l.Name)
		}
	}
	dir, base := filepath.Dir(src), filepath.Base(src)
	p := &Plan{
		SourcePath: src, DownloaderName: pathSource, Hash: pathHash(src), Name: base, SavePath: dir, LocalPath: dir,
		contentSrc: src, Items: []PlanItem{},
	}
	dctx, cancel := context.WithTimeout(ctx, pathTorrentTimeout)
	hit, terr := s.torrentAt(dctx, src, base)
	cancel()
	if hit != nil && !hit.ByName {
		if !hit.Exact {
			p.FailKind = models.MediaFailOther
			p.Problem = fmt.Sprintf("「%s」和下载器「%s」里的种子「%s」的文件重叠（在它里面或包含它），不整理：下载器里的种子在「下载器 Web UI」里选中它点「整理入库」，或在这里填这个种子自己的目录",
				base, hit.DownloaderName, hit.Name)
			p.noRecord = true
			return p, nil
		}
		// 就是下载器里的种子：按种子整理，记录记在种子下（删种时清理入库链接、定期扫描、低空间清理都认得它）
		treq := req
		treq.SourcePath, treq.DownloaderID, treq.Hash = "", hit.DownloaderID, hit.TaskID
		tp, perr := s.buildTorrentPlan(ctx, treq)
		if tp != nil {
			tp.Notes = append(tp.Notes, fmt.Sprintf("「%s」是下载器「%s」里的种子「%s」：按种子整理（整理记录记在这个种子下，删种时清理入库链接、定期扫描都认得它）",
				base, hit.DownloaderName, hit.Name))
		}
		return tp, perr
	}
	// 剩下按名字认出的（pt-tools 看不到那个种子的文件）或确认不了的：移动整理时不整理，别的方式写一条说明（见 finishPlan）
	p.nameHit, p.torrentErr = hit, terr
	files, newest, err := watchFiles(dir, base)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, errSourceMissing(src)
	case errors.Is(err, errWatchTooBig):
		p.FailKind = models.MediaFailOther
		p.Problem, p.noRecord = "「"+base+"」里的"+errWatchTooBig.Error()+"，不整理：选一部电影或一季剧集的文件夹", true
		return p, nil
	case err != nil:
		return nil, fmt.Errorf("读取 %s 失败: %w", src, err)
	}
	if s.cfg.Now().Sub(newest) < watchSettle {
		p.FailKind = models.MediaFailOther
		p.Problem, p.noRecord = fmt.Sprintf("「%s」最近 %d 分钟里还有改动（可能还在复制），等写完再整理", base, int(watchSettle/time.Minute)), true
		return p, nil
	}
	settings, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	// 不是下载器里的种子：没有做种这回事
	t := downloader.Torrent{Name: base, State: downloader.TorrentPaused, Progress: 1}
	return s.finishPlan(ctx, p, req, t, files, settings)
}
