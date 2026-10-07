package meta

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// corpusCase 是一条语料：expect 里写了的字段才核对（写成 false、0、"" 也核对）。
type corpusCase struct {
	Title    string         `json:"title"`
	Subtitle string         `json:"subtitle"`
	Expect   map[string]any `json:"expect"`
}

func loadCorpus(t testing.TB) []corpusCase {
	t.Helper()
	b, err := os.ReadFile("testdata/corpus.json")
	require.NoError(t, err)
	var cases []corpusCase
	require.NoError(t, json.Unmarshal(b, &cases))
	return cases
}

// 语料里每一条都要解析对；发现的新误判补进 testdata/corpus.json。
func TestParseCorpus(t *testing.T) {
	for _, c := range loadCorpus(t) {
		t.Run(c.Title, func(t *testing.T) {
			got := Parse(c.Title, c.Subtitle)
			raw, err := json.Marshal(got)
			require.NoError(t, err)
			var m map[string]any
			require.NoError(t, json.Unmarshal(raw, &m))
			for k, want := range c.Expect {
				have, ok := m[k]
				if !ok {
					have = zeroLike(want)
				}
				if wl, isList := want.([]any); isList {
					assert.ElementsMatch(t, wl, toList(have), "%s：%s", k, c.Title)
					continue
				}
				assert.Equal(t, want, have, "%s：%s", k, c.Title)
			}
		})
	}
}

func zeroLike(v any) any {
	switch v.(type) {
	case bool:
		return false
	case float64:
		return float64(0)
	case string:
		return ""
	case []any:
		return []any{}
	}
	return nil
}

func toList(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return []any{}
}

func TestParseBasics(t *testing.T) {
	m := Parse("", "")
	assert.Equal(t, Meta{}, m)
	assert.Equal(t, TypeTV, Parse("Show.S01E02.1080p", "").Type)
	assert.Equal(t, TypeMovie, Parse("Movie.2020.1080p", "").Type)
	assert.Equal(t, TypeUnknown, Parse("Something 1080p", "").Type)
	assert.Equal(t, "Movie 2020", Parse("Movie.2020.1080p", "").String())
	assert.Equal(t, "Show S01E02", Parse("Show.S01E02.1080p", "").String())
	assert.Equal(t, "Show S01E01-E03", Parse("Show.S01E01-E03.1080p", "").String())
	assert.Equal(t, "Show S02", Parse("Show.S02.1080p", "").String())
}

func TestChineseNumber(t *testing.T) {
	for in, want := range map[string]int{"一": 1, "十": 10, "十二": 12, "二十": 20, "二十三": 23, "九十九": 99, "12": 12, "": 0, "百": 0} {
		assert.Equal(t, want, chineseNumber(in), in)
	}
}

// 任何输入都不能 panic，解析结果要自洽。
func FuzzParse(f *testing.F) {
	for _, c := range loadCorpus(f) {
		f.Add(c.Title, c.Subtitle)
	}
	f.Add("[]【】 - - S99E9999 第九十九季 全9999集 9999p", "| / 【")
	f.Fuzz(func(t *testing.T, title, subtitle string) {
		m := Parse(title, subtitle)
		if m.Year != 0 && (m.Year < 1900 || m.Year > maxYear()) {
			t.Fatalf("year out of range: %d", m.Year)
		}
		if m.Season < 0 || m.Episode < 0 || m.EpisodeEnd < 0 || m.SeasonEnd < 0 || m.TotalEpisodes < 0 {
			t.Fatalf("negative numbers: %+v", m)
		}
		for _, s := range []string{m.NameCN, m.NameEN, m.Group, m.String()} {
			if !utf8.ValidString(s) {
				t.Fatalf("invalid utf8: %q", s)
			}
		}
		if !slices.Contains([]MediaType{TypeUnknown, TypeMovie, TypeTV}, m.Type) {
			t.Fatalf("bad type %q", m.Type)
		}
	})
}
