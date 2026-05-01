package client

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/disbug-io/disbug-cli/internal/seams"
)

var retryBackoffSchedule = []time.Duration{
	250 * time.Millisecond,
	500 * time.Millisecond,
	time.Second,
}

type retryTransport struct {
	base       http.RoundTripper
	sleeper    seams.Sleeper
	maxRetries int
}

func newRetryTransport(base http.RoundTripper, sleeper seams.Sleeper) *retryTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	if sleeper == nil {
		sleeper = seams.DefaultSleeper()
	}

	return &retryTransport{
		base:       base,
		sleeper:    sleeper,
		maxRetries: 3,
	}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	sleeper := t.sleeper
	if sleeper == nil {
		sleeper = seams.DefaultSleeper()
	}

	if !canRetryRequestBody(req) {
		return base.RoundTrip(req)
	}

	attempts := t.maxRetries + 1
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := range attempts {
		attemptReq, err := requestForAttempt(req, attempt)
		if err != nil {
			return nil, err
		}

		resp, err := base.RoundTrip(attemptReq)
		if err != nil {
			lastErr = err
			if req.Context().Err() != nil || attempt == attempts-1 {
				return resp, err
			}

			sleeper.Sleep(backoffForAttempt(attempt))
			continue
		}

		if !shouldRetryStatus(resp) || attempt == attempts-1 {
			return resp, nil
		}

		drainAndClose(resp.Body)
		sleeper.Sleep(retryDelay(resp, attempt))
	}

	return nil, lastErr
}

func canRetryRequestBody(req *http.Request) bool {
	return req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
}

func requestForAttempt(req *http.Request, attempt int) (*http.Request, error) {
	cloned := req.Clone(req.Context())
	if req.Body == nil || req.Body == http.NoBody {
		cloned.Body = req.Body
		return cloned, nil
	}

	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	cloned.Body = body
	if attempt == 0 {
		cloned.GetBody = req.GetBody
	}

	return cloned, nil
}

func shouldRetryStatus(resp *http.Response) bool {
	if resp == nil {
		return false
	}

	return resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError
}

func retryDelay(resp *http.Response, attempt int) time.Duration {
	if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
		if delay, ok := parseRetryAfter(resp.Header.Get("Retry-After")); ok {
			return delay
		}
	}

	return backoffForAttempt(attempt)
}

func parseRetryAfter(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0, false
		}

		return time.Duration(seconds) * time.Second, true
	}

	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}

	delay := time.Until(when)
	if delay < 0 {
		return 0, true
	}

	return delay, true
}

func backoffForAttempt(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt >= len(retryBackoffSchedule) {
		attempt = len(retryBackoffSchedule) - 1
	}

	base := retryBackoffSchedule[attempt]
	return base + jitter(base)
}

func jitter(base time.Duration) time.Duration {
	max := base / 5
	if max <= 0 {
		return 0
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)+1))
	if err != nil {
		return 0
	}

	return time.Duration(n.Int64())
}

func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}

	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}
