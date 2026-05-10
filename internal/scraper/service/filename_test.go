package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ParsedName
	}{
		{
			name:     "movie matrix with group",
			input:    `The Matrix (1999) 1080p BluRay x264-GROUP.mkv`,
			expected: ParsedName{Title: "The Matrix", Year: 1999, Quality: "1080p", Source: "BluRay", Codec: "x264", Group: "GROUP", Extension: "mkv"},
		},
		{
			name:     "movie inception dots",
			input:    `Inception.2010.1080p.BluRay.x265.HEVC.mkv`,
			expected: ParsedName{Title: "Inception", Year: 2010, Quality: "1080p", Source: "BluRay", Codec: "x265", Extension: "mkv"},
		},
		{
			name:     "movie chinese english mixed title",
			input:    `肖申克的救赎.The.Shawshank.Redemption.1994.BluRay.1080p.mkv`,
			expected: ParsedName{Title: "肖申克的救赎 The Shawshank Redemption", Year: 1994, Quality: "1080p", Source: "BluRay", Extension: "mkv"},
		},
		{
			name:     "movie 4k hdr atmos",
			input:    `Interstellar 2014 4K HDR Atmos.mkv`,
			expected: ParsedName{Title: "Interstellar", Year: 2014, Quality: "4K", Extension: "mkv"},
		},
		{
			name:     "movie webdl dv hevc group",
			input:    `Parasite.2019.WEB-DL.2160p.DV.HEVC-GROUP.mkv`,
			expected: ParsedName{Title: "Parasite", Year: 2019, Quality: "2160p", Source: "WEB-DL", Codec: "HEVC", Group: "GROUP", Extension: "mkv"},
		},
		{
			name:     "show sxxexx pilot",
			input:    `Breaking.Bad.S01E01.Pilot.1080p.BluRay.x264.mkv`,
			expected: ParsedName{Title: "Breaking Bad Pilot", Season: 1, Episode: 1, IsShow: true, Quality: "1080p", Source: "BluRay", Codec: "x264", Extension: "mkv"},
		},
		{
			name:     "show nxn format",
			input:    `Game.of.Thrones.1x05.mkv`,
			expected: ParsedName{Title: "Game of Thrones", Season: 1, Episode: 5, IsShow: true, Extension: "mkv"},
		},
		{
			name:     "show dashed title and episode",
			input:    `Attack on Titan - S04E29 - The Final Chapters.mkv`,
			expected: ParsedName{Title: "Attack on Titan The Final Chapters", Season: 4, Episode: 29, IsShow: true, Extension: "mkv"},
		},
		{
			name:     "show remux hdr",
			input:    `The.Mandalorian.S02E08.REMUX.2160p.HDR.mkv`,
			expected: ParsedName{Title: "The Mandalorian", Season: 2, Episode: 8, IsShow: true, Quality: "2160p", Source: "Remux", Extension: "mkv"},
		},
		{
			name:     "show hmax webdl group",
			input:    `Succession.S01E01.Celebration.1080p.HMAX.WEB-DL.DDP5.1.x264-NTb.mkv`,
			expected: ParsedName{Title: "Succession Celebration", Season: 1, Episode: 1, IsShow: true, Quality: "1080p", Source: "WEB-DL", Codec: "x264", Group: "NTb", Extension: "mkv"},
		},
		{
			name:     "movie extended yts group",
			input:    `The.Lord.of.the.Rings.The.Fellowship.of.the.Ring.Extended.2001.1080p.BluRay.x264.AAC5.1-[YTS.AG].mkv`,
			expected: ParsedName{Title: "The Lord of the Rings The Fellowship of the Ring", Year: 2001, Quality: "1080p", Source: "BluRay", Codec: "x264", Group: "YTS.AG", Extension: "mkv"},
		},
		{
			name:     "movie oppenheimer noisy",
			input:    `Oppenheimer.2023.IMAX.2160p.UHD.BluRay.HDR.DV.TrueHD.7.1.Atmos.x265-CMRG.mkv`,
			expected: ParsedName{Title: "Oppenheimer", Year: 2023, Quality: "2160p", Source: "BluRay", Codec: "x265", Group: "CMRG", Extension: "mkv"},
		},
		{
			name:     "movie no year",
			input:    `Big.Buck.Bunny.mkv`,
			expected: ParsedName{Title: "Big Buck Bunny", Extension: "mkv"},
		},
		{
			name:     "movie first valid year",
			input:    `My.2023.Film.2023.mkv`,
			expected: ParsedName{Title: "My Film 2023", Year: 2023, Extension: "mkv"},
		},
		{
			name:     "show episode only",
			input:    `E01 - Pilot.mkv`,
			expected: ParsedName{Title: "Pilot", Episode: 1, IsShow: true, Extension: "mkv"},
		},
		{
			name:     "show chinese season",
			input:    `庆余年.第二季.S02E10.1080p.WEB-DL.x265.mkv`,
			expected: ParsedName{Title: "庆余年 第二季", Season: 2, Episode: 10, IsShow: true, Quality: "1080p", Source: "WEB-DL", Codec: "x265", Extension: "mkv"},
		},
		{
			name:     "movie spaces and source",
			input:    `The Dark Knight 2008 1080p BluRay.mkv`,
			expected: ParsedName{Title: "The Dark Knight", Year: 2008, Quality: "1080p", Source: "BluRay", Extension: "mkv"},
		},
		{
			name:     "movie simple year",
			input:    `Movie.2023.mkv`,
			expected: ParsedName{Title: "Movie", Year: 2023, Extension: "mkv"},
		},
		{
			name:     "anime bracket group and numeric episode",
			input:    `[Group] Anime Title - 01 [1080p].mkv`,
			expected: ParsedName{Title: "Anime Title", Episode: 1, IsShow: true, Quality: "1080p", Extension: "mkv"},
		},
		{
			name:     "movie dir cut group",
			input:    `Apollo.13.1995.Dir.Cut.1080p.BluRay.x264.DTS-FGT.mkv`,
			expected: ParsedName{Title: "Apollo 13", Year: 1995, Quality: "1080p", Source: "BluRay", Codec: "x264", Group: "FGT", Extension: "mkv"},
		},
		{
			name:     "show season episode words",
			input:    `Show Name Season 1 Episode 2 1080p WEB-DL.mkv`,
			expected: ParsedName{Title: "Show Name", Season: 1, Episode: 2, IsShow: true, Quality: "1080p", Source: "WEB-DL", Extension: "mkv"},
		},
		{
			name:     "movie path basename and lowercase ext",
			input:    `/downloads/Alien.1979.1080p.BluRay.x264-FGT.MKV`,
			expected: ParsedName{Title: "Alien", Year: 1979, Quality: "1080p", Source: "BluRay", Codec: "x264", Group: "FGT", Extension: "mkv"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFilename(tt.input)
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, tt.expected.Title, got.Title)
			require.Equal(t, tt.expected.Year, got.Year)
			require.Equal(t, tt.expected.Season, got.Season)
			require.Equal(t, tt.expected.Episode, got.Episode)
			require.Equal(t, tt.expected.Quality, got.Quality)
			require.Equal(t, tt.expected.Source, got.Source)
			require.Equal(t, tt.expected.Codec, got.Codec)
			require.Equal(t, tt.expected.Group, got.Group)
			require.Equal(t, tt.expected.IsShow, got.IsShow)
			require.Equal(t, tt.expected.Extension, got.Extension)
		})
	}
}

func TestParseFilename_Empty(t *testing.T) {
	got, err := ParseFilename("")
	require.Error(t, err)
	require.Nil(t, got)
}
