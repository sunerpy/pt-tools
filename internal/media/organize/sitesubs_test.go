package organize

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/simplifiedchinese"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

const srtText = "1\n00:00:01,000 --> 00:00:02,000\n你好\n"

// fakeSubSite 是假的站点字幕：lists 按种子编号列出，files 按字幕编号给内容；记下列了哪些种子、下了哪些字幕。
type fakeSubSite struct {
	mu         sync.Mutex
	lists      map[string][]v2.Subtitle
	files      map[string]v2.SubtitleFile
	listErr    error
	dlErr      map[string]error
	listed     []string
	downloaded []string
}

func (f *fakeSubSite) ListSubtitles(_ context.Context, id string) ([]v2.Subtitle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listed = append(f.listed, id)
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.lists[id], nil
}

func (f *fakeSubSite) DownloadSubtitle(_ context.Context, sub v2.Subtitle) (v2.SubtitleFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downloaded = append(f.downloaded, sub.ID)
	if err := f.dlErr[sub.ID]; err != nil {
		return v2.SubtitleFile{}, err
	}
	file, ok := f.files[sub.ID]
	if !ok {
		return v2.SubtitleFile{}, v2.ErrSubtitleNotFound
	}
	return file, nil
}

// useSubSite 让站点 nexus 用 f 取字幕（别的站点不支持字幕）。
func (e *env) useSubSite(f *fakeSubSite) {
	e.svc.cfg.SubtitleSites = func(id string) v2.SubtitleProvider {
		if id == "nexus" {
			return f
		}
		return nil
	}
}

// pushedFrom 记下这个种子是 pt-tools 从站点推送的（种子表；detailID 是取详情用的编号，RSS 推送的才有）。
func (e *env) pushedFrom(hash, site, torrentID, detailID string) {
	e.t.Helper()
	h := hash
	require.NoError(e.t, e.db.Create(&models.TorrentInfo{SiteName: site, TorrentID: torrentID, DetailID: detailID, TorrentHash: &h}).Error)
}

