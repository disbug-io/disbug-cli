package localbundle

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSummaryDoesNotExposeRawArtifactContent(t *testing.T) {
	path := writeTestBundle(t)

	bundle, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	summary, err := bundle.Summary()
	if err != nil {
		t.Fatalf("Summary() error = %v, want nil", err)
	}

	if got, want := summary.Session.PinCount, 1; got != want {
		t.Fatalf("PinCount = %d, want %d", got, want)
	}
	if got, want := summary.Pins[0].Feedback, "button missing"; got != want {
		t.Fatalf("Feedback = %q, want %q", got, want)
	}
	if !summary.Pins[0].Artifacts.Screenshot {
		t.Fatal("Artifacts.Screenshot = false, want true")
	}
	if !summary.Pins[0].Artifacts.Replay {
		t.Fatal("Artifacts.Replay = false, want true")
	}
	if got, want := summary.Pins[0].Artifacts.ConsoleCount, 1; got != want {
		t.Fatalf("ConsoleCount = %d, want %d", got, want)
	}

	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("Marshal(summary) error = %v", err)
	}
	if bytes.Contains(encoded, []byte("iVBORw0KGgo=")) {
		t.Fatalf("summary contains raw screenshot base64: %s", encoded)
	}
}

func TestInspectPinExtractsRequestedArtifacts(t *testing.T) {
	path := writeTestBundle(t)
	cacheDir := t.TempDir()
	bundle, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	pin, err := bundle.InspectPin(1, []string{"screenshot", "replay", "console", "network", "events"}, cacheDir)
	if err != nil {
		t.Fatalf("InspectPin() error = %v, want nil", err)
	}

	if got, want := pin.Number, 1; got != want {
		t.Fatalf("Number = %d, want %d", got, want)
	}
	if got, want := len(pin.Console), 1; got != want {
		t.Fatalf("len(Console) = %d, want %d", got, want)
	}
	if got, want := len(pin.Network), 1; got != want {
		t.Fatalf("len(Network) = %d, want %d", got, want)
	}
	if got, want := len(pin.Events), 1; got != want {
		t.Fatalf("len(Events) = %d, want %d", got, want)
	}
	if pin.Screenshot == nil {
		t.Fatal("Screenshot = nil, want extracted artifact")
	}
	if got := readFileString(t, pin.Screenshot.Path); got != "png-bytes" {
		t.Fatalf("screenshot file content = %q, want png-bytes", got)
	}
	if pin.SessionReplay == nil {
		t.Fatal("SessionReplay = nil, want extracted artifact")
	}
	if got, want := pin.SessionReplay.EventCount, 2; got != want {
		t.Fatalf("Replay EventCount = %d, want %d", got, want)
	}
	if got := filepath.Base(pin.SessionReplay.Path); got != "replay.json.gz" {
		t.Fatalf("replay filename = %q, want replay.json.gz", got)
	}
}

func writeTestBundle(t *testing.T) string {
	t.Helper()

	replay := gzipJSON(t, map[string]any{
		"version":       1,
		"type":          "rrweb",
		"rrweb_version": "2.0.0",
		"started_at":    "2026-06-10T12:00:00Z",
		"ended_at":      "2026-06-10T12:00:01Z",
		"events": []map[string]any{
			{"type": 0, "timestamp": 1_786_275_000_000},
			{"type": 1, "timestamp": 1_786_275_001_000},
		},
	})
	payload := map[string]any{
		"schema_version": 1,
		"exported_at":    "2026-06-10T12:34:56Z",
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
				"content": `{
					"console":[{"level":"error","message":"boom"}],
					"network":[{"url":"/api/items","status":500}],
					"events":[{"type":"click"}]
				}`,
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

func gzipJSON(t *testing.T, value any) []byte {
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

func readFileString(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}
