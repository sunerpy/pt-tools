package dlassistant

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// fakeDL 只实现助手用到的方法；其余方法走嵌入的 nil 接口（调到就 panic，说明用错了方法）。
type fakeDL struct {
	downloader.Downloader
	torrents   []downloader.Torrent
	trackers   map[string][]downloader.TorrentTracker
	trackerErr map[string]error
	tagsSet    map[string]string
	removed    map[string]bool
	edits      [][3]string
	editErr    error
	setTagsErr error
}

func (f *fakeDL) GetAllTorrents() ([]downloader.Torrent, error) { return f.torrents, nil }

func (f *fakeDL) GetTorrentTrackers(id string) ([]downloader.TorrentTracker, error) {
	if err := f.trackerErr[id]; err != nil {
		return nil, err
	}
	return f.trackers[id], nil
}

func (f *fakeDL) SetTorrentTags(id, tags string) error {
	if f.setTagsErr != nil {
		return f.setTagsErr
	}
	f.tagsSet[id] = tags
	return nil
}

func (f *fakeDL) RemoveTorrent(id string, removeData bool) error {
	f.removed[id] = removeData
	return nil
}

type editorDL struct{ *fakeDL }

func (e editorDL) EditTracker(_ context.Context, hash, oldURL, newURL string) error {
	if e.editErr != nil {
		return e.editErr
	}
	e.edits = append(e.edits, [3]string{hash, oldURL, newURL})
	return nil
}

func newFake() *fakeDL {
	return &fakeDL{trackers: map[string][]downloader.TorrentTracker{}, trackerErr: map[string]error{}, tagsSet: map[string]string{}, removed: map[string]bool{}}
}

func resolver() *v2.TrackerResolver {
	return v2.NewTrackerResolverFrom(
		&v2.SiteDefinition{ID: "hdsky", Name: "HDSky", Schema: v2.SchemaNexusPHP, URLs: []string{"https://hdsky.me/"}},
		&v2.SiteDefinition{ID: "audiences", Name: "Audiences", Schema: v2.SchemaNexusPHP, URLs: []string{"https://audiences.me/"}},
	)
}

func TestFindAndApplySiteTags(t *testing.T) {
	f := newFake()
	f.torrents = []downloader.Torrent{
		{ID: "a", InfoHash: "A", Name: "already", Tags: "hdsky", Tracker: "https://tracker.hdsky.me/announce.php?passkey=1"},
		{ID: "b", InfoHash: "b", Name: "missing", Tags: "4k", Tracker: "https://tracker.hdsky.me/announce.php?passkey=1"},
		{ID: "c", InfoHash: "c", Name: "category", Category: "AUDIENCES", Tracker: "https://t.audiences.me/announce"},
		{ID: "d", InfoHash: "d", Name: "no main tracker"},
		{ID: "e", InfoHash: "e", Name: "unknown", Tracker: "https://tracker.unknown.net/announce"},
	}
	f.trackers["d"] = []downloader.TorrentTracker{{URL: "** [DHT] **", Status: 2}, {URL: "https://t.audiences.me/announce?passkey=2", Status: 4}}

	got, err := FindMissingSiteTags(context.Background(), f, resolver())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, SiteTagSuggestion{Hash: "d", Name: "no main tracker", Site: "audiences", SiteName: "Audiences", TrackerHost: "t.audiences.me"}, got[0])
	assert.Equal(t, "b", got[1].Hash)
	assert.Equal(t, "HDSky", got[1].SiteName)
	assert.Equal(t, "tracker.hdsky.me", got[1].TrackerHost)

	res, err := ApplySiteTags(context.Background(), f, resolver(), []SiteTagItem{
		{Hash: "B", Site: "hdsky"},
		{Hash: "d", Site: "audiences"},
		{Hash: "a", Site: "hdsky"},
		{Hash: "e", Site: "hdsky"},
		{Hash: "gone", Site: "hdsky"},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Done)
	assert.Equal(t, map[string]string{"b": "4k,hdsky", "d": "audiences"}, f.tagsSet, "保留原有标签再加站点名")
	assert.Len(t, res.Skipped, 3)
	assert.Empty(t, res.Failed)

	f.setTagsErr = errors.New("boom")
	f.tagsSet = map[string]string{}
	res, err = ApplySiteTags(context.Background(), f, resolver(), []SiteTagItem{{Hash: "b", Site: "hdsky"}})
	require.NoError(t, err)
	require.Len(t, res.Failed, 1)
	assert.Contains(t, res.Failed[0].Error, "boom")
}

