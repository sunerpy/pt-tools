package nfo

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEscape verifies XML entity encoding.
func TestEscape(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ampersand",
			input:    "Foo & Bar",
			expected: "Foo &amp; Bar",
		},
		{
			name:     "less_than",
			input:    "<tag>",
			expected: "&lt;tag&gt;",
		},
		{
			name:     "double_quote",
			input:    `"quoted"`,
			expected: "&#34;quoted&#34;",
		},
		{
			name:     "apostrophe",
			input:    "O'Brien",
			expected: "O&#39;Brien",
		},
		{
			name:     "no_change",
			input:    "Hello",
			expected: "Hello",
		},
		{
			name:     "mixed",
			input:    `The <quick> "brown" & 'fast'`,
			expected: "The &lt;quick&gt; &#34;brown&#34; &amp; &#39;fast&#39;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escape(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestWriteElement tests single-line element writing.
func TestWriteElement(t *testing.T) {
	tests := []struct {
		name     string
		tagName  string
		value    string
		expected string
	}{
		{
			name:     "simple_element",
			tagName:  "title",
			value:    "Inception",
			expected: "  <title>Inception</title>\n",
		},
		{
			name:     "empty_element_self_close",
			tagName:  "title",
			value:    "",
			expected: "  <title/>\n",
		},
		{
			name:     "escaped_ampersand",
			tagName:  "plot",
			value:    "A & B",
			expected: "  <plot>A &amp; B</plot>\n",
		},
		{
			name:     "escaped_angle_brackets",
			tagName:  "title",
			value:    "<Movie>",
			expected: "  <title>&lt;Movie&gt;</title>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := newXMLWriter(&buf)
			w.depth = 1
			err := w.writeElement(tt.tagName, tt.value)
			require.NoError(t, err)
			w.w.Flush()

			result := buf.String()
			assert.Contains(t, result, tt.expected)
		})
	}
}

// TestWriteElementAttr tests attributes.
func TestWriteElementAttr(t *testing.T) {
	tests := []struct {
		name     string
		tagName  string
		attrs    []string
		value    string
		contains string
	}{
		{
			name:     "single_attr",
			tagName:  "rating",
			attrs:    []string{"name", "imdb"},
			value:    "8.5",
			contains: `<rating name="imdb">8.5</rating>`,
		},
		{
			name:     "multiple_attrs",
			tagName:  "rating",
			attrs:    []string{"name", "imdb", "max", "10"},
			value:    "8.5",
			contains: `<rating name="imdb" max="10">8.5</rating>`,
		},
		{
			name:     "empty_with_attr_self_close",
			tagName:  "rating",
			attrs:    []string{"name", "imdb"},
			value:    "",
			contains: `<rating name="imdb"/>`,
		},
		{
			name:     "attr_value_escaped",
			tagName:  "tag",
			attrs:    []string{"data", "A & B"},
			value:    "test",
			contains: `<tag data="A &amp; B">test</tag>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := newXMLWriter(&buf)
			w.depth = 1
			err := w.writeElementAttr(tt.tagName, tt.attrs, tt.value)
			require.NoError(t, err)
			w.w.Flush()

			result := buf.String()
			assert.Contains(t, result, tt.contains)
		})
	}
}

// TestWriteBlock tests nested block structure with indentation.
func TestWriteBlock(t *testing.T) {
	var buf bytes.Buffer
	w := newXMLWriter(&buf)

	err := w.writeBlock("movie", func() error {
		if err := w.writeElement("title", "T"); err != nil {
			return err
		}
		return w.writeBlock("ratings", func() error {
			return w.writeElementAttr("rating", []string{"name", "imdb"}, "8.5")
		})
	})

	require.NoError(t, err)
	w.w.Flush()

	result := buf.String()

	expected := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<movie>
  <title>T</title>
  <ratings>
    <rating name="imdb">8.5</rating>
  </ratings>
</movie>
`
	assert.Equal(t, expected, result)
}

// TestWriteComment tests comment writing.
func TestWriteComment(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		contains string
	}{
		{
			name:     "simple_comment",
			text:     "This is a comment",
			contains: "<!-- This is a comment -->",
		},
		{
			name:     "double_dash_escape",
			text:     "foo -- bar",
			contains: "<!-- foo - - bar -->",
		},
		{
			name:     "multiple_dashes",
			text:     "a -- b -- c",
			contains: "<!-- a - - b - - c -->",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := newXMLWriter(&buf)
			w.depth = 1
			err := w.writeComment(tt.text)
			require.NoError(t, err)
			w.w.Flush()

			result := buf.String()
			assert.Contains(t, result, tt.contains)
		})
	}
}

// TestWriteRaw tests raw unescaped write.
func TestWriteRaw(t *testing.T) {
	var buf bytes.Buffer
	w := newXMLWriter(&buf)
	w.depth = 0

	jsonPayload := `{"key":"value"}`
	err := w.writeRaw(jsonPayload)
	require.NoError(t, err)
	w.w.Flush()

	result := buf.String()
	assert.Contains(t, result, jsonPayload)
	assert.Contains(t, result, "key")
}

