package cmd

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/disbug-io/disbug-cli/internal/localstore"
	"github.com/disbug-io/disbug-cli/internal/token"
)

func TestLocalBackfillEventsEmitChronologicalSessionNewEvents(t *testing.T) {
	ctx := context.Background()
	store, err := localstore.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	oldReport, err := store.BeginReport(ctx, localstore.ReportMetadata{
		URL:       "https://old.example.test/dashboard",
		CreatedAt: "2026-05-21T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("BeginReport(old) error = %v", err)
	}
	writeWatchReportFixture(t, oldReport, "old feedback")
	oldCommitted, err := oldReport.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit(old) error = %v", err)
	}

	newReport, err := store.BeginReport(ctx, localstore.ReportMetadata{
		URL:       "https://new.example.test/dashboard",
		CreatedAt: "2026-05-21T11:00:00Z",
	})
	if err != nil {
		t.Fatalf("BeginReport(new) error = %v", err)
	}
	writeWatchReportFixture(t, newReport, "new feedback")
	newCommitted, err := newReport.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit(new) error = %v", err)
	}

	events, err := localBackfillEvents(ctx, store, localWatchOptions{
		Since: time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC),
		Now:   fixedWatchNow,
	})
	if err != nil {
		t.Fatalf("localBackfillEvents() error = %v", err)
	}

	if got, want := len(events), 2; got != want {
		t.Fatalf("len(events) = %d, want %d", got, want)
	}
	if got, want := events[0].Session.ID, oldCommitted.ID; got != want {
		t.Fatalf("first event ID = %q, want %q", got, want)
	}
	if got, want := events[1].Session.ID, newCommitted.ID; got != want {
		t.Fatalf("second event ID = %q, want %q", got, want)
	}
	if got, want := events[0].Type, "session.new"; got != want {
		t.Fatalf("event Type = %q, want %q", got, want)
	}
	if got, want := events[0].Source, "local"; got != want {
		t.Fatalf("event Source = %q, want %q", got, want)
	}
	if !events[0].Backfill {
		t.Fatal("event Backfill = false, want true")
	}
	if got, want := events[0].EmittedAt, "2026-05-23T08:24:43Z"; got != want {
		t.Fatalf("event EmittedAt = %q, want %q", got, want)
	}
	if got, want := events[0].Session.Ref, "disbug://local/"+oldCommitted.ID; got != want {
		t.Fatalf("event ref = %q, want %q", got, want)
	}
	if got, want := events[0].Session.SourceURL, "https://old.example.test/dashboard"; got != want {
		t.Fatalf("event source_url = %q, want %q", got, want)
	}
	if got := events[0].Session.URL; got != "" {
		t.Fatalf("event url = %q, want omitted local public URL", got)
	}
	if got, want := len(events[0].Session.Pins), 1; got != want {
		t.Fatalf("event pins = %d, want %d", got, want)
	}
	if got, want := events[0].Session.Pins[0].PinNumber, 1; got != want {
		t.Fatalf("pin_number = %d, want %d", got, want)
	}
	if got, want := events[0].Session.Pins[0].Feedback, "old feedback"; got != want {
		t.Fatalf("pin feedback = %q, want %q", got, want)
	}
}

