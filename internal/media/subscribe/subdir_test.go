package subscribe

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestCleanSubdirTemplate(t *testing.T) {
	for _, c := range []struct{ in, want, err string }{
		{"", DefaultSubdirTemplate, ""},
		{"  ", DefaultSubdirTemplate, ""},
		{" {title} - S{season} ", "{title} - S{season}", ""},
		{"{original_title} [{tmdb_id}]", "{original_title} [{tmdb_id}]", ""},
		{"{name} ({year})", "", "不认识的变量 {name}"},
		{"下载", "", "要有 {title} 这类变量"},
		{"{title}" + strings.Repeat("x", 250), "", "太长"},
	} {
		got, err := cleanSubdirTemplate(c.in)
		if c.err != "" {
			require.ErrorIs(t, err, ErrInvalid, c.in)
			assert.Contains(t, err.Error(), c.err)
			continue
		}
		require.NoError(t, err, c.in)
		assert.Equal(t, c.want, got)
	}
}

func TestSubdirName(t *testing.T) {
	it := subdirItem{Title: "最后生还者", OriginalTitle: "The Last of Us", Year: 2023, Season: 2, TMDBID: 100088}
	assert.Equal(t, "最后生还者 (2023)", subdirName(DefaultSubdirTemplate, it))
	assert.Equal(t, "The Last of Us - S2 [100088]", subdirName("{original_title} - S{season} [{tmdb_id}]", it))
	assert.Equal(t, "最后生还者", subdirName(DefaultSubdirTemplate, subdirItem{Title: "最后生还者"}), "不知道年份：空括号去掉")
	assert.Equal(t, "星球大战 - 第四集 新希望 (1977)", subdirName(DefaultSubdirTemplate, subdirItem{Title: "星球大战：第四集/新希望", Year: 1977}),
		"冒号与斜杠不能进目录名：不会多一级目录")
	assert.Empty(t, subdirName("{original_title}", it.withoutNames()), "算出是空的")
	long := subdirName("{title}", subdirItem{Title: strings.Repeat("长", 100)})
	assert.LessOrEqual(t, len(long), maxSubdirName)
	assert.True(t, strings.HasPrefix(long, "长长"))
	assert.Equal(t, "长", string([]rune(long)[len([]rune(long))-1:]), "按字截断，不截出半个字")
}

func (it subdirItem) withoutNames() subdirItem {
	it.Title, it.OriginalTitle = "", ""
	return it
}

