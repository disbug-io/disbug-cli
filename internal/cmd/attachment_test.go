package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttachmentDownloadWritesFileAndMetadata(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			writePinCapabilities(w, "attachment_download")
		case "/api/teams/abb/projects/2/sessions/5/pins/by-number/2/attachments/9/download/":
			w.Header().Set("Content-Disposition", `attachment; filename="notes.md"`)
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = io.WriteString(w, "# Agent notes")
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)
	setupClient(t, backend)
	destination := filepath.Join(t.TempDir(), "saved.md")

	stdout, stderr, err := executeAttachment(
		t,
		"attachment",
		"download",
		testPinReportURL,
		"9",
		"--output",
		destination,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "# Agent notes"; got != want {
		t.Fatalf("saved attachment = %q, want %q", got, want)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode stdout: %v; stdout=%q", err, stdout)
	}
	if got, want := result["path"], destination; got != want {
		t.Fatalf("path = %#v, want %q", got, want)
	}
}

func TestAttachmentDownloadStdoutWritesRawBytes(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			writePinCapabilities(w, "attachment_download")
		case "/api/teams/abb/projects/2/sessions/5/pins/by-number/2/attachments/9/download/":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, "agent-readable")
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)
	setupClient(t, backend)

	stdout, stderr, err := executeAttachment(t, "attachment", "download", testPinReportURL, "9", "--stdout")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if got, want := stdout, "agent-readable"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestAttachmentDownloadRequiresBackendCapability(t *testing.T) {
	downloadCalled := false
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			writePinCapabilities(w, "scoped_pin_lookup")
		default:
			downloadCalled = true
			t.Fatalf("download endpoint called without capability: %s", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)
	setupClient(t, backend)

	stdout, stderr, err := executeAttachment(t, "attachment", "download", testPinReportURL, "9", "--stdout")
	if err == nil {
		t.Fatal("Execute() error = nil, want missing capability error")
	}
	if downloadCalled {
		t.Fatal("download endpoint was called")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "attachment_download") {
		t.Fatalf("stderr = %q, want missing capability", stderr)
	}
}

func executeAttachment(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(context.Background(), args, nil, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}
