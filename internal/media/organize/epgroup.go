package organize

import (
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/sunerpy/pt-tools/internal/media/epgroup"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

// 剧集组（媒体中心 M9b）：追剧选了这部剧的一个 TMDB 剧集组时，订阅推送或认领的种子整理入库前，每个文件名里按剧集组写的季集
// 先换成 TMDB 默认的季集，再命名与刮削。规则和订阅对种子时相同（见 internal/media/epgroup），两边算出的集一致。

// loadEpisodeGroup 读追剧选的剧集组（这次整理的条目是追剧的那部剧时）。读不到时这次不整理、写明原因（TMDB 暂时不能访问时稍后重试），
// 免得把按剧集组写的集号当成默认的季集放错集。
func (s *Service) loadEpisodeGroup(ctx context.Context, p *Plan) bool {
	f := p.follow
	if f == nil || f.EpisodeGroup == "" || f.MediaType != tmdb.KindTV || p.Match == nil || f.TMDBID != p.Match.ID {
		return true
	}
	var g *tmdb.EpisodeGroup
	err := errors.New("TMDB 不能用")
	if p.tmdbClient != nil {
		g, err = p.tmdbClient.EpisodeGroup(ctx, f.EpisodeGroup)
	}
	if err != nil {
		p.FailKind = models.MediaFailRecognize
		if errors.Is(err, tmdb.ErrNotFound) {
			p.Problem = "追剧选的剧集组在 TMDB 上找不到了：在追剧的「修改」里换一个剧集组或不用剧集组，再重新整理"
			return false
		}
		p.Problem, p.retryable = "读不到追剧选的剧集组，稍后重试："+err.Error(), true
		return false
	}
	p.group = epgroup.New(g)
	return true
}

// regroup 按追剧选的剧集组把一个文件的季集（文件名里的，没写季时是种子标题的）换成 TMDB 默认的。剧集组里没有写的组或集、
// 换算出的集不在同一季或不连着时照文件名里的季集；换算的结果与没换算的原因写进这个文件的整理记录与这次整理的说明。
func (p *Plan) regroup(it *PlanItem) {
	c := epgroup.Coord{Season: it.Season, Episode: it.Episode, EpisodeEnd: max(it.EpisodeEnd, it.Episode)}
	name := path.Base(it.Rel)
	refs, ok := p.group.Convert(c)
	if !ok {
		p.noteItem(it, fmt.Sprintf("%s里没有%s，按文件名里的季集整理", p.group.Label(), c), name)
		return
	}
	season := refs[0].Season
	eps := epgroup.Season(refs, season)
	if len(eps) != len(refs) || !epgroup.Contiguous(eps) {
		p.noteItem(it, fmt.Sprintf("按%s换算的%s不在同一季或不连着，按文件名里的季集整理", p.group.Label(), c), name)
		return
	}
	it.Season, it.Episode, it.EpisodeEnd = season, eps[0], 0
	if len(eps) > 1 {
		it.EpisodeEnd = eps[len(eps)-1]
	}
	it.groupNote = fmt.Sprintf("%s的%s是 %s", p.group.Label(), c, transfer.SeasonEpisode(it.Season, it.Episode, it.EpisodeEnd))
}

// noteItem 记下一个文件没按剧集组换算的原因：写进它的整理记录，也写进这次整理的说明。
func (p *Plan) noteItem(it *PlanItem, note, name string) {
	it.groupNote = note
	p.Notes = append(p.Notes, name+"："+note)
}
