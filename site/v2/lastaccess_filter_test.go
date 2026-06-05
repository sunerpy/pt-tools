package v2

import (
	"fmt"
	"testing"
	"time"
)

func TestLastAccessFilter(t *testing.T) {
	filters := []Filter{
		{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
		{Name: "parseTime"},
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load Asia/Shanghai location: %v", err)
	}

	tests := []struct {
		name      string
		input     string
		timestamp string
	}{
		{
			name:      "ourbits prefixed last access",
			input:     "网站访问: 2026-05-15 02:49:47 (7时1分前)",
			timestamp: "2026-05-15 02:49:47",
		},
		{
			name:      "plain nexusphp last access",
			input:     "2026-06-03 22:57:56 (< 1分前)",
			timestamp: "2026-06-03 22:57:56",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := time.ParseInLocation("2006-01-02 15:04:05", tt.timestamp, loc)
			if err != nil {
				t.Fatalf("parse expected timestamp: %v", err)
			}

			got := ApplyFilters(tt.input, filters)
			expected := fmt.Sprintf("%d", parsed.Unix())
			if fmt.Sprintf("%v", got) != expected {
				t.Fatalf("ApplyFilters(%q) = %v, want %s", tt.input, got, expected)
			}
		})
	}
}
