// Package selfupdate performs a best-effort check for a newer released disbug
// CLI and formats an upgrade nudge. It never blocks or fails a command: any
// error, missing network, or unparseable version simply yields no notice.
package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CheckInterval bounds how often the network is consulted for the latest release.
const CheckInterval = 24 * time.Hour

const stateFileName = "version-check.json"

// FetchFunc returns the latest released version tag, for example "v0.3.0".
type FetchFunc func(context.Context) (string, error)

type state struct {
	LastCheckUnix int64  `json:"last_check_unix"`
	LatestVersion string `json:"latest_version"`
}

// Notice returns an upgrade message when a released version newer than current
// exists. It returns an empty string when current is up to date, when current
// is not a released semantic version (for example a "dev" build), or when the
// check cannot complete. The result is safe to ignore.
func Notice(ctx context.Context, current, dir string, now time.Time, fetch FetchFunc) string {
	cur, ok := parseSemver(current)
	if !ok {
		return ""
	}
	latest := latestVersion(ctx, dir, now, fetch)
	lat, ok := parseSemver(latest)
	if !ok || !less(cur, lat) {
		return ""
	}
	return fmt.Sprintf(
		"A new version of disbug is available: %s -> %s\n"+
			"Upgrade with `brew upgrade disbug` (macOS or Linux) or `scoop update disbug` (Windows).\n"+
			"Set DISBUG_NO_UPDATE_CHECK=1 to silence this notice.",
		current, latest,
	)
}

// latestVersion returns the newest known release tag, consulting the on-disk
// cache first and only fetching over the network once per CheckInterval.
func latestVersion(ctx context.Context, dir string, now time.Time, fetch FetchFunc) string {
	cached := readState(dir)
	if cached.LastCheckUnix > 0 && now.Sub(time.Unix(cached.LastCheckUnix, 0)) < CheckInterval {
		return cached.LatestVersion
	}
	latest, err := fetch(ctx)
	if err != nil || latest == "" {
		// Record the attempt so a persistent outage does not retry every run,
		// keeping whatever version the cache already knew about.
		writeState(dir, state{LastCheckUnix: now.Unix(), LatestVersion: cached.LatestVersion})
		return cached.LatestVersion
	}
	writeState(dir, state{LastCheckUnix: now.Unix(), LatestVersion: latest})
	return latest
}

func readState(dir string) state {
	var st state
	data, err := os.ReadFile(filepath.Join(dir, stateFileName)) //nolint:gosec // path is the user's own config dir
	if err != nil {
		return state{}
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return state{}
	}
	return st
}

func writeState(dir string, st state) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	data, err := json.Marshal(st)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, stateFileName), data, 0o600)
}

type semver struct {
	major, minor, patch int
}

// parseSemver reads a "vMAJOR.MINOR.PATCH" tag, ignoring any pre-release or
// build metadata. It reports false for anything that is not a semantic version,
// including "dev" builds.
func parseSemver(value string) (semver, bool) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	if value == "" {
		return semver{}, false
	}
	if idx := strings.IndexAny(value, "-+"); idx >= 0 {
		value = value[:idx]
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	nums := [3]int{}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return semver{}, false
		}
		nums[i] = n
	}
	return semver{nums[0], nums[1], nums[2]}, true
}

func less(a, b semver) bool {
	if a.major != b.major {
		return a.major < b.major
	}
	if a.minor != b.minor {
		return a.minor < b.minor
	}
	return a.patch < b.patch
}