// zipOf 按顺序把文件放进 zip（名字原样写，可以不是 UTF-8）；名字以 / 结尾的是目录。
func zipOf(t *testing.T, files ...[2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f[0], Method: zip.Deflate})
		require.NoError(t, err)
		if !strings.HasSuffix(f[0], "/") {
			_, err = w.Write([]byte(f[1]))
			require.NoError(t, err)
		}
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func gbk(t *testing.T, s string) string {
	t.Helper()
	b, err := simplifiedchinese.GB18030.NewEncoder().String(s)
	require.NoError(t, err)
	return b
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// siteSubExtras 是记录里从站点放进来的字幕（目标的文件名 → 那一项）。
func siteSubExtras(r models.MediaTransferHistory) map[string]extra {
	out := map[string]extra{}
	for _, x := range decodeExtras(r.Extras) {
		if x.Kind == extraSiteSub {
			out[filepath.Base(x.Target)] = x
		}
	}
	return out
}

// bareOpp 是没有字幕的奥本海默。
func (e *env) bareOpp() {
	e.addTorrent(oppHash, oppName, map[string]int{oppName + "/" + oppName + ".mkv": 5}, nil)
}

const oppStem = "奥本海默 (2023) - 2160p BluRay HDR H.265"

func (e *env) oppTarget() string { return filepath.Join(e.movies, "奥本海默 (2023)", oppStem) }

func TestSiteSubtitlesMovie(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, SiteSubtitles: true})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.bareOpp()
	// RSS 推送的记录：TorrentID 是 info hash，取详情用 DetailID
	e.pushedFrom(oppHash, "nexus", oppHash, "123")
	site := &fakeSubSite{
		lists: map[string][]v2.Subtitle{"123": {
			{ID: "1", TorrentID: "123", Title: "Oppenheimer.2023.chs.srt", Language: "简体中文", Flag: "china.gif"},
			{ID: "2", TorrentID: "123", Title: "Oppenheimer.2023.eng.srt", Language: "English", Flag: "uk.gif"},
			// 标题不是文件名：按下载回来的文件名与内容认
			{ID: "3", TorrentID: "123", Title: "繁中字幕", Language: "繁體中文", Flag: "hongkong.gif"},
			// 别的种子的字幕，语言只有国旗图
			{ID: "4", TorrentID: "120", Title: "第二版.srt", Flag: "china.gif"},
		}},
		files: map[string]v2.SubtitleFile{
			"1": {Name: "Oppenheimer.2023.chs.srt", Data: []byte(srtText)},
			"2": {Name: "Oppenheimer.2023.eng.srt", Data: []byte(srtText)},
			"3": {Data: []byte("[Script Info]\nTitle: x\n")},
			"4": {Name: "第二版.srt", Data: []byte(srtText + "2")},
		},
	}
	e.useSubSite(site)

	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.Empty(t, res.Messages)
	assert.Equal(t, []string{"123"}, site.listed)
	assert.Equal(t, []string{"1", "4", "3"}, site.downloaded, "有中文的只下中文的，简体在前")
	base := e.oppTarget()
	assert.Equal(t, srtText, readFile(t, base+".zh-CN.srt"))
	assert.Equal(t, srtText+"2", readFile(t, base+".zh-CN.2.srt"), "同一种语言第二个加序号")
	assert.Equal(t, "[Script Info]\nTitle: x\n", readFile(t, base+".zh-TW.ass"), "没有文件名时按内容认出 ASS")
	assert.False(t, exists(base+".en.srt"))

	rows := e.history()
	require.Len(t, rows, 1)
	assert.Equal(t, models.MediaTransferDone, rows[0].Status)
	assert.Empty(t, rows[0].Message)
	got := siteSubExtras(rows[0])
	require.Len(t, got, 3)
	for name, x := range got {
		assert.NotEmpty(t, x.FileID, name)
		assert.Empty(t, x.Source, name)
		assert.Equal(t, sum(readFile(t, x.Target)), x.SHA256, name)
	}
	page, err := e.svc.History(e.ctx, HistoryQuery{})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, 3, page.Items[0].Subtitles, "整理历史的字幕数算上站点字幕")

	// 再整理一次（库里已经有了）：不再到站点上取
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Len(t, site.listed, 1)
	assert.Len(t, siteSubExtras(e.history()[0]), 3, "记录里的站点字幕留着")

	// 删除记录时一起删（按文件编号认，换过的留着）
	require.NoError(t, os.Remove(base+".zh-TW.ass"))
	require.NoError(t, os.WriteFile(base+".zh-TW.ass", []byte("别人的"), 0o644))
	kept, err := e.svc.DeleteHistory(e.ctx, rows[0].ID, true)
	require.NoError(t, err)
	assert.False(t, exists(base+".zh-CN.srt"))
	assert.False(t, exists(base+".zh-CN.2.srt"))
	assert.True(t, exists(base+".zh-TW.ass"), "换过的不删")
	assert.Len(t, kept, 1)
}

func TestSiteSubtitlesKeptOnReorganize(t *testing.T) {
	// 库里的视频没了、以前放的站点字幕还在：重新整理放回视频，不再到站点上取；字幕也没了时再取一次
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, SiteSubtitles: true})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.bareOpp()
	e.pushedFrom(oppHash, "nexus", "123", "")
	site := &fakeSubSite{
		lists: map[string][]v2.Subtitle{"123": {{ID: "1", TorrentID: "123", Title: "Oppenheimer.2023.chs.srt", Language: "简体中文"}}},
		files: map[string]v2.SubtitleFile{"1": {Name: "Oppenheimer.2023.chs.srt", Data: []byte(srtText)}},
	}
	e.useSubSite(site)
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	base := e.oppTarget()
	require.True(t, exists(base+".zh-CN.srt"))

	require.NoError(t, os.Remove(base+".mkv"))
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created, "视频放回库里")
	assert.Len(t, site.listed, 1, "以前放的站点字幕还在：不再到站点上取")
	assert.Len(t, siteSubExtras(e.history()[0]), 1)

	require.NoError(t, os.Remove(base+".mkv"))
	require.NoError(t, os.Remove(base+".zh-CN.srt"))
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Len(t, site.listed, 2, "字幕也没了：再取一次")
	assert.Equal(t, srtText, readFile(t, base+".zh-CN.srt"))
	assert.Len(t, siteSubExtras(e.history()[0]), 1, "同一个目标只记一项")
}