func TestTrackerReplace(t *testing.T) {
	f := newFake()
	f.torrents = []downloader.Torrent{
		{ID: "a", InfoHash: "a", Name: "match", Tracker: "https://old.hdsky.me/announce.php?passkey=SECRET"},
		{ID: "b", InfoHash: "b", Name: "other", Tracker: "https://tracker.audiences.me/announce"},
		{ID: "c", InfoHash: "c", Name: "no main"},
	}
	f.trackers["a"] = []downloader.TorrentTracker{{URL: "https://old.hdsky.me/announce.php?passkey=SECRET"}}
	f.trackers["c"] = []downloader.TorrentTracker{{URL: "udp://old.hdsky.me:8080/announce"}}

	_, err := PreviewTrackerReplace(context.Background(), f, "ol", "new")
	assert.ErrorContains(t, err, "至少 3 个字")
	_, err = PreviewTrackerReplace(context.Background(), f, "old.hdsky.me", "")
	assert.Error(t, err)
	_, err = PreviewTrackerReplace(context.Background(), f, "old.hdsky.me", "old.hdsky.me")
	assert.Error(t, err)

	got, err := PreviewTrackerReplace(context.Background(), f, "old.hdsky.me", "tracker.hdsky.me")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, TrackerMatch{Hash: "a", Name: "match", Old: "https://old.hdsky.me/announce.php?passkey=***", New: "https://tracker.hdsky.me/announce.php?passkey=***"}, got[0])
	assert.Equal(t, "udp://tracker.hdsky.me:8080/announce", got[1].New)
	for _, m := range got {
		assert.NotContains(t, m.Old+m.New, "SECRET", "预览里不出现 passkey")
	}

	_, err = ApplyTrackerReplace(context.Background(), f, "old.hdsky.me", "tracker.hdsky.me", []string{"a"})
	assert.ErrorIs(t, err, downloader.ErrCapabilityUnsupported)

	e := editorDL{f}
	res, err := ApplyTrackerReplace(context.Background(), e, "old.hdsky.me", "tracker.hdsky.me", []string{"A", "b", "c", "gone"})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Done)
	assert.Equal(t, [][3]string{
		{"a", "https://old.hdsky.me/announce.php?passkey=SECRET", "https://tracker.hdsky.me/announce.php?passkey=SECRET"},
		{"c", "udp://old.hdsky.me:8080/announce", "udp://tracker.hdsky.me:8080/announce"},
	}, f.edits, "替换用下载器里的原地址，passkey 保留")
	assert.Len(t, res.Skipped, 2)

	f.editErr = errors.New("409")
	res, err = ApplyTrackerReplace(context.Background(), e, "old.hdsky.me", "tracker.hdsky.me", []string{"a"})
	require.NoError(t, err)
	require.Len(t, res.Failed, 1)

	_, err = ReplaceTrackerURL("https://old.hdsky.me/announce", "https://old.hdsky.me", "ftp://x")
	assert.Error(t, err, "替换后必须仍是 tracker 地址")
}

func TestClassifyTrackerMessage(t *testing.T) {
	cases := map[string]DeadReason{
		"Unregistered torrent":                     "unregistered",
		"torrent not registered with this tracker": "unregistered",
		"种子未注册":                                    "unregistered",
		"Torrent not found":                        "not_found",
		"该种子不存在":                                   "not_found",
		"种子已被删除":                                   "not_found",
		"Unregistered passkey":                     "",
		"user not registered":                      "",
		"Your ratio is too low":                    "",
		"timed out":                                "",
		"":                                         "",
	}
	for msg, want := range cases {
		got, ok := classifyTrackerMessage(msg)
		assert.Equal(t, want, got, msg)
		assert.Equal(t, want != "", ok, msg)
	}
}

func TestScanAndDeleteDeadTorrents(t *testing.T) {
	f := newFake()
	f.torrents = []downloader.Torrent{
		{ID: "dead", InfoHash: "dead", Name: "dead one", TotalSize: 9},
		{ID: "ok", InfoHash: "ok", Name: "works"},
		{ID: "mixed", InfoHash: "mixed", Name: "one tracker works"},
		{ID: "acct", InfoHash: "acct", Name: "passkey problem"},
		{ID: "err", InfoHash: "err", Name: "cannot read"},
	}
	f.trackers["dead"] = []downloader.TorrentTracker{{URL: "** [DHT] **", Status: 2}, {URL: "https://tracker.hdsky.me/announce.php?passkey=x", Status: 4, Message: "Unregistered torrent"}}
	f.trackers["ok"] = []downloader.TorrentTracker{{URL: "https://tracker.hdsky.me/announce.php", Status: 2}}
	f.trackers["mixed"] = []downloader.TorrentTracker{{URL: "https://a.example/announce", Status: 4, Message: "torrent not found"}, {URL: "https://b.example/announce", Status: 2}}
	f.trackers["acct"] = []downloader.TorrentTracker{{URL: "https://tracker.hdsky.me/announce.php", Status: 4, Message: "Invalid passkey"}}
	f.trackerErr["err"] = errors.New("timeout")

	got, err := ScanDeadTorrents(context.Background(), f, resolver())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, DeadTorrent{Hash: "dead", Name: "dead one", Size: 9, Site: "hdsky", SiteName: "HDSky", TrackerHost: "tracker.hdsky.me", Reason: DeadUnregistered, Message: "Unregistered torrent"}, got[0])

	res, err := DeleteDeadTorrents(context.Background(), f, []string{"DEAD", "ok", "gone", "err"}, true)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Done)
	assert.Equal(t, map[string]bool{"dead": true}, f.removed)
	assert.Len(t, res.Skipped, 2, "已恢复正常的和已不在的不删")
	assert.Len(t, res.Failed, 1, "读不到状态的不删")
}

func TestRedactTrackerURL(t *testing.T) {
	assert.Equal(t, "https://t.example/announce.php?passkey=***", RedactTrackerURL("https://t.example/announce.php?passkey=abc"))
	assert.Equal(t, "https://t.example/announce?credential=***", RedactTrackerURL("https://t.example/announce?credential=abc"))
	assert.Equal(t, "https://u3d.example/announce/***", RedactTrackerURL("https://u3d.example/announce/0123456789abcdef0123"))
	assert.Equal(t, "https://t.example/announce", RedactTrackerURL("https://t.example/announce"))
}
