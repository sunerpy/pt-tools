package scrape

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/sunerpy/pt-tools/internal/media/metadata"
)

// 一个文件的结果。
const (
	// FileWritten 是写了（新写的，或内容变了）
	FileWritten = "written"
	// FileUnchanged 是已经有、内容和这次的一样，没有重写
	FileUnchanged = "unchanged"
	// FileKept 是已经有、不覆盖，没有动
	FileKept = "kept"
	// FileLocked 是媒体服务器里锁定了的 NFO（lockdata），没有动
	FileLocked = "locked"
	// FileMissing 是来源里没有这张图，没写
	FileMissing = "missing"
	// FileFailed 是写失败了（Err 是原因）
	FileFailed = "failed"
)

// FileResult 是一个文件的结果。SHA256 是写了的内容的摘要（Status 为 FileWritten 时才有）：记下来，以后能认出文件
// 是不是还是 pt-tools 写的那份（见 Writer.Replace）。
type FileResult struct {
	File
	Status string
	Err    string
	SHA256 string

	// pre 是决定写这个文件时它的样子：写之前再看一次，变了（期间有人改了、新放了或锁定了）就不换
	pre fileState
}

// fileState 是一个文件在某一刻的样子：有没有、内容的摘要。
type fileState struct {
	exists bool
	sum    string
}

// stateOf 读文件现在的样子。
func stateOf(path string) (fileState, error) {
	sum, err := FileSHA256(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fileState{}, nil
	}
	if err != nil {
		return fileState{}, err
	}
	return fileState{exists: true, sum: sum}, nil
}

// Report 是一次刮削的结果：每个文件的结果，取数据时的说明（Notes：用 TMDB 代替的、没填 Key 一类）与来源出错（Errors）。
type Report struct {
	Files  []FileResult
	Notes  []string
	Errors []string
	// Sources 是 NFO 里文字的每一项用了哪个来源（例如「简介 豆瓣，评分 豆瓣」），这次没写 NFO 时为空
	Sources string
	// Fetched 表示这次去取了数据（有要写的文件）
	Fetched bool
}

// Written 是这次写了的文件。
func (r Report) Written() []string {
	var out []string
	for _, f := range r.Files {
		if f.Status == FileWritten {
			out = append(out, f.Path)
		}
	}
	return out
}

// Problems 是出的问题：来源出错与说明在前，写失败的文件（「文件名: 原因」）在后。
func (r Report) Problems() []string {
	out := append(append([]string(nil), r.Errors...), r.Notes...)
	for _, f := range r.Files {
		if f.Status == FileFailed {
			out = append(out, fmt.Sprintf("%s: %s", filepath.Base(f.Path), f.Err))
		}
	}
	return out
}

// Writer 写 NFO 与图片。
type Writer struct {
	// Resolver 取数据（带上能用的来源，见 metadata.NewResolver）
	Resolver *metadata.Resolver
	// Sources 是媒体库的数据来源
	Sources metadata.Sources
	// Overwrite 为假时已经存在的文件不动（Replace 认可的除外）
	Overwrite bool
	// Replace 报告一个已经存在的文件能不能换掉（重新刮削时：pt-tools 写的、之后没被换过的文件）；为空时只看 Overwrite
	Replace func(path string) bool
	// ImageLang 是海报与 Logo 优先的语言（见 ImageLangs），RecogLang 是识别语言（例如 zh-CN）
	ImageLang, RecogLang string
}

// imageWorkers 是同时下载的图片数。
const imageWorkers = 3

// artLabels 是图片在说明里的名字（和界面上的一样）。
var artLabels = map[string]string{
	metadata.ArtPoster: "海报", metadata.ArtFanart: "背景图", metadata.ArtLogo: "Logo", metadata.ArtClearart: "Clearart",
	metadata.ArtBanner: "横幅", metadata.ArtLandscape: "横版海报", metadata.ArtDisc: "碟片图",
	metadata.ArtSeasonPoster: "季海报", metadata.ArtThumb: "集截图",
}

// ArtLabel 是这种图片在说明里的名字。
func ArtLabel(kind string) string {
	if l, ok := artLabels[kind]; ok {
		return l
	}
	return kind
}

