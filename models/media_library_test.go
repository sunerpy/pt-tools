package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ChooseLibrary 是整理与新建订阅共用的选库：在同类型、启用的库里选第一个，动画优先进只收动画的库；只收动画的库不收别的条目
func TestChooseLibrary(t *testing.T) {
	libs := []MediaLibrary{
		{ID: 1, Kind: MediaKindMovie, Enabled: true},
		{ID: 2, Kind: MediaKindTV, Anime: true, Enabled: true},
		{ID: 3, Kind: MediaKindTV, Enabled: false},
		{ID: 4, Kind: MediaKindTV, Enabled: true},
		{ID: 5, Kind: MediaKindTV, Enabled: true},
	}
	pick := func(libs []MediaLibrary, kind string, anime bool) uint {
		if l := ChooseLibrary(libs, kind, anime); l != nil {
			return l.ID
		}
		return 0
	}
	for _, c := range []struct {
		name  string
		libs  []MediaLibrary
		kind  string
		anime bool
		want  uint
	}{
		{"电影进第一个启用的电影库", libs, MediaKindMovie, false, 1},
		{"没有动画电影库时动画电影进普通电影库", libs, MediaKindMovie, true, 1},
		{"动画剧集进只收动画的库", libs, MediaKindTV, true, 2},
		{"普通剧集跳过只收动画的库与没启用的库", libs, MediaKindTV, false, 4},
		{"只有动画库、条目不是动画时不选它", libs[1:2], MediaKindTV, false, 0},
		{"没有动画库时动画进普通库", libs[2:], MediaKindTV, true, 4},
		{"同类型的库都没启用", []MediaLibrary{{ID: 6, Kind: MediaKindTV}}, MediaKindTV, false, 0},
		{"没有库", nil, MediaKindTV, false, 0},
	} {
		assert.Equal(t, c.want, pick(c.libs, c.kind, c.anime), c.name)
	}
	// 返回的是切片里的那一项：整理用它的目录与设置
	l := ChooseLibrary(libs, MediaKindTV, false)
	l.Name = "改过"
	assert.Equal(t, "改过", libs[3].Name)
}
