package organize

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
)

// nfoMaxBytes 是读 NFO 的上限：再大的不是正常的 NFO。
const nfoMaxBytes = 1 << 20

// nfoIDs 是 NFO 或目录名里写的条目编号。
type nfoIDs struct {
	TMDB int
	IMDb string
}

var (
	imdbIDRe = regexp.MustCompile(`^tt\d{5,10}$`)
	// nfoURLRe 与 nfoIMDbURLRe 是只写了网址的 NFO（Kodi 认这种）里的 TMDB 与 IMDb 网址
	nfoURLRe     = regexp.MustCompile(`themoviedb\.org/(movie|tv)/(\d{1,9})`)
	nfoIMDbURLRe = regexp.MustCompile(`imdb\.com/title/(tt\d{5,10})`)
	// dirTMDBRe 与 dirIMDbRe 是目录名里写的编号：Emby、Jellyfin 认的 [tmdbid=123]、[tmdbid-123]、[imdbid-tt123]，Plex 认的 {tmdb-123}
	dirTMDBRe = regexp.MustCompile(`(?i)[\[{(]?\btmdb(?:id)?[=\-_ :]?(\d{1,9})\b[\]})]?`)
	dirIMDbRe = regexp.MustCompile(`(?i)[\[{(]?\bimdb(?:id)?[=\-_ :]?(tt\d{5,10})\b[\]})]?`)
)

// readNFO 读 NFO 里这个条目的编号：Kodi、Emby、Jellyfin 与刮削工具写的 <uniqueid type="tmdb|imdb">、<tmdbid>、<imdbid>、
// <imdb_id>、<id>tt…</id>。只看根元素下一层（演员、合集里的编号不算），根元素要是 kind 对应的 movie 或 tvshow
// （剧集一集的 NFO 里是那一集的编号）。只写了网址的 NFO 看 themoviedb.org 与 imdb.com 的网址。读不了或没有时为零值。
func readNFO(path, kind string) nfoIDs {
	f, err := os.Open(path)
	if err != nil {
		return nfoIDs{}
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, nfoMaxBytes))
	if err != nil {
		return nfoIDs{}
	}
	want := "movie"
	if kind == tmdb.KindTV {
		want = "tvshow"
	}
	var ids nfoIDs
	d := xml.NewDecoder(bytes.NewReader(raw))
	d.Strict = false
	// 编号都是 ASCII：声明成 GBK 一类编码的 NFO 也照原样读
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	depth := 0
	var root, elem, typ string
	var text strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch depth {
			case 1:
				root = strings.ToLower(t.Name.Local)
			case 2:
				elem, typ = strings.ToLower(t.Name.Local), ""
				for _, a := range t.Attr {
					if strings.EqualFold(a.Name.Local, "type") {
						typ = strings.ToLower(strings.TrimSpace(a.Value))
					}
				}
				text.Reset()
			}
		case xml.CharData:
			if depth == 2 {
				text.Write(t)
			}
		case xml.EndElement:
			if depth == 2 && root == want {
				ids.take(elem, typ, strings.TrimSpace(text.String()))
			}
			depth--
		}
	}
	if ids.TMDB == 0 && ids.IMDb == "" && (root == "" || root == want) {
		if mm := nfoURLRe.FindSubmatch(raw); mm != nil && string(mm[1]) == kind {
			ids.TMDB, _ = strconv.Atoi(string(mm[2]))
		}
		if mm := nfoIMDbURLRe.FindSubmatch(raw); mm != nil {
			ids.IMDb = string(mm[1])
		}
	}
	return ids
}

// take 记下 NFO 里一个元素写的编号（同一种编号只取第一个）。
func (ids *nfoIDs) take(elem, typ, v string) {
	switch {
	case elem == "uniqueid" && typ == "tmdb", elem == "tmdbid", elem == "tmdb_id":
		if n, err := strconv.Atoi(v); err == nil && n > 0 && ids.TMDB == 0 {
			ids.TMDB = n
		}
	case elem == "uniqueid" && typ == "imdb", elem == "imdbid", elem == "imdb_id", elem == "id":
		if imdbIDRe.MatchString(v) && ids.IMDb == "" {
			ids.IMDb = v
		}
	}
}

// dirTags 是目录名（或文件名）里写的编号。
func dirTags(name string) nfoIDs {
	var ids nfoIDs
	if mm := dirTMDBRe.FindStringSubmatch(name); mm != nil {
		ids.TMDB, _ = strconv.Atoi(mm[1])
	}
	if mm := dirIMDbRe.FindStringSubmatch(name); mm != nil {
		ids.IMDb = strings.ToLower(mm[1])
	}
	return ids
}

// stripTags 去掉名字里写的编号（识别时它们不是片名的一部分）。
func stripTags(name string) string {
	name = dirIMDbRe.ReplaceAllString(dirTMDBRe.ReplaceAllString(name, " "), " ")
	return strings.Join(strings.Fields(name), " ")
}
