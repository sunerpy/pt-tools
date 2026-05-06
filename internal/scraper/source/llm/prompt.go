package llm

import (
	"fmt"
	"sort"
	"strings"
)

const systemPromptText = `You are a metadata extraction specialist for private tracker media releases.

RULES (strict):
1. Extract ONLY from input. Do NOT invent IDs, years, or titles.
2. If any field is uncertain, OMIT it (not "unknown" or 0).
3. tmdb_id / imdb_id: set ONLY if the input literally contains them.
4. Strip release tags: 2160p, 1080p, x265, HEVC, WEB-DL, BluRay, -GROUP, etc.
5. Respond with JSON only. No prose or markdown.`

func buildSystemPrompt() string {
	return systemPromptText
}

func buildUserPrompt(req ExtractRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Input title: %s\n", req.RawTitle)
	if req.Filename != "" {
		fmt.Fprintf(&b, "Filename: %s\n", req.Filename)
	}
	if req.FileSize > 0 {
		fmt.Fprintf(&b, "File size bytes: %d\n", req.FileSize)
	}
	if req.MediaType != "" {
		fmt.Fprintf(&b, "Media type hint: %s\n", req.MediaType)
	}
	if len(req.SiteHints) > 0 {
		keys := make([]string, 0, len(req.SiteHints))
		for k := range req.SiteHints {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("Site hints:\n")
		for _, k := range keys {
			fmt.Fprintf(&b, "  %s: %s\n", k, req.SiteHints[k])
		}
	}
	if req.UserContext != "" {
		fmt.Fprintf(&b, "\nAdditional context:\n%s\n", req.UserContext)
	}
	if req.Language != "" {
		fmt.Fprintf(&b, "\nTarget language for title/plot: %s\n", req.Language)
	}
	b.WriteString("\nExtract metadata as JSON matching the schema.")
	return b.String()
}
