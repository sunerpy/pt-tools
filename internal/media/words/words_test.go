package words

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/models"
)

func TestValidate(t *testing.T) {
	ok := []models.MediaWordRule{
		{Kind: models.MediaWordBlock, Pattern: "[禁转]"},
		{Kind: models.MediaWordReplace, Pattern: "庆余年2", Replacement: "庆余年 第二季"},
		{Kind: models.MediaWordReplace, Pattern: `(?i)^\[.*?\]\s*`, IsRegex: true},
		{Kind: models.MediaWordOffset, Pattern: "某剧", Offset: -12},
	}
	for _, r := range ok {
		assert.NoError(t, Validate(r), r.Pattern)
	}
	bad := []models.MediaWordRule{
		{Kind: "drop", Pattern: "x"},
		{Kind: models.MediaWordBlock, Pattern: "  "},
		{Kind: models.MediaWordBlock, Pattern: "(", IsRegex: true},
		{Kind: models.MediaWordOffset, Pattern: "x", Offset: 0},
		{Kind: models.MediaWordOffset, Pattern: "x", Offset: 100000},
		{Kind: models.MediaWordBlock, Pattern: string(make([]byte, 300))},
	}
	for _, r := range bad {
		assert.Error(t, Validate(r), r.Pattern)
	}
}

func TestApply(t *testing.T) {
	rules := []models.MediaWordRule{
		{ID: 1, Kind: models.MediaWordBlock, Pattern: "[禁转]"},
		{ID: 2, Kind: models.MediaWordReplace, Pattern: "joy.of.life.2", Replacement: "Joy.of.Life.S02"},
		{ID: 3, Kind: models.MediaWordReplace, Pattern: `\s*\(官方中字\)`, IsRegex: true},
		{ID: 4, Kind: models.MediaWordOffset, Pattern: "Joy.of.Life", Offset: -2},
		{ID: 5, Kind: models.MediaWordBlock, Pattern: "(", IsRegex: true}, // 无效规则跳过，不影响其它规则
		{ID: 6, Kind: models.MediaWordOffset, Pattern: "没有命中", Offset: 5},
	}
	got := Apply(rules, "[禁转] Joy.Of.Life.2.E03.2024.1080p.WEB-DL", "庆余年 第二季 (官方中字)")
	assert.Equal(t, "Joy.of.Life.S02.E03.2024.1080p.WEB-DL", got.Title, "屏蔽后去掉多余空白；纯文本不分大小写")
	assert.Equal(t, "庆余年 第二季", got.Subtitle)
	assert.Equal(t, -2, got.Offset)
	assert.Equal(t, []uint{1, 2, 3, 4}, got.Hits)

	m := meta.Parse(got.Title, got.Subtitle)
	got.ApplyOffset(&m)
	assert.Equal(t, 2, m.Season)
	assert.Equal(t, 1, m.Episode, "第 3 集减 2")

	m = meta.Meta{Episode: 1, EpisodeEnd: 3}
	Applied{Offset: -2}.ApplyOffset(&m)
	assert.Equal(t, 0, m.Episode, "减到 0 以下的集数当作没有")
	assert.Equal(t, 1, m.EpisodeEnd)

	none := Apply(nil, " a ", " b ")
	assert.Equal(t, "a", none.Title)
	assert.Equal(t, "b", none.Subtitle)
	assert.Zero(t, none.Offset)
	assert.Empty(t, none.Hits)
}
