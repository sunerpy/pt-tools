package tmdb

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// Extras 是刮削写 NFO 用的、详情里没有的那些：演职员、分级、出品公司（剧集是电视网）、所属系列、剧集的状态、
// 宣传语、评分人数与 TVDB 编号。用一次带 append_to_response 的详情请求取到，缓存一周。
type Extras struct {
	Tagline string `json:"tagline,omitempty"`
	// Status 是 TMDB 的状态原文（剧集：Returning Series、Ended、Canceled……）
	Status    string `json:"status,omitempty"`
	VoteCount int    `json:"vote_count,omitempty"`
	IMDbID    string `json:"imdb_id,omitempty"`
	TVDBID    int    `json:"tvdb_id,omitempty"`
	// Studios 是电影的出品公司、剧集的电视网
	Studios []string `json:"studios,omitempty"`
	// Certifications 是各国家或地区的分级（ISO 3166-1 → 分级，例如 US → PG-13）
	Certifications map[string]string `json:"certifications,omitempty"`
	Collection     *Collection       `json:"collection,omitempty"`
	Directors      []string          `json:"directors,omitempty"`
	Writers        []string          `json:"writers,omitempty"`
	Cast           []CastMember      `json:"cast,omitempty"`
}

// Collection 是电影所属的系列。
type Collection struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// CastMember 是一位演员。ProfilePath 是头像图片的路径（/xxx.jpg），没有时为空。
type CastMember struct {
	Name        string `json:"name"`
	Character   string `json:"character,omitempty"`
	Order       int    `json:"order"`
	ProfilePath string `json:"profile_path,omitempty"`
}

// 演员与编剧最多留几位（NFO 不写太长）。
const (
	maxCast    = 20
	maxWriters = 10
)

type rawCrew struct {
	Name       string `json:"name"`
	Job        string `json:"job"`
	Department string `json:"department"`
}

// crewNames 是职员里的导演与编剧（编剧是 Writing 部门的，去掉重复）。
func crewNames(crew []rawCrew) (directors, writers []string) {
	for _, c := range crew {
		name := strings.TrimSpace(c.Name)
		switch {
		case name == "":
		case c.Job == "Director" && !slices.Contains(directors, name):
			directors = append(directors, name)
		case c.Department == "Writing" && !slices.Contains(writers, name) && len(writers) < maxWriters:
			writers = append(writers, name)
		}
	}
	return directors, writers
}

// Extras 取一个条目的 Extras（见 Extras 类型）。
func (c *Client) Extras(ctx context.Context, kind string, id int) (*Extras, error) {
	if err := checkKind(kind); err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, ErrNotFound
	}
	ratings := "release_dates"
	if kind == KindTV {
		ratings = "content_ratings"
	}
	var out Extras
	err := c.cached(ctx, fmt.Sprintf("extras:%s:%d:%s", kind, id, c.language), DetailsTTL, &out, func() (any, error) {
		var raw struct {
			Tagline   string `json:"tagline"`
			Status    string `json:"status"`
			VoteCount int    `json:"vote_count"`
			IMDbID    string `json:"imdb_id"`
			Companies []struct {
				Name string `json:"name"`
			} `json:"production_companies"`
			Networks []struct {
				Name string `json:"name"`
			} `json:"networks"`
			Collection *struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"belongs_to_collection"`
			Credits struct {
				Cast []CastMember `json:"cast"`
				Crew []rawCrew    `json:"crew"`
			} `json:"credits"`
			ExternalIDs struct {
				IMDbID string `json:"imdb_id"`
				TVDBID int    `json:"tvdb_id"`
			} `json:"external_ids"`
			ReleaseDates struct {
				Results []struct {
					Country string `json:"iso_3166_1"`
					Dates   []struct {
						Certification string `json:"certification"`
					} `json:"release_dates"`
				} `json:"results"`
			} `json:"release_dates"`
			ContentRatings struct {
				Results []struct {
					Country string `json:"iso_3166_1"`
					Rating  string `json:"rating"`
				} `json:"results"`
			} `json:"content_ratings"`
		}
		q := url.Values{"language": {c.language}, "append_to_response": {"credits,external_ids," + ratings}}
		if err := c.get(ctx, fmt.Sprintf("/%s/%d", kind, id), q, &raw); err != nil {
			return nil, err
		}
		x := Extras{
			Tagline: strings.TrimSpace(raw.Tagline), Status: raw.Status, VoteCount: raw.VoteCount, TVDBID: raw.ExternalIDs.TVDBID,
		}
		x.IMDbID = firstIMDb(raw.IMDbID, raw.ExternalIDs.IMDbID)
		studios := raw.Companies
		if kind == KindTV && len(raw.Networks) > 0 {
			studios = raw.Networks
		}
		for _, s := range studios {
			if n := strings.TrimSpace(s.Name); n != "" && !slices.Contains(x.Studios, n) {
				x.Studios = append(x.Studios, n)
			}
		}
		if raw.Collection != nil && raw.Collection.ID > 0 && strings.TrimSpace(raw.Collection.Name) != "" {
			x.Collection = &Collection{ID: raw.Collection.ID, Name: strings.TrimSpace(raw.Collection.Name)}
		}
		cast := slices.Clone(raw.Credits.Cast)
		slices.SortStableFunc(cast, func(a, b CastMember) int { return a.Order - b.Order })
		for _, m := range cast {
			if strings.TrimSpace(m.Name) == "" {
				continue
			}
			if !imagePathRe.MatchString(m.ProfilePath) {
				m.ProfilePath = ""
			}
			x.Cast = append(x.Cast, m)
			if len(x.Cast) >= maxCast {
				break
			}
		}
		x.Directors, x.Writers = crewNames(raw.Credits.Crew)
		certs := map[string]string{}
		for _, r := range raw.ReleaseDates.Results {
			for _, d := range r.Dates {
				if cert := strings.TrimSpace(d.Certification); cert != "" && r.Country != "" {
					if _, ok := certs[r.Country]; !ok {
						certs[r.Country] = cert
					}
				}
			}
		}
		for _, r := range raw.ContentRatings.Results {
			if cert := strings.TrimSpace(r.Rating); cert != "" && r.Country != "" {
				certs[r.Country] = cert
			}
		}
		if len(certs) > 0 {
			x.Certifications = certs
		}
		return x, nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// firstIMDb 是第一个格式对的 IMDb 编号。
func firstIMDb(ids ...string) string {
	for _, id := range ids {
		if v := v2.NormalizeIMDbID(id); v != "" {
			return v
		}
	}
	return ""
}

// ProfileURL 是演员头像的地址（w185）：用这个客户端的图片地址。路径格式不对时为空。
func (c *Client) ProfileURL(path string) string {
	if !imagePathRe.MatchString(path) {
		return ""
	}
	return c.imageBase + "w185" + path
}

// TVDBIDString 是 TVDB 编号的字符串（没有时为空）。
func (x *Extras) TVDBIDString() string {
	if x == nil || x.TVDBID <= 0 {
		return ""
	}
	return strconv.Itoa(x.TVDBID)
}
