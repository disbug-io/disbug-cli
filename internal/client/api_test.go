package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

var testSessionRef = ref.SessionRef{TeamSlug: "acme", ProjectID: 42, SessionNumber: 5}

func TestSetPinStatusDoesNotRetryAndConfirmsAmbiguousSuccess(t *testing.T) {
	postCalls := 0
	getCalls := 0
	doer := doerFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/pins/by-number/2/status/"):
			postCalls++
			return nil, errors.New("response connection reset")
		case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/pins/by-number/2/"):
			getCalls++
			return response(http.StatusOK, io.NopCloser(strings.NewReader(`{
				"number":2,
				"status":"resolved",
				"agent_log":[{
					"action":"status_changed",
					"pin_number":2,
					"status":"resolved",
					"note":"Fixed validation"
				}]
			}`))), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	})
	c := New("https://api.example.test", "token", "test", &recordingSleeper{}, doer, nil)
	pinRef := ref.PinRef{Session: testSessionRef, Pin: 2}

	pin, err := c.SetPinStatus(context.Background(), pinRef, "resolved", "  Fixed validation  ")
	if err != nil {
		t.Fatalf("SetPinStatus() error = %v, want nil", err)
	}
	if got, want := pin.Status, "resolved"; got != want {
		t.Fatalf("Status = %q, want %q", got, want)
	}
	if got, want := postCalls, 1; got != want {
		t.Fatalf("POST calls = %d, want %d", got, want)
	}
	if got, want := getCalls, 1; got != want {
		t.Fatalf("GET calls = %d, want %d", got, want)
	}
}

func TestSetSessionStatusDoesNotRetryAndConfirmsAmbiguousSuccess(t *testing.T) {
	postCalls := 0
	getCalls := 0
	doer := doerFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/sessions/5/status/"):
			postCalls++
			return nil, errors.New("response connection reset")
		case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/sessions/5/"):
			getCalls++
			return response(http.StatusOK, io.NopCloser(strings.NewReader(`{
				"project_session_number":5,
				"status":"resolved",
				"agent_log":[{
					"action":"status_changed",
					"pin_number":null,
					"status":"resolved",
					"note":"Verified all pins"
				}]
			}`))), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	})
	c := New("https://api.example.test", "token", "test", &recordingSleeper{}, doer, nil)

	session, err := c.SetSessionStatus(context.Background(), testSessionRef, "resolved", "Verified all pins")
	if err != nil {
		t.Fatalf("SetSessionStatus() error = %v, want nil", err)
	}
	if got, want := session.Status, "resolved"; got != want {
		t.Fatalf("Status = %q, want %q", got, want)
	}
	if got, want := postCalls, 1; got != want {
		t.Fatalf("POST calls = %d, want %d", got, want)
	}
	if got, want := getCalls, 1; got != want {
		t.Fatalf("GET calls = %d, want %d", got, want)
	}
}

func TestSetPinStatusReturnsAmbiguousErrorWhenReadbackDoesNotConfirm(t *testing.T) {
	postCalls := 0
	getCalls := 0
	wantErr := errors.New("response connection reset")
	doer := doerFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodPost {
			postCalls++
			return nil, wantErr
		}
		getCalls++
		return response(http.StatusOK, io.NopCloser(strings.NewReader(`{
			"number":2,
			"status":"open",
			"agent_log":[]
		}`))), nil
	})
	c := New("https://api.example.test", "token", "test", &recordingSleeper{}, doer, nil)
	pinRef := ref.PinRef{Session: testSessionRef, Pin: 2}

	_, err := c.SetPinStatus(context.Background(), pinRef, "resolved", "Fixed validation")
	if err == nil {
		t.Fatal("SetPinStatus() error = nil, want original network error")
	}
	var networkErr *errfmt.NetworkError
	if !errors.As(err, &networkErr) || !errors.Is(err, wantErr) {
		t.Fatalf("SetPinStatus() error = %v, want wrapped network error", err)
	}
	if got, want := postCalls, 1; got != want {
		t.Fatalf("POST calls = %d, want %d", got, want)
	}
	if got, want := getCalls, 1; got != want {
		t.Fatalf("GET calls = %d, want %d", got, want)
	}
}

