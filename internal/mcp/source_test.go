package mcp

import (
	"strings"
	"testing"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

func TestNormalizeSourceRejectsLocal(t *testing.T) {
	_, err := normalizeSource("local")
	if err == nil {
		t.Fatal("normalizeSource(local) error = nil, want usage error")
	}
	if !strings.Contains(errfmt.Format(err), "source must be auto or cloud") {
		t.Fatalf("normalizeSource(local) error = %q, want auto/cloud guidance", errfmt.Format(err))
	}
}

func TestLatestLocalSessionToolIsNotRegistered(t *testing.T) {
	srv := newServer(nil)

	_, err := callTool(t, srv, "get_latest_local_session", map[string]any{})
	if err == nil {
		t.Fatal("CallTool(get_latest_local_session) error = nil, want unknown tool")
	}
}
