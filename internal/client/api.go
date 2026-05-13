package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/ref"
)

// Project is the project attached to a session.
type Project struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// UnmarshalJSON handles cases where the API returns a project ID instead of an object.
func (p *Project) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] >= '0' && data[0] <= '9' {
		return nil // Ignore integer IDs
	}
	type Alias Project
	var a Alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*p = Project(a)
	return nil
}

// Reporter is the user-facing reporter identity attached to a session.
type Reporter struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// UnmarshalJSON handles cases where the API returns a reporter ID instead of an object.
func (r *Reporter) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] >= '0' && data[0] <= '9' {
		return nil // Ignore integer IDs
	}
	type Alias Reporter
	var a Alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*r = Reporter(a)
	return nil
}

// SessionSummary is a compact session record returned by ListSessions.
type SessionSummary struct {
	ID               int64     `json:"id"`
	Project          *Project  `json:"project"`
	URL              string    `json:"url"`
	Status           string    `json:"status"`
	PinCount         int       `json:"pin_count"`
	FirstPinFeedback string    `json:"first_pin_feedback"`
	Reporter         *Reporter `json:"reporter"`
	UpdatedAt        string    `json:"updated_at"`
	FreeTierLocked   bool      `json:"free_tier_locked"`
}

// ListSessionsParams holds optional filters for ListSessions.
type ListSessionsParams struct {
	Status  string
	Project string
	Limit   int
	Cursor  string
}

// ListSessionsResponse is the paginated session list response.
type ListSessionsResponse struct {
	Results           []SessionSummary `json:"results"`
	NextCursor        *string          `json:"next_cursor"`
	Count             int              `json:"count"`
	FreeTierTruncated bool             `json:"free_tier_truncated"`
}

// SearchParams configures a /api/search/ call.
type SearchParams struct {
	Query string
	Scope string // "sessions" or "pins"; SearchSessions defaults empty scope to "sessions"
	Limit int
}

// SearchSessionsResponse is the session search response.
type SearchSessionsResponse struct {
	Results []SessionSummary `json:"results"`
	Total   int              `json:"total"`
}

// SearchPinsHit is a pin search result with its parent session.
type SearchPinsHit struct {
	Pin     PinLite        `json:"pin"`
	Session SessionSummary `json:"session"`
}

// SearchPinsResponse is the pin search response.
type SearchPinsResponse struct {
	Results []SearchPinsHit `json:"results"`
	Total   int             `json:"total"`
}

// ListSessions calls GET /api/sessions/.
func (c *Client) ListSessions(ctx context.Context, p *ListSessionsParams) (*ListSessionsResponse, error) {
	path := "/api/sessions/"
	if p != nil {
		query := url.Values{}
		if p.Status != "" {
			query.Set("status", p.Status)
		}
		if p.Project != "" {
			query.Set("project", p.Project)
		}
		if p.Limit > 0 {
			query.Set("limit", strconv.Itoa(p.Limit))
		}
		if p.Cursor != "" {
			query.Set("cursor", p.Cursor)
		}
		if encoded := query.Encode(); encoded != "" {
			path += "?" + encoded
		}
	}

	var resp ListSessionsResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// SearchSessions calls GET /api/search/ and returns session summaries.
// When Scope is "pins", the pin search hits are mapped to their parent sessions.
func (c *Client) SearchSessions(ctx context.Context, p *SearchParams) (*SearchSessionsResponse, error) {
	scope := "sessions"
	if p != nil && p.Scope != "" {
		scope = p.Scope
	}

	if scope == "pins" {
		var resp SearchPinsResponse
		if err := c.doJSON(ctx, http.MethodGet, searchPath(p, scope), nil, &resp); err != nil {
			return nil, err
		}

		results := make([]SessionSummary, 0, len(resp.Results))
		for _, hit := range resp.Results {
			results = append(results, hit.Session)
		}

		return &SearchSessionsResponse{
			Results: results,
			Total:   resp.Total,
		}, nil
	}

	var resp SearchSessionsResponse
	if err := c.doJSON(ctx, http.MethodGet, searchPath(p, scope), nil, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// SearchPins calls GET /api/search/ with scope=pins.
func (c *Client) SearchPins(ctx context.Context, p *SearchParams) (*SearchPinsResponse, error) {
	var resp SearchPinsResponse
	if err := c.doJSON(ctx, http.MethodGet, searchPath(p, "pins"), nil, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func searchPath(p *SearchParams, scope string) string {
	query := url.Values{}
	query.Set("scope", scope)
	if p != nil {
		query.Set("q", p.Query)
		if p.Limit > 0 {
			query.Set("limit", strconv.Itoa(p.Limit))
		}
	}

	return "/api/search/?" + query.Encode()
}

// PinLite is a compact pin record embedded in session responses.
type PinLite struct {
	ID          int64          `json:"id"`
	Number      int64          `json:"number"`
	Feedback    string         `json:"feedback"`
	URL         *string        `json:"url"`
	Selector    *string        `json:"selector"`
	ElementInfo map[string]any `json:"element_info"`
	Metadata    map[string]any `json:"metadata"`
}

// SessionDetail is a full session record with its pins.
type SessionDetail struct {
	ID        int64     `json:"id"`
	Status    string    `json:"status"`
	Project   *Project  `json:"project"`
	Reporter  *Reporter `json:"reporter"`
	URL       string    `json:"url"`
	UpdatedAt string    `json:"updated_at"`
	Pins      []PinLite `json:"pins"`
}

// GetSession calls GET /api/sessions/{id}/.
func (c *Client) GetSession(ctx context.Context, sessionID int64) (*SessionDetail, error) {
	var session SessionDetail
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/sessions/%d/", sessionID), nil, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

// Asset is a signed or public asset reference returned by the API.
type Asset struct {
	URL         string `json:"url"`
	ExpiresAt   string `json:"expires_at"`
	ContentType string `json:"content_type"`
	SizeBytes   *int64 `json:"size_bytes"`
}

// PinFull is a full pin record, including optional heavy fields.
type PinFull struct {
	PinLite
	Screenshot     *Asset           `json:"screenshot"`
	SessionReplay  *Asset           `json:"session_replay"`
	VoiceNote      *Asset           `json:"voice_note"`
	VideoRecording *Asset           `json:"video_recording"`
	Console        []map[string]any `json:"console"`
	Network        []map[string]any `json:"network"`
	Events         []map[string]any `json:"events"`
}

// GetPinByNumber calls GET /api/sessions/{id}/pins/by-number/{n}/.
func (c *Client) GetPinByNumber(
	ctx context.Context,
	sessionID int64,
	pinNumber int64,
	fields []string,
) (*PinFull, error) {
	path := fmt.Sprintf("/api/sessions/%d/pins/by-number/%d/", sessionID, pinNumber)
	if len(fields) > 0 {
		normalizedFields, err := ref.NormalizeFields(fields)
		if err != nil {
			return nil, err
		}
		if len(normalizedFields) != 1 || normalizedFields[0] != "all" {
			wireFields := make([]string, 0, len(normalizedFields))
			for _, field := range normalizedFields {
				wireFields = append(wireFields, ref.WireFieldName(field))
			}
			query := url.Values{}
			query.Set("fields", strings.Join(wireFields, ","))
			path += "?" + query.Encode()
		}
	}

	var pin PinFull
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &pin); err != nil {
		return nil, err
	}

	return &pin, nil
}
