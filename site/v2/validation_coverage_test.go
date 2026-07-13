package v2

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultValidationConfig(t *testing.T) {
	cfg := DefaultValidationConfig()
	assert.Equal(t, 10000, cfg.MaxStringLength)
	assert.Equal(t, 2048, cfg.MaxURLLength)
	assert.Equal(t, int64(100*1024*1024), cfg.MaxFileSizeBytes)
	assert.False(t, cfg.AllowHTML)
	assert.True(t, cfg.StripHTML)
}

func TestValidator_ValidateString(t *testing.T) {
	v := NewValidator(DefaultValidationConfig())

	_, err := v.ValidateString("")
	assert.ErrorIs(t, err, ErrEmptyInput)

	_, err = v.ValidateString(strings.Repeat("a", 10001))
	assert.ErrorIs(t, err, ErrInputTooLong)

	_, err = v.ValidateString("bad\xffutf8")
	assert.ErrorIs(t, err, ErrInvalidCharacter)

	// StripHTML on by default
	out, err := v.ValidateString("<b>hello</b>")
	require.NoError(t, err)
	assert.Equal(t, "hello", out)

	// AllowHTML preserves content
	v2 := NewValidator(ValidationConfig{MaxStringLength: 100, AllowHTML: true})
	out, err = v2.ValidateString("<b>hi</b>")
	require.NoError(t, err)
	assert.Equal(t, "<b>hi</b>", out)
}

func TestValidator_ValidateURL(t *testing.T) {
	v := NewValidator(DefaultValidationConfig())

	_, err := v.ValidateURL("")
	assert.ErrorIs(t, err, ErrEmptyInput)

	_, err = v.ValidateURL("http://" + strings.Repeat("a", 2048) + ".com")
	assert.ErrorIs(t, err, ErrInputTooLong)

	_, err = v.ValidateURL("://bad url with space")
	assert.Error(t, err)

	_, err = v.ValidateURL("/relative/path")
	assert.ErrorIs(t, err, ErrInvalidURL)

	_, err = v.ValidateURL("ftp://example.com")
	assert.ErrorIs(t, err, ErrInvalidURL)

	out, err := v.ValidateURL("https://example.com/path")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/path", out)
}

func TestValidator_ValidateInfoHash(t *testing.T) {
	v := NewValidator(DefaultValidationConfig())

	_, err := v.ValidateInfoHash("")
	assert.ErrorIs(t, err, ErrEmptyInput)

	_, err = v.ValidateInfoHash("tooshort")
	assert.ErrorIs(t, err, ErrInvalidHash)

	_, err = v.ValidateInfoHash(strings.Repeat("z", 40))
	assert.ErrorIs(t, err, ErrInvalidHash)

	hash := "ABCDEF0123456789ABCDEF0123456789ABCDEF01"
	out, err := v.ValidateInfoHash(hash)
	require.NoError(t, err)
	assert.Equal(t, strings.ToLower(hash), out)
}

func TestValidator_ValidateFileSize(t *testing.T) {
	v := NewValidator(ValidationConfig{MaxFileSizeBytes: 1000})
	assert.ErrorIs(t, v.ValidateFileSize(0), ErrEmptyInput)
	assert.ErrorIs(t, v.ValidateFileSize(-5), ErrEmptyInput)
	assert.ErrorIs(t, v.ValidateFileSize(2000), ErrFileTooLarge)
	assert.NoError(t, v.ValidateFileSize(500))
}

func TestValidator_ValidateSearchQuery(t *testing.T) {
	v := NewValidator(DefaultValidationConfig())
	assert.ErrorIs(t, v.ValidateSearchQuery(SearchQuery{Keyword: ""}), ErrEmptyInput)
	assert.ErrorIs(t, v.ValidateSearchQuery(SearchQuery{Keyword: strings.Repeat("x", 201)}), ErrInputTooLong)
	assert.NoError(t, v.ValidateSearchQuery(SearchQuery{Keyword: "ubuntu"}))
}

func TestSanitizeSearchKeyword(t *testing.T) {
	assert.Equal(t, "hello world", SanitizeSearchKeyword("  hello   world  "))
	assert.Equal(t, "a&amp;b", SanitizeSearchKeyword("a&b"))
}

func TestStripHTMLTags(t *testing.T) {
	assert.Equal(t, "hello", StripHTMLTags("<div><b>hello</b></div>"))
	assert.Equal(t, "a<b", StripHTMLTags("a&lt;b"))
}

func TestSanitizeHTML(t *testing.T) {
	in := `<div onclick="evil()">hi</div><script>alert(1)</script><style>x{}</style><a href="javascript:bad">link</a>`
	out := SanitizeHTML(in)
	assert.NotContains(t, out, "<script")
	assert.NotContains(t, out, "<style")
	assert.NotContains(t, out, "onclick")
	assert.NotContains(t, out, "javascript:")
	assert.Contains(t, out, "hi")
}

func TestValidateTorrentFile(t *testing.T) {
	assert.ErrorIs(t, ValidateTorrentFile(nil), ErrEmptyInput)
	assert.Error(t, ValidateTorrentFile([]byte("not bencode")))
	assert.Error(t, ValidateTorrentFile([]byte("d5:hello5:world")))
	assert.NoError(t, ValidateTorrentFile([]byte("d4:infod6:lengthi100eee")))
}

func TestValidateCookie(t *testing.T) {
	assert.ErrorIs(t, ValidateCookie(""), ErrEmptyInput)
	assert.ErrorIs(t, ValidateCookie("uid=1\nmalicious"), ErrInvalidCharacter)
	assert.NoError(t, ValidateCookie("uid=1; pass=abc"))
}

func TestValidateAPIKey(t *testing.T) {
	assert.ErrorIs(t, ValidateAPIKey(""), ErrEmptyInput)
	assert.ErrorIs(t, ValidateAPIKey("has space"), ErrInvalidCharacter)
	assert.ErrorIs(t, ValidateAPIKey("has@symbol"), ErrInvalidCharacter)
	assert.NoError(t, ValidateAPIKey("abc-123_XYZ.key"))
}

func TestDefaultValidator(t *testing.T) {
	require.NotNil(t, DefaultValidator)
	out, err := DefaultValidator.ValidateString("test")
	require.NoError(t, err)
	assert.Equal(t, "test", out)
}
