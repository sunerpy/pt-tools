package tmdb

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// FindTVDBTTL 是按 TVDB 编号找到的剧集的缓存时间：一部剧的 TVDB 编号与 TMDB 编号对应好以后不会变。
const FindTVDBTTL = 30 * 24 * time.Hour

// FindByTVDB 按 TVDB 编号找 TMDB 上的剧集（GET /find/{tvdb}?external_source=tvdb_id，缓存 30 天）。Servarr 兼容入口用：
// Seerr 请求剧集时只给 TVDB 编号。找不到或编号不对时是 ErrNotFound。
func (c *Client) FindByTVDB(ctx context.Context, tvdbID int) (*Result, error) {
	if tvdbID <= 0 {
		return nil, ErrNotFound
	}
	q := url.Values{"external_source": {"tvdb_id"}, "language": {c.language}}
	var out []Result
	err := c.cached(ctx, fmt.Sprintf("findtvdb:%d:%s", tvdbID, c.language), FindTVDBTTL, &out, func() (any, error) {
		var raw struct {
			TVResults []rawItem `json:"tv_results"`
		}
		if err := c.get(ctx, "/find/"+strconv.Itoa(tvdbID), q, &raw); err != nil {
			return nil, err
		}
		res := make([]Result, 0, len(raw.TVResults))
		for _, r := range raw.TVResults {
			res = append(res, r.result(KindTV))
		}
		return res, nil
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, ErrNotFound
	}
	return &out[0], nil
}