func TestJoinSavePath(t *testing.T) {
	assert.Equal(t, "/downloads/最后生还者 (2023)", joinSavePath("/downloads", "最后生还者 (2023)"))
	assert.Equal(t, "/downloads/最后生还者 (2023)", joinSavePath("/downloads//", "最后生还者 (2023)"))
	assert.Equal(t, `D:\Downloads\最后生还者 (2023)`, joinSavePath(`D:\Downloads\`, "最后生还者 (2023)"), "Windows 路径")
	assert.Equal(t, "D:/Downloads/x", joinSavePath("D:/Downloads", "x"))
}

func TestSubdirSettings(t *testing.T) {
	e := newEnv(t)
	set := e.enable(nil)
	assert.False(t, set.SubdirEnabled)
	assert.Equal(t, DefaultSubdirTemplate, set.SubdirTemplate, "没设过时是默认模板")
	set = e.enable(func(s *Settings) { s.SubdirEnabled, s.SubdirTemplate = true, " {title} [{tmdb_id}] " })
	assert.True(t, set.SubdirEnabled)
	assert.Equal(t, "{title} [{tmdb_id}]", set.SubdirTemplate)
	_, err := e.svc.SaveSettings(e.ctx, Settings{SubdirEnabled: true, SubdirTemplate: "{nope}"})
	require.ErrorIs(t, err, ErrInvalid)
	got, err := e.svc.Settings(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, "{title} [{tmdb_id}]", got.SubdirTemplate, "写错的没保存")
}

// 订阅推送：开了按条目建子目录时放进保存路径（没填时是下载器的默认保存路径）下的子目录，要下载器照这个目录放；
// 读不到默认保存路径时不建子目录，写进运行记录；没开时和原来一样。
func TestSubdirPush(t *testing.T) {
	pushOnce := func(t *testing.T, e *env, sub *models.MediaSubscription) (string, bool) {
		t.Helper()
		e.search.set(e.item("hdsky", "s1", "Dune.Part.Two.2024.2160p.WEB-DL.H265-GRP", "", 20, 10))
		_, err := e.svc.SearchNow(e.ctx, sub.ID)
		require.NoError(t, err)
		pushes := e.gotPushes()
		require.Len(t, pushes, 1)
		return pushes[0].SavePath, pushes[0].ForceSavePath
	}
	movie := SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134}

	t.Run("没开", func(t *testing.T) {
		e := newEnv(t)
		e.enable(nil)
		in := movie
		in.SavePath = "/data/movies"
		path, force := pushOnce(t, e, e.sub(in))
		assert.Equal(t, "/data/movies", path)
		assert.False(t, force)
	})
	t.Run("订阅填了保存路径", func(t *testing.T) {
		e := newEnv(t)
		e.enable(func(s *Settings) { s.SubdirEnabled = true })
		in := movie
		in.SavePath = "/data/movies/"
		path, force := pushOnce(t, e, e.sub(in))
		assert.Equal(t, "/data/movies/沙丘2 (2024)", path)
		assert.True(t, force)
	})
	t.Run("下载器的默认保存路径", func(t *testing.T) {
		e := newEnv(t)
		e.enable(func(s *Settings) { s.SubdirEnabled = true })
		e.dl.paths = []string{`D:\Downloads`}
		path, force := pushOnce(t, e, e.sub(movie))
		assert.Equal(t, `D:\Downloads\沙丘2 (2024)`, path)
		assert.True(t, force)
	})
	t.Run("读不到默认保存路径", func(t *testing.T) {
		e := newEnv(t)
		e.enable(func(s *Settings) { s.SubdirEnabled = true })
		e.dl.pathsErr = errors.New("连不上")
		sub := e.sub(movie)
		path, force := pushOnce(t, e, sub)
		assert.Empty(t, path)
		assert.False(t, force)
		assert.Contains(t, runSteps(e, sub.ID), "info:读不到下载器的默认保存路径（连不上），这次不建子目录「沙丘2 (2024)」")

		e.dl.pathsErr = nil
		e.dl.paths = nil
		path, force = e.svc.savePathFor(e.ctx, sub, Settings{SubdirEnabled: true, SubdirTemplate: DefaultSubdirTemplate}, 1)
		assert.Empty(t, path, "下载器没给出默认保存路径")
		assert.False(t, force)
	})
	t.Run("剧集用首播年份，算出空的名字不建", func(t *testing.T) {
		e := newEnv(t)
		e.dl.paths = []string{"/downloads"}
		sub := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
		set := Settings{SubdirEnabled: true, SubdirTemplate: "{title} ({year}) S{season}"}
		path, force := e.svc.savePathFor(e.ctx, sub, set, 1)
		assert.Equal(t, "/downloads/最后生还者 (2023) S2", path, "订阅记的是这一季的年份（2025），子目录用首播年份")
		assert.True(t, force)
		set.SubdirTemplate = "{original_title}"
		sub.OriginalTitle = ""
		path, force = e.svc.savePathFor(e.ctx, sub, set, 1)
		assert.Empty(t, path)
		assert.False(t, force)
	})
}

// 按条目推送的子目录：不看开关（弹窗里选了才算），按设置里的模板。
func TestItemDownloadDir(t *testing.T) {
	e := newEnv(t)
	e.enable(func(s *Settings) { s.SubdirTemplate = "{title} [{tmdb_id}]" })
	e.dl.paths = []string{"/downloads"}
	got, err := e.svc.ItemDownloadDir(e.ctx, models.MediaKindMovie, 693134, 0, 1, "")
	require.NoError(t, err)
	assert.Equal(t, "/downloads/沙丘2 [693134]", got)
	got, err = e.svc.ItemDownloadDir(e.ctx, models.MediaKindTV, 100088, 2, 1, " /data/tv ")
	require.NoError(t, err)
	assert.Equal(t, "/data/tv/最后生还者 [100088]", got)

	_, err = e.svc.ItemDownloadDir(e.ctx, "book", 1, 0, 1, "")
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.ItemDownloadDir(e.ctx, models.MediaKindMovie, 404404, 0, 1, "")
	require.ErrorContains(t, err, "取 TMDB 条目失败")
	e.dl.pathsErr = errors.New("连不上")
	_, err = e.svc.ItemDownloadDir(e.ctx, models.MediaKindMovie, 693134, 0, 1, "")
	require.ErrorContains(t, err, "读不到下载器的默认保存路径: 连不上")
	e.enable(func(s *Settings) { s.SubdirTemplate = "{season}" })
	_, err = e.svc.ItemDownloadDir(e.ctx, models.MediaKindMovie, 693134, 0, 1, "/data")
	require.ErrorContains(t, err, "算出的子目录名是空的", "电影没有季")
}