func TestSiteSubtitlesSymlinkLibrary(t *testing.T) {
	// 软链接方式整理的库：站点字幕是 pt-tools 写的普通文件，删除记录时按文件编号认出来一起删
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: models.MediaModeSymlink, SiteSubtitles: true})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.bareOpp()
	e.pushedFrom(oppHash, "nexus", "123", "")
	e.useSubSite(&fakeSubSite{
		lists: map[string][]v2.Subtitle{"123": {{ID: "1", TorrentID: "123", Title: "Oppenheimer.2023.chs.srt", Language: "简体中文"}}},
		files: map[string]v2.SubtitleFile{"1": {Name: "Oppenheimer.2023.chs.srt", Data: []byte(srtText)}},
	})
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	base := e.oppTarget()
	fi, err := os.Lstat(base + ".mkv")
	require.NoError(t, err)
	require.NotZero(t, fi.Mode()&os.ModeSymlink, "视频是软链接")
	fi, err = os.Lstat(base + ".zh-CN.srt")
	require.NoError(t, err)
	require.True(t, fi.Mode().IsRegular(), "站点字幕是普通文件")

	kept, err := e.svc.DeleteHistory(e.ctx, e.history()[0].ID, true)
	require.NoError(t, err)
	assert.Empty(t, kept)
	assert.False(t, exists(base+".mkv"))
	assert.False(t, exists(base+".zh-CN.srt"), "站点字幕一起删")
}

func TestSiteSubtitlesWhenNotUsed(t *testing.T) {
	cases := []struct {
		name  string
		setup func(e *env)
	}{
		{"媒体库没开", func(e *env) {
			e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies})
			e.bareOpp()
			e.pushedFrom(oppHash, "nexus", "123", "")
		}},
		{"种子里有字幕", func(e *env) {
			e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, SiteSubtitles: true})
			e.oppenheimer()
			e.pushedFrom(oppHash, "nexus", "123", "")
		}},
		{"不是 pt-tools 推送的", func(e *env) {
			e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, SiteSubtitles: true})
			e.bareOpp()
		}},
		{"站点不支持", func(e *env) {
			e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, SiteSubtitles: true})
			e.bareOpp()
			e.pushedFrom(oppHash, "mteam", "123", "")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.settings(SettingsInput{MinVideoMB: 1})
			site := &fakeSubSite{lists: map[string][]v2.Subtitle{"123": {{ID: "1", TorrentID: "123", Title: "a.chs.srt"}}}}
			e.useSubSite(site)
			tc.setup(e)
			res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
			require.NoError(t, err)
			assert.Equal(t, 1, res.Created)
			assert.Empty(t, res.Messages)
			assert.Empty(t, site.listed)
			assert.Empty(t, siteSubExtras(e.history()[0]))
			assert.Empty(t, e.history()[0].Message)
		})
	}

	t.Run("没有接上站点", func(t *testing.T) {
		e := newEnv(t)
		e.settings(SettingsInput{MinVideoMB: 1})
		e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, SiteSubtitles: true})
		e.bareOpp()
		e.pushedFrom(oppHash, "nexus", "123", "")
		res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
		require.NoError(t, err)
		assert.Equal(t, 1, res.Created)
		assert.Empty(t, e.history()[0].Message)
	})
}

