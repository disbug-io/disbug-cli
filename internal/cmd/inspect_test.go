package cmd

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

func TestInspectLocalReportSummary(t *testing.T) {
	reportPath := writeInspectBundle(t)

	stdout, stderr, err := executeInspect(t, "inspect", reportPath)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	if bytes.Contains([]byte(stdout), []byte("iVBORw0KGgo=")) {
		t.Fatalf("summary output contains raw base64: %s", stdout)
	}

	var summary struct {
		Source  string `json:"source"`
		Session struct {
			PinCount int `json:"pin_count"`
		} `json:"session"`
		Pins []struct {
			Number    int    `json:"number"`
			Feedback  string `json:"feedback"`
			Artifacts struct {
				Screenshot   bool `json:"screenshot"`
				Replay       bool `json:"replay"`
				ConsoleCount int  `json:"console_count"`
			} `json:"artifacts"`
		} `json:"pins"`
	}
	if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v; stdout=%s", err, stdout)
	}
	if got, want := summary.Source, "local"; got != want {
		t.Fatalf("Source = %q, want %q", got, want)
	}
	if got, want := summary.Session.PinCount, 1; got != want {
		t.Fatalf("PinCount = %d, want %d", got, want)
	}
	if got, want := summary.Pins[0].Feedback, "button missing"; got != want {
		t.Fatalf("Feedback = %q, want %q", got, want)
	}
	if !summary.Pins[0].Artifacts.Screenshot || !summary.Pins[0].Artifacts.Replay {
		t.Fatalf("Artifacts = %+v, want screenshot and replay", summary.Pins[0].Artifacts)
	}
}

func TestInspectLocalReportPinFieldsExtractsArtifacts(t *testing.T) {
	reportPath := writeInspectBundle(t)
	cacheDir := t.TempDir()

	stdout, stderr, err := executeInspect(t,
		"inspect",
		reportPath,
		"--pin", "1",
		"--fields", "screenshot,replay,console,network,events",
		"--cache-dir", cacheDir,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}

	var detail struct {
		Pin struct {
			Number        int              `json:"number"`
			Console       []map[string]any `json:"console"`
			Network       []map[string]any `json:"network"`
			Events        []map[string]any `json:"events"`
			Screenshot    *localAsset      `json:"screenshot"`
			SessionReplay *localReplay     `json:"session_replay"`
		} `json:"pin"`
	}
	if err := json.Unmarshal([]byte(stdout), &detail); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v; stdout=%s", err, stdout)
	}
	if got, want := detail.Pin.Number, 1; got != want {
		t.Fatalf("Pin.Number = %d, want %d", got, want)
	}
	if detail.Pin.Screenshot == nil {
		t.Fatal("Pin.Screenshot = nil, want extracted artifact")
	}
	if got := readInspectFile(t, detail.Pin.Screenshot.Path); got != "png-bytes" {
		t.Fatalf("screenshot file = %q, want png-bytes", got)
	}
	if detail.Pin.SessionReplay == nil {
		t.Fatal("Pin.SessionReplay = nil, want extracted artifact")
	}
	if got, want := detail.Pin.SessionReplay.EventCount, 2; got != want {
		t.Fatalf("Replay EventCount = %d, want %d", got, want)
	}
	if got, want := len(detail.Pin.Console), 1; got != want {
		t.Fatalf("len(Console) = %d, want %d", got, want)
	}
	if got, want := len(detail.Pin.Network), 1; got != want {
		t.Fatalf("len(Network) = %d, want %d", got, want)
	}
	if got, want := len(detail.Pin.Events), 1; got != want {
		t.Fatalf("len(Events) = %d, want %d", got, want)
	}
}

func TestInspectInvalidLocalReportReturnsUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte(`{"files": "not an array"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	stdout, _, err := executeInspect(t, "inspect", path)

	if err == nil {
		t.Fatal("Execute() error = nil, want usage error")
	}
	var usage *errfmt.UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("Execute() error = %T, want errfmt.UsageError", err)
	}
	if got := stdout; got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

type localAsset struct {
	Path string `json:"path"`
}

type localReplay struct {
	Path       string `json:"path"`
	EventCount int    `json:"event_count"`
}

func executeInspect(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(context.Background(), args, nil, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func writeInspectBundle(t *testing.T) string {
	t.Helper()

	replay := gzipInspectJSON(t, map[string]any{
		"version":       1,
		"type":          "rrweb",
		"rrweb_version": "2.0.0",
		"events": []map[string]any{
			{"type": 0, "timestamp": 1_786_275_000_000},
			{"type": 1, "timestamp": 1_786_275_001_000},
		},
	})
	payload := map[string]any{
		"schema_version": 1,
		"manifest": map[string]any{
			"source_url": "https://example.test/path",
			"pin_count":  1,
		},
		"session": map[string]any{
			"id":         "local",
			"status":     "open",
			"url":        "https://example.test/path",
			"updated_at": "2026-06-10T12:34:56Z",
			"pins": []map[string]any{{
				"id":       "pin_1",
				"number":   1,
				"feedback": "button missing",
				"url":      "https://example.test/path",
			}},
		},
		"files": []map[string]any{
			{
				"path":         "pin_1/logs.json",
				"content_type": "application/json",
				"encoding":     "utf-8",
				"content":      `{"console":[{"level":"error","message":"boom"}],"network":[{"url":"/api/items","status":500}],"events":[{"type":"click"}]}`,
			},
			{
				"path":         "pin_1/screenshot.png",
				"content_type": "image/png",
				"encoding":     "base64",
				"content":      base64.StdEncoding.EncodeToString([]byte("png-bytes")),
			},
			{
				"path":         "pin_1/replay.json.gz",
				"content_type": "application/gzip",
				"encoding":     "base64",
				"content":      base64.StdEncoding.EncodeToString(replay),
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal(test bundle) error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "disbug-report.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func gzipInspectJSON(t *testing.T, value any) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if err := json.NewEncoder(gz).Encode(value); err != nil {
		t.Fatalf("Encode(replay) error = %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("Close(gzip) error = %v", err)
	}
	return buf.Bytes()
}

func readInspectFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}
