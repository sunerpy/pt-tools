package v2

import (
	"testing"
	"time"
)

func TestNexusPHPDiscountTimeTimezone(t *testing.T) {
	for _, tc := range []struct {
		name, offset, input, want string
	}{
		{"site timezone", "+0800", "2026-10-26 18:46:52", "2026-10-26T10:46:52Z"},
		{"negative offset", "-0500", "2026-10-26 18:46:52", "2026-10-26T23:46:52Z"},
		{"explicit timezone", "+0800", "2026-10-26T18:46:52+02:00", "2026-10-26T16:46:52Z"},
		{"no timezone", "", "2026-10-26 18:46:52", "2026-10-26T18:46:52Z"},
		{"invalid timezone", "invalid", "2026-10-26 18:46:52", "2026-10-26T18:46:52Z"},
		{"missing time", "+0800", "", ""},
		{"invalid time", "+0800", "unknown", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &NexusPHPDriver{siteDefinition: &SiteDefinition{TimezoneOffset: tc.offset}}
			got := d.parseDiscountTime(tc.input)
			if tc.want == "" {
				if !got.IsZero() {
					t.Fatalf("expected unknown expiry, got %v", got)
				}
				return
			}
			if got.UTC().Format(time.RFC3339) != tc.want {
				t.Fatalf("got %v, want %s", got, tc.want)
			}
		})
	}
}
