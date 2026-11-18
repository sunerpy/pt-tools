package web

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 关键字搜索的每个结果带上标题解析出的信息（分辨率、来源、平台、编码、HDR、制作组、中字、季集），只增字段。
func TestTorrentItemResponseMeta(t *testing.T) {
	got := toTorrentItemResponse(v2.TorrentItem{
		ID: "8", SourceSite: "ourbits",
		Title:    "Chang.Ye.Jiang.Jin.S02E08.2160p.WEB-DL.H265.HDR10.DDP5.1-CHDWEB",
		Subtitle: "长夜将尽 第二季 第08集 中字",
	})
	require.NotNil(t, got.Meta)
	assert.Equal(t, "2160p", got.Meta.Resolution)
	assert.Equal(t, "WEB-DL", got.Meta.Source)
	assert.Equal(t, "H.265", got.Meta.VideoCodec)
	assert.Equal(t, "CHDWEB", got.Meta.Group)
	assert.Equal(t, 2, got.Meta.Season)
	assert.Equal(t, 8, got.Meta.Episode)
	assert.True(t, got.Meta.ChineseSubs)
	assert.Contains(t, got.Meta.HDR, "HDR10")

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	meta, ok := m["meta"].(map[string]any)
	require.True(t, ok, "结果里有 meta")
	assert.Equal(t, "2160p", meta["resolution"])
	assert.Equal(t, "ourbits", m["sourceSite"], "原有字段不变")

	empty := toTorrentItemResponse(v2.TorrentItem{ID: "9", SourceSite: "ourbits"})
	assert.Nil(t, empty.Meta, "没有标题时不带")
}
