package seams

import (
	crand "crypto/rand"
	"fmt"
	"hash/fnv"
	"io"
	mrand "math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Clock provides the current time.
type Clock interface {
	Now() time.Time
}

// Sleeper pauses execution for a duration.
type Sleeper interface {
	Sleep(time.Duration)
}

// BrowserOpener opens a URL in the user's browser.
type BrowserOpener func(url string) error

// ListenerFactory creates a network listener bound to an address.
type ListenerFactory func(addr string) (net.Listener, error)

// HTTPDoer executes HTTP requests.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

type realSleeper struct{}

func (realSleeper) Sleep(d time.Duration) {
	time.Sleep(d)
}

type noOpSleeper struct{}

func (noOpSleeper) Sleep(time.Duration) {}

var testHooksEnabled atomic.Bool

// FixedClock is a concurrency-safe clock for tests.
type FixedClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFixedClock returns a clock fixed at t.
func NewFixedClock(t time.Time) *FixedClock {
	return &FixedClock{now: t}
}

// Now returns the clock's current fixed time.
func (f *FixedClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.now
}

// Advance moves the clock forward by d.
func (f *FixedClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.now = f.now.Add(d)
}

// Set changes the clock to t.
func (f *FixedClock) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.now = t
}

// DefaultClock returns the production clock or a frozen test clock when hooks are enabled.
func DefaultClock() Clock {
	if devBuild() {
		if frozen := os.Getenv("DISBUG_TEST_FROZEN_TIME"); frozen != "" {
			if t, err := time.Parse(time.RFC3339, frozen); err == nil {
				return NewFixedClock(t)
			}
		}
	}

	return realClock{}
}

// DefaultSleeper returns the production sleeper or a no-op sleeper when hooks are enabled.
func DefaultSleeper() Sleeper {
	if devBuild() && os.Getenv("DISBUG_TEST_FAST_SLEEP") == "1" {
		return noOpSleeper{}
	}

	return realSleeper{}
}

// DefaultRandom returns crypto randomness or a deterministic reader when hooks are enabled.
func DefaultRandom() io.Reader {
	if devBuild() {
		if seed := os.Getenv("DISBUG_TEST_DETERMINISTIC_RANDOM"); seed != "" {
			return newSeededReader(seed)
		}
	}

	return crand.Reader
}

// DefaultListener returns the production TCP listener factory.
func DefaultListener() ListenerFactory {
	return func(addr string) (net.Listener, error) {
		return net.Listen("tcp", addr)
	}
}

// DefaultHTTPDoer returns the production HTTP client.
func DefaultHTTPDoer() HTTPDoer {
	return &http.Client{}
}

// DefaultBrowserOpener returns the platform browser opener or a no-op opener when hooks are enabled.
func DefaultBrowserOpener() BrowserOpener {
	if devBuild() && os.Getenv("DISBUG_TEST_NO_BROWSER") == "1" {
		return noOpBrowserOpener
	}

	return platformBrowserOpener
}

func noOpBrowserOpener(url string) error {
	_, err := fmt.Fprintln(os.Stderr, url)
	return err
}

func platformBrowserOpener(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		// #nosec G204 -- browser opener intentionally passes the caller-provided URL to the platform command.
		cmd = exec.Command("open", url)
	case "windows":
		// #nosec G204 -- browser opener intentionally passes the caller-provided URL to the platform command.
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "linux":
		// #nosec G204 -- browser opener intentionally passes the caller-provided URL to the platform command.
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("unsupported platform for browser opener: %s", runtime.GOOS)
	}

	return cmd.Start()
}

type seededReader struct {
	mu sync.Mutex
	r  *mrand.Rand
}

func newSeededReader(seed string) *seededReader {
	h := fnv.New64a()
	_, _ = h.Write([]byte(seed))
	sourceSeed := h.Sum64() & 0x7fffffffffffffff

	return &seededReader{
		// #nosec G404 -- deterministic test seam, not used for production cryptographic randomness.
		r: mrand.New(mrand.NewSource(int64(sourceSeed))),
	}
}

func (s *seededReader) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range p {
		// #nosec G115 -- Intn(256) is intentionally bounded to one byte.
		p[i] = byte(s.r.Intn(256))
	}

	return len(p), nil
}

func devBuild() bool {
	return testHooksAllowed()
}

func testHooksAllowed() bool {
	return testHooksEnabled.Load() || os.Getenv("DISBUG_ENABLE_TEST_HOOKS") == "1"
}

func enableTestHooksForTesting() func() {
	previous := testHooksEnabled.Swap(true)

	return func() {
		testHooksEnabled.Store(previous)
	}
}
