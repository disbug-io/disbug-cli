package setup

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallWritesBrowserManifestsWithExplicitExtensionID(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "bin", "disbug")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatalf("MkdirAll(bin dir) error = %v", err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(bin) error = %v", err)
	}

	result, err := Install(Options{
		HomeDir:      home,
		GOOS:         "darwin",
		BinaryPath:   bin,
		ExtensionIDs: []string{"abcdefghijklmnopabcdefghijklmnop"},
		SkipMCP:      true,
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(result.Manifests) < 3 {
		t.Fatalf("installed manifests = %#v, want Chrome/Brave/Chromium", result.Manifests)
	}

	raw, err := os.ReadFile(filepath.Join(
		home,
		"Library/Application Support/Google/Chrome/NativeMessagingHosts/io.disbug.bridge.json",
	))
	if err != nil {
		t.Fatalf("ReadFile(chrome manifest) error = %v", err)
	}
	var manifest HostManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("Unmarshal(manifest) error = %v", err)
	}
	if got, want := manifest.Name, "io.disbug.bridge"; got != want {
		t.Fatalf("manifest name = %q, want %q", got, want)
	}
	if got, want := manifest.Path, bin; got != want {
		t.Fatalf("manifest path = %q, want %q", got, want)
	}
	if got, want := manifest.AllowedOrigins[0], "chrome-extension://abcdefghijklmnopabcdefghijklmnop/"; got != want {
		t.Fatalf("allowed origin = %q, want %q", got, want)
	}
}

func TestInstallRequiresExtensionID(t *testing.T) {
	_, err := Install(Options{
		HomeDir:    t.TempDir(),
		GOOS:       "linux",
		BinaryPath: "/usr/local/bin/disbug",
		SkipMCP:    true,
	})
	if err == nil {
		t.Fatal("Install() error = nil, want missing extension id error")
	}
}

func TestInstallRegistersDetectedMCPConfigsAndSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldLookPath := lookPath
	lookPath = func(string) (string, error) {
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() {
		lookPath = oldLookPath
	})

	mustMkdir(t, filepath.Join(home, ".codex"))
	mustMkdir(t, filepath.Join(home, ".cursor"))
	mustMkdir(t, filepath.Join(home, ".claude"))
	mustMkdir(t, filepath.Join(home, "Library/Application Support/Claude"))

	result, err := Install(Options{
		HomeDir:      home,
		GOOS:         "linux",
		BinaryPath:   "/usr/local/bin/disbug",
		ExtensionIDs: []string{"abcdefghijklmnopabcdefghijklmnop"},
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	for agent, want := range map[string]string{
		"codex":         "registered",
		"cursor":        "registered",
		"claudeDesktop": "registered",
		"claudeCode":    "not detected",
	} {
		if got := result.MCP[agent]; got != want {
			t.Fatalf("MCP[%s] = %q, want %q; all=%#v", agent, got, want, result.MCP)
		}
	}
	for agent, want := range map[string]string{
		"codex":  "registered",
		"claude": "registered",
	} {
		if got := result.Skills[agent]; got != want {
			t.Fatalf("Skills[%s] = %q, want %q; all=%#v", agent, got, want, result.Skills)
		}
	}

	assertFileContains(t, filepath.Join(home, ".codex", "config.toml"), "[mcp_servers.disbug]")
	assertFileContains(t, filepath.Join(home, ".cursor", "mcp.json"), `"disbug"`)
	assertFileContains(t, filepath.Join(home, "Library/Application Support/Claude/claude_desktop_config.json"), `"disbug"`)
	assertFileContains(t, filepath.Join(home, ".codex", "skills", "disbug-local", "SKILL.md"), "get_session")
	assertFileContains(t, filepath.Join(home, ".claude", "skills", "disbug-local", "SKILL.md"), "Disbug Local Report")

	status := MCPStatuses(home)
	if got, want := status["codex"], "registered"; got != want {
		t.Fatalf("MCPStatuses(codex) = %q, want %q", got, want)
	}
	skillStatus := SkillStatuses(home)
	if got, want := skillStatus["claude"], "registered"; got != want {
		t.Fatalf("SkillStatuses(claude) = %q, want %q", got, want)
	}
}

func TestManifestDiagnosticsDetectsStaleBinaryPath(t *testing.T) {
	home := t.TempDir()
	_, err := Install(Options{
		HomeDir:      home,
		GOOS:         "linux",
		BinaryPath:   "/old/disbug",
		ExtensionIDs: []string{"abcdefghijklmnopabcdefghijklmnop"},
		SkipMCP:      true,
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	diagnostics := ManifestDiagnostics(Options{
		HomeDir:    home,
		GOOS:       "linux",
		BinaryPath: "/new/disbug",
	})
	if len(diagnostics) == 0 {
		t.Fatal("ManifestDiagnostics() returned no diagnostics")
	}
	if got, want := diagnostics[0].Status, "outdated"; got != want {
		t.Fatalf("diagnostic status = %q, want %q; diagnostics=%#v", got, want, diagnostics)
	}
	if got, want := diagnostics[0].ActualPath, "/old/disbug"; got != want {
		t.Fatalf("diagnostic actual path = %q, want %q", got, want)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", path, err)
	}
}

func assertFileContains(t *testing.T, path string, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("%s = %q, want substring %q", path, data, want)
	}
}

func TestMCPStatusesReportsClaudeCodeConfig(t *testing.T) {
	home := t.TempDir()
	oldLookPath := lookPath
	lookPath = func(string) (string, error) {
		return "", errors.New("not on PATH")
	}
	t.Cleanup(func() {
		lookPath = oldLookPath
	})

	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"mcpServers":{"disbug":{"command":"disbug","args":["mcp"]}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile(.claude.json) error = %v", err)
	}
	if got, want := MCPStatuses(home)["claudeCode"], "registered"; got != want {
		t.Fatalf("claudeCode status = %q, want %q", got, want)
	}
}
