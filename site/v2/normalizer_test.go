package v2

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewNormalizer(t *testing.T) {
	n := NewNormalizer()
	assert.NotNil(t, n)
	assert.NotNil(t, n.resolutionPatterns)
	assert.NotNil(t, n.encodingPatterns)
	assert.NotNil(t, n.formatPatterns)
}

func TestNormalizer_NormalizeTitle_Resolution(t *testing.T) {
	n := NewNormalizer()

	tests := []struct {
		input    string
		expected string
	}{
		{"Movie 1080P BluRay", "Movie 1080p BluRay"},
		{"Movie 1080p BluRay", "Movie 1080p BluRay"},
		{"Movie 1080i BluRay", "Movie 1080p BluRay"},
		{"Movie 720P WEB-DL", "Movie 720p WEB-DL"},
		{"Movie 2160P HDR", "Movie 2160p HDR"},
		{"Movie 4K HDR", "Movie 2160p HDR"},
		{"Movie UHD BluRay", "Movie 2160p BluRay"},
		{"Movie 480P DVDRip", "Movie 480p DVDRip"},
		{"Movie SD DVDRip", "Movie 480p DVDRip"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := n.NormalizeTitle(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizer_NormalizeTitle_Encoding(t *testing.T) {
	n := NewNormalizer()

	tests := []struct {
		input    string
		expected string
	}{
		{"Movie x264 BluRay", "Movie H.264 BluRay"},
		{"Movie H264 BluRay", "Movie H.264 BluRay"},
		{"Movie h.264 BluRay", "Movie H.264 BluRay"},
		{"Movie AVC BluRay", "Movie H.264 BluRay"},
		{"Movie x265 BluRay", "Movie H.265 BluRay"},
		{"Movie H265 BluRay", "Movie H.265 BluRay"},
		{"Movie HEVC BluRay", "Movie H.265 BluRay"},
		{"Movie AV1 WEB-DL", "Movie AV1 WEB-DL"},
		{"Movie VP9 WEB-DL", "Movie VP9 WEB-DL"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := n.NormalizeTitle(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizer_NormalizeTitle_Format(t *testing.T) {
	n := NewNormalizer()

	tests := []struct {
		input    string
		expected string
	}{
		{"Movie BluRay 1080p", "Movie BluRay 1080p"},
		{"Movie Blu-Ray 1080p", "Movie BluRay 1080p"},
		{"Movie BDRip 1080p", "Movie BluRay 1080p"},
		{"Movie BDRemux 1080p", "Movie BluRay 1080p"},
		{"Movie WEB-DL 1080p", "Movie WEB-DL 1080p"},
		{"Movie WEBDL 1080p", "Movie WEB-DL 1080p"},
		{"Movie WebRip 1080p", "Movie WEBRip 1080p"},
		{"Movie HDTV 720p", "Movie HDTV 720p"},
		{"Movie DVDRip 480p", "Movie DVDRip 480p"},
		{"Movie DVD-R 480p", "Movie DVDRip 480p"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := n.NormalizeTitle(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizer_NormalizeTitle_SitePrefix(t *testing.T) {
	n := NewNormalizer()

	tests := []struct {
		input    string
		expected string
	}{
		{"[HDSky] Movie 1080p", "Movie 1080p"},
		{"[CHDBits] Movie 1080p", "Movie 1080p"},
		{"[M-Team] Movie 1080p", "Movie 1080p"},
		{"  [Site]  Movie 1080p", "Movie 1080p"},
		{"Movie 1080p", "Movie 1080p"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := n.NormalizeTitle(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizer_NormalizeTitle_Whitespace(t *testing.T) {
	n := NewNormalizer()

	tests := []struct {
		input    string
		expected string
	}{
		{"  Movie  1080p  ", "Movie 1080p"},
		{"Movie   1080p", "Movie 1080p"},
		{"Movie\t1080p", "Movie 1080p"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := n.NormalizeTitle(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizer_ExtractResolution(t *testing.T) {
	n := NewNormalizer()

	tests := []struct {
		input    string
		expected string
	}{
		{"Movie 1080p BluRay", "1080p"},
		{"Movie 720p WEB-DL", "720p"},
		{"Movie 2160p UHD", "2160p"},
		{"Movie 4K HDR", "2160p"},
		{"Movie 480p DVDRip", "480p"},
		{"Movie BluRay", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := n.ExtractResolution(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizer_ExtractEncoding(t *testing.T) {
	n := NewNormalizer()

	tests := []struct {
		input    string
		expected string
	}{
		{"Movie x264 BluRay", "H.264"},
		{"Movie x265 BluRay", "H.265"},
		{"Movie AV1 WEB-DL", "AV1"},
		{"Movie VP9 WEB-DL", "VP9"},
		{"Movie BluRay", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := n.ExtractEncoding(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizer_ExtractFormat(t *testing.T) {
	n := NewNormalizer()

	tests := []struct {
		input    string
		expected string
	}{
		{"Movie BluRay 1080p", "BluRay"},
		{"Movie WEB-DL 1080p", "WEB-DL"},
		{"Movie WEBRip 1080p", "WEBRip"},
		{"Movie HDTV 720p", "HDTV"},
		{"Movie DVDRip 480p", "DVDRip"},
		{"Movie 1080p", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := n.ExtractFormat(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizer_NormalizeTags(t *testing.T) {
	n := NewNormalizer()

	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "basic normalization",
			input:    []string{"Action", "COMEDY", "drama"},
			expected: []string{"action", "comedy", "drama"},
		},
		{
			name:     "remove duplicates",
			input:    []string{"action", "Action", "ACTION"},
			expected: []string{"action"},
		},
		{
			name:     "trim whitespace",
			input:    []string{"  action  ", "comedy"},
			expected: []string{"action", "comedy"},
		},
		{
			name:     "remove empty",
			input:    []string{"action", "", "  ", "comedy"},
			expected: []string{"action", "comedy"},
		},
		{
			name:     "empty input",
			input:    []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := n.NormalizeTags(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
