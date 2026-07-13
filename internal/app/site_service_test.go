// MIT License
// Copyright (c) 2025 pt-tools

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestClassOf_FallsBackToRank(t *testing.T) {
	assert.Equal(t, "VIP", classOf(v2.UserInfo{LevelName: "VIP", Rank: "R"}))
	assert.Equal(t, "R", classOf(v2.UserInfo{Rank: "R"}))
	assert.Equal(t, "", classOf(v2.UserInfo{}))
}

func TestFormatBytes_Units(t *testing.T) {
	assert.Equal(t, "512 B", formatBytes(512))
	assert.Equal(t, "1.00 KiB", formatBytes(1024))
	assert.Equal(t, "1.00 MiB", formatBytes(1024*1024))
	assert.Equal(t, "1.00 GiB", formatBytes(1024*1024*1024))
}

func TestGetSiteUserInfo_EmptyName(t *testing.T) {
	svc := NewSiteService(nil, nil)
	_, err := svc.GetSiteUserInfo(context.Background(), "  ")
	require.ErrorIs(t, err, ErrSiteNotFound)
}

func TestListSites_UserGetError(t *testing.T) {
	store := &stubSiteLister{sites: map[models.SiteGroup]models.SiteConfig{"mteam": {Enabled: boolPtr(true)}}}
	users := &stubUserInfoSource{err: v2.ErrSiteNotFound}
	svc := newSiteServiceWithDeps(store, users)
	got, err := svc.ListSites(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.True(t, got[0].LastScrapedAt.IsZero())
}

func TestGetSiteUserInfo_ListError(t *testing.T) {
	store := &stubSiteLister{err: assertGenericErr}
	svc := newSiteServiceWithDeps(store, &stubUserInfoSource{})
	_, err := svc.GetSiteUserInfo(context.Background(), "mteam")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list sites")
}

func TestGetSiteUserInfo_UsersNil(t *testing.T) {
	store := &stubSiteLister{sites: map[models.SiteGroup]models.SiteConfig{"mteam": {}}}
	svc := newSiteServiceWithDeps(store, nil)
	_, err := svc.GetSiteUserInfo(context.Background(), "mteam")
	require.ErrorIs(t, err, ErrUserInfoUnavailable)
}

func TestSiteService_GetSiteUserInfo_UsesLevelName(t *testing.T) {
	store := &stubSiteLister{sites: map[models.SiteGroup]models.SiteConfig{"mteam": {}}}
	users := &stubUserInfoSource{infos: map[string]v2.UserInfo{
		"mteam": {Site: "mteam", Username: "bob", LevelName: "Elite", Uploaded: 1 << 30, Downloaded: 1 << 20, Ratio: 2.5, Bonus: 100},
	}}
	svc := newSiteServiceWithDeps(store, users)

	dto, err := svc.GetSiteUserInfo(context.Background(), "mteam")
	require.NoError(t, err)
	assert.Equal(t, "bob", dto.Username)
	assert.Equal(t, "Elite", dto.Class)
}

func TestSiteService_GetSiteUserInfo_Errors(t *testing.T) {
	store := &stubSiteLister{sites: map[models.SiteGroup]models.SiteConfig{"mteam": {}}}
	svc := newSiteServiceWithDeps(store, nil)

	_, err := svc.GetSiteUserInfo(context.Background(), "")
	require.ErrorIs(t, err, ErrSiteNotFound)

	_, err = svc.GetSiteUserInfo(context.Background(), "unknown")
	require.ErrorIs(t, err, ErrSiteNotFound)

	// users nil -> unavailable for a known site.
	_, err = svc.GetSiteUserInfo(context.Background(), "mteam")
	require.ErrorIs(t, err, ErrUserInfoUnavailable)
}

type stubSiteLister struct {
	sites map[models.SiteGroup]models.SiteConfig
	err   error
}

func (s *stubSiteLister) ListSites() (map[models.SiteGroup]models.SiteConfig, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := map[models.SiteGroup]models.SiteConfig{}
	for k, v := range s.sites {
		out[k] = v
	}
	return out, nil
}

type stubUserInfoSource struct {
	infos map[string]v2.UserInfo
	err   error
}

func (s *stubUserInfoSource) Get(_ context.Context, site string) (v2.UserInfo, error) {
	if s.err != nil {
		return v2.UserInfo{}, s.err
	}
	info, ok := s.infos[site]
	if !ok {
		return v2.UserInfo{}, v2.ErrSiteNotFound
	}
	return info, nil
}

func boolPtr(b bool) *bool { return &b }

func TestSiteService_ListSites_RoundTrip(t *testing.T) {
	store := &stubSiteLister{
		sites: map[models.SiteGroup]models.SiteConfig{
			"mteam": {Enabled: boolPtr(true)},
			"hdsky": {Enabled: boolPtr(false)},
		},
	}
	users := &stubUserInfoSource{
		infos: map[string]v2.UserInfo{
			"mteam": {Site: "mteam", Username: "alice", LastUpdate: 1700000000},
		},
	}
	svc := newSiteServiceWithDeps(store, users)

	got, err := svc.ListSites(context.Background())

	require.NoError(t, err)
	require.Len(t, got, 2)
	byName := map[string]SiteSummaryDTO{}
	for _, s := range got {
		byName[s.Name] = s
	}
	assert.True(t, byName["mteam"].Enabled)
	assert.Equal(t, "enabled", byName["mteam"].Status)
	assert.False(t, byName["mteam"].LastScrapedAt.IsZero())
	assert.False(t, byName["hdsky"].Enabled)
	assert.Equal(t, "disabled", byName["hdsky"].Status)
	assert.True(t, byName["hdsky"].LastScrapedAt.IsZero())
}

func TestSiteService_GetUserInfo_HappyPath(t *testing.T) {
	store := &stubSiteLister{
		sites: map[models.SiteGroup]models.SiteConfig{"mteam": {Enabled: boolPtr(true)}},
	}
	users := &stubUserInfoSource{
		infos: map[string]v2.UserInfo{
			"mteam": {
				Site:       "mteam",
				Username:   "alice",
				Uploaded:   3 * 1024 * 1024 * 1024,
				Downloaded: 1024 * 1024 * 1024,
				Ratio:      3.5,
				Bonus:      12345,
				LevelName:  "Power User",
			},
		},
	}
	svc := newSiteServiceWithDeps(store, users)

	got, err := svc.GetSiteUserInfo(context.Background(), "mteam")

	require.NoError(t, err)
	assert.Equal(t, "alice", got.Username)
	assert.Equal(t, "mteam", got.SiteName)
	assert.Equal(t, "3.500", got.Ratio)
	assert.Equal(t, "12345", got.Bonus)
	assert.Equal(t, "Power User", got.Class)
	assert.Contains(t, got.Uploaded, "GiB")
	assert.Contains(t, got.Downloaded, "GiB")
}

func TestSiteService_GetUserInfo_SiteNotFound(t *testing.T) {
	store := &stubSiteLister{sites: map[models.SiteGroup]models.SiteConfig{}}
	users := &stubUserInfoSource{}
	svc := newSiteServiceWithDeps(store, users)

	_, err := svc.GetSiteUserInfo(context.Background(), "missing")

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSiteNotFound))
}

func TestSiteService_GetUserInfo_RepoMissingReturnsUnavailable(t *testing.T) {
	store := &stubSiteLister{sites: map[models.SiteGroup]models.SiteConfig{"mteam": {Enabled: boolPtr(true)}}}
	users := &stubUserInfoSource{infos: map[string]v2.UserInfo{}}
	svc := newSiteServiceWithDeps(store, users)

	_, err := svc.GetSiteUserInfo(context.Background(), "mteam")

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUserInfoUnavailable))
}

func TestSiteService_GetUserInfo_EmptyName(t *testing.T) {
	svc := newSiteServiceWithDeps(&stubSiteLister{}, &stubUserInfoSource{})

	_, err := svc.GetSiteUserInfo(context.Background(), "  ")

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSiteNotFound))
}
