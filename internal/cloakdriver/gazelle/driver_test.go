package gazelle

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustReadFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	b, err := os.ReadFile(path)
	require.NoError(t, err, "read fixture %s", path)
	return string(b)
}

func TestGazelleDriverParse(t *testing.T) {
	t.Run("happy_last_seen_extracted", func(t *testing.T) {
		html := mustReadFixture(t, "user_info.html")
		la, err := parseGazelleUserPage(html)
		require.NoError(t, err)

		// Fixture title attribute "2026-05-18 09:00:00" — Gazelle convention
		// is server-local UTC. Per Metis EC-9 we normalise to UTC explicitly.
		want := time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC)
		assert.True(t, la.UTC().Equal(want),
			"last_access UTC mismatch: got %s want %s", la.UTC(), want)
	})
}

func TestGazelleDriverParseEmpty(t *testing.T) {
	// Paranoia mode: stats hidden — Last seen row absent.
	html := mustReadFixture(t, "user_info_empty.html")
	la, err := parseGazelleUserPage(html)
	require.Error(t, err, "empty fixture must return parse error")
	assert.True(t, la.IsZero(), "last_access must be zero on parse error")
}
