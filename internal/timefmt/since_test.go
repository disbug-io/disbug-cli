package timefmt

import (
	"testing"
	"time"
)

func TestParseSinceAcceptsSecondsMinutesHoursAndCombinations(t *testing.T) {
	tests := map[string]time.Duration{
		"30s":     30 * time.Second,
		"15m":     15 * time.Minute,
		"2h":      2 * time.Hour,
		"1h30m":   90 * time.Minute,
		"1.5h":    90 * time.Minute,
		"8760h":   8760 * time.Hour,
		" 45m \t": 45 * time.Minute,
	}

	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			got, err := ParseSince(raw)
			if err != nil {
				t.Fatalf("ParseSince(%q) error = %v, want nil", raw, err)
			}
			if got != want {
				t.Fatalf("ParseSince(%q) = %s, want %s", raw, got, want)
			}
		})
	}
}

func TestParseSinceRejectsUnsupportedOrUnsafeDurations(t *testing.T) {
	tests := []string{
		"",
		"0s",
		"-1h",
		"1d",
		"1w",
		"100ms",
		"9000h",
		"forever",
	}

	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseSince(raw); err == nil {
				t.Fatalf("ParseSince(%q) error = nil, want error", raw)
			}
		})
	}
}
