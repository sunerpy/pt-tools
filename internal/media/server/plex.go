package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/models"
)

type plex struct {
	h *httpc
}

type plexSection struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Location []struct {
		Path string `json:"path"`
	} `json:"Location"`
}

func (p *plex) sections(ctx context.Context) ([]plexSection, error) {
	var out struct {
		MediaContainer struct {
			Directory []plexSection `json:"Directory"`
		} `json:"MediaContainer"`
	}
	if err := p.h.do(ctx, http.MethodGet, "/library/sections", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.MediaContainer.Directory, nil
}

func (p *plex) Test(ctx context.Context) (Info, error) {
	var root struct {
		MediaContainer struct {
			FriendlyName      string `json:"friendlyName"`
			Version           string `json:"version"`
			MachineIdentifier string `json:"machineIdentifier"`
		} `json:"MediaContainer"`
	}
	if err := p.h.do(ctx, http.MethodGet, "/", nil, nil, &root); err != nil {
		return Info{}, err
	}
	if root.MediaContainer.Version == "" {
		return Info{}, fmt.Errorf("%w: 没有读到 Plex 的版本（地址是不是填成了别的服务）", ErrUnavailable)
	}
	secs, err := p.sections(ctx)
	if err != nil {
		return Info{}, err
	}
	return Info{Name: root.MediaContainer.FriendlyName, Version: root.MediaContainer.Version, Libraries: len(secs), ID: root.MediaContainer.MachineIdentifier}, nil
}

// RefreshPaths 找到包含每个目录的分区，按路径刷新（只扫这个目录）。
func (p *plex) RefreshPaths(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	secs, err := p.sections(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, path := range paths {
		key := ""
		best := -1
		for _, s := range secs {
			for _, loc := range s.Location {
				if within(loc.Path, path) && len(loc.Path) > best {
					key, best = s.Key, len(loc.Path)
				}
			}
		}
		if key == "" {
			errs = append(errs, fmt.Errorf("%w: %s", ErrNoLibrary, path))
			continue
		}
		if err := p.h.do(ctx, http.MethodGet, "/library/sections/"+url.PathEscape(key)+"/refresh", url.Values{"path": {path}}, nil, nil); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (p *plex) RefreshAll(ctx context.Context) error {
	secs, err := p.sections(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, s := range secs {
		if err := p.h.do(ctx, http.MethodGet, "/library/sections/"+url.PathEscape(s.Key)+"/refresh", nil, nil, nil); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Exists 在电影或剧集分区里按名字找，再比对 Guid 里的 tmdb:// 与 imdb:// 编号（新版 Plex 代理才有）。
func (p *plex) Exists(ctx context.Context, q Query) (bool, error) {
	if strings.TrimSpace(q.Title) == "" {
		return false, errors.New("查询是否入库需要名称")
	}
	want := "movie"
	if q.Kind == "tv" {
		want = "show"
	}
	secs, err := p.sections(ctx)
	if err != nil {
		return false, err
	}
	var ids []string
	if q.TMDBID > 0 {
		ids = append(ids, "tmdb://"+strconv.Itoa(q.TMDBID))
	}
	if q.IMDbID != "" {
		ids = append(ids, "imdb://"+strings.ToLower(q.IMDbID))
	}
	for _, s := range secs {
		if s.Type != want {
			continue
		}
		var out struct {
			MediaContainer struct {
				Metadata []struct {
					Guid []struct {
						ID string `json:"id"`
					} `json:"Guid"`
				} `json:"Metadata"`
			} `json:"MediaContainer"`
		}
		v := url.Values{"title": {q.Title}, "includeGuids": {"1"}}
		if err := p.h.do(ctx, http.MethodGet, "/library/sections/"+url.PathEscape(s.Key)+"/all", v, nil, &out); err != nil {
			return false, err
		}
		for _, m := range out.MediaContainer.Metadata {
			for _, g := range m.Guid {
				for _, id := range ids {
					if strings.EqualFold(g.ID, id) {
						return true, nil
					}
				}
			}
		}
	}
	return false, nil
}

// plexListing 是一次列表：一个分区里的一种条目（1 电影、2 剧、4 集）。
type plexListing struct {
	key string
	typ int
}

// plexListings 是要列的：电影分区列电影，剧集分区先列剧再列集；别的分区（音乐、照片）不列。
func plexListings(secs []plexSection) []plexListing {
	var out []plexListing
	for _, s := range secs {
		switch s.Type {
		case "movie":
			out = append(out, plexListing{key: s.Key, typ: 1})
		case "show":
			out = append(out, plexListing{key: s.Key, typ: 2}, plexListing{key: s.Key, typ: 4})
		}
	}
	return out
}

type plexMeta struct {
	RatingKey            string `json:"ratingKey"`
	Type                 string `json:"type"`
	Title                string `json:"title"`
	Year                 int    `json:"year"`
	AddedAt              int64  `json:"addedAt"`
	GrandparentRatingKey string `json:"grandparentRatingKey"`
	ParentIndex          *int   `json:"parentIndex"`
	Index                *int   `json:"index"`
	Guid                 []struct {
		ID string `json:"id"`
	} `json:"Guid"`
	Media []struct {
		Part []struct {
			File string `json:"file"`
		} `json:"Part"`
	} `json:"Media"`
}

// guidIDs 从 Plex 的 Guid（tmdb://、imdb://、tvdb://，新版代理才有）里取编号。
func (m plexMeta) guidIDs() (tmdb int, imdb, tvdb string) {
	for _, g := range m.Guid {
		scheme, id, ok := strings.Cut(g.ID, "://")
		if !ok {
			continue
		}
		switch strings.ToLower(scheme) {
		case "tmdb":
			if n, err := strconv.Atoi(id); err == nil && n > 0 {
				tmdb = n
			}
		case "imdb":
			imdb = strings.ToLower(id)
		case "tvdb":
			tvdb = id
		}
	}
	return tmdb, imdb, tvdb
}

func (m plexMeta) item() (Item, bool) {
	out := Item{ID: m.RatingKey, Title: m.Title, Year: m.Year}
	switch m.Type {
	case "movie":
		out.Kind = models.MediaServerItemMovie
	case "show":
		out.Kind = models.MediaServerItemSeries
	case "episode":
		out.Kind = models.MediaServerItemEpisode
		out.SeriesID, out.Season, out.Episode = m.GrandparentRatingKey, intOr(m.ParentIndex), intOr(m.Index)
	default:
		return Item{}, false
	}
	if m.RatingKey == "" {
		return Item{}, false
	}
	if len(m.Media) > 0 && len(m.Media[0].Part) > 0 {
		out.Path = m.Media[0].Part[0].File
	}
	out.TMDBID, out.IMDbID, out.TVDBID = m.guidIDs()
	if m.AddedAt > 0 {
		out.AddedAt = time.Unix(m.AddedAt, 0).UTC()
	}
	return out, true
}

// ListItems 依次列各分区：位置写成「分区:种类:第几项:第一页的总数」。分区在列的过程中没了时当作列完了、记成变了。
func (p *plex) ListItems(ctx context.Context, cursor string) (ItemPage, error) {
	secs, err := p.sections(ctx)
	if err != nil {
		return ItemPage{}, err
	}
	lists := plexListings(secs)
	if len(lists) == 0 {
		return ItemPage{}, nil
	}
	idx, pc := 0, pageCursor{total: -1}
	if cursor != "" {
		parts := strings.SplitN(cursor, ":", 3)
		if len(parts) != 3 {
			return ItemPage{}, fmt.Errorf("%w: 分页位置 %q 不对", ErrBadConfig, cursor)
		}
		typ, terr := strconv.Atoi(parts[1])
		if pc, err = parseCursor(parts[2]); terr != nil || err != nil {
			return ItemPage{}, fmt.Errorf("%w: 分页位置 %q 不对", ErrBadConfig, cursor)
		}
		idx = -1
		for i, l := range lists {
			if l.key == parts[0] && l.typ == typ {
				idx = i
			}
		}
		if idx < 0 {
			return ItemPage{Changed: true}, nil
		}
	}
	cur := lists[idx]
	v := url.Values{
		"type": {strconv.Itoa(cur.typ)}, "includeGuids": {"1"},
		"X-Plex-Container-Start": {strconv.Itoa(pc.start)}, "X-Plex-Container-Size": {strconv.Itoa(itemPageSize)},
	}
	var out struct {
		MediaContainer struct {
			Size      int        `json:"size"`
			TotalSize *int       `json:"totalSize"`
			Metadata  []plexMeta `json:"Metadata"`
		} `json:"MediaContainer"`
	}
	if err := p.h.do(ctx, http.MethodGet, "/library/sections/"+url.PathEscape(cur.key)+"/all", v, nil, &out); err != nil {
		return ItemPage{}, err
	}
	mc := out.MediaContainer
	page := ItemPage{Items: make([]Item, 0, len(mc.Metadata))}
	for _, m := range mc.Metadata {
		if got, ok := m.item(); ok {
			page.Items = append(page.Items, got)
		}
	}
	var next string
	if mc.TotalSize != nil {
		next, page.Changed = pc.next(len(mc.Metadata), *mc.TotalSize)
	} else if len(mc.Metadata) == itemPageSize {
		// 没报总数（旧版本）时满一页就再看下一页，看不出列的过程中变没变
		next = strconv.Itoa(pc.start+itemPageSize) + ":0"
	}
	switch {
	case next != "":
		page.Next = cur.key + ":" + strconv.Itoa(cur.typ) + ":" + next
	case idx+1 < len(lists):
		nl := lists[idx+1]
		page.Next = nl.key + ":" + strconv.Itoa(nl.typ) + ":"
	}
	return page, nil
}

func (p *plex) Users(context.Context) ([]User, error) {
	return nil, fmt.Errorf("%w: Plex 不提供继续观看的用户接口", ErrUnsupported)
}

func (p *plex) Resume(context.Context, string, int) ([]ResumeItem, error) {
	return nil, fmt.Errorf("%w: Plex 的继续观看没有做", ErrUnsupported)
}
