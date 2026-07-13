package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestParseTimeInCST covers the previously-uncovered ParseTimeInCST helper.
func TestParseTimeInCST(t *testing.T) {
	got, err := ParseTimeInCST("2006-01-02 15:04:05", "2024-05-01 12:00:00")
	require.NoError(t, err)
	// CST is UTC+8, so the parsed wall-clock time offset should be 8h.
	_, offset := got.Zone()
	require.Equal(t, 8*3600, offset)
	require.Equal(t, 2024, got.Year())
	require.Equal(t, time.Month(5), got.Month())

	// Invalid layout/value should surface a parse error.
	_, err = ParseTimeInCST("2006-01-02", "not-a-date")
	require.Error(t, err)
}
