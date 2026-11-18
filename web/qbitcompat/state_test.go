package qbitcompat

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 状态值：后端是 qB 时用它原样的（5.x 的 stopped* 换回 paused*）；别的下载器按状态与速度换算
func TestQBState(t *testing.T) {
	cases := []struct {
		name string
		t    downloader.Torrent
		want string
	}{
		{"qB 5 的 stoppedDL", downloader.Torrent{Raw: map[string]any{"state": "stoppedDL"}}, "pausedDL"},
		{"qB 5 的 stoppedUP", downloader.Torrent{Raw: map[string]any{"state": "stoppedUP"}}, "pausedUP"},
		{"qB 原样", downloader.Torrent{Raw: map[string]any{"state": "forcedUP"}}, "forcedUP"},
		{"下载中有速度", downloader.Torrent{State: downloader.TorrentDownloading, DownloadSpeed: 1}, "downloading"},
		{"下载中没速度", downloader.Torrent{State: downloader.TorrentDownloading}, "stalledDL"},
		{"做种有速度", downloader.Torrent{State: downloader.TorrentSeeding, UploadSpeed: 1}, "uploading"},
		{"做种没速度", downloader.Torrent{State: downloader.TorrentSeeding}, "stalledUP"},
		{"暂停、没下完", downloader.Torrent{State: downloader.TorrentPaused, Progress: 0.5}, "pausedDL"},
		{"停止、下完了", downloader.Torrent{State: downloader.TorrentStopped, IsCompleted: true}, "pausedUP"},
		{"排队、没下完", downloader.Torrent{State: downloader.TorrentQueued}, "queuedDL"},
		{"排队、下完了", downloader.Torrent{State: downloader.TorrentQueued, Progress: 1}, "queuedUP"},
		{"校验、没下完", downloader.Torrent{State: downloader.TorrentChecking}, "checkingDL"},
		{"校验、下完了", downloader.Torrent{State: downloader.TorrentChecking, Progress: 1}, "checkingUP"},
		{"出错", downloader.Torrent{State: downloader.TorrentError}, "error"},
		{"不认识", downloader.Torrent{State: "weird"}, "unknown"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, qbState(c.t), c.name)
	}
}

// torrents/info 的 filter：按 qB 的分组；不认识的当成 all
func TestMatchFilter(t *testing.T) {
	q := func(state string, dl, up int64) qbTorrent { return qbTorrent{State: state, Dlspeed: dl, Upspeed: up} }
	cases := []struct {
		filter string
		yes    []qbTorrent
		no     []qbTorrent
	}{
		{"downloading", []qbTorrent{q("downloading", 0, 0), q("stalledDL", 0, 0), q("pausedDL", 0, 0)}, []qbTorrent{q("uploading", 0, 0)}},
		{"seeding", []qbTorrent{q("uploading", 0, 0), q("stalledUP", 0, 0)}, []qbTorrent{q("pausedUP", 0, 0)}},
		{"completed", []qbTorrent{q("pausedUP", 0, 0), q("uploading", 0, 0)}, []qbTorrent{q("downloading", 0, 0)}},
		{"paused", []qbTorrent{q("pausedDL", 0, 0)}, []qbTorrent{q("downloading", 0, 0)}},
		{"stopped", []qbTorrent{q("pausedUP", 0, 0)}, []qbTorrent{q("uploading", 0, 0)}},
		{"resumed", []qbTorrent{q("uploading", 0, 0)}, []qbTorrent{q("pausedUP", 0, 0)}},
		{"running", []qbTorrent{q("stalledDL", 0, 0)}, []qbTorrent{q("pausedDL", 0, 0)}},
		{"active", []qbTorrent{q("stalledUP", 0, 5), q("downloading", 0, 0)}, []qbTorrent{q("stalledUP", 0, 0)}},
		{"inactive", []qbTorrent{q("stalledUP", 0, 0)}, []qbTorrent{q("stalledDL", 3, 0)}},
		{"stalled", []qbTorrent{q("stalledUP", 0, 0), q("stalledDL", 0, 0)}, []qbTorrent{q("uploading", 0, 0)}},
		{"stalled_uploading", []qbTorrent{q("stalledUP", 0, 0)}, []qbTorrent{q("stalledDL", 0, 0)}},
		{"stalled_downloading", []qbTorrent{q("stalledDL", 0, 0)}, []qbTorrent{q("stalledUP", 0, 0)}},
		{"checking", []qbTorrent{q("checkingUP", 0, 0), q("checkingResumeData", 0, 0)}, []qbTorrent{q("uploading", 0, 0)}},
		{"moving", []qbTorrent{q("moving", 0, 0)}, []qbTorrent{q("uploading", 0, 0)}},
		{"errored", []qbTorrent{q("error", 0, 0), q("missingFiles", 0, 0)}, []qbTorrent{q("uploading", 0, 0)}},
		{"no-such-filter", []qbTorrent{q("error", 0, 0), q("uploading", 0, 0)}, nil},
	}
	for _, c := range cases {
		for _, x := range c.yes {
			assert.Truef(t, matchFilter(x, c.filter), "%s 应当包含 %s", c.filter, x.State)
		}
		for _, x := range c.no {
			assert.Falsef(t, matchFilter(x, c.filter), "%s 不应包含 %s", c.filter, x.State)
		}
	}
}

