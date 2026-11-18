package scrape

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sunerpy/pt-tools/internal/media/metadata"
)

// 一个剧集文件的 NFO 里来源还没有的内容：任一集没有标题或没有简介都算；季没取到时两样都缺。
func TestEpisodeLacks(t *testing.T) {
	s := &metadata.Season{Number: 1, Episodes: []metadata.Episode{
		{Number: 1, Title: "当你迷失在黑暗中", Plot: "乔尔遇到了艾莉。"},
		{Number: 2, Title: "感染", Plot: "  "},
		{Number: 3, Title: " ", Plot: "他们穿过城市。"},
	}}
	cases := []struct {
		name         string
		season       *metadata.Season
		episode, end int
		want         []string
	}{
		{"都有", s, 1, 0, nil},
		{"没有简介", s, 2, 0, []string{LackEpisodePlot}},
		{"没有标题", s, 3, 0, []string{LackEpisodeTitle}},
		{"季里没有这一集", s, 4, 0, []string{LackEpisodeTitle, LackEpisodePlot}},
		{"多集的文件里有一集缺", s, 1, 2, []string{LackEpisodePlot}},
		{"季没取到", nil, 1, 0, []string{LackEpisodeTitle, LackEpisodePlot}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, episodeLacks(c.season, c.episode, c.end))
		})
	}
}