func TestSetPinStatusDoesNotReadBackDefinitiveClientError(t *testing.T) {
	postCalls := 0
	getCalls := 0
	doer := doerFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodGet {
			getCalls++
			t.Fatal("unexpected readback for definitive client error")
		}
		postCalls++
		return response(http.StatusBadRequest, io.NopCloser(strings.NewReader(
			`{"code":"unchanged_status","detail":"Pin is already resolved."}`,
		))), nil
	})
	c := New("https://api.example.test", "token", "test", &recordingSleeper{}, doer, nil)
	pinRef := ref.PinRef{Session: testSessionRef, Pin: 2}

	_, err := c.SetPinStatus(context.Background(), pinRef, "resolved", "")
	if err == nil {
		t.Fatal("SetPinStatus() error = nil, want API error")
	}
	if got, want := postCalls, 1; got != want {
		t.Fatalf("POST calls = %d, want %d", got, want)
	}
	if getCalls != 0 {
		t.Fatalf("GET calls = %d, want 0", getCalls)
	}
}

func TestListSessions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/sessions/"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		query := r.URL.Query()
		if got, want := query.Get("status"), "open"; got != want {
			t.Fatalf("status query = %q, want %q", got, want)
		}
		if got, want := query.Get("project"), "web"; got != want {
			t.Fatalf("project query = %q, want %q", got, want)
		}
		if got, want := query.Get("limit"), "25"; got != want {
			t.Fatalf("limit query = %q, want %q", got, want)
		}
		if got, want := query.Get("cursor"), "next-1"; got != want {
			t.Fatalf("cursor query = %q, want %q", got, want)
		}
		if got, want := query.Get("created_at_after"), "2026-05-23T09:30:00Z"; got != want {
			t.Fatalf("created_at_after query = %q, want %q", got, want)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"results":[{
				"id":123,
				"title":"Checkout failure",
				"team_slug":"acme",
				"project":{"id":42,"slug":"42","name":"Website"},
				"project_session_number":5,
				"report_url":"https://app.disbug.test/acme/projects/42/sessions/5/",
				"url":"https://example.test/page",
				"status":"open",
				"pin_count":2,
				"first_pin_feedback":"broken button",
				"reporter":{"email":"r@example.test","display_name":"Reporter"},
				"updated_at":"2026-05-01T12:00:00Z",
				"free_tier_locked":true,
				"attachments":[{"id":9,"pin_number":2,"filename":"notes.md","content_type":"text/markdown","size_bytes":42}]
			}],
			"next_cursor":"next-2",
			"count":3,
			"free_tier_truncated":true
		}`)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "t", "test", nil, server.Client(), nil)

	resp, err := c.ListSessions(context.Background(), &ListSessionsParams{
		Status:         "open",
		Project:        "web",
		Limit:          25,
		Cursor:         "next-1",
		CreatedAtAfter: "2026-05-23T09:30:00Z",
	})
	if err != nil {
		t.Fatalf("ListSessions() error = %v, want nil", err)
	}
	if got, want := len(resp.Results), 1; got != want {
		t.Fatalf("len(Results) = %d, want %d", got, want)
	}
	if got, want := resp.Results[0].Project.Slug, "42"; got != want {
		t.Fatalf("Project.Slug = %q, want %q", got, want)
	}
	if got, want := resp.Results[0].Reporter.Email, "r@example.test"; got != want {
		t.Fatalf("Reporter.Email = %q, want %q", got, want)
	}
	if got, want := resp.Results[0].FirstPinFeedback, "broken button"; got != want {
		t.Fatalf("FirstPinFeedback = %q, want %q", got, want)
	}
	if got, want := resp.Results[0].Title, "Checkout failure"; got != want {
		t.Fatalf("Title = %q, want %q", got, want)
	}
	if got, want := resp.Results[0].Attachments[0].Filename, "notes.md"; got != want {
		t.Fatalf("Attachments[0].Filename = %q, want %q", got, want)
	}
	if got, want := resp.Results[0].Attachments[0].PinNumber, int64(2); got != want {
		t.Fatalf("Attachments[0].PinNumber = %d, want %d", got, want)
	}
	if resp.NextCursor == nil || *resp.NextCursor != "next-2" {
		t.Fatalf("NextCursor = %v, want next-2", resp.NextCursor)
	}
	if !resp.FreeTierTruncated {
		t.Fatal("FreeTierTruncated = false, want true")
	}
}

func TestListSessionsDecodesNullableProjectAndReporter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"results":[{
				"id":123,
				"project":null,
				"url":"https://example.test/page",
				"status":"open",
				"pin_count":2,
				"first_pin_feedback":"broken button",
				"reporter":null,
				"updated_at":"2026-05-01T12:00:00Z",
				"free_tier_locked":false
			}],
			"next_cursor":null,
			"count":1,
			"free_tier_truncated":false
		}`)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "t", "test", nil, server.Client(), nil)

	resp, err := c.ListSessions(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListSessions() error = %v, want nil", err)
	}
	if got, want := len(resp.Results), 1; got != want {
		t.Fatalf("len(Results) = %d, want %d", got, want)
	}
	if resp.Results[0].Project != nil {
		t.Fatalf("Project = %#v, want nil", resp.Results[0].Project)
	}
	if resp.Results[0].Reporter != nil {
		t.Fatalf("Reporter = %#v, want nil", resp.Results[0].Reporter)
	}
}