// 后端 qB 原样给的字段照传（自动管理、私有、分享率上限、首尾块优先）；没有 info hash 时用下载器的编号；
// 按字符串字段排序不分大小写；tag 为空筛出没有标签的
func TestTorrentsInfoRawFieldsAndSort(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	e.dl.torrents[0].Raw = map[string]any{"auto_tmm": true, "isPrivate": false, "max_ratio": 1.5, "f_l_piece_prio": true}
	e.dl.torrents = append(e.dl.torrents, downloader.Torrent{ID: "ABCDEF", Name: "alpha", State: downloader.TorrentSeeding, Tags: "x"})

	items := decode[[]map[string]any](t, e.do(http.MethodGet, "/api/v2/torrents/info", url.Values{"sort": {"name"}}, ck))
	require.Len(t, items, 4)
	names := make([]any, 0, len(items))
	for _, it := range items {
		names = append(names, it["name"])
	}
	assert.Equal(t, []any{"alpha", "debian-8.1.0-amd64-CD-1.iso", "Dune.Part.Two.2024.2160p", "tr-torrent"}, names, "按名字排序，不分大小写")
	assert.Equal(t, "abcdef", items[0]["hash"], "没有 info hash：用下载器的编号（小写）")
	debian := items[1]
	assert.Equal(t, true, debian["auto_tmm"])
	assert.Equal(t, false, debian["isPrivate"])
	assert.EqualValues(t, 1.5, debian["max_ratio"])
	assert.Equal(t, true, debian["f_l_piece_prio"])

	untagged := decode[[]map[string]any](t, e.do(http.MethodGet, "/api/v2/torrents/info", url.Values{"tag": {""}}, ck))
	hashes := make([]any, 0, len(untagged))
	for _, it := range untagged {
		hashes = append(hashes, it["hash"])
	}
	assert.ElementsMatch(t, []any{hashDebian, hashTR}, hashes, "tag 为空：没有标签的")
}

// app 的几个接口：buildInfo；默认保存目录取后端给的第一个非空目录，读不到时是空的；开了限速时 transfer/info 报限速
func TestAppEndpoints(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	info := decode[map[string]any](t, e.do(http.MethodGet, "/api/v2/app/buildInfo", nil, ck))
	assert.Contains(t, info, "libtorrent")

	e.dl.paths = []string{"", "/data"}
	assert.Equal(t, "/data", e.do(http.MethodGet, "/api/v2/app/defaultSavePath", nil, ck).Body.String())
	e.dl.paths = nil
	assert.Empty(t, e.do(http.MethodGet, "/api/v2/app/defaultSavePath", nil, ck).Body.String(), "没有目录")

	e.dl.limit = downloader.SpeedLimit{LimitEnabled: true, DownloadLimit: 100, UploadLimit: 200}
	tr := decode[map[string]any](t, e.do(http.MethodGet, "/api/v2/transfer/info", nil, ck))
	assert.EqualValues(t, 100, tr["dl_rate_limit"])
	assert.EqualValues(t, 200, tr["up_rate_limit"])
}
