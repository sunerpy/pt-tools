package sitelogin

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sitev2 "github.com/sunerpy/pt-tools/site/v2"
)

type fakeGazelleSite struct {
	info sitev2.UserInfo
	err  error
}

func (f *fakeGazelleSite) ID() string            { return "gazelle-fake" }
func (f *fakeGazelleSite) Name() string          { return "Gazelle Fake" }
func (f *fakeGazelleSite) Kind() sitev2.SiteKind { return sitev2.SiteGazelle }
func (f *fakeGazelleSite) Login(context.Context, sitev2.Credentials) error {
	return nil
}

func (f *fakeGazelleSite) Search(context.Context, sitev2.SearchQuery) ([]sitev2.TorrentItem, error) {
	return nil, nil
}

func (f *fakeGazelleSite) GetUserInfo(context.Context) (sitev2.UserInfo, error) {
	return f.info, f.err
}
func (f *fakeGazelleSite) Download(context.Context, string) ([]byte, error) { return nil, nil }
func (f *fakeGazelleSite) Close() error                                     { return nil }

func TestProbeGazelleHappy(t *testing.T) {
	access := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC).Unix()
	site := &fakeGazelleSite{info: sitev2.UserInfo{LastAccess: access}}
	clock := NewFakeClock(time.Now())

	got, err := ProbeGazelle(context.Background(), site, clock)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, OK, got.Status)
	require.NotNil(t, got.LastAccessAt)
	assert.Equal(t, access, got.LastAccessAt.Unix())
	assert.Nil(t, got.LastLoginAt, "Gazelle has no last_login concept; LastLoginAt must be nil")
}

func TestProbeGazelleParseErrorWhenLastAccessMissing(t *testing.T) {
	site := &fakeGazelleSite{info: sitev2.UserInfo{Username: "x", LastAccess: 0}}
	got, err := ProbeGazelle(context.Background(), site, NewFakeClock(time.Now()))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, PARSE_ERROR, got.Status)
	assert.Nil(t, got.LastAccessAt)
	assert.Nil(t, got.LastLoginAt)
}

func TestProbeGazelleSessionExpired(t *testing.T) {
	site := &fakeGazelleSite{err: fmt.Errorf("wrap: %w", sitev2.ErrSessionExpired)}
	got, err := ProbeGazelle(context.Background(), site, NewFakeClock(time.Now()))
	require.NoError(t, err)
	assert.Equal(t, SESSION_EXPIRED, got.Status)
}

func TestProbeGazelleInvalidCredentials(t *testing.T) {
	site := &fakeGazelleSite{err: sitev2.ErrInvalidCredentials}
	got, err := ProbeGazelle(context.Background(), site, NewFakeClock(time.Now()))
	require.NoError(t, err)
	assert.Equal(t, SESSION_EXPIRED, got.Status)
}

func TestProbeGazelleCircuitOpen(t *testing.T) {
	site := &fakeGazelleSite{err: fmt.Errorf("rate boom: %w", sitev2.ErrCircuitOpen)}
	got, err := ProbeGazelle(context.Background(), site, NewFakeClock(time.Now()))
	require.NoError(t, err)
	assert.Equal(t, RATE_LIMITED, got.Status)
}

func TestProbeGazelleRateLimited(t *testing.T) {
	site := &fakeGazelleSite{err: sitev2.ErrRateLimited}
	got, err := ProbeGazelle(context.Background(), site, NewFakeClock(time.Now()))
	require.NoError(t, err)
	assert.Equal(t, RATE_LIMITED, got.Status)
}

func TestProbeGazelleNetworkError(t *testing.T) {
	site := &fakeGazelleSite{err: context.DeadlineExceeded}
	got, err := ProbeGazelle(context.Background(), site, NewFakeClock(time.Now()))
	require.NoError(t, err)
	assert.Equal(t, NETWORK_ERROR, got.Status)
}

func TestProbeGazelleChallenge(t *testing.T) {
	site := &fakeGazelleSite{err: errors.New("blocked by Cloudflare challenge")}
	got, err := ProbeGazelle(context.Background(), site, NewFakeClock(time.Now()))
	require.NoError(t, err)
	assert.Equal(t, CHALLENGE, got.Status)
}

func TestProbeGazelleParseError(t *testing.T) {
	site := &fakeGazelleSite{err: fmt.Errorf("bad json: %w", sitev2.ErrParseError)}
	got, err := ProbeGazelle(context.Background(), site, NewFakeClock(time.Now()))
	require.NoError(t, err)
	assert.Equal(t, PARSE_ERROR, got.Status)
}

func TestProbeGazelleKeyError(t *testing.T) {
	site := &fakeGazelleSite{err: errors.New("invalid api key supplied")}
	got, err := ProbeGazelle(context.Background(), site, NewFakeClock(time.Now()))
	require.NoError(t, err)
	assert.Equal(t, KEY_ERROR, got.Status)
}

func TestProbeGazelleUnknown(t *testing.T) {
	site := &fakeGazelleSite{err: errors.New("totally novel failure")}
	got, err := ProbeGazelle(context.Background(), site, NewFakeClock(time.Now()))
	require.NoError(t, err)
	assert.Equal(t, UNKNOWN, got.Status)
}

func TestProbeGazelleNilSite(t *testing.T) {
	got, err := ProbeGazelle(context.Background(), nil, NewFakeClock(time.Now()))
	require.Error(t, err)
	assert.Equal(t, UNKNOWN, got.Status)
}
