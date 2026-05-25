package cmd

import (
	"strings"
	"time"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/timefmt"
)

func parseSinceFlag(raw string) (string, time.Duration, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", 0, nil
	}

	duration, err := timefmt.ParseSince(trimmed)
	if err != nil {
		return "", 0, &errfmt.UsageError{Message: "--since must be a duration using s, m, or h up to 8760h"}
	}
	return trimmed, duration, nil
}
