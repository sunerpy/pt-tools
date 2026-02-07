package definitions

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/require"
)

// --- Fixture Suite Registry ---

type FixtureSuite struct {
	SiteID   string
	Search   func(*testing.T)
	Detail   func(*testing.T)
	UserInfo func(*testing.T)
}

var fixtureRegistry = map[string]FixtureSuite{}

func RegisterFixtureSuite(s FixtureSuite) {
	if s.SiteID == "" {
		panic("RegisterFixtureSuite: empty SiteID")
	}
	if _, dup := fixtureRegistry[s.SiteID]; dup {
		panic("RegisterFixtureSuite: duplicate SiteID " + s.SiteID)
	}
	fixtureRegistry[s.SiteID] = s
}

// --- Secret Detection ---

var bearerTokenPattern = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9\-_.]{32,}`)

var secretDenyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)c_secure_(uid|pass|login|tracker_ssl)\s*=`),
	regexp.MustCompile(`(?i)phpsessid\s*=`),
	regexp.MustCompile(`(?i)(passkey|apikey|api_key)\s*=\s*[a-f0-9]{32,64}\b`),
	bearerTokenPattern,
}

func RequireNoSecrets(t *testing.T, name, data string) {
	t.Helper()
	for _, re := range secretDenyPatterns {
		if loc := re.FindStringIndex(data); loc != nil {
			match := data[loc[0]:loc[1]]
			if re == bearerTokenPattern {
				upper := strings.ToUpper(match)
				if strings.Contains(upper, "BEARER FAKE_") || strings.Contains(upper, "BEARER TEST_") {
					continue
				}
			}
			start := loc[0]
			end := loc[1]
			if start > 20 {
				start = loc[0] - 20
			}
			if end+20 < len(data) {
				end = loc[1] + 20
			}
			t.Fatalf("fixture %q contains suspected credential: ...%s...\nMatched pattern: %s",
				name, data[start:end], re.String())
		}
	}
}

// --- Fixture Decode Helpers ---

func DecodeFixtureJSON[T any](t *testing.T, name, raw string) T {
	t.Helper()
	RequireNoSecrets(t, name, raw)
	var v T
	require.NoError(t, json.Unmarshal([]byte(raw), &v), "decode fixture %q", name)
	return v
}

func FixtureDoc(t *testing.T, name, html string) *goquery.Document {
	t.Helper()
	RequireNoSecrets(t, name, html)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	require.NoError(t, err, "parse fixture %q", name)
	return doc
}
