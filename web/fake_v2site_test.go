package web

import (
	"context"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// fakeV2Site is a minimal v2.Site used to drive torrent download/push handlers.
type fakeV2Site struct {
	id       string
	name     string
	data     []byte
	err      error
	hashData []byte
	hashErr  error
}

func (f *fakeV2Site) ID() string        { return f.id }
func (f *fakeV2Site) Name() string      { return f.name }
func (f *fakeV2Site) Kind() v2.SiteKind { return v2.SiteNexusPHP }
func (f *fakeV2Site) Login(_ context.Context, _ v2.Credentials) error {
	return nil
}

func (f *fakeV2Site) Search(_ context.Context, _ v2.SearchQuery) ([]v2.TorrentItem, error) {
	return nil, nil
}

func (f *fakeV2Site) GetUserInfo(_ context.Context) (v2.UserInfo, error) { return v2.UserInfo{}, nil }

func (f *fakeV2Site) Download(_ context.Context, _ string) ([]byte, error) {
	return f.data, f.err
}
func (f *fakeV2Site) Close() error { return nil }

func (f *fakeV2Site) DownloadWithHash(_ context.Context, _, _ string) ([]byte, error) {
	return f.hashData, f.hashErr
}

// withOrchestrator installs a CachedSearchOrchestrator holding the given sites
// for the duration of the test and restores the previous global afterwards.
func withOrchestrator(t interface{ Cleanup(func()) }, sites ...v2.Site) {
	prev := searchOrchestrator
	orch := v2.NewSearchOrchestrator(v2.SearchOrchestratorConfig{})
	for _, s := range sites {
		orch.RegisterSite(s)
	}
	searchOrchestrator = v2.NewCachedSearchOrchestrator(orch, v2.SearchCacheConfig{})
	t.Cleanup(func() { searchOrchestrator = prev })
}
