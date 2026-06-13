package mcp

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectLocalReportSummary(t *testing.T) {
	reportPath := writeMCPLocalReport(t)
	srv := newServer(nil)

	res, err := callTool(t, srv, "inspect_local_report", map[string]any{"path": reportPath})
	if err != nil {
		t.Fatalf("CallTool(inspect_local_report) error = %v, want nil", err)
	}
	if res.IsError {
		t.Fatalf("inspect_local_report IsError = true, want false: %#v", res.Content)
	}

	text := firstTextContent(t, res)
	if !strings.Contains(text, `"source":"local"`) {
		t.Fatalf("inspect_local_report content = %q, want local source", text)
	}
	if !strings.Contains(text, "button missing") {
		t.Fatalf("inspect_local_report content = %q, want pin feedback", text)
	}
	if strings.Contains(text, "cG5nLWJ5dGVz") {
		t.Fatalf("inspect_local_report summary contains raw screenshot base64: %s", text)
	}
}

func TestInspectLocalReportPinFieldsExtractsArtifacts(t *testing.T) {
	reportPath := writeMCPLocalReport(t)
	cacheDir := t.TempDir()
	srv := newServer(nil)

	res, err := callTool(t, srv, "inspect_local_report", map[string]any{
		"path":      reportPath,
		"pin":       1,
		"fields":    []string{"screenshot", "replay", "console"},
		"cache_dir": cacheDir,
	})
	if err != nil {
		t.Fatalf("CallTool(inspect_local_report) error = %v, want nil", err)
	}
	if res.IsError {
		t.Fatalf("inspect_local_report IsError = true, want false: %#v", res.Content)
	}

	var detail struct {
		Pin struct {
			Number        int              `json:"number"`
			Console       []map[string]any `json:"console"`
			Screenshot    *localMCPAsset   `json:"screenshot"`
			SessionReplay *localMCPReplay  `json:"session_replay"`
		} `json:"pin"`
	}
	text := firstTextContent(t, res)
	if err := json.Unmarshal([]byte(text), &detail); err != nil {
		t.Fatalf("Unmarshal(inspect_local_report) error = %v; text=%s", err, text)
	}
	if got, want := detail.Pin.Number, 1; got != want {
		t.Fatalf("Pin.Number = %d, want %d", got, want)
	}
	if got, want := len(detail.Pin.Console), 1; got != want {
		t.Fatalf("len(Console) = %d, want %d", got, want)
	}
	if detail.Pin.Screenshot == nil {
		t.Fatal("Pin.Screenshot = nil, want extracted artifact")
	}
	if got := readMCPFile(t, detail.Pin.Screenshot.Path); got != "png-bytes" {
		t.Fatalf("screenshot file = %q, want png-bytes", got)
	}
	if !strings.HasPrefix(detail.Pin.Screenshot.Path, cacheDir) {
		t.Fatalf("screenshot path = %q, want under cache dir %q", detail.Pin.Screenshot.Path, cacheDir)
	}
	if detail.Pin.SessionReplay == nil {
		t.Fatal("Pin.SessionReplay = nil, want extracted artifact")
	}
	if got, want := detail.Pin.SessionReplay.EventCount, 2; got != want {
		t.Fatalf("Replay EventCount = %d, want %d", got, want)
	}
}

func TestInspectLocalReportInvalidFieldReturnsToolError(t *testing.T) {
	reportPath := writeMCPLocalReport(t)
	srv := newServer(nil)

	res, err := callTool(t, srv, "inspect_local_report", map[string]any{
		"path":   reportPath,
		"pin":    1,
		"fields": []string{"console", "unknown"},
	})
	if err != nil {
		t.Fatalf("CallTool(inspect_local_report) error = %v, want nil tool error result", err)
	}
	if !res.IsError {
		t.Fatalf("inspect_local_report IsError = false, want true")
	}
}

type localMCPAsset struct {
	Path string `json:"path"`
}

type localMCPReplay struct {
	Path       string `json:"path"`
	EventCount int    `json:"event_count"`
}

func writeMCPLocalReport(t *testing.T) string {
	t.Helper()

	replay := gzipMCPJSON(t, map[string]any{
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
				"content":      `{"console":[{"level":"error","message":"boom"}]}`,
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
		t.Fatalf("WriteFile(test bundle) error = %v", err)
	}
	return path
}

func gzipMCPJSON(t *testing.T, value any) []byte {
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

func readMCPFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}
