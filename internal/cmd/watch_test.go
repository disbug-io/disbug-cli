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
		Type:      "session.new",
		Source:    "cloud",
		Backfill:  false,
		EmittedAt: "2026-05-23T08:24:43Z",
		Session: watchSession{
			ID:  "123",
			Ref: "disbug://cloud/123",
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
		"DISBUG_EVENT_ID=123",
		"DISBUG_EVENT_SOURCE=cloud",
		"DISBUG_EVENT_REF=disbug://cloud/123",
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
		Source:    "cloud",
		Backfill:  true,
		EmittedAt: "2026-05-23T08:24:43Z",
		Session: watchSession{
			ID:  "123",
			Ref: "disbug://cloud/123",
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sessions/":
			listRequests.Add(1)
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
					"team_slug":"abb",
					"project":{"id":2,"slug":"2","name":"Website"},
					"project_session_number":5,
					"report_url":"https://staging.disbug.us/abb/projects/2/sessions/5/",
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
		case "/api/teams/abb/projects/2/sessions/5/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{
				"id":123,
				"team_slug":"abb",
				"project_session_number":5,
				"status":"open",
				"project":{"id":2,"slug":"2","name":"Website"},
				"reporter":{"email":"r@example.test","display_name":"Reporter"},
				"report_url":"https://staging.disbug.us/abb/projects/2/sessions/5/",
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
		`"id":"abb/projects/2/sessions/5"`,
		`"ref":"https://staging.disbug.us/abb/projects/2/sessions/5/"`,
		`"project":"2"`,
		`"pin_number":1`,
		`"feedback":"cloud feedback"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
	if got := strings.Count(output, `"id":"abb/projects/2/sessions/5"`); got != 1 {
		t.Fatalf("cloud session emitted %d times, want once; stdout=%q", got, output)
	}
	if got := listRequests.Load(); got < 2 {
		t.Fatalf("listRequests = %d, want backfill and live poll", got)
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
