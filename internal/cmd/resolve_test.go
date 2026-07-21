package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveSuccess(t *testing.T) {
	reportURL := "https://staging.disbug.us/abb/projects/2/sessions/5/"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/me/" {
			_, _ = io.WriteString(w, `{"capabilities":["resolve_session"]}`)
			return
		}
		if got, want := r.URL.Path, "/api/teams/abb/projects/2/sessions/5/resolve/"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := r.Method, http.MethodPost; got != want {
			t.Fatalf("method = %q, want %q", got, want)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got, want := body["summary"], "Fixed checkout and ran tests."; got != want {
			t.Fatalf("summary = %q, want %q", got, want)
		}
		_, _ = io.WriteString(w, `{
			"team_slug":"abb",
			"project":{"id":2,"slug":"2","name":"Web"},
			"project_session_number":5,
			"report_url":"https://staging.disbug.us/abb/projects/2/sessions/5/",
			"status":"resolved",
			"url":"https://example.test",
			"updated_at":"2026-07-20T00:00:00Z",
			"pins":[]
		}`)
	}))
	t.Cleanup(server.Close)
	setupClient(t, server)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(
		context.Background(),
		[]string{"resolve", reportURL, "--summary", "Fixed checkout and ran tests."},
		nil,
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"status":"resolved"`)) {
		t.Fatalf("stdout = %q, want resolved status", stdout.String())
	}
}

func TestResolveRequiresSummary(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(
		context.Background(),
		[]string{"resolve", "https://app.disbug.io/abb/projects/2/sessions/5/"},
		nil,
		&stdout,
		&stderr,
	)

	if err == nil {
		t.Fatal("Execute() error = nil, want usage error")
	}
	if got, want := ExitCode(err), 2; got != want {
		t.Fatalf("ExitCode() = %d, want %d", got, want)
	}
}
