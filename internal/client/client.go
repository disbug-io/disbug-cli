package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/seams"
)

const (
	defaultAPIURL        = "https://disbug.io"
	defaultTimeout       = 30 * time.Second
	maxConcurrentDoJSON  = 16
	meCacheTTL           = 30 * time.Second
	requireCapabilityURL = "https://disbug.io"
)

// Client talks to the Disbug API.
type Client struct {
	client    *http.Client
	apiURL    string
	userAgent string
	sem       chan struct{}
	cache     meCache
	clock     seams.Clock
}

type meCache struct {
	mu  sync.Mutex
	me  *Me
	exp time.Time
}

// Me is the response from GET /api/me/.
type Me struct {
	AgentName      string   `json:"agent_name"`
	Team           string   `json:"team"`
	TeamSlug       string   `json:"team_slug"`
	CreatedByEmail string   `json:"created_by_email"`
	TokenPrefix    string   `json:"token_prefix"`
	LastUsedAt     string   `json:"last_used_at"`
	APIVersion     string   `json:"api_version"`
	Capabilities   []string `json:"capabilities"`
}

// New constructs a Disbug API client.
func New(apiURL, token, userAgent string, sleeper seams.Sleeper, doer seams.HTTPDoer, clock seams.Clock) *Client {
	if sleeper == nil {
		sleeper = seams.DefaultSleeper()
	}
	if doer == nil {
		doer = seams.DefaultHTTPDoer()
	}
	if clock == nil {
		clock = seams.DefaultClock()
	}

	transport := newRetryTransport(&authTransport{
		base:      doerRoundTripper{doer: doer},
		token:     token,
		userAgent: userAgent,
	}, sleeper)

	return &Client{
		client: &http.Client{
			Transport: transport,
			Timeout:   defaultTimeout,
		},
		apiURL:    normalizeAPIURL(apiURL),
		userAgent: userAgent,
		sem:       make(chan struct{}, maxConcurrentDoJSON),
		clock:     clock,
	}
}

type doerRoundTripper struct {
	doer seams.HTTPDoer
}

func (t doerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.doer.Do(req)
}

func normalizeAPIURL(apiURL string) string {
	apiURL = strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if apiURL == "" {
		return defaultAPIURL
	}

	return apiURL
}

// Me calls GET /api/me/.
func (c *Client) Me(ctx context.Context) (*Me, error) {
	var me Me
	if err := c.doJSON(ctx, http.MethodGet, "/api/me/", nil, &me); err != nil {
		return nil, err
	}

	return &me, nil
}

// RevokeToken revokes the agent token currently in use.
func (c *Client) RevokeToken(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodPost, "/api/agent/revoke/", nil, nil)
}

// HasCapability reports whether the API advertised a capability.
func (m *Me) HasCapability(name string) bool {
	if m == nil {
		return false
	}
	for _, capability := range m.Capabilities {
		if capability == name {
			return true
		}
	}

	return false
}

// MeCached returns GET /api/me/ with a 30 second in-memory success cache.
func (c *Client) MeCached(ctx context.Context) (*Me, error) {
	now := c.clock.Now()
	c.cache.mu.Lock()
	if c.cache.me != nil && now.Before(c.cache.exp) {
		me := c.cache.me
		c.cache.mu.Unlock()

		return me, nil
	}
	c.cache.mu.Unlock()

	me, err := c.Me(ctx)
	if err != nil {
		return nil, err
	}

	c.cache.mu.Lock()
	c.cache.me = me
	c.cache.exp = c.clock.Now().Add(meCacheTTL)
	c.cache.mu.Unlock()

	return me, nil
}

// RequireCapability returns a user-facing error when the API does not advertise want.
func (c *Client) RequireCapability(ctx context.Context, want string) error {
	me, err := c.MeCached(ctx)
	if err != nil {
		return err
	}
	if me.HasCapability(want) {
		return nil
	}

	return &errfmt.UserFacingError{
		Message: fmt.Sprintf(
			"This feature requires Disbug API capability %q; your team's instance does not advertise it. Update at %s.",
			want,
			requireCapabilityURL,
		),
	}
}

func (c *Client) doJSON(ctx context.Context, method, path string, body io.Reader, out any) error {
	if err := c.acquire(ctx); err != nil {
		return err
	}
	defer c.release()

	absoluteURL := c.absoluteURL(path)
	req, err := http.NewRequestWithContext(ctx, method, absoluteURL, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}

		return &errfmt.NetworkError{URL: absoluteURL, Cause: err}
	}
	if resp == nil {
		return &errfmt.NetworkError{URL: absoluteURL, Cause: errors.New("nil HTTP response")}
	}
	if resp.Body != nil {
		defer func() {
			_ = resp.Body.Close()
		}()
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(resp)
	}

	if out == nil || resp.StatusCode == http.StatusNoContent {
		discardBody(resp.Body)
		return nil
	}
	if resp.Body == nil {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return err
	}
	discardBody(resp.Body)

	return nil
}

func (c *Client) acquire(ctx context.Context) error {
	select {
	case c.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) release() {
	<-c.sem
}

func (c *Client) absoluteURL(path string) string {
	return c.apiURL + "/" + strings.TrimLeft(path, "/")
}

type errorEnvelope struct {
	Code      string `json:"code"`
	Detail    string `json:"detail"`
	RequestID string `json:"request_id"`
}

func decodeAPIError(resp *http.Response) error {
	apiErr := &errfmt.APIError{
		StatusCode: resp.StatusCode,
		Code:       fallbackErrorCode(resp.StatusCode),
		RequestID:  resp.Header.Get("X-Request-ID"),
	}

	if resp.Body == nil {
		return apiErr
	}

	var envelope errorEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err == nil {
		if envelope.Code != "" {
			apiErr.Code = envelope.Code
		}
		if envelope.Detail != "" {
			apiErr.Detail = envelope.Detail
		}
		if envelope.RequestID != "" {
			apiErr.RequestID = envelope.RequestID
		}
	}

	return apiErr
}

func fallbackErrorCode(statusCode int) string {
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

func discardBody(body io.Reader) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, body)
}