func TestSearchSessionsPinsScopeMapsPinHitsToSessions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/search/"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		query := r.URL.Query()
		if got, want := query.Get("q"), "checkout"; got != want {
			t.Fatalf("q query = %q, want %q", got, want)
		}
		if got, want := query.Get("scope"), "pins"; got != want {
			t.Fatalf("scope query = %q, want %q", got, want)
		}
		if got, want := query.Get("limit"), "10"; got != want {
			t.Fatalf("limit query = %q, want %q", got, want)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"results":[{
				"pin":{
					"id":456,
					"number":2,
					"feedback":"checkout button broken",
					"url":"https://example.test/page#pin",
					"selector":"#checkout",
					"element_info":{"tag":"button"},
					"metadata":{"severity":"high"}
				},
				"session":{
					"id":123,
					"team_slug":"acme",
					"project":{"id":42,"slug":"42","name":"Website"},
					"project_session_number":5,
					"url":"https://example.test/page",
					"status":"open",
					"pin_count":2,
					"first_pin_feedback":"checkout broken",
					"reporter":{"email":"r@example.test","display_name":"Reporter"},
					"updated_at":"2026-05-01T12:00:00Z",
					"free_tier_locked":false
				}
			}],
			"total":7
		}`)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "t", "test", nil, server.Client(), nil)

	resp, err := c.SearchSessions(context.Background(), &SearchParams{
		Query: "checkout",
		Scope: "pins",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchSessions() error = %v, want nil", err)
	}
	if got, want := resp.Total, 7; got != want {
		t.Fatalf("Total = %d, want %d", got, want)
	}
	if got, want := len(resp.Results), 1; got != want {
		t.Fatalf("len(Results) = %d, want %d", got, want)
	}
	if got, want := resp.Results[0].Status, "open"; got != want {
		t.Fatalf("Status = %q, want %q", got, want)
	}
	if resp.Results[0].Project == nil || resp.Results[0].Project.ID != 42 {
		t.Fatalf("Project = %#v, want project 42", resp.Results[0].Project)
	}
	if got, want := resp.Results[0].FirstPinFeedback, "checkout broken"; got != want {
		t.Fatalf("FirstPinFeedback = %q, want %q", got, want)
	}
	if got, want := resp.Results[0].ScopedID(), "acme/projects/42/sessions/5"; got != want {
		t.Fatalf("ScopedID() = %q, want %q", got, want)
	}
}

func TestSearchSessionsDefaultsScopeToSessions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/search/"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("scope"), "sessions"; got != want {
			t.Fatalf("scope query = %q, want %q", got, want)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[],"total":0}`)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "t", "test", nil, server.Client(), nil)

	resp, err := c.SearchSessions(context.Background(), &SearchParams{Query: "checkout"})
	if err != nil {
		t.Fatalf("SearchSessions() error = %v, want nil", err)
	}
	if got, want := resp.Total, 0; got != want {
		t.Fatalf("Total = %d, want %d", got, want)
	}
}