func TestSiteSubtitlesArchivedRecord(t *testing.T) {
	// 完成两周后种子的记录挪进归档表：照样认站点与编号（DetailID 没有时用 TorrentID）
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, SiteSubtitles: true})
	e.bareOpp()
	h := oppHash
	require.NoError(t, e.db.Create(&models.TorrentInfoArchive{SiteName: "nexus", TorrentID: "77", TorrentHash: &h}).Error)
	site := &fakeSubSite{
		lists: map[string][]v2.Subtitle{"77": {{ID: "9", TorrentID: "77", Title: "a.chs.srt", Language: "简体中文"}}},
		files: map[string]v2.SubtitleFile{"9": {Name: "a.chs.srt", Data: []byte(srtText)}},
	}
	e.useSubSite(site)
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, []string{"77"}, site.listed)
	assert.True(t, exists(e.oppTarget()+".zh-CN.srt"))
}

func TestSiteSubtitlesProblems(t *testing.T) {
	t.Run("取不到列表", func(t *testing.T) {
		e := newEnv(t)
		e.settings(SettingsInput{MinVideoMB: 1})
		e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, SiteSubtitles: true})
		e.bareOpp()
		e.pushedFrom(oppHash, "nexus", "123", "")
		e.useSubSite(&fakeSubSite{listErr: v2.ErrSessionExpired})
		res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
		require.NoError(t, err)
		assert.Equal(t, 1, res.Created, "字幕取不到不影响整理")
		r := e.history()[0]
		assert.Equal(t, models.MediaTransferDone, r.Status)
		assert.Contains(t, r.Message, "站点字幕：取不到站点上的字幕（")
		assert.Contains(t, r.Message, v2.ErrSessionExpired.Error())
		require.Len(t, res.Messages, 1)
		assert.Equal(t, r.Message, res.Messages[0])
	})

	t.Run("站点上没有", func(t *testing.T) {
		e := newEnv(t)
		e.settings(SettingsInput{MinVideoMB: 1})
		e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, SiteSubtitles: true})
		e.bareOpp()
		e.pushedFrom(oppHash, "nexus", "123", "")
		e.useSubSite(&fakeSubSite{})
		_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
		require.NoError(t, err)
		assert.Equal(t, "站点字幕：站点上没有这个种子的字幕", e.history()[0].Message)
	})

	t.Run("压缩包、下载失败、库里已有、不是字幕", func(t *testing.T) {
		e := newEnv(t)
		e.settings(SettingsInput{MinVideoMB: 1})
		e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, SiteSubtitles: true})
		e.bareOpp()
		e.pushedFrom(oppHash, "nexus", "123", "")
		// 用户自己先放了一份简体字幕：不覆盖
		require.NoError(t, os.MkdirAll(filepath.Dir(e.oppTarget()), 0o755))
		require.NoError(t, os.WriteFile(e.oppTarget()+".zh-CN.srt", []byte("自己的"), 0o644))
		e.useSubSite(&fakeSubSite{
			lists: map[string][]v2.Subtitle{"123": {
				{ID: "1", TorrentID: "123", Title: "字幕.rar", Language: "简体中文"},
				{ID: "2", TorrentID: "123", Title: "pack", Language: "简体中文"},
				{ID: "3", TorrentID: "123", Title: "a.chs.srt", Language: "简体中文"},
				{ID: "4", TorrentID: "123", Title: "b.chs.srt", Language: "简体中文"},
				{ID: "5", TorrentID: "123", Title: "说明", Language: "简体中文"},
			}},
			files: map[string]v2.SubtitleFile{
				"1": {Name: "字幕.rar", Data: []byte("Rar!\x1a\x07\x00xx")},
				"2": {Name: "pack", Data: []byte("7z\xbc\xaf\x27\x1cxx")},
				"3": {Name: "a.chs.srt", Data: []byte(srtText)},
				"5": {Name: "说明.html", Data: []byte("<html>hello</html>")},
			},
			dlErr: map[string]error{"4": v2.ErrSubtitleNotFound},
		})
		res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
		require.NoError(t, err)
		assert.Equal(t, 1, res.Created)
		assert.Equal(t, "自己的", readFile(t, e.oppTarget()+".zh-CN.srt"))
		assert.Equal(t, srtText, readFile(t, e.oppTarget()+".zh-CN.2.srt"), "自己的占着第一个名字，放成第二个")
		msg := e.history()[0].Message
		for _, want := range []string{
			"站点字幕：字幕.rar 是 RAR 压缩包，没有解开（只解 zip）",
			"pack 是 7z 压缩包，没有解开（只解 zip）",
			"b.chs.srt 没下载成（" + v2.ErrSubtitleNotFound.Error() + "）",
			"说明.html 不是字幕文件，没有放",
		} {
			assert.Contains(t, msg, want)
		}
		assert.Len(t, siteSubExtras(e.history()[0]), 1)
		assert.Len(t, res.Messages, 4)
	})
}

