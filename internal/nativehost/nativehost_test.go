package nativehost

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"
)

func TestRunHelloAndChunkedCommit(t *testing.T) {
	ctx := context.Background()
	var stdin bytes.Buffer
	for _, msg := range []any{
		map[string]any{"type": "hello", "protocol": 1},
		map[string]any{"type": "begin_report", "metadata": map[string]any{
			"url": "https://example.test/path", "created_at": "2026-05-06T10:00:00Z", "extension_version": "4.0.0",
		}},
		putJSON("session.json", `{"id":"pending","status":"open","url":"https://example.test/path","updated_at":"2026-05-06T10:00:00Z","pins":[{"id":"pin_1","number":1,"feedback":"button missing","url":"https://example.test/path","selector":"#submit","element_info":{},"metadata":{}}]}`),
		putJSON("pin_1/pin.json", `{"id":"pin_1","number":1,"feedback":"button missing","url":"https://example.test/path","selector":"#submit","element_info":{},"metadata":{}}`),
		putJSON("pin_1/logs.json", `{"console":[],"network":[],"events":[]}`),
		map[string]any{"type": "put_file_chunk", "path": "pin_1/screenshot.png", "index": 0, "content_type": "image/png", "data_base64": base64.StdEncoding.EncodeToString([]byte{0x89, 'P'})},
		map[string]any{"type": "put_file_chunk", "path": "pin_1/screenshot.png", "index": 1, "content_type": "image/png", "data_base64": base64.StdEncoding.EncodeToString([]byte{'N', 'G'})},
		map[string]any{"type": "finish_file", "path": "pin_1/screenshot.png"},
		map[string]any{"type": "commit_report"},
	} {
		if err := WriteFrame(&stdin, msg); err != nil {
			t.Fatalf("WriteFrame(input) error = %v", err)
		}
	}

	var stdout bytes.Buffer
	if err := Run(ctx, &stdin, &stdout, Options{StoreRoot: t.TempDir(), Version: "1.2.3"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	frames := readAllFrames(t, &stdout)
	assertFrameType(t, frames[0], "hello_ack")
	committed := findFrame(t, frames, "committed")
	if committed["report_id"] == "" {
		t.Fatalf("committed report_id = %#v, want non-empty", committed["report_id"])
	}
	prompt, _ := committed["prompt"].(string)
	if !bytes.Contains([]byte(prompt), []byte(`source="local"`)) || !bytes.Contains([]byte(prompt), []byte(`Path:`)) {
		t.Fatalf("committed prompt = %q, want local MCP prompt with path", prompt)
	}
	if committed["report_path"] == "" {
		t.Fatalf("committed report_path = %#v, want non-empty", committed["report_path"])
	}
}

func TestRunRejectsTraversalPath(t *testing.T) {
	var stdin bytes.Buffer
	for _, msg := range []any{
		map[string]any{"type": "begin_report", "metadata": map[string]any{"url": "https://example.test"}},
		putJSON("../escape.json", `{}`),
	} {
		if err := WriteFrame(&stdin, msg); err != nil {
			t.Fatalf("WriteFrame(input) error = %v", err)
		}
	}

	var stdout bytes.Buffer
	if err := Run(context.Background(), &stdin, &stdout, Options{StoreRoot: t.TempDir(), Version: "1.2.3"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	errFrame := findFrame(t, readAllFrames(t, &stdout), "error")
	if got, want := errFrame["code"], "retryable"; got != want {
		t.Fatalf("error code = %#v, want %#v", got, want)
	}
}

func putJSON(path string, raw string) map[string]any {
	return map[string]any{
		"type":         "put_file",
		"path":         path,
		"content_type": "application/json",
		"data_base64":  base64.StdEncoding.EncodeToString([]byte(raw)),
	}
}

func readAllFrames(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var frames []map[string]any
	for buf.Len() > 0 {
		var frame map[string]any
		if err := ReadFrame(buf, &frame); err != nil {
			t.Fatalf("ReadFrame() error = %v", err)
		}
		frames = append(frames, frame)
	}
	return frames
}

func assertFrameType(t *testing.T, frame map[string]any, want string) {
	t.Helper()
	if got := frame["type"]; got != want {
		t.Fatalf("frame type = %#v, want %#v; frame=%#v", got, want, frame)
	}
}

func findFrame(t *testing.T, frames []map[string]any, typ string) map[string]any {
	t.Helper()
	for _, frame := range frames {
		if frame["type"] == typ {
			return frame
		}
	}
	t.Fatalf("frame type %q not found in %#v", typ, frames)
	return nil
}