func TestSearchPinsSendsQueryParamsAndDecodesHit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/search/"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		query := r.URL.Query()
		if got, want := query.Get("q"), "checkout"; got != want {
			t.Fatalf("q query = %q, want %q", got, want)
		}
		if got, want := query.Get("scope"), "pins"; got != want {
			t.Fatalf("scope query = %q, want %q", got, want)
		}
		if got, want := query.Get("limit"), "5"; got != want {
			t.Fatalf("limit query = %q, want %q", got, want)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"results":[{
				"pin":{
					"id":456,
					"number":2,
					"feedback":"checkout button broken",
					"url":"https://example.test/page#pin",
					"selector":"#checkout",
					"element_info":{"tag":"button"},
					"metadata":{"severity":"high"}
				},
				"session":{
					"id":123,
					"team_slug":"acme",
					"project":{"id":42,"slug":"42","name":"Website"},
					"project_session_number":5,
					"url":"https://example.test/page",
					"status":"open",
					"pin_count":2,
					"first_pin_feedback":"checkout button broken",
					"reporter":{"email":"r@example.test","display_name":"Reporter"},
					"updated_at":"2026-05-01T12:00:00Z",
					"free_tier_locked":false
				}
			}],
			"total":1
		}`)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "t", "test", nil, server.Client(), nil)

	resp, err := c.SearchPins(context.Background(), &SearchParams{
		Query: "checkout",
		Scope: "sessions",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("SearchPins() error = %v, want nil", err)
	}
	if got, want := resp.Total, 1; got != want {
		t.Fatalf("Total = %d, want %d", got, want)
	}
	if got, want := len(resp.Results), 1; got != want {
		t.Fatalf("len(Results) = %d, want %d", got, want)
	}
	if got, want := resp.Results[0].Pin.Number, int64(2); got != want {
		t.Fatalf("Pin.Number = %d, want %d", got, want)
	}
	if got, want := resp.Results[0].Session.ScopedID(), "acme/projects/42/sessions/5"; got != want {
		t.Fatalf("Session.ScopedID() = %q, want %q", got, want)
	}
}

func TestGetSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/teams/acme/projects/42/sessions/5/"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":123,
			"title":"Checkout failure",
			"team_slug":"acme",
			"project_session_number":5,
			"report_url":"https://app.disbug.test/acme/projects/42/sessions/5/",
			"status":"open",
			"project":{"id":42,"slug":"42","name":"Website"},
			"reporter":{"email":"r@example.test","display_name":"Reporter"},
			"url":"https://example.test/page",
			"updated_at":"2026-05-01T12:00:00Z",
			"pins":[{
				"id":456,
				"number":7,
				"feedback":"broken button",
				"url":"https://example.test/page#pin",
				"selector":"#submit",
				"element_info":{"tag":"button"},
				"metadata":{"viewport":"mobile"},
				"attachments":[{"id":9,"filename":"notes.md","content_type":"text/markdown","size_bytes":42}]
			}]
		}`)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "t", "test", nil, server.Client(), nil)

	session, err := c.GetSession(context.Background(), testSessionRef)
	if err != nil {
		t.Fatalf("GetSession() error = %v, want nil", err)
	}
	if session.Project == nil {
		t.Fatal("Project = nil, want project")
	}
	if got, want := session.Project.Name, "Website"; got != want {
		t.Fatalf("Project.Name = %q, want %q", got, want)
	}
	if got, want := session.Title, "Checkout failure"; got != want {
		t.Fatalf("Title = %q, want %q", got, want)
	}
	if got, want := len(session.Pins), 1; got != want {
		t.Fatalf("len(Pins) = %d, want %d", got, want)
	}
	if got, want := session.Pins[0].Number, int64(7); got != want {
		t.Fatalf("Pins[0].Number = %d, want %d", got, want)
	}
	if session.Pins[0].URL == nil || *session.Pins[0].URL != "https://example.test/page#pin" {
		t.Fatalf("Pins[0].URL = %v, want https://example.test/page#pin", session.Pins[0].URL)
	}
	if session.Pins[0].Selector == nil || *session.Pins[0].Selector != "#submit" {
		t.Fatalf("Pins[0].Selector = %v, want #submit", session.Pins[0].Selector)
	}
	if got, want := session.Pins[0].ElementInfo["tag"], "button"; got != want {
		t.Fatalf("ElementInfo[tag] = %v, want %q", got, want)
	}
	if got, want := session.Pins[0].Attachments[0].Filename, "notes.md"; got != want {
		t.Fatalf("Attachments[0].Filename = %q, want %q", got, want)
	}
}

