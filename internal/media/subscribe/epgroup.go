package subscribe

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/sunerpy/pt-tools/internal/media/epgroup"
	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// 剧集组（媒体中心 M9b）：追剧选了这部剧在 TMDB 上的一个剧集组时，种子标题与文件名里的季集是按剧集组的排法写的
// （动画的绝对集号、另一种分季），先换成 TMDB 默认的季集（见 internal/media/epgroup），再照默认的季集对种子、
// 映射集号、记台账；台账、日历与详情都按默认的季集。剧集组里没有写的组或集时照默认的季集算，并写进运行记录。

// EpisodeGroups 是一部剧在 TMDB 上的剧集组（追剧的「修改」里选）。没有时是空的。
func (s *Service) EpisodeGroups(ctx context.Context, tvID int) ([]tmdb.EpisodeGroupSummary, error) {
	if tvID <= 0 {
		return nil, fmt.Errorf("%w: 要指定 TMDB 编号", ErrInvalid)
	}
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return nil, err
	}
	list, err := c.EpisodeGroups(ctx, tvID)
	if err != nil {
		return nil, fmt.Errorf("取 TMDB 剧集组失败: %w", err)
	}
	return list, nil
}

// cleanEpisodeGroup 检查追剧选的剧集组：空串是不用剧集组；不空时要是 TMDB 上这部剧的剧集组。返回去掉空白的编号。
func (s *Service) cleanEpisodeGroup(ctx context.Context, tvID int, id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", nil
	}
	if !tmdb.ValidEpisodeGroupID(id) {
		return "", fmt.Errorf("%w: 剧集组编号不对", ErrInvalid)
	}
	list, err := s.EpisodeGroups(ctx, tvID)
	if err != nil {
		return "", err
	}
	if !slices.ContainsFunc(list, func(g tmdb.EpisodeGroupSummary) bool { return g.ID == id }) {
		return "", fmt.Errorf("%w: TMDB 上这部剧没有这个剧集组", ErrInvalid)
	}
	return id, nil
}

// episodeGroupFor 读追剧选的剧集组（TMDB 的结果缓存 24 小时）。
func (s *Service) episodeGroupFor(ctx context.Context, sub *models.MediaSubscription) (*epgroup.Map, error) {
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return nil, err
	}
	g, err := c.EpisodeGroup(ctx, sub.EpisodeGroupID)
	if err != nil {
		return nil, err
	}
	return epgroup.New(g), nil
}

// grouped 报告这个映射要按剧集组换算（追剧选了剧集组；读不到剧集组时也是，换算时不要种子）。
func (mp *episodeMapper) grouped() bool {
	return mp != nil && (mp.group != nil || mp.groupErr != "")
}

// forTorrent 是给这个种子里的文件用的映射：文件名里只写了集、没写季时按种子标题写的组（见 epgroup.CoordOf）。
// 没选剧集组时就是 mp。
func (mp *episodeMapper) forTorrent(m meta.Meta) *episodeMapper {
	if !mp.grouped() {
		return mp
	}
	cp := *mp
	cp.packSeason = epgroup.TitleSeason(m)
	return &cp
}

// regrouped 是按剧集组换算的结果（见 regroup）。
type regrouped struct {
	// other：换算出的集都不在这一季（不是这个追剧的种子，不写原因）
	other bool
	// note：剧集组里没有写的组或集，照 TMDB 默认的季集算（写进运行记录）
	note string
	// reject：不要这个种子的原因（读不到剧集组时是 episodeMapper.groupErr）
	reject string
}

// regroup 把按剧集组写的季集换成 TMDB 默认排法里这一季的集（没选剧集组时原样返回）。换算出的这一季的集要连着：
// 季换成这一季，集换成那几集；写的是一整组（或几组）、正好是这一季的全部集时还是整季。
func (mp *episodeMapper) regroup(m meta.Meta) (meta.Meta, regrouped) {
	if !mp.grouped() {
		return m, regrouped{}
	}
	if mp.group == nil {
		return m, regrouped{reject: mp.groupErr}
	}
	c, ok := epgroup.CoordOf(m, mp.packSeason)
	if !ok {
		return m, regrouped{}
	}
	refs, ok := mp.group.Convert(c)
	if !ok {
		return m, regrouped{note: fmt.Sprintf("%s里没有%s，按 TMDB 默认的季集算", mp.group.Label(), c)}
	}
	eps := epgroup.Season(refs, mp.season)
	if len(eps) == 0 {
		return m, regrouped{other: true}
	}
	if !epgroup.Contiguous(eps) {
		return m, regrouped{reject: fmt.Sprintf("按%s换算是第 %d 季的 %s，不连着，没有用它", mp.group.Label(), mp.season, episodeList(eps))}
	}
	out := m
	out.Season, out.SeasonEnd, out.Episode, out.EpisodeEnd = mp.season, 0, eps[0], 0
	if len(eps) > 1 {
		out.EpisodeEnd = eps[len(eps)-1]
	}
	if (c.Episode == 0 || c.SeasonEnd > c.Season) && mp.wholeSeason(eps) {
		out.Episode, out.EpisodeEnd = 0, 0
	}
	return out, regrouped{}
}

// wholeSeason 报告 eps 是不是 TMDB 上这一季的全部集（不知道这一季有哪些集时为假）。
func (mp *episodeMapper) wholeSeason(eps []int) bool {
	if mp.index == nil || len(mp.index.numbers) == 0 {
		return false
	}
	for n := range mp.index.numbers {
		if !slices.Contains(eps, n) {
			return false
		}
	}
	return true
}
