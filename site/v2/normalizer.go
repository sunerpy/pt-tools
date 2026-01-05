package v2

import (
	"regexp"
	"strings"
)

// Normalizer standardizes torrent titles and tags
type Normalizer struct {
	resolutionPatterns map[string]*regexp.Regexp
	encodingPatterns   map[string]*regexp.Regexp
	formatPatterns     map[string]*regexp.Regexp
	sitePrefixPattern  *regexp.Regexp
}

// NewNormalizer creates a new Normalizer
func NewNormalizer() *Normalizer {
	n := &Normalizer{
		resolutionPatterns: make(map[string]*regexp.Regexp),
		encodingPatterns:   make(map[string]*regexp.Regexp),
		formatPatterns:     make(map[string]*regexp.Regexp),
	}

	// Resolution patterns (case-insensitive)
	n.resolutionPatterns["2160p"] = regexp.MustCompile(`(?i)\b(2160p|4k|uhd)\b`)
	n.resolutionPatterns["1080p"] = regexp.MustCompile(`(?i)\b(1080p|1080i)\b`)
	n.resolutionPatterns["720p"] = regexp.MustCompile(`(?i)\b720p\b`)
	n.resolutionPatterns["480p"] = regexp.MustCompile(`(?i)\b(480p|sd)\b`)

	// Encoding patterns
	n.encodingPatterns["H.264"] = regexp.MustCompile(`(?i)\b(h\.?264|x264|avc)\b`)
	n.encodingPatterns["H.265"] = regexp.MustCompile(`(?i)\b(h\.?265|x265|hevc)\b`)
	n.encodingPatterns["AV1"] = regexp.MustCompile(`(?i)\bav1\b`)
	n.encodingPatterns["VP9"] = regexp.MustCompile(`(?i)\bvp9\b`)

	// Format patterns
	n.formatPatterns["BluRay"] = regexp.MustCompile(`(?i)\b(blu-?ray|bdrip|bdremux)\b`)
	n.formatPatterns["WEB-DL"] = regexp.MustCompile(`(?i)\b(web-?dl|webdl)\b`)
	n.formatPatterns["WEBRip"] = regexp.MustCompile(`(?i)\bwebrip\b`)
	n.formatPatterns["HDTV"] = regexp.MustCompile(`(?i)\bhdtv\b`)
	n.formatPatterns["DVDRip"] = regexp.MustCompile(`(?i)\b(dvdrip|dvd-?r)\b`)

	// Site prefix pattern (matches common site tags like [HDSky], [CHDBits], etc.)
	n.sitePrefixPattern = regexp.MustCompile(`^\s*\[[^\]]+\]\s*`)

	return n
}

// NormalizeTitle standardizes a torrent title
func (n *Normalizer) NormalizeTitle(title string) string {
	// Remove site prefixes
	title = n.sitePrefixPattern.ReplaceAllString(title, "")

	// Normalize resolution
	for standard, pattern := range n.resolutionPatterns {
		if pattern.MatchString(title) {
			title = pattern.ReplaceAllString(title, standard)
			break
		}
	}

	// Normalize encoding
	for standard, pattern := range n.encodingPatterns {
		if pattern.MatchString(title) {
			title = pattern.ReplaceAllString(title, standard)
			break
		}
	}

	// Normalize format
	for standard, pattern := range n.formatPatterns {
		if pattern.MatchString(title) {
			title = pattern.ReplaceAllString(title, standard)
			break
		}
	}

	// Clean up whitespace
	title = strings.TrimSpace(title)
	title = regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")

	return title
}

// ExtractResolution extracts the resolution from a title
func (n *Normalizer) ExtractResolution(title string) string {
	for standard, pattern := range n.resolutionPatterns {
		if pattern.MatchString(title) {
			return standard
		}
	}
	return ""
}

// ExtractEncoding extracts the encoding from a title
func (n *Normalizer) ExtractEncoding(title string) string {
	for standard, pattern := range n.encodingPatterns {
		if pattern.MatchString(title) {
			return standard
		}
	}
	return ""
}

// ExtractFormat extracts the format from a title
func (n *Normalizer) ExtractFormat(title string) string {
	for standard, pattern := range n.formatPatterns {
		if pattern.MatchString(title) {
			return standard
		}
	}
	return ""
}

// NormalizeTags standardizes a list of tags
func (n *Normalizer) NormalizeTags(tags []string) []string {
	normalized := make([]string, 0, len(tags))
	seen := make(map[string]bool)

	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		tag = strings.ToLower(tag)

		// Skip empty or duplicate tags
		if tag == "" || seen[tag] {
			continue
		}

		seen[tag] = true
		normalized = append(normalized, tag)
	}

	return normalized
}