func TestGetSessionDecodesNullableProjectAndReporter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":123,
			"status":"open",
			"project":null,
			"reporter":null,
			"url":"https://example.test/page",
			"updated_at":"2026-05-01T12:00:00Z",
			"pins":[]
		}`)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "t", "test", nil, server.Client(), nil)

	session, err := c.GetSession(context.Background(), testSessionRef)
	if err != nil {
		t.Fatalf("GetSession() error = %v, want nil", err)
	}
	if session.Project != nil {
		t.Fatalf("Project = %#v, want nil", session.Project)
	}
	if session.Reporter != nil {
		t.Fatalf("Reporter = %#v, want nil", session.Reporter)
	}
}

func TestPinLitePreservesNullableURLAndSelector(t *testing.T) {
	var pin PinLite
	if err := json.Unmarshal([]byte(`{
		"id":456,
		"number":7,
		"feedback":"broken button",
		"url":null,
		"selector":null,
		"element_info":{},
		"metadata":{}
	}`), &pin); err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	if pin.URL != nil {
		t.Fatalf("URL = %v, want nil", *pin.URL)
	}
	if pin.Selector != nil {
		t.Fatalf("Selector = %v, want nil", *pin.Selector)
	}

	marshaled, err := json.Marshal(pin)
	if err != nil {
		t.Fatalf("Marshal() error = %v, want nil", err)
	}
	var remarshal map[string]any
	if err := json.Unmarshal(marshaled, &remarshal); err != nil {
		t.Fatalf("Unmarshal(remarshal) error = %v, want nil", err)
	}
	if value, ok := remarshal["url"]; !ok || value != nil {
		t.Fatalf("remarshaled url = %#v, want explicit null", value)
	}
	if value, ok := remarshal["selector"]; !ok || value != nil {
		t.Fatalf("remarshaled selector = %#v, want explicit null", value)
	}
}

func TestGetPin_FieldsParam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/teams/acme/projects/42/sessions/5/pins/by-number/7/"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("fields"), "console_logs,network_logs"; got != want {
			t.Fatalf("fields query = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":456,
			"number":7,
			"feedback":"broken button",
			"attachments":[{"id":9,"filename":"notes.md","content_type":"text/markdown","size_bytes":42}],
			"console":[{"level":"error","message":"boom"}],
			"network":[{"method":"GET","url":"https://example.test/api","status":500}]
		}`)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "t", "test", nil, server.Client(), nil)

	pin, err := c.GetPinByNumber(context.Background(), testSessionRef, 7, []string{"network", "console"})
	if err != nil {
		t.Fatalf("GetPinByNumber() error = %v, want nil", err)
	}
	if got, want := len(pin.Console), 1; got != want {
		t.Fatalf("len(Console) = %d, want %d", got, want)
	}
	if got, want := len(pin.Network), 1; got != want {
		t.Fatalf("len(Network) = %d, want %d", got, want)
	}
	if got, want := pin.Attachments[0].Filename, "notes.md"; got != want {
		t.Fatalf("Attachments[0].Filename = %q, want %q", got, want)
	}
}

func TestGetPin_AllOmitsFields(t *testing.T) {
	assertGetPinOmitsFields(t, []string{"all"})
}

