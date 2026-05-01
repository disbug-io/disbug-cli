package client

import (
	"context"
	"crypto/rand"
	"errors"
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

const (
	maxRetryDrainBytes    int64 = 64 << 10
	maxRetryDrainDuration       = 10 * time.Millisecond
)

type retryTransport struct {
	base            http.RoundTripper
	sleeper         seams.Sleeper
	maxRetries      int
	useTimerSleeper bool
}

func newRetryTransport(base http.RoundTripper, sleeper seams.Sleeper) *retryTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	useTimerSleeper := sleeper == nil
	if sleeper == nil {
		sleeper = seams.DefaultSleeper()
	}

	return &retryTransport{
		base:            base,
		sleeper:         sleeper,
		maxRetries:      3,
		useTimerSleeper: useTimerSleeper,
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
	useTimerSleeper := t.useTimerSleeper
	if sleeper == nil {
		sleeper = seams.DefaultSleeper()
		useTimerSleeper = true
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
		if attempt > 0 {
			if err := req.Context().Err(); err != nil {
				return nil, err
			}
		}

		attemptReq, err := requestForAttempt(req, attempt)
		if err != nil {
			return nil, err
		}

		resp, err := base.RoundTrip(attemptReq)
		if err != nil {
			lastErr = err
			if isTerminalContextError(err) || req.Context().Err() != nil || attempt == attempts-1 {
				return resp, err
			}

			if err := waitForRetry(req.Context(), sleeper, useTimerSleeper, backoffForAttempt(attempt)); err != nil {
				return nil, err
			}
			continue
		}

		if !shouldRetryStatus(resp) || attempt == attempts-1 {
			return resp, nil
		}

		if err := req.Context().Err(); err != nil {
			drainAndClose(resp)
			return nil, err
		}
		drainAndClose(resp)
		if err := waitForRetry(req.Context(), sleeper, useTimerSleeper, retryDelay(resp, attempt)); err != nil {
			return nil, err
		}
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
	if attempt == 0 {
		cloned.Body = req.Body
		return cloned, nil
	}

	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	cloned.Body = body

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

func isTerminalContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
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

func waitForRetry(ctx context.Context, sleeper seams.Sleeper, useTimerSleeper bool, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		if !useTimerSleeper {
			sleeper.Sleep(d)
		}

		return ctx.Err()
	}
	if useTimerSleeper {
		timer := time.NewTimer(d)
		defer timer.Stop()

		select {
		case <-timer.C:
			return ctx.Err()
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		sleeper.Sleep(d)
	}()

	select {
	case <-done:
		return ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
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

func drainAndClose(resp *http.Response) {
	if resp == nil {
		return
	}
	body := resp.Body
	if body == nil {
		return
	}

	if resp.ContentLength > 0 {
		drained := make(chan struct{})
		go func() {
			defer close(drained)
			_, _ = io.Copy(io.Discard, io.LimitReader(body, maxRetryDrainBytes))
		}()

		select {
		case <-drained:
		case <-time.After(maxRetryDrainDuration):
		}
	}
	_ = body.Close()
}