// TestElementInt tests integer convenience method.
func TestElementInt(t *testing.T) {
	tests := []struct {
		name     string
		value    int
		contains string
	}{
		{
			name:     "positive",
			value:    2010,
			contains: "<year>2010</year>",
		},
		{
			name:     "zero",
			value:    0,
			contains: "<year>0</year>",
		},
		{
			name:     "negative",
			value:    -5,
			contains: "<year>-5</year>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := newXMLWriter(&buf)
			w.depth = 1
			err := w.writeElementInt("year", tt.value)
			require.NoError(t, err)
			w.w.Flush()

			result := buf.String()
			assert.Contains(t, result, tt.contains)
		})
	}
}

// TestElementFloat tests float convenience method with 1 decimal.
func TestElementFloat(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		contains string
	}{
		{
			name:     "single_decimal",
			value:    8.5,
			contains: "<rating>8.5</rating>",
		},
		{
			name:     "whole_number_with_decimal",
			value:    8.0,
			contains: "<rating>8.0</rating>",
		},
		{
			name:     "rounding",
			value:    8.55,
			contains: "<rating>8.6</rating>",
		},
		{
			name:     "zero",
			value:    0.0,
			contains: "<rating>0.0</rating>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := newXMLWriter(&buf)
			w.depth = 1
			err := w.writeElementFloat("rating", tt.value)
			require.NoError(t, err)
			w.w.Flush()

			result := buf.String()
			assert.Contains(t, result, tt.contains)
		})
	}
}

// TestXmllintValid validates XML using xmllint if available.
// Skips if xmllint not found in PATH.
func TestXmllintValid(t *testing.T) {
	_, err := exec.LookPath("xmllint")
	if err != nil {
		t.Skip("xmllint not found in PATH, skipping validation")
	}

	var buf bytes.Buffer
	w := newXMLWriter(&buf)

	err = w.writeBlock("movie", func() error {
		if err1 := w.writeElement("title", "Test Movie"); err1 != nil {
			return err1
		}
		if err1 := w.writeElement("year", "2020"); err1 != nil {
			return err1
		}
		if err1 := w.writeElement("plot", "A test & movie"); err1 != nil {
			return err1
		}
		return w.writeBlock("ratings", func() error {
			return w.writeElementAttr("rating", []string{"name", "imdb", "max", "10"}, "8.5")
		})
	})

	require.NoError(t, err)
	w.w.Flush()

	tmpFile := filepath.Join(os.TempDir(), "test-nfo-xmllint.xml")
	defer os.Remove(tmpFile)

	err = os.WriteFile(tmpFile, buf.Bytes(), 0o644)
	require.NoError(t, err)

	cmd := exec.Command("xmllint", "--noout", tmpFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("xmllint output: %s", string(output))
		t.Fatalf("xmllint validation failed: %v", err)
	}
}

// TestComplexNFOStructure tests a realistic NFO with multiple element types.
func TestComplexNFOStructure(t *testing.T) {
	var buf bytes.Buffer
	w := newXMLWriter(&buf)

	err := w.writeBlock("movie", func() error {
		if err := w.writeElement("title", "Inception"); err != nil {
			return err
		}
		if err := w.writeElement("originaltitle", "Inception"); err != nil {
			return err
		}
		if err := w.writeElementInt("year", 2010); err != nil {
			return err
		}
		if err := w.writeElement("plot", "A thief who steals corporate secrets"); err != nil {
			return err
		}
		if err := w.writeBlock("ratings", func() error {
			if err := w.writeElementAttr("rating", []string{"name", "imdb", "max", "10"}, "8.8"); err != nil {
				return err
			}
			return w.writeElementAttr("rating", []string{"name", "tmdb", "max", "10"}, "8.4")
		}); err != nil {
			return err
		}
		if err := w.writeBlock("uniqueids", func() error {
			if err := w.writeElementAttr("uniqueid", []string{"type", "imdb", "default", "true"}, "tt1375666"); err != nil {
				return err
			}
			return w.writeElementAttr("uniqueid", []string{"type", "tmdb"}, "tt27205")
		}); err != nil {
			return err
		}
		return nil
	})

	require.NoError(t, err)
	w.w.Flush()

	result := buf.String()

	assert.Contains(t, result, "<movie>")
	assert.Contains(t, result, "<title>Inception</title>")
	assert.Contains(t, result, "<year>2010</year>")
	assert.Contains(t, result, `<rating name="imdb" max="10">8.8</rating>`)
	assert.Contains(t, result, `<rating name="tmdb" max="10">8.4</rating>`)
	assert.Contains(t, result, `<uniqueid type="imdb" default="true">tt1375666</uniqueid>`)
	assert.Contains(t, result, `<uniqueid type="tmdb">tt27205</uniqueid>`)
	assert.Contains(t, result, "</movie>")
	assert.Contains(t, result, "<?xml version")
}
