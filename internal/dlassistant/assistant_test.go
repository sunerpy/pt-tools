package dlassistant

import (
	"context"
	"errors"
	"sync/atomic"
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

	got, info, err := FindMissingSiteTags(context.Background(), f, resolver())
	require.NoError(t, err)
	assert.Equal(t, ScanInfo{Total: 5, Scanned: 5}, info)
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
		{ID: "b", InfoHash: "b", Name: "secondary", Tracker: "https://tracker.audiences.me/announce"},
		{ID: "c", InfoHash: "c", Name: "no main"},
	}
	f.trackers["a"] = []downloader.TorrentTracker{{URL: "https://old.hdsky.me/announce.php?passkey=SECRET"}, {URL: "https://old.hdsky.me/announce.php?passkey=SECRET"}}
	// 主 tracker 不含原内容，第二个 tracker 含：也要列出来
	f.trackers["b"] = []downloader.TorrentTracker{{URL: "https://tracker.audiences.me/announce"}, {URL: "https://old.hdsky.me/backup"}}
	f.trackers["c"] = []downloader.TorrentTracker{{URL: "udp://old.hdsky.me:8080/announce"}}

	_, _, err := PreviewTrackerReplace(context.Background(), f, "ol", "new")
	assert.ErrorContains(t, err, "至少 3 个字")
	_, _, err = PreviewTrackerReplace(context.Background(), f, "old.hdsky.me", "")
	assert.Error(t, err)
	_, _, err = PreviewTrackerReplace(context.Background(), f, "old.hdsky.me", "old.hdsky.me")
	assert.Error(t, err)

	got, info, err := PreviewTrackerReplace(context.Background(), f, "old.hdsky.me", "tracker.hdsky.me")
	require.NoError(t, err)
	assert.Equal(t, 3, info.Scanned)
	require.Len(t, got, 3, "重复的地址只列一次")
	byName := map[string]TrackerMatch{}
	for _, m := range got {
		byName[m.Name] = m
		assert.NotContains(t, m.Old+m.New+m.ID, "SECRET", "预览里不出现 passkey")
		assert.NotEmpty(t, m.ID)
	}
	assert.Equal(t, "https://old.hdsky.me/announce.php?passkey=***", byName["match"].Old)
	assert.Equal(t, "https://tracker.hdsky.me/announce.php?passkey=***", byName["match"].New)
	assert.Equal(t, "https://tracker.hdsky.me/backup", byName["secondary"].New)
	assert.Equal(t, "udp://tracker.hdsky.me:8080/announce", byName["no main"].New)

	sel := func(names ...string) []TrackerSelection {
		out := []TrackerSelection{}
		for _, n := range names {
			out = append(out, TrackerSelection{Hash: byName[n].Hash, ID: byName[n].ID})
		}
		return out
	}
	_, err = ApplyTrackerReplace(context.Background(), f, "old.hdsky.me", "tracker.hdsky.me", sel("match"))
	assert.ErrorIs(t, err, downloader.ErrCapabilityUnsupported)

	e := editorDL{f}
	// 预览之后 c 的地址被别人改了：指纹对不上，不改
	f.trackers["c"] = []downloader.TorrentTracker{{URL: "udp://old.hdsky.me:9090/announce"}}
	res, err := ApplyTrackerReplace(context.Background(), e, "old.hdsky.me", "tracker.hdsky.me",
		append(sel("match", "no main"), TrackerSelection{Hash: "gone", ID: "x"}))
	require.NoError(t, err)
	assert.Equal(t, 1, res.Done)
	assert.Equal(t, [][3]string{
		{"a", "https://old.hdsky.me/announce.php?passkey=SECRET", "https://tracker.hdsky.me/announce.php?passkey=SECRET"},
	}, f.edits, "只改选中且没变的地址；重复地址只改一次；passkey 保留")
	require.Len(t, res.Skipped, 2)
	assert.Contains(t, res.Skipped[1].Error+res.Skipped[0].Error, "预览之后 tracker 地址变了")
	assert.NotContains(t, f.edits, [3]string{"b", "https://old.hdsky.me/backup", "https://tracker.hdsky.me/backup"}, "没选中的不改")

	f.edits = nil
	f.editErr = errors.New("409")
	res, err = ApplyTrackerReplace(context.Background(), e, "old.hdsky.me", "tracker.hdsky.me", sel("secondary"))
	require.NoError(t, err)
	require.Len(t, res.Failed, 1)

	_, err = ReplaceTrackerURL("https://old.hdsky.me/announce", "https://old.hdsky.me", "ftp://x")
	assert.Error(t, err, "替换后必须仍是 tracker 地址")
}