func TestSiteSubtitlesSeason(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, SiteSubtitles: true})
	e.addTorrent(tlouHash, tlouName, map[string]int{
		tlouName + "/The.Last.of.Us.S01E01.2160p.WEB-DL.DV.H.265-HHWEB.mkv": 5,
		tlouName + "/The.Last.of.Us.S01E02.2160p.WEB-DL.DV.H.265-HHWEB.mkv": 5,
	}, func(t *downloader.Torrent) { t.Category = "tv" })
	e.pushedFrom(tlouHash, "nexus", "300", "")
	pack := zipOf(t,
		[2]string{"Subs/", ""},
		[2]string{"Subs/The.Last.of.Us.S01E01.2160p.WEB-DL.DV.H.265-HHWEB.chs.ass", "e1"},
		[2]string{gbk(t, "最后生还者.S01E02.繁體.ass"), "e2"},
		[2]string{"说明.txt", "readme"},
	)
	site := &fakeSubSite{
		lists: map[string][]v2.Subtitle{"300": {
			{ID: "10", TorrentID: "300", Title: "全集字幕.zip", Language: "简体中文"},
			{ID: "11", TorrentID: "300", Title: "The.Last.of.Us.S01E02.2160p.WEB-DL.DV.H.265-HHWEB.cht.srt", Language: "繁體中文"},
			{ID: "12", TorrentID: "300", Title: "The.Last.of.Us.S01E05.chs.srt", Language: "简体中文"},
			{ID: "13", TorrentID: "300", Title: "The.Last.of.Us.S01E01.eng.srt", Language: "English"},
		}},
		files: map[string]v2.SubtitleFile{
			"10": {Name: "全集字幕.zip", Data: pack},
			"11": {Name: "The.Last.of.Us.S01E02.2160p.WEB-DL.DV.H.265-HHWEB.cht.srt", Data: []byte("e2-cht")},
		},
	}
	e.useSubSite(site)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Created)
	assert.Equal(t, []string{"10", "11"}, site.downloaded, "对不上哪一集的不下，英文的不下")
	rows := e.history()
	require.Len(t, rows, 2)
	byEp := map[int]models.MediaTransferHistory{}
	for _, r := range rows {
		byEp[r.Episode] = r
	}
	stem := func(ep int) string { return strings.TrimSuffix(byEp[ep].TargetPath, filepath.Ext(byEp[ep].TargetPath)) }
	assert.Equal(t, "e1", readFile(t, stem(1)+".zh-CN.ass"))
	assert.Equal(t, "e2", readFile(t, stem(2)+".zh-TW.ass"), "压缩包里不是 UTF-8 的名字按 GB18030 认（名字里的繁體认出繁体，不用列表上的简体），按季集分")
	assert.Equal(t, "e2-cht", readFile(t, stem(2)+".zh-TW.srt"))
	assert.Len(t, siteSubExtras(byEp[1]), 1)
	assert.Len(t, siteSubExtras(byEp[2]), 2)
	assert.Empty(t, byEp[1].Message)
	assert.Empty(t, res.Messages)
}