func TestGetPin_EmptyFieldsOmitFields(t *testing.T) {
	assertGetPinOmitsFields(t, nil)
	assertGetPinOmitsFields(t, []string{})
}

func assertGetPinOmitsFields(t *testing.T, fields []string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.URL.Query()["fields"]; ok {
			t.Fatalf("fields query was set to %q, want omitted", r.URL.Query().Get("fields"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":456,"number":7,"feedback":"broken button"}`)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "t", "test", nil, server.Client(), nil)

	if _, err := c.GetPinByNumber(context.Background(), testSessionRef, 7, fields); err != nil {
		t.Fatalf("GetPinByNumber() error = %v, want nil", err)
	}
}

func TestGetPin_UnknownFieldReturnsErrorWithoutHTTPRequest(t *testing.T) {
	calls := 0
	c := New("https://api.example.test", "t", "test", nil, doerFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected call")
	}), nil)

	_, err := c.GetPinByNumber(context.Background(), testSessionRef, 7, []string{"unknown"})
	if err == nil {
		t.Fatal("GetPinByNumber() error = nil, want validation error")
	}
	if calls != 0 {
		t.Fatalf("HTTP calls = %d, want 0", calls)
	}
}

func TestGetPin_DecodesHeavyFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":456,
			"number":7,
			"feedback":"broken button",
			"screenshot":{"url":"https://cdn.example.test/s.png","content_type":"image/png","size_bytes":null,"expires_at":"2026-05-01T13:00:00Z"},
			"session_replay":{"url":"https://cdn.example.test/r.webm"},
			"voice_note":{"url":"https://cdn.example.test/v.mp3"},
			"video_recording":{"url":"https://cdn.example.test/v.webm"},
			"console":[{"type":"console","datetime":"2026-05-01T12:00:00Z","value":"boom","extra":{"level":"error"},"full_url":"https://example.test/page","short_url":"/page","epochTime":1777636800000}],
			"network":[{"type":"network","status":201,"method":"POST","full_url":"https://example.test/api","short_url":"/api","epochTime":1777636800001,"headers":{"x-test":"1"},"body":"ok"}],
			"events":[{"target":"#submit","type":"click","timestamp":"2026-05-01T12:00:01Z","x":1}]
		}`)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "t", "test", nil, server.Client(), nil)

	pin, err := c.GetPinByNumber(context.Background(), testSessionRef, 7, []string{"all"})
	if err != nil {
		t.Fatalf("GetPinByNumber() error = %v, want nil", err)
	}
	if pin.Screenshot == nil || pin.Screenshot.URL == "" {
		t.Fatal("Screenshot was not decoded")
	}
	if pin.Screenshot.SizeBytes != nil {
		t.Fatalf("Screenshot.SizeBytes = %v, want nil", *pin.Screenshot.SizeBytes)
	}
	if pin.SessionReplay == nil || pin.VoiceNote == nil || pin.VideoRecording == nil {
		t.Fatal("one or more asset fields were not decoded")
	}
	if got, want := len(pin.Events), 1; got != want {
		t.Fatalf("len(Events) = %d, want %d", got, want)
	}
}

func TestBulkResultExitCodeFromFirstFailure(t *testing.T) {
	result := BulkResult{Errors: []BulkErrItem{
		{Code: "network_error", Message: "dial failed"},
		{Code: "internal_error", Message: "server failed"},
	}}

	if !result.AllFailed() {
		t.Fatal("AllFailed() = false, want true")
	}
	if got, want := result.FirstFailureExitCode(), 5; got != want {
		t.Fatalf("FirstFailureExitCode() = %d, want %d", got, want)
	}

	result = BulkResult{
		Pins:   []*PinFull{{PinLite: PinLite{Number: 1}}},
		Errors: []BulkErrItem{{Code: "not_found", Message: "missing"}},
	}
	if result.AllFailed() {
		t.Fatal("AllFailed() = true, want false with a successful pin")
	}
	if got, want := result.FirstFailureExitCode(), errfmt.ExitCode(&errfmt.APIError{StatusCode: http.StatusNotFound}); got != want {
		t.Fatalf("FirstFailureExitCode() = %d, want %d", got, want)
	}
}
