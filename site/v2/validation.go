// Package v2 provides input validation utilities
package v2

import (
	"errors"
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Validation errors
var (
	ErrEmptyInput       = errors.New("input cannot be empty")
	ErrInputTooLong     = errors.New("input exceeds maximum length")
	ErrInvalidURL       = errors.New("invalid URL format")
	ErrInvalidEmail     = errors.New("invalid email format")
	ErrInvalidHash      = errors.New("invalid hash format")
	ErrInvalidCharacter = errors.New("input contains invalid characters")
	ErrFileTooLarge     = errors.New("file exceeds maximum size")
)

// ValidationConfig configures validation limits
type ValidationConfig struct {
	MaxStringLength  int   // Maximum string length (default: 10000)
	MaxURLLength     int   // Maximum URL length (default: 2048)
	MaxFileSizeBytes int64 // Maximum file size in bytes (default: 100MB)
	AllowHTML        bool  // Whether to allow HTML content
	StripHTML        bool  // Whether to strip HTML tags
}

// DefaultValidationConfig returns default validation configuration
func DefaultValidationConfig() ValidationConfig {
	return ValidationConfig{
		MaxStringLength:  10000,
		MaxURLLength:     2048,
		MaxFileSizeBytes: 100 * 1024 * 1024, // 100MB
		AllowHTML:        false,
		StripHTML:        true,
	}
}

// Validator provides input validation methods
type Validator struct {
	config ValidationConfig
}

// NewValidator creates a new validator with the given configuration
func NewValidator(config ValidationConfig) *Validator {
	return &Validator{config: config}
}

// ValidateString validates a string input
func (v *Validator) ValidateString(input string) (string, error) {
	if input == "" {
		return "", ErrEmptyInput
	}

	if len(input) > v.config.MaxStringLength {
		return "", ErrInputTooLong
	}

	// Check for valid UTF-8
	if !utf8.ValidString(input) {
		return "", ErrInvalidCharacter
	}

	// Sanitize if needed
	result := input
	if v.config.StripHTML && !v.config.AllowHTML {
		result = StripHTMLTags(result)
	}

	return result, nil
}

// ValidateURL validates a URL input
func (v *Validator) ValidateURL(input string) (string, error) {
	if input == "" {
		return "", ErrEmptyInput
	}

	if len(input) > v.config.MaxURLLength {
		return "", ErrInputTooLong
	}

	parsed, err := url.Parse(input)
	if err != nil {
		return "", ErrInvalidURL
	}

	// Must have scheme and host
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", ErrInvalidURL
	}

	// Only allow http and https
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrInvalidURL
	}

	return input, nil
}

// ValidateInfoHash validates a torrent info hash
func (v *Validator) ValidateInfoHash(input string) (string, error) {
	if input == "" {
		return "", ErrEmptyInput
	}

	// Normalize to lowercase
	input = strings.ToLower(input)

	// SHA1 hash is 40 hex characters
	if len(input) != 40 {
		return "", ErrInvalidHash
	}

	// Check for valid hex characters
	for _, c := range input {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", ErrInvalidHash
		}
	}

	return input, nil
}

// ValidateFileSize validates a file size
func (v *Validator) ValidateFileSize(size int64) error {
	if size <= 0 {
		return ErrEmptyInput
	}

	if size > v.config.MaxFileSizeBytes {
		return ErrFileTooLarge
	}

	return nil
}

// ValidateSearchQuery validates a search query
func (v *Validator) ValidateSearchQuery(query SearchQuery) error {
	if query.Keyword == "" {
		return ErrEmptyInput
	}

	if len(query.Keyword) > 200 {
		return ErrInputTooLong
	}

	// Sanitize keyword
	query.Keyword = SanitizeSearchKeyword(query.Keyword)

	return nil
}

// SanitizeSearchKeyword sanitizes a search keyword
func SanitizeSearchKeyword(keyword string) string {
	// Trim whitespace
	keyword = strings.TrimSpace(keyword)

	// Remove multiple spaces
	spaceRegex := regexp.MustCompile(`\s+`)
	keyword = spaceRegex.ReplaceAllString(keyword, " ")

	// Escape HTML entities
	keyword = html.EscapeString(keyword)

	return keyword
}

// StripHTMLTags removes HTML tags from a string
func StripHTMLTags(input string) string {
	// Simple regex-based HTML tag removal
	tagRegex := regexp.MustCompile(`<[^>]*>`)
	result := tagRegex.ReplaceAllString(input, "")

	// Decode HTML entities
	result = html.UnescapeString(result)

	return result
}

// SanitizeHTML sanitizes HTML content by escaping dangerous tags
func SanitizeHTML(input string) string {
	// Remove script tags
	scriptRegex := regexp.MustCompile(`(?i)<script[^>]*>[\s\S]*?</script>`)
	result := scriptRegex.ReplaceAllString(input, "")

	// Remove style tags
	styleRegex := regexp.MustCompile(`(?i)<style[^>]*>[\s\S]*?</style>`)
	result = styleRegex.ReplaceAllString(result, "")

	// Remove event handlers
	eventRegex := regexp.MustCompile(`(?i)\s+on\w+\s*=\s*["'][^"']*["']`)
	result = eventRegex.ReplaceAllString(result, "")

	// Remove javascript: URLs
	jsURLRegex := regexp.MustCompile(`(?i)javascript:`)
	result = jsURLRegex.ReplaceAllString(result, "")

	return result
}

// ValidateTorrentFile validates torrent file content
func ValidateTorrentFile(data []byte) error {
	if len(data) == 0 {
		return ErrEmptyInput
	}

	// Check for bencode format (starts with 'd')
	if data[0] != 'd' {
		return errors.New("invalid torrent file: not bencode format")
	}

	// Check for required keys
	content := string(data)
	if !strings.Contains(content, "info") {
		return errors.New("invalid torrent file: missing info dictionary")
	}

	return nil
}

// ValidateCookie validates a cookie string
func ValidateCookie(cookie string) error {
	if cookie == "" {
		return ErrEmptyInput
	}

	// Basic validation: should not contain newlines
	if strings.ContainsAny(cookie, "\r\n") {
		return ErrInvalidCharacter
	}

	return nil
}

// ValidateAPIKey validates an API key
func ValidateAPIKey(apiKey string) error {
	if apiKey == "" {
		return ErrEmptyInput
	}

	// API keys should be alphanumeric with some special characters
	validChars := regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)
	if !validChars.MatchString(apiKey) {
		return ErrInvalidCharacter
	}

	return nil
}

// DefaultValidator is the default validator instance
var DefaultValidator = NewValidator(DefaultValidationConfig())
