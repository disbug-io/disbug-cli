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

func TestInstallWritesWindowsManifestAndRegistryKeys(t *testing.T) {
	home := t.TempDir()
	localAppData := filepath.Join(home, "AppData", "Local")
	bin := filepath.Join(home, "bin", "disbug.exe")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatalf("MkdirAll(bin dir) error = %v", err)
	}
	if err := os.WriteFile(bin, []byte("windows binary"), 0o755); err != nil {
		t.Fatalf("WriteFile(bin) error = %v", err)
	}

	writes := map[string]string{}
	oldRegistryWrite := registryWrite
	registryWrite = func(key, value string) error {
		writes[key] = value
		return nil
	}
	t.Cleanup(func() {
		registryWrite = oldRegistryWrite
	})

	result, err := Install(Options{
		HomeDir:      home,
		GOOS:         "windows",
		BinaryPath:   bin,
		LocalAppData: localAppData,
		ExtensionIDs: []string{"abcdefghijklmnopabcdefghijklmnop"},
		SkipMCP:      true,
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if got, want := len(result.Manifests), 1; got != want {
		t.Fatalf("installed manifests = %#v, want %d Windows manifest", result.Manifests, want)
	}

	manifestPath := filepath.Join(localAppData, "disbug", "NativeMessagingHosts", "io.disbug.bridge.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(windows manifest) error = %v", err)
	}
	var manifest HostManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("Unmarshal(manifest) error = %v", err)
	}
	if got, want := manifest.Path, bin; got != want {
		t.Fatalf("manifest path = %q, want %q", got, want)
	}
	if got, want := len(writes), len(windowsRegistryTargets()); got != want {
		t.Fatalf("registry writes = %#v, want %d browser keys", writes, want)
	}
	for _, target := range windowsRegistryTargets() {
		if got, want := writes[target.key], manifestPath; got != want {
			t.Fatalf("registry write for %s = %q, want %q", target.key, got, want)
		}
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

func TestManifestDiagnosticsReportsWindowsRegistryState(t *testing.T) {
	home := t.TempDir()
	localAppData := filepath.Join(home, "AppData", "Local")
	manifestPath := filepath.Join(localAppData, "disbug", "NativeMessagingHosts", "io.disbug.bridge.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o700); err != nil {
		t.Fatalf("MkdirAll(manifest dir) error = %v", err)
	}
	data, err := json.MarshalIndent(HostManifest{
		Name:           hostName,
		Description:    "Disbug local AI bridge",
		Path:           `C:\tools\disbug.exe`,
		Type:           "stdio",
		AllowedOrigins: []string{"chrome-extension://abcdefghijklmnopabcdefghijklmnop/"},
	}, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(manifest) error = %v", err)
	}
	if err := os.WriteFile(manifestPath, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	oldRegistryQuery := registryQuery
	registryQuery = func(key string) (string, error) {
		switch key {
		case windowsRegistryTargets()[0].key:
			return manifestPath, nil
		case windowsRegistryTargets()[1].key:
			return `C:\wrong\io.disbug.bridge.json`, nil
		default:
			return "", errors.New("not found")
		}
	}
	t.Cleanup(func() {
		registryQuery = oldRegistryQuery
	})

	diagnostics := ManifestDiagnostics(Options{
		HomeDir:      home,
		GOOS:         "windows",
		BinaryPath:   `C:\tools\disbug.exe`,
		LocalAppData: localAppData,
	})
	if len(diagnostics) != 1+len(windowsRegistryTargets()) {
		t.Fatalf("diagnostics = %#v, want manifest plus registry entries", diagnostics)
	}
	if got, want := diagnostics[0].Status, "registered"; got != want {
		t.Fatalf("manifest diagnostic status = %q, want %q", got, want)
	}
	if got, want := diagnostics[1].Status, "registered"; got != want {
		t.Fatalf("chrome registry status = %q, want %q", got, want)
	}
	if got, want := diagnostics[2].Status, "outdated"; got != want {
		t.Fatalf("chromium registry status = %q, want %q", got, want)
	}
	if got, want := diagnostics[3].Status, "missing"; got != want {
		t.Fatalf("brave registry status = %q, want %q", got, want)
	}
}

func TestParseRegistryDefaultValue(t *testing.T) {
	value, err := parseRegistryDefaultValue(`
HKEY_CURRENT_USER\Software\Google\Chrome\NativeMessagingHosts\io.disbug.bridge
    (Default)    REG_SZ    C:\Users\test\AppData\Local\disbug\NativeMessagingHosts\io.disbug.bridge.json
`)
	if err != nil {
		t.Fatalf("parseRegistryDefaultValue() error = %v", err)
	}
	want := `C:\Users\test\AppData\Local\disbug\NativeMessagingHosts\io.disbug.bridge.json`
	if value != want {
		t.Fatalf("parseRegistryDefaultValue() = %q, want %q", value, want)
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
