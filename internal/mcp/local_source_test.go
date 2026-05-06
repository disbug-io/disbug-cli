package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/disbug-io/disbug-cli/internal/localstore"
)

func TestLocalSourceToolsWorkWithoutToken(t *testing.T) {
	store, id := seedLocalStore(t)
	srv := newServer(&Deps{LocalStore: store, CloudAvailable: false})

	listRes, err := callTool(t, srv, "list_sessions", map[string]any{"source": "local", "limit": 10})
	if err != nil {
		t.Fatalf("CallTool(list_sessions local) error = %v, want nil", err)
	}
	if listRes.IsError {
		t.Fatalf("list_sessions local IsError = true: %#v", listRes.Content)
	}
	if text := firstTextContent(t, listRes); !strings.Contains(text, id) || !strings.Contains(text, "button missing") {
		t.Fatalf("list_sessions local content = %q, want local id and feedback", text)
	}

	sessionRes, err := callTool(t, srv, "get_session", map[string]any{"id": id, "source": "local"})
	if err != nil {
		t.Fatalf("CallTool(get_session local) error = %v, want nil", err)
	}
	if sessionRes.IsError {
		t.Fatalf("get_session local IsError = true: %#v", sessionRes.Content)
	}
	if text := firstTextContent(t, sessionRes); !strings.Contains(text, `"id":"`+id+`"`) {
		t.Fatalf("get_session local content = %q, want local id", text)
	}

	pinRes, err := callTool(t, srv, "get_pin", map[string]any{"pin": id + ".1", "source": "local"})
	if err != nil {
		t.Fatalf("CallTool(get_pin local) error = %v, want nil", err)
	}
	if pinRes.IsError {
		t.Fatalf("get_pin local IsError = true: %#v", pinRes.Content)
	}
	if text := firstTextContent(t, pinRes); !strings.Contains(text, `"path":`) || !strings.Contains(text, "screenshot.png") {
		t.Fatalf("get_pin local content = %q, want local asset path envelope", text)
	}

	latestRes, err := callTool(t, srv, "get_latest_local_session", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool(get_latest_local_session) error = %v, want nil", err)
	}
	if latestRes.IsError {
		t.Fatalf("get_latest_local_session IsError = true: %#v", latestRes.Content)
	}
	if text := firstTextContent(t, latestRes); !strings.Contains(text, id) {
		t.Fatalf("get_latest_local_session content = %q, want local id", text)
	}
}

func TestCloudSourceWithoutTokenReturnsLoginGuidance(t *testing.T) {
	store, _ := seedLocalStore(t)
	srv := newServer(&Deps{LocalStore: store, CloudAvailable: false})

	res, err := callTool(t, srv, "get_session", map[string]any{"id": "7392", "source": "cloud"})
	if err != nil {
		t.Fatalf("CallTool(get_session cloud) error = %v, want nil tool error result", err)
	}
	if !res.IsError {
		t.Fatal("get_session cloud IsError = false, want true")
	}
	if text := firstTextContent(t, res); !strings.Contains(text, "Run: disbug login") {
		t.Fatalf("get_session cloud error = %q, want login guidance", text)
	}
}

func seedLocalStore(t *testing.T) (*localstore.Store, string) {
	t.Helper()
	store, err := localstore.Open(t.TempDir())
	if err != nil {
		t.Fatalf("localstore.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("store.Close() error = %v", err)
		}
	})

	report, err := store.BeginReport(context.Background(), localstore.ReportMetadata{
		URL:       "https://example.test/path",
		CreatedAt: "2026-05-06T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("BeginReport() error = %v", err)
	}
	if err := report.WriteFile("session.json", "application/json", []byte(`{"id":"pending","status":"open","url":"https://example.test/path","updated_at":"2026-05-06T10:00:00Z","pins":[{"id":"pin_1","number":1,"feedback":"button missing","url":"https://example.test/path","selector":"#submit","element_info":{},"metadata":{}}]}`)); err != nil {
		t.Fatalf("WriteFile(session) error = %v", err)
	}
	if err := report.WriteFile("pin_1/pin.json", "application/json", []byte(`{"id":"pin_1","number":1,"feedback":"button missing","url":"https://example.test/path","selector":"#submit","element_info":{},"metadata":{}}`)); err != nil {
		t.Fatalf("WriteFile(pin) error = %v", err)
	}
	if err := report.WriteFile("pin_1/logs.json", "application/json", []byte(`{"console":[],"network":[],"events":[]}`)); err != nil {
		t.Fatalf("WriteFile(logs) error = %v", err)
	}
	if err := report.WriteFile("pin_1/screenshot.png", "image/png", []byte{0x89, 'P', 'N', 'G'}); err != nil {
		t.Fatalf("WriteFile(screenshot) error = %v", err)
	}
	committed, err := report.Commit(context.Background())
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	return store, committed.ID
}
