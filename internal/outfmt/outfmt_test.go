package outfmt

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteJSONCompact(t *testing.T) {
	var buf bytes.Buffer

	err := WriteJSON(&buf, map[string]string{"a": "b"}, false)
	if err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	if got, want := buf.String(), "{\"a\":\"b\"}\n"; got != want {
		t.Fatalf("WriteJSON() = %q, want %q", got, want)
	}
	if strings.Contains(buf.String(), "\n  ") {
		t.Fatalf("WriteJSON() should not indent compact output: %q", buf.String())
	}
}

func TestWriteJSONPretty(t *testing.T) {
	var buf bytes.Buffer

	err := WriteJSON(&buf, map[string]string{"a": "b"}, true)
	if err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	if !strings.Contains(buf.String(), "  \"a\"") {
		t.Fatalf("WriteJSON() should use two-space indentation: %q", buf.String())
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Fatalf("WriteJSON() should end with newline: %q", buf.String())
	}
}

func TestWriteJSONDoesNotEscapeHTML(t *testing.T) {
	var buf bytes.Buffer

	err := WriteJSON(&buf, map[string]string{"html": "<&>"}, false)
	if err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	if got, want := buf.String(), "{\"html\":\"<&>\"}\n"; got != want {
		t.Fatalf("WriteJSON() = %q, want %q", got, want)
	}
}

func TestWriteJSONWrapsEncodeErrors(t *testing.T) {
	var buf bytes.Buffer

	err := WriteJSON(&buf, func() {}, false)

	if err == nil {
		t.Fatal("WriteJSON() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "encode json:") {
		t.Fatalf("WriteJSON() error = %q, want context", err.Error())
	}
}
