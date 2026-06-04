package sitelogin

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

type fakeDispatchSite struct {
	info       v2.UserInfo
	err        error
	panicOnGet bool
}

func (f *fakeDispatchSite) ID() string                                  { return "dispatch-fake" }
func (f *fakeDispatchSite) Name() string                                { return "Dispatch Fake" }
func (f *fakeDispatchSite) Kind() v2.SiteKind                           { return v2.SiteNexusPHP }
func (f *fakeDispatchSite) Login(context.Context, v2.Credentials) error { return nil }
func (f *fakeDispatchSite) Search(context.Context, v2.SearchQuery) ([]v2.TorrentItem, error) {
	return nil, nil
}

func (f *fakeDispatchSite) GetUserInfo(context.Context) (v2.UserInfo, error) {
	if f.panicOnGet {
		panic("simulated probe panic")
	}
	return f.info, f.err
}
func (f *fakeDispatchSite) Download(context.Context, string) ([]byte, error) { return nil, nil }
func (f *fakeDispatchSite) Close() error                                     { return nil }

func newDispatchClock() Clock {
	return NewFakeClock(time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
}

func TestProbeDispatcherDispatchMatrix(t *testing.T) {
	access := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC).Unix()
	happyInfo := v2.UserInfo{LastAccess: access, LastLogin: access}

	tests := []struct {
		name         string
		schema       v2.Schema
		err          error
		expectStatus ProbeStatus
	}{
		{"nexusphp/happy", v2.SchemaNexusPHP, nil, OK},
		{"nexusphp/session-expired", v2.SchemaNexusPHP, fmt.Errorf("login required: %w", v2.ErrSessionExpired), SESSION_EXPIRED},
		{"mtorrent/happy", v2.SchemaMTorrent, nil, OK},
		{"mtorrent/session-expired", v2.SchemaMTorrent, fmt.Errorf("api key invalid: %w", v2.ErrSessionExpired), SESSION_EXPIRED},
		{"gazelle/happy", v2.SchemaGazelle, nil, OK},
		{"gazelle/session-expired", v2.SchemaGazelle, fmt.Errorf("auth: %w", v2.ErrSessionExpired), SESSION_EXPIRED},
		{"unit3d/happy", v2.SchemaUnit3D, nil, OK},
		{"unit3d/session-expired", v2.SchemaUnit3D, fmt.Errorf("token: %w", v2.ErrSessionExpired), SESSION_EXPIRED},
	}

	clock := newDispatchClock()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			site := &fakeDispatchSite{info: happyInfo, err: tt.err}
			def := &v2.SiteDefinition{ID: "fake-" + string(tt.schema), Schema: tt.schema}

			got, err := Probe(context.Background(), def, site, clock)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.expectStatus, got.Status, "schema=%s err=%v", tt.schema, tt.err)

			if tt.expectStatus == SESSION_EXPIRED {
				require.NotNil(t, got.RawError)
				assert.True(t, errors.Is(got.RawError, v2.ErrSessionExpired), "RawError must wrap ErrSessionExpired; got %v", got.RawError)
			}
		})
	}
}

func TestProbeDispatcherUnknownSchema(t *testing.T) {
	site := &fakeDispatchSite{}
	def := &v2.SiteDefinition{ID: "fake", Schema: v2.Schema("ZZZ")}

	got, err := Probe(context.Background(), def, site, newDispatchClock())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, UNKNOWN, got.Status)
	assert.Contains(t, got.Diagnostic, "unknown schema")
}

func TestProbeDispatcherNilDef(t *testing.T) {
	site := &fakeDispatchSite{}

	got, err := Probe(context.Background(), nil, site, newDispatchClock())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, UNKNOWN, got.Status)
	assert.Contains(t, got.Diagnostic, "nil site definition")
}

func TestProbeDispatcherPanicRecovery(t *testing.T) {
	site := &fakeDispatchSite{panicOnGet: true}
	def := &v2.SiteDefinition{ID: "panicsite", Schema: v2.SchemaNexusPHP}

	got, err := Probe(context.Background(), def, site, newDispatchClock())
	require.NoError(t, err, "panic must be recovered, not propagated")
	require.NotNil(t, got)
	assert.Equal(t, UNKNOWN, got.Status)
	require.NotNil(t, got.RawError)
	assert.Contains(t, got.RawError.Error(), "panic")
}

func TestProbeDispatcherHDDolbyAndRousi(t *testing.T) {
	access := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC).Unix()
	happyInfo := v2.UserInfo{LastAccess: access, LastLogin: access}

	for _, schema := range []v2.Schema{v2.SchemaHDDolby, v2.SchemaRousi} {
		t.Run(string(schema), func(t *testing.T) {
			site := &fakeDispatchSite{info: happyInfo}
			def := &v2.SiteDefinition{ID: "fake-" + string(schema), Schema: schema}

			got, err := Probe(context.Background(), def, site, newDispatchClock())
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, OK, got.Status, "%s should route to NexusPHP probe and classify as OK", schema)
			require.NotNil(t, got.LastAccessAt, "LastAccessAt should be populated by NexusPHP probe")
			require.NotNil(t, got.LastLoginAt, "LastLoginAt should be populated by NexusPHP probe")
		})
	}
}
