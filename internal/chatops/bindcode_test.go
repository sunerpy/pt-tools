// MIT License
// Copyright (c) 2025 pt-tools

package chatops

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBindCode_Length(t *testing.T) {
	for i := 0; i < 100; i++ {
		code, err := GenerateBindCode()
		require.NoError(t, err)
		require.Len(t, code, 8, "bindcode must be exactly 8 characters")
	}
}

func TestBindCode_NoAmbiguous(t *testing.T) {
	// Ambiguous chars to exclude: 0, O, 1, l, I
	ambiguousChars := map[rune]bool{
		'0': true,
		'O': true,
		'1': true,
		'l': true,
		'I': true,
	}

	for i := 0; i < 100; i++ {
		code, err := GenerateBindCode()
		require.NoError(t, err)

		for _, ch := range code {
			require.NotContains(t, ambiguousChars, ch, "bindcode must not contain ambiguous chars: 0/O/1/l/I")
		}
	}
}

func TestBindCode_Uniqueness(t *testing.T) {
	codes := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		code, err := GenerateBindCode()
		require.NoError(t, err)
		codes[code] = true
	}

	unique := len(codes)
	require.GreaterOrEqual(t, unique, 999, "1000 generations should produce at least 999 unique codes (pigeonhole principle)")
}

func TestBindCode_AlphaNumeric(t *testing.T) {
	// Valid charset: 23456789ABCDEFGHJKMNPQRSTUVWXYZ (excluding 0,O,1,l,I)
	validPattern := regexp.MustCompile(`^[23456789ABCDEFGHJKMNPQRSTUVWXYZ]{8}$`)

	for i := 0; i < 100; i++ {
		code, err := GenerateBindCode()
		require.NoError(t, err)
		require.True(t, validPattern.MatchString(code), "bindcode %q must match pattern [A-Z0-9]{8} excluding 0/O/1/l/I", code)
	}
}

func TestGenerateBindCode_UniqueValidChars(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		code, err := GenerateBindCode()
		require.NoError(t, err)
		require.Len(t, code, 8)
		for _, c := range code {
			assert.Contains(t, bindcodeCharset, string(c), "code must only use unambiguous charset")
		}
		seen[code] = true
	}
	assert.Greater(t, len(seen), 40, "codes should be effectively unique")
}
