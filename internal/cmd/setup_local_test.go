package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestSetupLocalQuietOutputByDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(context.Background(), []string{
		"setup-local",
		"--extension-id", "abcdefghijklmnopabcdefghijklmnop",
		"--skip-mcp",
	}, nil, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Execute(setup-local) error = %v", err)
	}
	if got, want := stdout.String(), "Disbug local setup complete. You can now click Copy in the Disbug extension.\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if strings.Contains(stdout.String(), "native_messaging_manifests") {
		t.Fatalf("stdout contains verbose manifest details: %q", stdout.String())
	}
}

func TestSetupLocalVerboseOutputIncludesDetails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(context.Background(), []string{
		"--verbose",
		"setup-local",
		"--extension-id", "abcdefghijklmnopabcdefghijklmnop",
		"--skip-mcp",
	}, nil, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Execute(--verbose setup-local) error = %v", err)
	}
	if !strings.Contains(stdout.String(), "native_messaging_manifests:") {
		t.Fatalf("stdout = %q, want verbose manifest details", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Local reports will be saved to") {
		t.Fatalf("stdout = %q, want local report path", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}
