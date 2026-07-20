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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/disbug-io/disbug-cli/internal/token"
)

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
		ReportURL: "https://app.disbug.test/acme/projects/2/sessions/5/",
		Title:     "Checkout button fails",
		CreatedAt: "2026-05-23T08:24:43Z",
		Pins:      []watchPin{{Number: 1, Feedback: "Button does nothing"}},
		id:        "123",
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
		"DISBUG_EVENT_ID=123",
		"DISBUG_EVENT_SOURCE=cloud",
		"DISBUG_EVENT_REF=https://app.disbug.test/acme/projects/2/sessions/5/",
		"DISBUG_EVENT_JSON=",
	} {
		if !strings.Contains(envText, want) {
			t.Fatalf("exec env missing %q in:\n%s", want, envText)
		}
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	if got := stdout.String(); !strings.Contains(got, `"title":"Checkout button fails"`) {
		t.Fatalf("stdout = %q, want JSONL event", got)
	}
	for _, unwanted := range []string{`"type"`, `"pin_count"`, `"source"`} {
		if got := stdout.String(); strings.Contains(got, unwanted) {
			t.Fatalf("stdout = %q, does not want %s", got, unwanted)
		}
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
		ReportURL: "https://app.disbug.test/acme/projects/2/sessions/5/",
		Title:     "Checkout button fails",
		CreatedAt: "2026-05-23T08:24:43Z",
		id:        "123",
		backfill:  true,
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

func TestWatchLocalOnlyFlagIsNotSupported(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Execute(context.Background(), []string{"watch", "--local-only"}, nil, &stdout, &stderr)

	if err == nil {
		t.Fatal("Execute(watch --local-only) error = nil, want usage error")
	}
	if !strings.Contains(err.Error(), "unknown flag --local-only") && !strings.Contains(stderr.String(), "unknown flag --local-only") {
		t.Fatalf("error = %v stderr = %q, want unknown --local-only flag", err, stderr.String())
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

func TestWatchCloudOnlyBackfillsAndPollsCloudSessions(t *testing.T) {
	var listRequests atomic.Int32
	var createdAtAfterMu sync.Mutex
	var createdAtAfter []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sessions/":
			listRequests.Add(1)
			if got := r.URL.Query().Get("created_at_after"); got == "" {
				t.Fatal("created_at_after query is empty")
			} else {
				createdAtAfterMu.Lock()
				createdAtAfter = append(createdAtAfter, got)
				createdAtAfterMu.Unlock()
			}
			if got, want := r.URL.Query().Get("status"), "open"; got != want {
				t.Fatalf("status query = %q, want %q", got, want)
			}
			if got, want := r.URL.Query().Get("project"), "web"; got != want {
				t.Fatalf("project query = %q, want %q", got, want)
			}
			if got, want := r.URL.Query().Get("include"), "pins"; got != want {
				t.Fatalf("include query = %q, want %q", got, want)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{
				"results":[{
					"title":"Checkout button fails",
					"team_slug":"abb",
					"project":{"id":2,"slug":"2","name":"Website"},
					"project_session_number":5,
					"report_url":"https://staging.disbug.us/abb/projects/2/sessions/5/",
					"url":"https://example.test/page",
					"status":"open",
					"pins":[{"number":1,"feedback":"cloud feedback"}],
					"reporter":{"email":"r@example.test","display_name":"Reporter"},
					"created_at":"2026-05-23T08:20:00Z",
					"updated_at":"2026-05-23T08:21:00Z",
					"free_tier_locked":false
				}],
				"next_cursor":null,
				"count":1,
				"free_tier_truncated":false
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
		`"report_url":"https://staging.disbug.us/abb/projects/2/sessions/5/"`,
		`"title":"Checkout button fails"`,
		`"number":1`,
		`"feedback":"cloud feedback"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
	if got := strings.Count(output, `"report_url":"https://staging.disbug.us/abb/projects/2/sessions/5/"`); got != 1 {
		t.Fatalf("cloud session emitted %d times, want once; stdout=%q", got, output)
	}
	for _, unwanted := range []string{`"type"`, `"source"`, `"backfill"`, `"pin_count"`} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("stdout = %q, does not want %s", output, unwanted)
		}
	}
	if got := listRequests.Load(); got < 2 {
		t.Fatalf("listRequests = %d, want backfill and live poll", got)
	}
	createdAtAfterMu.Lock()
	defer createdAtAfterMu.Unlock()
	if len(createdAtAfter) < 2 || createdAtAfter[0] == createdAtAfter[1] {
		t.Fatalf("created_at_after queries = %v, want advancing live watermark", createdAtAfter)
	}
	if got := stderr.String(); !strings.Contains(got, "watching: cloud") {
		t.Fatalf("stderr = %q, want cloud startup line", got)
	}
}

func setupWatchClient(t *testing.T, srv *httptest.Server) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_TOKEN", "")
	t.Setenv("DISBUG_API_URL", "")

	if err := token.Write("default", token.Token{Token: "test-token", APIURL: srv.URL}, false); err != nil {
		t.Fatalf("token.Write() error = %v", err)
	}
}