// Join 把两段说明接起来：英文与中文相邻时中间空一格（「来源里没有 Logo」「Clearart 只有 Fanart.tv 有」）。
func Join(a, b string) string {
	l, _ := utf8.DecodeLastRuneInString(a)
	r, _ := utf8.DecodeRuneInString(b)
	latin := func(c rune) bool { return c < utf8.RuneSelf && (unicode.IsLetter(c) || unicode.IsDigit(c)) }
	cjk := func(c rune) bool { return c >= utf8.RuneSelf && unicode.IsLetter(c) }
	if (latin(l) && cjk(r)) || (cjk(l) && latin(r)) {
		return a + " " + b
	}
	return a + b
}

// NoFanartKey 是只有 Fanart.tv 有、却没填它的 Key 而没写的图片的 Err（Report.Notes 里已经写明）。
const NoFanartKey = "没填 Fanart.tv 的 API Key"

// Write 写一部电影或剧集的这些文件：先看哪些要写（不存在的；已经有的要开着覆盖或 Replace 认可，媒体服务器里锁定了的
// NFO 不动），只为要写的去取数据，再写 NFO、同时下载几张图片。q 与 base 是识别出的 TMDB 条目（见 metadata.QueryFor 与
// metadata.FromTMDB）。内容和已有的一样时不重写。
func (w Writer) Write(ctx context.Context, q metadata.Query, base *metadata.Item, files []File) Report {
	rep := Report{Files: make([]FileResult, len(files))}
	olds := make([][]root, len(files))
	var todo []int
	for i, f := range files {
		rep.Files[i] = FileResult{File: f}
		_, err := os.Lstat(f.Path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			todo = append(todo, i)
			continue
		case err != nil:
			rep.Files[i].Status, rep.Files[i].Err = FileFailed, err.Error()
			continue
		}
		if !w.Overwrite && (w.Replace == nil || !w.Replace(f.Path)) {
			rep.Files[i].Status = FileKept
			continue
		}
		b, err := os.ReadFile(f.Path)
		if err != nil {
			rep.Files[i].Status, rep.Files[i].Err = FileFailed, err.Error()
			continue
		}
		if f.NFO != "" {
			roots, _ := parseDoc(b)
			if locked(b, roots) {
				rep.Files[i].Status = FileLocked
				continue
			}
			olds[i] = roots
		}
		sum := sha256.Sum256(b)
		rep.Files[i].pre = fileState{exists: true, sum: hex.EncodeToString(sum[:])}
		todo = append(todo, i)
	}
	if len(todo) == 0 {
		return rep
	}

	need := metadata.Need{Art: metadata.ArtRequest{ImageLang: w.ImageLang, RecogLang: w.RecogLang}}
	nfo := false
	for _, i := range todo {
		f := files[i]
		switch f.NFO {
		case NFOMovie, NFOShow:
			need.Item, nfo = true, true
		case NFOSeason:
			need.Seasons, nfo = addOnce(need.Seasons, f.Season), true
		case NFOEpisode:
			need.Episodes, nfo = addOnce(need.Episodes, f.Season), true
		default:
			if !slices.Contains(need.Art.Kinds, f.Art) {
				need.Art.Kinds = append(need.Art.Kinds, f.Art)
			}
			if f.Art == metadata.ArtSeasonPoster || f.Art == metadata.ArtThumb {
				need.Art.Seasons = addOnce(need.Art.Seasons, f.Season)
			}
		}
	}
	resolver := w.Resolver
	if resolver == nil {
		resolver = metadata.NewResolver()
	}
	res := resolver.Resolve(ctx, q, base, w.Sources, need)
	rep.Notes, rep.Errors, rep.Fetched = res.Notes, res.Errors, true
	if nfo {
		var eps []metadata.EpisodeRef
		for _, i := range todo {
			if f := files[i]; f.NFO == NFOEpisode {
				for n := f.Episode; n <= max(f.Episode, f.EpisodeEnd); n++ {
					eps = append(eps, metadata.EpisodeRef{Season: f.Season, Episode: n})
				}
			}
		}
		rep.Sources = res.Summary(eps)
	}

	type task struct {
		i     int
		picks []metadata.Artwork
	}
	var tasks []task
	var noFanart []string
	for _, i := range todo {
		f := files[i]
		r := &rep.Files[i]
		if f.NFO != "" {
			var docs []any
			switch f.NFO {
			case NFOMovie:
				docs = movieDoc(res.Item)
			case NFOShow:
				docs = showDoc(res.Item)
			case NFOSeason:
				docs = seasonDoc(res.Season(f.Season))
			case NFOEpisode:
				docs = episodeDoc(res.Item, res.Season(f.Season), f.Season, f.Episode, f.EpisodeEnd)
			}
			data, err := build(docs, olds[i])
			if err == nil {
				err = r.put(data)
			}
			if err != nil {
				r.Status, r.Err = FileFailed, err.Error()
			}
			continue
		}
		picks := res.Picks(f.Art, f.Season, f.Episode)
		if len(picks) == 0 {
			r.Status = FileMissing
			if metadata.FanartOnly(f.Art) && !resolver.Has(metadata.SourceFanart) {
				r.Err = NoFanartKey
				if label := ArtLabel(f.Art); !slices.Contains(noFanart, label) {
					noFanart = append(noFanart, label)
				}
			}
			continue
		}
		tasks = append(tasks, task{i: i, picks: picks})
	}
	if len(noFanart) > 0 {
		rep.Notes = append(rep.Notes, Join(strings.Join(noFanart, "、"), "只有 Fanart.tv 有：在「媒体识别」里填了 Fanart.tv 的 API Key 才写"))
	}

	sem := make(chan struct{}, imageWorkers)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, t := range tasks {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			r := &rep.Files[t.i]
			// 选的来源下载不了（404、超时、不是图片）时用下一个来源的那张
			var data []byte
			var err, first error
			for k, a := range t.picks {
				if data, err = res.Download(ctx, a); err == nil {
					if k > 0 {
						// 选的来源出错、用了别的来源的：和来源取不到一样算来源出错（记录记成部分刮削）
						mu.Lock()
						rep.Errors = append(rep.Errors, fmt.Sprintf("%s：%s的下载不了（%v），用了 %s 的",
							filepath.Base(r.Path), metadata.SourceLead(t.picks[0].Source), first, metadata.SourceName(a.Source)))
						mu.Unlock()
					}
					break
				}
				if first == nil {
					first = err
				}
			}
			if err == nil {
				err = r.put(data)
			}
			if err != nil {
				r.Status, r.Err = FileFailed, err.Error()
			}
		})
	}
	wg.Wait()
	return rep
}

