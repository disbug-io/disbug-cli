package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fetchReturns(version string) FetchFunc {
	return func(context.Context) (string, error) { return version, nil }
}

func fetchFails() FetchFunc {
	return func(context.Context) (string, error) { return "", errors.New("network down") }
}

func TestNoticeSkipsNonSemverCurrent(t *testing.T) {
	dir := t.TempDir()
	fetch := func(context.Context) (string, error) {
		t.Fatal("fetch must not run for a dev build")
		return "", nil
	}
	if notice := Notice(context.Background(), "dev", dir, time.Now(), fetch); notice != "" {
		t.Fatalf("notice = %q, want empty for dev build", notice)
	}
}

func TestNoticeReportsNewerRelease(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1_700_000_000, 0)

	notice := Notice(context.Background(), "v0.2.0", dir, now, fetchReturns("v0.3.0"))

	if !strings.Contains(notice, "v0.2.0 -> v0.3.0") {
		t.Fatalf("notice = %q, want upgrade from v0.2.0 to v0.3.0", notice)
	}
	if !strings.Contains(notice, "brew upgrade disbug") || !strings.Contains(notice, "scoop update disbug") {
		t.Fatalf("notice = %q, want both upgrade commands", notice)
	}

	// The successful fetch must be cached with the check timestamp.
	data, err := os.ReadFile(filepath.Join(dir, stateFileName))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	if st.LatestVersion != "v0.3.0" || st.LastCheckUnix != now.Unix() {
		t.Fatalf("state = %#v, want cached v0.3.0 at now", st)
	}
}

func TestNoticeSilentWhenCurrent(t *testing.T) {
	dir := t.TempDir()
	if notice := Notice(context.Background(), "v0.3.0", dir, time.Now(), fetchReturns("v0.3.0")); notice != "" {
		t.Fatalf("notice = %q, want empty when up to date", notice)
	}
}

func TestNoticeSilentWhenLocalIsNewer(t *testing.T) {
	dir := t.TempDir()
	if notice := Notice(context.Background(), "v1.0.0", dir, time.Now(), fetchReturns("v0.9.9")); notice != "" {
		t.Fatalf("notice = %q, want empty when local build is ahead", notice)
	}
}

func TestNoticeUsesFreshCacheWithoutFetching(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1_700_000_000, 0)
	writeState(dir, state{LastCheckUnix: now.Unix(), LatestVersion: "v0.4.0"})

	fetch := func(context.Context) (string, error) {
		t.Fatal("fetch must not run while the cache is fresh")
		return "", nil
	}
	notice := Notice(context.Background(), "v0.2.0", dir, now.Add(time.Hour), fetch)
	if !strings.Contains(notice, "v0.2.0 -> v0.4.0") {
		t.Fatalf("notice = %q, want cached v0.4.0", notice)
	}
}

func TestNoticeRefetchesAfterInterval(t *testing.T) {
	dir := t.TempDir()
	stale := time.Unix(1_700_000_000, 0)
	writeState(dir, state{LastCheckUnix: stale.Unix(), LatestVersion: "v0.4.0"})

	now := stale.Add(CheckInterval + time.Minute)
	notice := Notice(context.Background(), "v0.2.0", dir, now, fetchReturns("v0.5.0"))
	if !strings.Contains(notice, "v0.2.0 -> v0.5.0") {
		t.Fatalf("notice = %q, want refetched v0.5.0", notice)
	}
}

func TestNoticeToleratesFetchFailure(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1_700_000_000, 0)

	// No cache and a failing fetch must not panic or produce a notice.
	if notice := Notice(context.Background(), "v0.2.0", dir, now, fetchFails()); notice != "" {
		t.Fatalf("notice = %q, want empty on fetch failure", notice)
	}

	// The failed attempt is timestamped so the next run does not immediately retry.
	st := readState(dir)
	if st.LastCheckUnix != now.Unix() {
		t.Fatalf("state = %#v, want failed attempt recorded", st)
	}
}

func TestParseSemver(t *testing.T) {
	cases := []struct {
		in    string
		ok    bool
		major int
		minor int
		patch int
	}{
		{"v0.2.0", true, 0, 2, 0},
		{"1.10.3", true, 1, 10, 3},
		{" v2.0.1 ", true, 2, 0, 1},
		{"v0.3.0-rc1", true, 0, 3, 0},
		{"dev", false, 0, 0, 0},
		{"", false, 0, 0, 0},
		{"v1.2", false, 0, 0, 0},
		{"v1.2.x", false, 0, 0, 0},
	}
	for _, tc := range cases {
		got, ok := parseSemver(tc.in)
		if ok != tc.ok {
			t.Fatalf("parseSemver(%q) ok = %v, want %v", tc.in, ok, tc.ok)
		}
		if ok && (got.major != tc.major || got.minor != tc.minor || got.patch != tc.patch) {
			t.Fatalf("parseSemver(%q) = %#v, want %d.%d.%d", tc.in, got, tc.major, tc.minor, tc.patch)
		}
	}
}