// 替换后不是有效地址的那条照样列出来并写明原因，但没有指纹，不能执行。
func TestTrackerReplaceInvalidResult(t *testing.T) {
	f := newFake()
	f.torrents = []downloader.Torrent{{ID: "a", InfoHash: "a", Name: "bad"}}
	f.trackers["a"] = []downloader.TorrentTracker{{URL: "https://old.example.org/announce?passkey=SECRET"}}
	got, _, err := PreviewTrackerReplace(context.Background(), f, "https://old.example.org", "ftp://x")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Empty(t, got[0].ID, "不能勾选")
	assert.Empty(t, got[0].New)
	assert.Contains(t, got[0].Error, "不是有效的 tracker 地址")
	assert.NotContains(t, got[0].Error+got[0].Old, "SECRET")

	e := editorDL{f}
	res, err := ApplyTrackerReplace(context.Background(), e, "https://old.example.org", "ftp://x",
		[]TrackerSelection{{Hash: "a", ID: got[0].ID}})
	require.NoError(t, err)
	assert.Zero(t, res.Done)
	assert.Empty(t, f.edits)
}

// 同一种子选了两条地址，改完第一条后第二条失败：写明已经改了几条。
func TestTrackerReplacePartialFailure(t *testing.T) {
	f := newFake()
	f.torrents = []downloader.Torrent{{ID: "a", InfoHash: "a", Name: "two"}}
	f.trackers["a"] = []downloader.TorrentTracker{{URL: "https://old.example.org/1"}, {URL: "https://old.example.org/2"}}
	got, _, err := PreviewTrackerReplace(context.Background(), f, "old.example.org", "new.example.org")
	require.NoError(t, err)
	require.Len(t, got, 2)
	e := &failSecondEditor{fakeDL: f}
	res, err := ApplyTrackerReplace(context.Background(), e, "old.example.org", "new.example.org",
		[]TrackerSelection{{Hash: "a", ID: got[0].ID}, {Hash: "a", ID: got[1].ID}})
	require.NoError(t, err)
	require.Len(t, res.Failed, 1)
	assert.Contains(t, res.Failed[0].Error, "已改 1 个地址，之后失败")
}

type failSecondEditor struct {
	*fakeDL
	calls int
}

func (e *failSecondEditor) EditTracker(context.Context, string, string, string) error {
	e.calls++
	if e.calls > 1 {
		return errors.New("network down")
	}
	return nil
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
	f.trackers["dead"] = []downloader.TorrentTracker{{URL: "** [DHT] **", Status: 2}, {URL: "https://tracker.hdsky.me/announce.php?passkey=x", Status: 4, Message: "Unregistered torrent: https://tracker.hdsky.me/announce.php?passkey=TOPSECRET"}}
	f.trackers["ok"] = []downloader.TorrentTracker{{URL: "https://tracker.hdsky.me/announce.php", Status: 2}}
	f.trackers["mixed"] = []downloader.TorrentTracker{{URL: "https://a.example/announce", Status: 4, Message: "torrent not found"}, {URL: "https://b.example/announce", Status: 2}}
	f.trackers["acct"] = []downloader.TorrentTracker{{URL: "https://tracker.hdsky.me/announce.php", Status: 4, Message: "Invalid passkey"}}
	f.trackerErr["err"] = errors.New("timeout")

	got, info, err := ScanDeadTorrents(context.Background(), f, resolver())
	require.NoError(t, err)
	assert.Equal(t, ScanInfo{Total: 5, Scanned: 5}, info)
	require.Len(t, got, 1)
	assert.Equal(t, DeadTorrent{
		Hash: "dead", Name: "dead one", Size: 9, Site: "hdsky", SiteName: "HDSky", TrackerHost: "tracker.hdsky.me", Reason: DeadUnregistered,
		Message: "Unregistered torrent: https://tracker.hdsky.me/announce.php?passkey=***",
	}, got[0], "tracker 回复里的凭证也遮住")

	res, err := DeleteDeadTorrents(context.Background(), f, []string{"DEAD", "dead", "ok", "gone", "err"}, true)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Done)
	assert.Equal(t, map[string]bool{"dead": true}, f.removed)
	assert.Len(t, res.Skipped, 2, "已恢复正常的和已不在的不删")
	assert.Len(t, res.Failed, 1, "读不到状态的不删")
}

