package localstore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreCommitRejectsTraversalAndIndexesReport(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	pragmas, err := store.Pragmas(ctx)
	if err != nil {
		t.Fatalf("Pragmas() error = %v", err)
	}
	if got, want := strings.ToLower(pragmas.JournalMode), "wal"; got != want {
		t.Fatalf("JournalMode = %q, want %q", got, want)
	}
	if got, want := strings.ToUpper(pragmas.Synchronous), "NORMAL"; got != want {
		t.Fatalf("Synchronous = %q, want %q", got, want)
	}

	report, err := store.BeginReport(ctx, ReportMetadata{
		URL:              "https://example.test/path",
		CreatedAt:        "2026-05-06T10:00:00Z",
		ExtensionVersion: "4.0.0",
		SchemaVersion:    1,
		TotalSize:        512,
	})
	if err != nil {
		t.Fatalf("BeginReport() error = %v", err)
	}

	if err := report.WriteFile("../escape.json", "application/json", []byte(`{}`)); err == nil {
		t.Fatal("WriteFile(../escape.json) error = nil, want traversal rejection")
	}

	writeReportFixture(t, report)

	committed, err := report.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if !strings.HasPrefix(committed.ID, "local_") {
		t.Fatalf("committed ID = %q, want local_ prefix", committed.ID)
	}
	if !strings.Contains(committed.Prompt, `get_session(id="`+committed.ID+`", source="local")`) {
		t.Fatalf("prompt = %q, want MCP get_session guidance", committed.Prompt)
	}
	if !strings.Contains(committed.Prompt, committed.Path) {
		t.Fatalf("prompt = %q, want report path %q", committed.Prompt, committed.Path)
	}
	if _, err := os.Stat(filepath.Join(committed.Path, "pin_1", "pin.json")); err != nil {
		t.Fatalf("committed pin file missing: %v", err)
	}

	session, err := store.GetSession(ctx, committed.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if got, want := session["id"], committed.ID; got != want {
		t.Fatalf("session id = %#v, want %#v", got, want)
	}
	if got, want := session["url"], "https://example.test/path"; got != want {
		t.Fatalf("session url = %#v, want %#v", got, want)
	}

	pin, err := store.GetPin(ctx, committed.ID, 1, nil)
	if err != nil {
		t.Fatalf("GetPin() error = %v", err)
	}
	if got, want := pin["feedback"], "button missing"; got != want {
		t.Fatalf("pin feedback = %#v, want %#v", got, want)
	}
	screenshot, ok := pin["screenshot"].(map[string]any)
	if !ok {
		t.Fatalf("pin screenshot = %#v, want asset envelope", pin["screenshot"])
	}
	if path, _ := screenshot["path"].(string); !filepath.IsAbs(path) || !strings.HasSuffix(filepath.ToSlash(path), "pin_1/screenshot.png") {
		t.Fatalf("screenshot path = %#v, want absolute pin path", screenshot["path"])
	}

	list, err := store.ListSessions(ctx, ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if got, want := len(list.Results), 1; got != want {
		t.Fatalf("ListSessions results = %d, want %d", got, want)
	}
	if got, want := list.Results[0].FirstPinFeedback, "button missing"; got != want {
		t.Fatalf("FirstPinFeedback = %q, want %q", got, want)
	}

	latest, err := store.LatestSession(ctx)
	if err != nil {
		t.Fatalf("LatestSession() error = %v", err)
	}
	if got, want := latest.ID, committed.ID; got != want {
		t.Fatalf("LatestSession ID = %q, want %q", got, want)
	}

	summary, err := store.GetSessionSummary(ctx, committed.ID)
	if err != nil {
		t.Fatalf("GetSessionSummary() error = %v", err)
	}
	if got, want := summary.ReportPath, committed.Path; got != want {
		t.Fatalf("GetSessionSummary ReportPath = %q, want %q", got, want)
	}

	if err := store.DeleteSession(ctx, committed.ID); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if _, err := os.Stat(committed.Path); !os.IsNotExist(err) {
		t.Fatalf("committed path exists after delete; stat err = %v", err)
	}
}

func TestStorePruneRemovesOldCommittedSessions(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	oldReport, err := store.BeginReport(ctx, ReportMetadata{
		URL:       "https://old.example.test",
		CreatedAt: "2026-04-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("BeginReport(old) error = %v", err)
	}
	writeReportFixture(t, oldReport)
	oldCommitted, err := oldReport.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit(old) error = %v", err)
	}

	newReport, err := store.BeginReport(ctx, ReportMetadata{
		URL:       "https://new.example.test",
		CreatedAt: "2026-05-05T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("BeginReport(new) error = %v", err)
	}
	writeReportFixture(t, newReport)
	newCommitted, err := newReport.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit(new) error = %v", err)
	}

	removed, err := store.Prune(ctx, PruneOptions{
		OlderThan: 30 * 24 * time.Hour,
		Now:       time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if got, want := removed, 1; got != want {
		t.Fatalf("Prune removed = %d, want %d", got, want)
	}
	if _, err := os.Stat(oldCommitted.Path); !os.IsNotExist(err) {
		t.Fatalf("old session path exists after prune; stat err = %v", err)
	}
	if _, err := os.Stat(newCommitted.Path); err != nil {
		t.Fatalf("new session path missing after prune: %v", err)
	}
}

func TestStoreListSessionsFiltersSinceCutoff(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	oldReport, err := store.BeginReport(ctx, ReportMetadata{
		URL:       "https://old.example.test",
		CreatedAt: "2026-05-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("BeginReport(old) error = %v", err)
	}
	writeReportFixture(t, oldReport)
	oldCommitted, err := oldReport.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit(old) error = %v", err)
	}

	newReport, err := store.BeginReport(ctx, ReportMetadata{
		URL:       "https://new.example.test",
		CreatedAt: "2026-05-03T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("BeginReport(new) error = %v", err)
	}
	writeReportFixture(t, newReport)
	newCommitted, err := newReport.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit(new) error = %v", err)
	}

	since := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	list, err := store.ListSessions(ctx, ListOptions{Limit: 10, Since: since})
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if got, want := len(list.Results), 1; got != want {
		t.Fatalf("ListSessions results = %d, want %d", got, want)
	}
	if got, want := list.Results[0].ID, newCommitted.ID; got != want {
		t.Fatalf("ListSessions result ID = %q, want %q", got, want)
	}
	if got, notWant := list.Results[0].ID, oldCommitted.ID; got == notWant {
		t.Fatalf("ListSessions included old ID = %q", got)
	}
}

func writeReportFixture(t *testing.T, report *Report) {
	t.Helper()

	files := map[string]string{
		"session.json": `{
			"id": "pending",
			"status": "open",
			"url": "https://example.test/path",
			"updated_at": "2026-05-06T10:00:00Z",
			"pins": [{"id": "pin_1", "number": 1, "feedback": "button missing", "url": "https://example.test/path", "selector": "#submit", "element_info": {}, "metadata": {}}]
		}`,
		"pin_1/pin.json": `{
			"id": "pin_1",
			"number": 1,
			"feedback": "button missing",
			"url": "https://example.test/path",
			"selector": "#submit",
			"element_info": {},
			"metadata": {}
		}`,
		"pin_1/logs.json": `{"console":[{"level":"error","message":"boom"}],"network":[],"events":[]}`,
	}
	for path, content := range files {
		if err := report.WriteFile(path, "application/json", []byte(content)); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}
	if err := report.WriteFile("pin_1/screenshot.png", "image/png", []byte{0x89, 'P', 'N', 'G'}); err != nil {
		t.Fatalf("WriteFile(screenshot) error = %v", err)
	}
}
