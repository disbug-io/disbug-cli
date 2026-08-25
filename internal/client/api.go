package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// Project is the project attached to a session.
type Project struct {
	ID   int64  `json:"id"`
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
	Title                string              `json:"title"`
	TeamSlug             string              `json:"team_slug"`
	Project              *Project            `json:"project"`
	ProjectSessionNumber int64               `json:"project_session_number"`
	ReportURL            string              `json:"report_url"`
	URL                  string              `json:"url"`
	Status               string              `json:"status"`
	PinCount             int                 `json:"pin_count"`
	FirstPinFeedback     string              `json:"first_pin_feedback"`
	Reporter             *Reporter           `json:"reporter"`
	CreatedAt            string              `json:"created_at"`
	UpdatedAt            string              `json:"updated_at"`
	FreeTierLocked       bool                `json:"free_tier_locked"`
	Attachments          []SessionAttachment `json:"attachments"`
}

// ListSessionsParams holds optional filters for ListSessions.
type ListSessionsParams struct {
	Status          string
	Project         string
	Limit           int
	Cursor          string
	CreatedAtAfter  string
	CreatedAtBefore string
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
		if p.CreatedAtAfter != "" {
			query.Set("created_at_after", p.CreatedAtAfter)
		}
		if p.CreatedAtBefore != "" {
			query.Set("created_at_before", p.CreatedAtBefore)
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
	Number      int64          `json:"number"`
	Feedback    string         `json:"feedback"`
	Status      string         `json:"status"`
	URL         *string        `json:"url"`
	Selector    *string        `json:"selector"`
	ElementInfo map[string]any `json:"element_info"`
	Metadata    map[string]any `json:"metadata"`
	Attachments []Attachment   `json:"attachments"`
}

// Attachment is compact metadata for a file attached to a pin.
type Attachment struct {
	ID          int64  `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

// SessionAttachment identifies an attachment and the pin that owns it in session summaries.
type SessionAttachment struct {
	Attachment
	PinNumber int64 `json:"pin_number"`
}

// AgentActivity is an append-only agent pickup or status-change entry.
type AgentActivity struct {
	ID           int64  `json:"id"`
	Action       string `json:"action"`
	AgentID      *int64 `json:"agent_id"`
	AgentDisplay string `json:"agent_display"`
	PinNumber    *int64 `json:"pin_number"`
	Field        string `json:"field"`
	Status       string `json:"status"`
	Note         string `json:"note"`
	CreatedAt    string `json:"created_at"`
}

// SessionDetail is a full session record with its pins.
type SessionDetail struct {
	Title                string          `json:"title"`
	TeamSlug             string          `json:"team_slug"`
	Project              *Project        `json:"project"`
	ProjectSessionNumber int64           `json:"project_session_number"`
	ReportURL            string          `json:"report_url"`
	Status               string          `json:"status"`
	Reporter             *Reporter       `json:"reporter"`
	URL                  string          `json:"url"`
	UpdatedAt            string          `json:"updated_at"`
	Pins                 []PinLite       `json:"pins"`
	AgentLog             []AgentActivity `json:"agent_log"`
}

// SessionRef returns the scoped reference for this session summary.
func (s SessionSummary) SessionRef() (ref.SessionRef, error) {
	if s.TeamSlug == "" || s.Project == nil || s.Project.ID <= 0 || s.ProjectSessionNumber <= 0 {
		return ref.SessionRef{}, fmt.Errorf("session is missing scoped identity")
	}
	return ref.SessionRef{TeamSlug: s.TeamSlug, ProjectID: s.Project.ID, SessionNumber: s.ProjectSessionNumber}, nil
}

// ScopedID returns a stable cloud watch identifier that does not expose DB primary keys.
func (s SessionSummary) ScopedID() string {
	sessionRef, err := s.SessionRef()
	if err != nil {
		return ""
	}
	return sessionRef.RefString()
}

// GetSession calls GET /api/teams/{team}/projects/{project}/sessions/{number}/.
func (c *Client) GetSession(ctx context.Context, sessionRef ref.SessionRef) (*SessionDetail, error) {
	var session SessionDetail
	if err := c.doJSON(ctx, http.MethodGet, scopedSessionPath(sessionRef), nil, &session); err != nil {
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
	AgentLog       []AgentActivity  `json:"agent_log"`
}

// PinFullResolved is a PinFull with asset URLs resolved to local file paths.
type PinFullResolved struct {
	PinLite
	Screenshot     *Asset           `json:"screenshot"`
	SessionReplay  *ReplayFile      `json:"session_replay"`
	VoiceNote      *Asset           `json:"voice_note"`
	VideoRecording *Asset           `json:"video_recording"`
	Console        []map[string]any `json:"console"`
	Network        []map[string]any `json:"network"`
	Events         []map[string]any `json:"events"`
	AgentLog       []AgentActivity  `json:"agent_log"`
}

// ResolveReplay downloads the replay asset to a local cache file and returns
// a PinFullResolved with the replay field replaced by a local file path.
func (c *Client) ResolveReplay(ctx context.Context, pin *PinFull, sessionNumber, pinNumber int64) (*PinFullResolved, error) {
	resolved := &PinFullResolved{
		PinLite:        pin.PinLite,
		Screenshot:     pin.Screenshot,
		VoiceNote:      pin.VoiceNote,
		VideoRecording: pin.VideoRecording,
		Console:        pin.Console,
		Network:        pin.Network,
		Events:         pin.Events,
		AgentLog:       pin.AgentLog,
	}

	if pin.SessionReplay != nil && pin.SessionReplay.URL != "" {
		replayFile, err := DownloadReplay(ctx, pin.SessionReplay, sessionNumber, pinNumber)
		if err != nil {
			return nil, err
		}
		resolved.SessionReplay = replayFile
	}

	return resolved, nil
}

// StatusUpdate requests an explicit status transition and optional agent note.
type StatusUpdate struct {
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

// SessionStatusResult is the compact result of a session status mutation.
type SessionStatusResult struct {
	TeamSlug             string          `json:"team_slug"`
	Project              *Project        `json:"project"`
	ProjectSessionNumber int64           `json:"project_session_number"`
	ReportURL            string          `json:"report_url"`
	Status               string          `json:"status"`
	AgentLog             []AgentActivity `json:"agent_log"`
}

// PinStatusResult is the compact result of a pin status mutation.
type PinStatusResult struct {
	Number   int64           `json:"number"`
	Status   string          `json:"status"`
	AgentLog []AgentActivity `json:"agent_log"`
}

// SetSessionStatus updates a scoped session and returns its identity, status, and agent activity.
func (c *Client) SetSessionStatus(
	ctx context.Context,
	sessionRef ref.SessionRef,
	status string,
	note string,
) (*SessionStatusResult, error) {
	note = strings.TrimSpace(note)
	body, err := json.Marshal(StatusUpdate{Status: status, Note: note})
	if err != nil {
		return nil, err
	}

	var session SessionStatusResult
	if err := c.doJSONWithoutRetry(
		ctx,
		http.MethodPost,
		scopedSessionPath(sessionRef)+"status/",
		bytes.NewReader(body),
		&session,
	); err != nil {
		if c.shouldConfirmStatusUpdate(ctx, err) {
			confirmed, readErr := c.GetSession(ctx, sessionRef)
			if readErr == nil && confirmed.Status == status && latestStatusActivityMatches(confirmed.AgentLog, nil, status, note) {
				return sessionStatusResultFromDetail(confirmed), nil
			}
		}
		return nil, err
	}

	return &session, nil
}

// SetPinStatus updates a scoped pin and returns its identity, status, and pin-specific agent activity.
func (c *Client) SetPinStatus(
	ctx context.Context,
	pinRef ref.PinRef,
	status string,
	note string,
) (*PinStatusResult, error) {
	note = strings.TrimSpace(note)
	body, err := json.Marshal(StatusUpdate{Status: status, Note: note})
	if err != nil {
		return nil, err
	}

	path := scopedSessionPath(pinRef.Session) + fmt.Sprintf("pins/by-number/%d/status/", pinRef.Pin)
	var pin PinStatusResult
	if err := c.doJSONWithoutRetry(ctx, http.MethodPost, path, bytes.NewReader(body), &pin); err != nil {
		if c.shouldConfirmStatusUpdate(ctx, err) {
			confirmed, readErr := c.GetPinByNumber(ctx, pinRef.Session, pinRef.Pin, nil)
			if readErr == nil && confirmed.Status == status &&
				latestStatusActivityMatches(confirmed.AgentLog, &pinRef.Pin, status, note) {
				return pinStatusResultFromDetail(confirmed), nil
			}
		}
		return nil, err
	}

	return &pin, nil
}

func sessionStatusResultFromDetail(session *SessionDetail) *SessionStatusResult {
	return &SessionStatusResult{
		TeamSlug:             session.TeamSlug,
		Project:              session.Project,
		ProjectSessionNumber: session.ProjectSessionNumber,
		ReportURL:            session.ReportURL,
		Status:               session.Status,
		AgentLog:             session.AgentLog,
	}
}

func pinStatusResultFromDetail(pin *PinFull) *PinStatusResult {
	return &PinStatusResult{
		Number:   pin.Number,
		Status:   pin.Status,
		AgentLog: pin.AgentLog,
	}
}

func (c *Client) shouldConfirmStatusUpdate(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}

	var networkErr *errfmt.NetworkError
	if errors.As(err, &networkErr) {
		return true
	}

	var apiErr *errfmt.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode >= http.StatusInternalServerError
	}

	// A successful response with a truncated or malformed body is also ambiguous:
	// the server may have committed the update before the response became unusable.
	return true
}

func latestStatusActivityMatches(
	activities []AgentActivity,
	pinNumber *int64,
	status string,
	note string,
) bool {
	for i := len(activities) - 1; i >= 0; i-- {
		activity := activities[i]
		if activity.Action != "status_changed" || !samePinNumber(activity.PinNumber, pinNumber) {
			continue
		}

		return activity.Status == status && activity.Note == note
	}

	return false
}

func samePinNumber(left *int64, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return *left == *right
}

// GetPinByNumber calls GET /api/teams/{team}/projects/{project}/sessions/{number}/pins/by-number/{n}/.
func (c *Client) GetPinByNumber(
	ctx context.Context,
	sessionRef ref.SessionRef,
	pinNumber int64,
	fields []string,
) (*PinFull, error) {
	path := scopedSessionPath(sessionRef) + fmt.Sprintf("pins/by-number/%d/", pinNumber)
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

func scopedSessionPath(sessionRef ref.SessionRef) string {
	return fmt.Sprintf(
		"/api/teams/%s/projects/%d/sessions/%d/",
		url.PathEscape(sessionRef.TeamSlug),
		sessionRef.ProjectID,
		sessionRef.SessionNumber,
	)
}