// 要删数据时，数据还被别的种子用着的（如辅种）只删种子；一起删的共用数据的种子，最后一个连数据删。
func TestDeleteDeadTorrentsKeepsSharedData(t *testing.T) {
	f := newFake()
	f.torrents = []downloader.Torrent{
		{ID: "d1", InfoHash: "d1", Name: "dead shared", ContentPath: "/d/Movie"},
		{ID: "live", InfoHash: "live", Name: "reseed", ContentPath: "/d/Movie"},
		{ID: "d2", InfoHash: "d2", Name: "dead pair a", ContentPath: "/d/Show"},
		{ID: "d3", InfoHash: "d3", Name: "dead pair b", ContentPath: "/d/Show"},
	}
	for _, id := range []string{"d1", "d2", "d3"} {
		f.trackers[id] = []downloader.TorrentTracker{{URL: "https://tracker.hdsky.me/announce.php", Status: 4, Message: "Unregistered torrent"}}
	}
	res, err := DeleteDeadTorrents(context.Background(), f, []string{"d1", "d2", "d3"}, true)
	require.NoError(t, err)
	assert.Equal(t, 3, res.Done)
	assert.Equal(t, 2, res.KeptData)
	assert.Equal(t, map[string]bool{"d1": false, "d2": false, "d3": true}, f.removed)

	f.removed = map[string]bool{}
	res, err = DeleteDeadTorrents(context.Background(), f, []string{"d1"}, false)
	require.NoError(t, err)
	assert.Zero(t, res.KeptData, "本来就不删数据")
}

func TestRedactTrackerMessage(t *testing.T) {
	assert.Equal(t, "Unregistered torrent (passkey=*** authkey=***)", RedactTrackerMessage("Unregistered torrent (passkey=abc authkey=def)"))
	assert.Equal(t, "see https://u3d.example/announce/*** now", RedactTrackerMessage("see https://u3d.example/announce/0123456789abcdef0123 now"))
	assert.Equal(t, "token ***", RedactTrackerMessage("token 0123456789abcdef0123456789"))
	assert.Equal(t, "种子不存在", RedactTrackerMessage("种子不存在"))
}

func TestLimitTorrents(t *testing.T) {
	all := make([]downloader.Torrent, MaxScanTorrents+5)
	got, info := limitTorrents(all)
	assert.Len(t, got, MaxScanTorrents)
	assert.Equal(t, ScanInfo{Total: MaxScanTorrents + 5, Scanned: MaxScanTorrents}, info)
	got, info = limitTorrents(all[:3])
	assert.Len(t, got, 3)
	assert.Equal(t, ScanInfo{Total: 3, Scanned: 3}, info)
}

// 能按 ctx 读 tracker 的下载器走可取消的接口；能一次读全部的下载器只请求一次。
type readerDL struct {
	*fakeDL
	ctxCalls  atomic.Int32 // 逐个读是并发的
	bulkCalls atomic.Int32
	bulkErr   error
}

func (r *readerDL) GetTorrentTrackersContext(ctx context.Context, id string) ([]downloader.TorrentTracker, error) {
	r.ctxCalls.Add(1)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return r.trackers[id], nil
}

func (r *readerDL) GetAllTorrentTrackers(context.Context) (map[string][]downloader.TorrentTracker, error) {
	r.bulkCalls.Add(1)
	if r.bulkErr != nil {
		return nil, r.bulkErr
	}
	return r.trackers, nil
}

func TestTrackerReaders(t *testing.T) {
	f := newFake()
	f.torrents = []downloader.Torrent{{ID: "a", InfoHash: "a", Name: "dead"}, {ID: "b", InfoHash: "b", Name: "untagged"}}
	f.trackers["a"] = []downloader.TorrentTracker{{URL: "https://tracker.hdsky.me/a", Status: 4, Message: "Torrent not found"}}
	f.trackers["b"] = []downloader.TorrentTracker{{URL: "https://tracker.hdsky.me/b", Status: 2}}
	r := &readerDL{fakeDL: f}

	dead, _, err := ScanDeadTorrents(context.Background(), r, resolver())
	require.NoError(t, err)
	require.Len(t, dead, 1)
	assert.EqualValues(t, 1, r.bulkCalls.Load(), "一次读全部")
	assert.Zero(t, r.ctxCalls.Load())

	tags, _, err := FindMissingSiteTags(context.Background(), r, resolver())
	require.NoError(t, err)
	assert.Len(t, tags, 2)
	assert.EqualValues(t, 2, r.bulkCalls.Load())

	r.bulkErr = errors.New("too big")
	_, _, err = ScanDeadTorrents(context.Background(), r, resolver())
	require.NoError(t, err)
	assert.EqualValues(t, 2, r.ctxCalls.Load(), "一次读失败时退回逐个读")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = getTrackers(ctx, r, "a")
	assert.ErrorIs(t, err, context.Canceled)
	_, _, err = ScanDeadTorrents(ctx, r, resolver())
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRedactTrackerURL(t *testing.T) {
	assert.Equal(t, "https://t.example/announce.php?passkey=***", RedactTrackerURL("https://t.example/announce.php?passkey=abc"))
	assert.Equal(t, "https://t.example/announce?credential=***", RedactTrackerURL("https://t.example/announce?credential=abc"))
	assert.Equal(t, "https://u3d.example/announce/***", RedactTrackerURL("https://u3d.example/announce/0123456789abcdef0123"))
	assert.Equal(t, "https://t.example/announce", RedactTrackerURL("https://t.example/announce"))
}
