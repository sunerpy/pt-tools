package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
)

func TestProbeSourceDispatch_HTTPCookie(t *testing.T) {
	state := &models.SiteLoginState{}
	login := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	access := time.Date(2026, 5, 18, 10, 30, 0, 0, time.UTC)
	result := &sitelogin.ProbeResult{
		Status:       sitelogin.OK,
		Source:       sitelogin.ProbeSourceHTTPCookie,
		LastLoginAt:  &login,
		LastAccessAt: &access,
	}

	dispatchProbeTimestamps(state, result)

	if assert.NotNil(t, state.CookieLastLoginAt) {
		assert.True(t, state.CookieLastLoginAt.Equal(login))
	}
	assert.Nil(t, state.ApiLastLoginAt)
	if assert.NotNil(t, state.LastLoginAt) {
		assert.True(t, state.LastLoginAt.Equal(login))
	}
	if assert.NotNil(t, state.LastAccessAt) {
		assert.True(t, state.LastAccessAt.Equal(access))
	}
}

func TestProbeSourceDispatch_HTTPAPIKey(t *testing.T) {
	state := &models.SiteLoginState{}
	login := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	access := time.Date(2026, 5, 18, 10, 30, 0, 0, time.UTC)
	result := &sitelogin.ProbeResult{
		Status:       sitelogin.OK,
		Source:       sitelogin.ProbeSourceHTTPAPIKey,
		LastLoginAt:  &login,
		LastAccessAt: &access,
	}

	dispatchProbeTimestamps(state, result)

	if assert.NotNil(t, state.ApiLastLoginAt) {
		assert.True(t, state.ApiLastLoginAt.Equal(login))
	}
	assert.Nil(t, state.CookieLastLoginAt)
	if assert.NotNil(t, state.LastLoginAt) {
		assert.True(t, state.LastLoginAt.Equal(login))
	}
	if assert.NotNil(t, state.LastAccessAt) {
		assert.True(t, state.LastAccessAt.Equal(access))
	}
}

func TestProbeSourceDispatch_Cloak(t *testing.T) {
	state := &models.SiteLoginState{}
	login := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	result := &sitelogin.ProbeResult{
		Status:      sitelogin.OK,
		Source:      sitelogin.ProbeSourceCloak,
		LastLoginAt: &login,
	}

	dispatchProbeTimestamps(state, result)

	if assert.NotNil(t, state.CookieLastLoginAt) {
		assert.True(t, state.CookieLastLoginAt.Equal(login))
	}
	assert.Nil(t, state.ApiLastLoginAt)
}

func TestProbeSourceDispatch_AccessOnlyNoLogin(t *testing.T) {
	state := &models.SiteLoginState{}
	access := time.Date(2026, 5, 18, 10, 30, 0, 0, time.UTC)
	result := &sitelogin.ProbeResult{
		Status:       sitelogin.OK,
		Source:       sitelogin.ProbeSourceHTTPCookie,
		LastAccessAt: &access,
	}

	dispatchProbeTimestamps(state, result)

	assert.Nil(t, state.ApiLastLoginAt)
	assert.Nil(t, state.CookieLastLoginAt)
	assert.Nil(t, state.LastLoginAt)
	if assert.NotNil(t, state.LastAccessAt) {
		assert.True(t, state.LastAccessAt.Equal(access))
	}
}

func TestProbeSourceDispatch_NormalizesToUTC(t *testing.T) {
	state := &models.SiteLoginState{}
	loc8 := time.FixedZone("CST", 8*3600)
	login := time.Date(2026, 5, 18, 10, 0, 0, 0, loc8)
	result := &sitelogin.ProbeResult{
		Status:      sitelogin.OK,
		Source:      sitelogin.ProbeSourceHTTPAPIKey,
		LastLoginAt: &login,
	}

	dispatchProbeTimestamps(state, result)

	if assert.NotNil(t, state.ApiLastLoginAt) {
		assert.Equal(t, time.UTC, state.ApiLastLoginAt.Location())
		assert.Equal(t, "2026-05-18T02:00:00Z", state.ApiLastLoginAt.Format(time.RFC3339))
	}
}

func TestProbeSourceDispatch_UnknownSourceFallsBackToCookie(t *testing.T) {
	state := &models.SiteLoginState{}
	login := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	result := &sitelogin.ProbeResult{
		Status:      sitelogin.OK,
		Source:      sitelogin.ProbeSource(""),
		LastLoginAt: &login,
	}

	dispatchProbeTimestamps(state, result)

	if assert.NotNil(t, state.CookieLastLoginAt) {
		assert.True(t, state.CookieLastLoginAt.Equal(login))
	}
	assert.Nil(t, state.ApiLastLoginAt)
}