func TestLocalBackfillEventsHonorsStatusFilter(t *testing.T) {
	ctx := context.Background()
	store, err := localstore.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	report, err := store.BeginReport(ctx, localstore.ReportMetadata{
		URL:       "https://example.test/dashboard",
		CreatedAt: "2026-05-21T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("BeginReport() error = %v", err)
	}
	writeWatchReportFixture(t, report, "open feedback")
	if _, err := report.Commit(ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	events, err := localBackfillEvents(ctx, store, localWatchOptions{
		Since:  time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC),
		Status: "resolved",
		Now:    fixedWatchNow,
	})
	if err != nil {
		t.Fatalf("localBackfillEvents() error = %v", err)
	}
	if got := len(events); got != 0 {
		t.Fatalf("len(events) = %d, want 0", got)
	}
}

func TestWatchEmitterRunsExecForLiveEventWithEnvironment(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	envPath := filepath.Join(t.TempDir(), "env.txt")
	emitter := watchEmitter{
		stdout: &stdout,
		stderr: &stderr,
		format: "jsonl",
		exec:   "env > " + envPath,
	}

	event := watchEvent{
		Type:      "session.new",
		Source:    "local",
		Backfill:  false,
		EmittedAt: "2026-05-23T08:24:43Z",
		Session: watchSession{
			ID:  "local_123",
			Ref: "disbug://local/local_123",
		},
	}
	if err := emitter.emit(context.Background(), event); err != nil {
		t.Fatalf("emit() error = %v", err)
	}

	env, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile(env) error = %v", err)
	}
	envText := string(env)
	for _, want := range []string{
		"DISBUG_EVENT_ID=local_123",
		"DISBUG_EVENT_SOURCE=local",
		"DISBUG_EVENT_REF=disbug://local/local_123",
		"DISBUG_EVENT_JSON=",
	} {
		if !strings.Contains(envText, want) {
			t.Fatalf("exec env missing %q in:\n%s", want, envText)
		}
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	if got := stdout.String(); !strings.Contains(got, `"type":"session.new"`) {
		t.Fatalf("stdout = %q, want JSONL event", got)
	}
}

func TestWatchEmitterDoesNotRunExecForBackfillEvent(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	envPath := filepath.Join(t.TempDir(), "env.txt")
	emitter := watchEmitter{
		stdout: &stdout,
		stderr: &stderr,
		format: "jsonl",
		exec:   "env > " + envPath,
	}

	event := watchEvent{
		Type:      "session.new",
		Source:    "local",
		Backfill:  true,
		EmittedAt: "2026-05-23T08:24:43Z",
		Session: watchSession{
			ID:  "local_123",
			Ref: "disbug://local/local_123",
		},
	}
	if err := emitter.emit(context.Background(), event); err != nil {
		t.Fatalf("emit() error = %v", err)
	}

	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Fatalf("exec output exists for backfill event; stat err = %v", err)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestWatchLocalPollWithSinceDoesNotEmitOlderExistingSessions(t *testing.T) {
	ctx := context.Background()
	store, err := localstore.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	oldReport, err := store.BeginReport(ctx, localstore.ReportMetadata{
		URL:       "https://old.example.test/dashboard",
		CreatedAt: "2026-05-21T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("BeginReport(old) error = %v", err)
	}
	writeWatchReportFixture(t, oldReport, "old feedback")
	if _, err := oldReport.Commit(ctx); err != nil {
		t.Fatalf("Commit(old) error = %v", err)
	}

	recentReport, err := store.BeginReport(ctx, localstore.ReportMetadata{
		URL:       "https://new.example.test/dashboard",
		CreatedAt: "2026-05-23T07:00:00Z",
	})
	if err != nil {
		t.Fatalf("BeginReport(recent) error = %v", err)
	}
	writeWatchReportFixture(t, recentReport, "new feedback")
	recentCommitted, err := recentReport.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit(recent) error = %v", err)
	}

	pollCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var stdout bytes.Buffer
	err = watchLocalPoll(
		pollCtx,
		store,
		localWatchOptions{Since: time.Date(2026, 5, 23, 6, 0, 0, 0, time.UTC), Now: fixedWatchNow},
		time.Millisecond,
		map[string]bool{dedupeKey("local", recentCommitted.ID): true},
		watchEmitter{stdout: &stdout, stderr: &bytes.Buffer{}, format: "jsonl"},
	)
	if err != nil {
		t.Fatalf("watchLocalPoll() error = %v, want nil", err)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("watchLocalPoll() stdout = %q, want no older live events", got)
	}
}

func TestWatchCloudOnlyBackfillsAndPollsCloudSessions(t *testing.T) {
	var listRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sessions/":
			listRequests++
			if got := r.URL.Query().Get("created_at_after"); got == "" {
				t.Fatal("created_at_after query is empty")
			}
			if got, want := r.URL.Query().Get("status"), "open"; got != want {
				t.Fatalf("status query = %q, want %q", got, want)
			}
			if got, want := r.URL.Query().Get("project"), "web"; got != want {
				t.Fatalf("project query = %q, want %q", got, want)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{
				"results":[{
					"id":123,
					"project":{"slug":"web","name":"Website"},
					"url":"https://example.test/page",
					"status":"open",
					"pin_count":1,
					"first_pin_feedback":"cloud feedback",
					"reporter":{"email":"r@example.test","display_name":"Reporter"},
					"created_at":"2026-05-23T08:20:00Z",
					"updated_at":"2026-05-23T08:21:00Z",
					"free_tier_locked":false
				}],
				"next_cursor":null,
				"count":1,
				"free_tier_truncated":false
			}`)
		case "/api/sessions/123/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{
				"id":123,
				"status":"open",
				"project":{"slug":"web","name":"Website"},
				"reporter":{"email":"r@example.test","display_name":"Reporter"},
				"url":"https://example.test/page",
				"updated_at":"2026-05-23T08:21:00Z",
				"pins":[{"id":456,"number":1,"feedback":"cloud feedback","url":"https://example.test/page#pin"}]
			}`)
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	setupWatchClient(t, server)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(
		ctx,
		[]string{"watch", "--cloud-only", "--since", "2h", "--status", "open", "--project", "web", "--poll-interval", "1ms"},
		nil,
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr.String())
	}

	output := stdout.String()
	for _, want := range []string{
		`"source":"cloud"`,
		`"backfill":true`,
		`"id":"123"`,
		`"ref":"disbug://cloud/123"`,
		`"project":"web"`,
		`"pin_number":1`,
		`"feedback":"cloud feedback"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
	if got := strings.Count(output, `"id":"123"`); got != 1 {
		t.Fatalf("cloud session emitted %d times, want once; stdout=%q", got, output)
	}
	if listRequests < 2 {
		t.Fatalf("listRequests = %d, want backfill and live poll", listRequests)
	}
	if got := stderr.String(); !strings.Contains(got, "watching: cloud") {
		t.Fatalf("stderr = %q, want cloud startup line", got)
	}
}

func fixedWatchNow() time.Time {
	return time.Date(2026, 5, 23, 8, 24, 43, 0, time.UTC)
}

func setupWatchClient(t *testing.T, srv *httptest.Server) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_LOCAL_STORE_DIR", t.TempDir())
	t.Setenv("DISBUG_TOKEN", "")
	t.Setenv("DISBUG_API_URL", "")

	if err := token.Write("default", token.Token{Token: "test-token", APIURL: srv.URL}, false); err != nil {
		t.Fatalf("token.Write() error = %v", err)
	}
}

func writeWatchReportFixture(t *testing.T, report *localstore.Report, feedback string) {
	t.Helper()

	session := `{
		"id": "pending",
		"status": "open",
		"url": "https://example.test/path",
		"updated_at": "2026-05-21T10:00:00Z",
		"pins": [
			{"id": "pin_1", "number": 1, "feedback": "` + feedback + `", "url": "https://example.test/path#pin-1"}
		]
	}`
	if err := report.WriteFile("session.json", "application/json", []byte(session)); err != nil {
		t.Fatalf("WriteFile(session.json) error = %v", err)
	}
	pin := `{
		"id": "pin_1",
		"number": 1,
		"feedback": "` + feedback + `",
		"url": "https://example.test/path#pin-1"
	}`
	if err := report.WriteFile("pin_1/pin.json", "application/json", []byte(pin)); err != nil {
		t.Fatalf("WriteFile(pin_1/pin.json) error = %v", err)
	}
}