func TestSiteSubtitlesOnlyNewEpisode(t *testing.T) {
	// 第 1 集以前整理过：这次只给新放进库的第 2 集放字幕，第 1 集的字幕不会分给第 2 集
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, SiteSubtitles: true})
	ep1 := tlouName + "/The.Last.of.Us.S01E01.2160p.WEB-DL.DV.H.265-HHWEB.mkv"
	e.addTorrent(tlouHash, tlouName, map[string]int{ep1: 5}, func(t *downloader.Torrent) { t.Category = "tv" })
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	e.addTorrent(tlouHash, tlouName, map[string]int{ep1: 5, tlouName + "/The.Last.of.Us.S01E02.2160p.WEB-DL.DV.H.265-HHWEB.mkv": 5}, func(t *downloader.Torrent) { t.Category = "tv" })
	e.pushedFrom(tlouHash, "nexus", "300", "")
	site := &fakeSubSite{
		lists: map[string][]v2.Subtitle{"300": {
			{ID: "1", TorrentID: "300", Title: "The.Last.of.Us.S01E01.2160p.WEB-DL.DV.H.265-HHWEB.chs.srt", Language: "简体中文"},
			{ID: "2", TorrentID: "300", Title: "The.Last.of.Us.S01E02.2160p.WEB-DL.DV.H.265-HHWEB.chs.srt", Language: "简体中文"},
		}},
		files: map[string]v2.SubtitleFile{
			"1": {Name: "The.Last.of.Us.S01E01.2160p.WEB-DL.DV.H.265-HHWEB.chs.srt", Data: []byte("e1")},
			"2": {Name: "The.Last.of.Us.S01E02.2160p.WEB-DL.DV.H.265-HHWEB.chs.srt", Data: []byte("e2")},
		},
	}
	e.useSubSite(site)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.Equal(t, []string{"2"}, site.downloaded, "第 1 集这次不放，不下它的字幕")
	var r2 models.MediaTransferHistory
	for _, r := range e.history() {
		if r.Episode == 2 {
			r2 = r
		}
	}
	stem := strings.TrimSuffix(r2.TargetPath, filepath.Ext(r2.TargetPath))
	assert.Equal(t, "e2", readFile(t, stem+".zh-CN.srt"))
	assert.False(t, exists(stem+".zh-CN.2.srt"))
	assert.Len(t, siteSubExtras(r2), 1)
}

func TestSiteSubtitlesLimits(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, SiteSubtitles: true})
	e.bareOpp()
	e.pushedFrom(oppHash, "nexus", "123", "")
	site := &fakeSubSite{lists: map[string][]v2.Subtitle{}, files: map[string]v2.SubtitleFile{}}
	for i := range 12 {
		id := string(rune('a' + i))
		site.lists["123"] = append(site.lists["123"], v2.Subtitle{ID: id, TorrentID: "123", Title: id + ".chs.srt", Language: "简体中文"})
		site.files[id] = v2.SubtitleFile{Name: id + ".chs.srt", Data: []byte(id)}
	}
	e.useSubSite(site)
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Len(t, site.downloaded, siteSubDownloads, "一个种子最多下载这么多份")
	assert.Len(t, siteSubExtras(e.history()[0]), siteSubPerVideo, "一个视频最多放这么多个")
	assert.Contains(t, e.history()[0].Message, "站点字幕：还有 6 个没有放（一个视频最多放 4 个）")
}

func TestSiteSubtitlesRecordFails(t *testing.T) {
	// 字幕记不进整理记录时删掉放进去的（不然删除记录时没人认领）
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, SiteSubtitles: true})
	e.bareOpp()
	e.pushedFrom(oppHash, "nexus", "123", "")
	e.useSubSite(&fakeSubSite{
		lists: map[string][]v2.Subtitle{"123": {{ID: "1", TorrentID: "123", Title: "a.chs.srt", Language: "简体中文"}}},
		files: map[string]v2.SubtitleFile{"1": {Name: "a.chs.srt", Data: []byte(srtText)}},
	})
	// 整理记录是新建的（INSERT … ON CONFLICT），之后的改动（记字幕）都失败
	require.NoError(t, e.db.Callback().Update().Before("gorm:update").Register("test:fail-history", func(db *gorm.DB) {
		if db.Statement.Table == "media_transfer_histories" {
			_ = db.AddError(errors.New("数据库只读"))
		}
	}))
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.False(t, exists(e.oppTarget()+".zh-CN.srt"), "记不进去的字幕撤回")
	require.Len(t, res.Messages, 1)
	assert.Contains(t, res.Messages[0], "字幕记不进整理记录")
}

