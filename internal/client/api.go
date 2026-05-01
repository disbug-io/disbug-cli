package client

import (
	"context"
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

// Reporter is the user-facing reporter identity attached to a session.
type Reporter struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// SessionSummary is a compact session record returned by ListSessions.
type SessionSummary struct {
	ID               int64    `json:"id"`
	Project          Project  `json:"project"`
	URL              string   `json:"url"`
	Status           string   `json:"status"`
	PinCount         int      `json:"pin_count"`
	FirstPinFeedback string   `json:"first_pin_feedback"`
	Reporter         Reporter `json:"reporter"`
	UpdatedAt        string   `json:"updated_at"`
	FreeTierLocked   bool     `json:"free_tier_locked"`
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

// PinLite is a compact pin record embedded in session responses.
type PinLite struct {
	ID          int64          `json:"id"`
	Number      int64          `json:"number"`
	Feedback    string         `json:"feedback"`
	URL         string         `json:"url"`
	Selector    string         `json:"selector"`
	ElementInfo map[string]any `json:"element_info"`
	Metadata    map[string]any `json:"metadata"`
}

// SessionDetail is a full session record with its pins.
type SessionDetail struct {
	ID        int64     `json:"id"`
	Status    string    `json:"status"`
	Project   Project   `json:"project"`
	Reporter  Reporter  `json:"reporter"`
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
	SizeBytes   int64  `json:"size_bytes"`
}

// ConsoleLogItem is a parsed browser console log entry.
type ConsoleLogItem struct {
	Level     string `json:"level"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
	Source    string `json:"source"`
}

// NetworkLogItem is a parsed network request entry.
type NetworkLogItem struct {
	Method     string  `json:"method"`
	URL        string  `json:"url"`
	Status     int     `json:"status"`
	DurationMS float64 `json:"duration_ms"`
	Timestamp  string  `json:"timestamp"`
}

// UserEventItem is a parsed browser user event entry.
type UserEventItem struct {
	Type      string `json:"type"`
	Selector  string `json:"selector"`
	Value     string `json:"value"`
	Timestamp string `json:"timestamp"`
}

// PinFull is a full pin record, including optional heavy fields.
type PinFull struct {
	PinLite
	Screenshot     *Asset           `json:"screenshot"`
	SessionReplay  *Asset           `json:"session_replay"`
	VoiceNote      *Asset           `json:"voice_note"`
	VideoRecording *Asset           `json:"video_recording"`
	Console        []ConsoleLogItem `json:"console"`
	Network        []NetworkLogItem `json:"network"`
	Events         []UserEventItem  `json:"events"`
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
