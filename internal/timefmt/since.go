package timefmt

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const MaxSince = 8760 * time.Hour

var sincePattern = regexp.MustCompile(`^([0-9]+(\.[0-9]+)?(s|m|h))+$`)

// ParseSince parses the shared --since duration syntax for session backfills.
func ParseSince(raw string) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("duration is required")
	}
	if !sincePattern.MatchString(value) {
		return 0, fmt.Errorf("duration must use s, m, or h units")
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("duration must be greater than 0")
	}
	if duration > MaxSince {
		return 0, fmt.Errorf("duration must be less than or equal to 8760h")
	}

	return duration, nil
}