func TestPickSiteSubs(t *testing.T) {
	videos := []transfer.File{{Rel: "S/Show.S01E01.mkv"}, {Rel: "S/Show.S01E02.mkv"}}
	key := func(f transfer.File) string {
		switch {
		case strings.Contains(f.Rel, "E01"):
			return "1:1"
		case strings.Contains(f.Rel, "E02"):
			return "1:2"
		}
		return ""
	}
	list := []v2.Subtitle{
		{ID: "1", Title: "Show.S01E01.eng.srt", Language: "English"},
		{ID: "2", Title: "Show.S01E09.chs.srt", Language: "简体中文"},
		{ID: "3", Title: "合集.7z", Language: "繁體中文"},
		{ID: "4", Title: "某字幕组", Flag: "china.gif"},
		{ID: "5", Title: "Show.S01E02.cht.ass", Language: "繁體中文"},
		{ID: "6", Title: "S01E01.zh.srt"},
	}
	got := pickSiteSubs(list, videos, key, nil)
	ids := make([]string, 0, len(got))
	for _, g := range got {
		ids = append(ids, g.sub.ID+":"+g.lang)
	}
	// 中文的才要；对不上哪一集的不要；简体 → 繁体 → 中文
	assert.Equal(t, []string{"4:zh-CN", "3:zh-TW", "5:zh-TW", "6:zh"}, ids)

	// 对得上的那一集这次不放（以前整理过）：不下它的字幕；压缩包照下
	got = pickSiteSubs(list, videos, key, func(i int) bool { return i == 1 })
	ids = ids[:0]
	for _, g := range got {
		ids = append(ids, g.sub.ID)
	}
	assert.Equal(t, []string{"4", "3", "5"}, ids)

	// 一个视频时标题不筛；没有中文的时候都要
	got = pickSiteSubs([]v2.Subtitle{{ID: "1", Title: "x.eng.srt", Language: "English"}, {ID: "2", Title: "y.srt", Language: "Français"}}, videos[:1], key, nil)
	require.Len(t, got, 2)
	assert.Equal(t, "en", got[0].lang)
	assert.Equal(t, "", got[1].lang)
}

func TestListedLang(t *testing.T) {
	for _, c := range []struct {
		sub  v2.Subtitle
		want string
	}{
		{v2.Subtitle{Language: "简体中文"}, "zh-CN"},
		{v2.Subtitle{Language: "簡體中文"}, "zh-CN"},
		{v2.Subtitle{Language: "繁體中文"}, "zh-TW"},
		{v2.Subtitle{Language: "Traditional Chinese"}, "zh-TW"},
		{v2.Subtitle{Language: "中文"}, "zh"},
		{v2.Subtitle{Language: "English"}, "en"},
		{v2.Subtitle{Language: "日本語"}, "ja"},
		{v2.Subtitle{Language: "한국어"}, "ko"},
		{v2.Subtitle{Language: "Other", Flag: "jollyroger.gif", Title: "x.cht.srt"}, "zh-TW"},
		{v2.Subtitle{Flag: "hongkong.gif"}, "zh-TW"},
		{v2.Subtitle{Flag: "CHINA.GIF"}, "zh-CN"},
		{v2.Subtitle{Flag: "uk.gif"}, "en"},
		{v2.Subtitle{Flag: "japan.gif"}, "ja"},
		{v2.Subtitle{Flag: "southkorea.gif"}, "ko"},
		{v2.Subtitle{Title: "Movie.chs.srt"}, "zh-CN"},
		{v2.Subtitle{Language: "Français", Flag: "france.gif", Title: "a.srt"}, ""},
	} {
		assert.Equal(t, c.want, listedLang(c.sub), "%+v", c.sub)
	}
}

