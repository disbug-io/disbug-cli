package seams

import (
	"bytes"
	crand "crypto/rand"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDefaultClockHonorsFrozenTime(t *testing.T) {
	restoreHooks := enableTestHooksForTesting()
	defer restoreHooks()
	t.Setenv("DISBUG_TEST_FROZEN_TIME", "2026-01-02T03:04:05Z")

	got := DefaultClock().Now()
	want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	if !got.Equal(want) {
		t.Fatalf("DefaultClock().Now() = %s, want %s", got, want)
	}
}

func TestDefaultClockFallsBackOnInvalidFrozenTime(t *testing.T) {
	restoreHooks := enableTestHooksForTesting()
	defer restoreHooks()
	t.Setenv("DISBUG_TEST_FROZEN_TIME", "not-rfc3339")

	before := time.Now()
	got := DefaultClock().Now()
	after := time.Now()

	if got.Before(before) || got.After(after) {
		t.Fatalf("DefaultClock().Now() = %s, want between %s and %s", got, before, after)
	}
}

func TestFixedClockNowAndAdvance(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	clock := NewFixedClock(start)

	if got := clock.Now(); !got.Equal(start) {
		t.Fatalf("Now() = %s, want %s", got, start)
	}

	clock.Advance(2 * time.Hour)

	want := start.Add(2 * time.Hour)
	if got := clock.Now(); !got.Equal(want) {
		t.Fatalf("Now() after Advance = %s, want %s", got, want)
	}
}

func TestDefaultSleeperHonorsFastSleep(t *testing.T) {
	restoreHooks := enableTestHooksForTesting()
	defer restoreHooks()
	t.Setenv("DISBUG_TEST_FAST_SLEEP", "1")

	start := time.Now()
	DefaultSleeper().Sleep(100 * time.Millisecond)
	elapsed := time.Since(start)

	if elapsed > 20*time.Millisecond {
		t.Fatalf("fast sleeper slept for %s", elapsed)
	}
}

func TestDefaultRandomDeterministicSeed(t *testing.T) {
	restoreHooks := enableTestHooksForTesting()
	defer restoreHooks()
	t.Setenv("DISBUG_TEST_DETERMINISTIC_RANDOM", "same-seed")

	first := make([]byte, 32)
	second := make([]byte, 32)
	if _, err := io.ReadFull(DefaultRandom(), first); err != nil {
		t.Fatalf("read first seeded random: %v", err)
	}
	if _, err := io.ReadFull(DefaultRandom(), second); err != nil {
		t.Fatalf("read second seeded random: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("same seed produced different bytes: %x != %x", first, second)
	}

	reader := DefaultRandom()
	chunkA := make([]byte, 16)
	chunkB := make([]byte, 16)
	if _, err := io.ReadFull(reader, chunkA); err != nil {
		t.Fatalf("read first chunk: %v", err)
	}
	if _, err := io.ReadFull(reader, chunkB); err != nil {
		t.Fatalf("read second chunk: %v", err)
	}
	if bytes.Equal(chunkA, chunkB) {
		t.Fatalf("sequential reads repeated bytes: %x", chunkA)
	}
}

func TestDefaultRandomIgnoresDeterministicSeedWithoutTestHooks(t *testing.T) {
	t.Setenv("DISBUG_TEST_DETERMINISTIC_RANDOM", "same-seed")

	if got := DefaultRandom(); got != crand.Reader {
		t.Fatalf("DefaultRandom() = %T, want crypto/rand.Reader when test hooks are disabled", got)
	}
}

func TestDefaultListenerBindsAndCloses(t *testing.T) {
	listener, err := DefaultListener()("127.0.0.1:0")
	if err != nil {
		t.Fatalf("DefaultListener() failed to bind: %v", err)
	}
	if listener.Addr().String() == "" {
		t.Fatal("listener address is empty")
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("listener close failed: %v", err)
	}
}

func TestDefaultHTTPDoer(t *testing.T) {
	doer := DefaultHTTPDoer()
	if doer == nil {
		t.Fatal("DefaultHTTPDoer() returned nil")
	}
}

func TestDefaultBrowserOpenerNoBrowserWritesURLToStderr(t *testing.T) {
	restoreHooks := enableTestHooksForTesting()
	defer restoreHooks()
	t.Setenv("DISBUG_TEST_NO_BROWSER", "1")

	oldStderr := os.Stderr
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Stderr = writeEnd
	t.Cleanup(func() {
		os.Stderr = oldStderr
		readEnd.Close()
		writeEnd.Close()
	})

	url := "https://example.test/bug/123"
	if err := DefaultBrowserOpener()(url); err != nil {
		t.Fatalf("DefaultBrowserOpener() returned error: %v", err)
	}
	if err := writeEnd.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}

	output, err := io.ReadAll(readEnd)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if !strings.Contains(string(output), url) {
		t.Fatalf("stderr output %q does not mention URL %q", string(output), url)
	}
}

func TestHTTPClientImplementsHTTPDoer(t *testing.T) {
	var _ HTTPDoer = (*http.Client)(nil)
}