func addOnce(list []int, n int) []int {
	if slices.Contains(list, n) {
		return list
	}
	return append(list, n)
}

// put 原子地写这个文件：先写同目录的临时文件，改名前再看一次文件现在的样子——和决定写它时（pre）不一样了
// （取数据的这段时间里有人改了、新放了或锁定了）就不换，记成 FileKept；内容和要写的一样时记成 FileUnchanged。
func (r *FileResult) put(data []byte) error {
	sum := sha256.Sum256(data)
	want := hex.EncodeToString(sum[:])
	if err := os.MkdirAll(filepath.Dir(r.Path), 0o755); err != nil {
		return err
	}
	var rnd [6]byte
	_, _ = rand.Read(rnd[:])
	tmp := r.Path + ".pt-tools-" + hex.EncodeToString(rnd[:]) + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	cur, err := stateOf(r.Path)
	switch {
	case err != nil:
		_ = os.Remove(tmp)
		return err
	case cur != r.pre:
		_ = os.Remove(tmp)
		r.Status = FileKept
		return nil
	case cur.exists && cur.sum == want:
		_ = os.Remove(tmp)
		r.Status = FileUnchanged
		return nil
	}
	if err := os.Rename(tmp, r.Path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	r.Status, r.SHA256 = FileWritten, want
	return nil
}

// FileSHA256 是文件内容的摘要（和 FileResult.SHA256 一样算）。
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