func TestUnpackSiteSub(t *testing.T) {
	ls := listedSub{sub: v2.Subtitle{ID: "1", Title: "标题.srt"}, lang: "zh-CN"}

	got, notes := unpackSiteSub(v2.SubtitleFile{Name: "a.chs.SRT", Data: []byte("x")}, ls)
	assert.Empty(t, notes)
	assert.Equal(t, []gotSub{{name: "a.chs.SRT", data: []byte("x"), lang: "zh-CN"}}, got)

	got, _ = unpackSiteSub(v2.SubtitleFile{Data: []byte("x")}, ls)
	assert.Equal(t, "标题.srt", got[0].name, "没有文件名时用列表上的标题")

	for data, ext := range map[string]string{
		"WEBVTT\n\n00:01.000 --> 00:02.000\nhi":      ".vtt",
		"\ufeff[Script Info]\nScriptType: v4.00+":    ".ass",
		"\r\n1\r\n00:00:01,000 --> 00:00:02,000\r\n": ".srt",
	} {
		got, notes = unpackSiteSub(v2.SubtitleFile{Name: "字幕", Data: []byte(data)}, listedSub{sub: v2.Subtitle{Title: "字幕"}})
		require.Empty(t, notes, ext)
		assert.Equal(t, "字幕"+ext, got[0].name)
	}

	// zip：只取字幕文件；太大的不要；没有字幕的写明
	big := strings.Repeat("a", siteSubFileMax+1)
	got, notes = unpackSiteSub(v2.SubtitleFile{Name: "p.zip", Data: zipOf(t, [2]string{"a.srt", "1"}, [2]string{"big.ass", big}, [2]string{"c.txt", "2"})}, ls)
	require.Len(t, got, 1)
	assert.Equal(t, gotSub{name: "a.srt", data: []byte("1"), lang: "zh-CN"}, got[0])
	assert.Equal(t, []string{"p.zip 里的 big.ass 太大，没有放"}, notes)

	_, notes = unpackSiteSub(v2.SubtitleFile{Name: "p.zip", Data: zipOf(t, [2]string{"c.txt", "2"})}, ls)
	assert.Equal(t, []string{"p.zip 里没有字幕文件"}, notes)
	// GBK 写的名字（没有 UTF-8 标记）转成 UTF-8，只留最后一段
	got, notes = unpackSiteSub(v2.SubtitleFile{Name: "p.zip", Data: zipOf(t, [2]string{gbk(t, "字幕/某剧.S01E01.繁體.srt"), "1"})}, ls)
	require.Empty(t, notes)
	require.Len(t, got, 1)
	assert.Equal(t, "某剧.S01E01.繁體.srt", got[0].name)
	_, notes = unpackSiteSub(v2.SubtitleFile{Name: "p.zip", Data: []byte("PK\x03\x04broken")}, ls)
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0], "p.zip 解不开")

	// 压缩包里的字幕太多
	var many [][2]string
	for i := range siteSubZipFiles + 3 {
		many = append(many, [2]string{strings.Repeat("x", i+1) + ".srt", "s"})
	}
	got, notes = unpackSiteSub(v2.SubtitleFile{Name: "m.zip", Data: zipOf(t, many...)}, ls)
	assert.Len(t, got, siteSubZipFiles)
	assert.Equal(t, []string{"m.zip 里的字幕太多，只取了前 50 个"}, notes)

	// 解出来的合计太大
	var heavy [][2]string
	for i := range 6 {
		heavy = append(heavy, [2]string{strings.Repeat("h", i+1) + ".ass", strings.Repeat("b", siteSubFileMax)})
	}
	got, notes = unpackSiteSub(v2.SubtitleFile{Name: "h.zip", Data: zipOf(t, heavy...)}, ls)
	assert.Len(t, got, siteSubZipTotal/siteSubFileMax)
	assert.Equal(t, []string{"h.zip 解出来的字幕太大，后面的没有放"}, notes)

	// 单个字幕文件太大
	_, notes = unpackSiteSub(v2.SubtitleFile{Name: "big.srt", Data: []byte(big)}, ls)
	assert.Equal(t, []string{"big.srt 太大，没有放"}, notes)
}
