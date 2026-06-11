package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

const (
	defaultBulkConcurrency = 8
	maxBulkConcurrency     = 32
)

// BulkResult contains successful pins and per-pin failures from a bulk fetch.
type BulkResult struct {
	Pins   []*PinFull    `json:"pins"`
	Errors []BulkErrItem `json:"errors"`
}

// BulkErrItem is the stable JSON representation of a per-pin bulk failure.
type BulkErrItem struct {
	Pin       string `json:"pin"`
	Code      string `json:"error_code"`
	Message   string `json:"error_message"`
	RequestID string `json:"request_id,omitempty"`
}

// AllFailed reports whether every attempted pin fetch failed.
func (r BulkResult) AllFailed() bool {
	return len(r.Pins) == 0 && len(r.Errors) > 0
}

// FirstFailureExitCode maps the first bulk error to the stable CLI exit code.
func (r BulkResult) FirstFailureExitCode() int {
	if len(r.Errors) == 0 {
		return 0
	}

	switch r.Errors[0].Code {
	case "network_error":
		return 5
	case "auth_required", "token_revoked", "owner_team_lost":
		return 4
	case "not_found":
		return 6
	case "forbidden", "agent_read_only", "free_tier_locked":
		return 7
	case "rate_limited":
		return 8
	case "internal_error":
		return 9
	default:
		return 1
	}
}

type bulkItemResult struct {
	index int
	pin   *PinFull
	err   BulkErrItem
	ok    bool
}

// GetPinsBulk fetches pins concurrently while isolating per-pin failures.
func (c *Client) GetPinsBulk(ctx context.Context, items []ref.PinFetch) BulkResult {
	results := make([]bulkItemResult, len(items))
	jobs := make(chan int)
	var wg sync.WaitGroup

	workers := bulkConcurrency()
	if workers > len(items) {
		workers = len(items)
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				item := items[index]
				pin, err := c.GetPinByNumber(ctx, item.Pin.Session, item.Pin.Pin, item.Fields)
				if err != nil {
					results[index] = bulkItemResult{
						index: index,
						err:   bulkErrItem(item.Pin, err),
					}
					continue
				}
				results[index] = bulkItemResult{index: index, pin: pin, ok: true}
			}
		}()
	}

sendJobs:
	for index := range items {
		if ctx.Err() != nil {
			break
		}
		select {
		case jobs <- index:
		case <-ctx.Done():
			break sendJobs
		}
	}
	close(jobs)
	wg.Wait()

	var result BulkResult
	ctxErr := ctx.Err()
	for index, itemResult := range results {
		switch {
		case itemResult.ok:
			result.Pins = append(result.Pins, itemResult.pin)
		case itemResult.err.Code != "":
			result.Errors = append(result.Errors, itemResult.err)
		case ctxErr != nil:
			result.Errors = append(result.Errors, bulkErrItem(items[index].Pin, ctxErr))
		}
	}

	return result
}

func bulkConcurrency() int {
	value := strings.TrimSpace(os.Getenv("DISBUG_BULK_CONCURRENCY"))
	if value == "" {
		return defaultBulkConcurrency
	}

	concurrency, err := strconv.Atoi(value)
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: DISBUG_BULK_CONCURRENCY must be an integer; using 8")
		return defaultBulkConcurrency
	}
	if concurrency <= 0 {
		return 1
	}
	if concurrency > maxBulkConcurrency {
		return maxBulkConcurrency
	}

	return concurrency
}

func bulkErrItem(pin ref.PinRef, err error) BulkErrItem {
	item := BulkErrItem{
		Pin:     pin.RefString(),
		Code:    "unknown_error",
		Message: errfmt.Format(err),
	}

	var networkErr *errfmt.NetworkError
	if errors.As(err, &networkErr) {
		item.Code = "network_error"
		return item
	}

	var apiErr *errfmt.APIError
	if errors.As(err, &apiErr) {
		item.Code = apiErr.Code
		if item.Code == "" {
			item.Code = codeFromStatus(apiErr.StatusCode)
		}
		if apiErr.Detail != "" {
			item.Message = apiErr.Detail
		}
		item.RequestID = apiErr.RequestID
		return item
	}

	var apiValue errfmt.APIError
	if errors.As(err, &apiValue) {
		item.Code = apiValue.Code
		if item.Code == "" {
			item.Code = codeFromStatus(apiValue.StatusCode)
		}
		if apiValue.Detail != "" {
			item.Message = apiValue.Detail
		}
		item.RequestID = apiValue.RequestID
		return item
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		item.Code = "network_error"
		return item
	}

	return item
}

func codeFromStatus(statusCode int) string {
	switch {
	case statusCode == http.StatusUnauthorized:
		return "auth_required"
	case statusCode == http.StatusForbidden:
		return "forbidden"
	case statusCode == http.StatusNotFound:
		return "not_found"
	case statusCode == http.StatusTooManyRequests:
		return "rate_limited"
	case statusCode >= http.StatusInternalServerError:
		return "internal_error"
	default:
		return "bad_request"
	}
}
