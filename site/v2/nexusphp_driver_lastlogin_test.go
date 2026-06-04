package v2

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNexusPHPDriverLastLogin(t *testing.T) {
	raw, err := os.ReadFile("testdata/nexusphp_userdetails_lastlogin.html")
	require.NoError(t, err)

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(raw)))
	require.NoError(t, err)

	driver := &NexusPHPDriver{}
	res := NexusPHPResponse{Document: doc, RawBody: raw, StatusCode: 200}

	info, err := driver.ParseUserDetails(res)
	require.NoError(t, err)

	expectedLogin := time.Date(2026, 5, 15, 12, 0, 0, 0, CSTLocation).Unix()
	expectedAccess := time.Date(2026, 5, 16, 10, 0, 0, 0, CSTLocation).Unix()

	assert.Equal(t, expectedLogin, info.LastLogin, "LastLogin must match fixture")
	assert.Equal(t, expectedAccess, info.LastAccess, "LastAccess must match fixture")
}
