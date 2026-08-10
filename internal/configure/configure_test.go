package configure

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCursorPlanAndApplyPreserveOtherMCPServers(t *testing.T) {
	home := t.TempDir()
	cursorDir := filepath.Join(home, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(cursorDir, "mcp.json")
	if err := os.WriteFile(configPath, []byte(`{"mcpServers":{"other":{"command":"other-server"}},"setting":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := testManager(home, "default")

	changes, err := manager.Plan(context.Background(), Options{Agents: []AgentID{Cursor}})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("changes = %#v, want MCP and skill", changes)
	}
	requireApplySuccess(t, manager.Apply(context.Background(), changes))

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	servers := root["mcpServers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Fatalf("other MCP server was removed: %s", data)
	}
	disbug := servers["disbug"].(map[string]any)
	if got := disbug["command"]; got != "/usr/local/bin/disbug" {
		t.Fatalf("disbug command = %v", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "using-disbug", "SKILL.md")); err != nil {
		t.Fatalf("shared skill was not installed: %v", err)
	}

	changes, err = manager.Plan(context.Background(), Options{Agents: []AgentID{Cursor}})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("second plan = %#v, want no changes", changes)
	}
}

func TestPlanProtectsUnmanagedSkillUnlessForced(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, ".agents", "skills", "using-disbug", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("user-owned skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := testManager(home, "default")

	_, err := manager.Plan(context.Background(), Options{Agents: []AgentID{Cursor}, SkipMCP: true})
	if err == nil {
		t.Fatal("Plan() error = nil, want unmanaged skill conflict")
	}

	changes, err := manager.Plan(context.Background(), Options{Agents: []AgentID{Cursor}, SkipMCP: true, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Action != "update" {
		t.Fatalf("changes = %#v, want one skill update", changes)
	}
}

func TestCodexConfigureUsesProfileBeforeMCPCommand(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	manager := testManager(home, "work")
	var calls [][]string
	manager.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		call := append([]string{name}, args...)
		calls = append(calls, call)
		if len(args) >= 3 && args[0] == "mcp" && args[1] == "get" {
			return nil, errors.New("not configured")
		}
		return nil, nil
	}

	changes, err := manager.Plan(context.Background(), Options{Agents: []AgentID{Codex}, SkipSkill: true})
	if err != nil {
		t.Fatal(err)
	}
	requireApplySuccess(t, manager.Apply(context.Background(), changes))
	want := []string{"codex", "mcp", "add", "disbug", "--", "/usr/local/bin/disbug", "--profile", "work", "mcp"}
	if !containsCall(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestClaudeCodeUsesUserScopeAndClaudeSkillDirectory(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	manager := testManager(home, "default")
	var calls [][]string
	manager.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return nil, nil
	}

	changes, err := manager.Plan(context.Background(), Options{Agents: []AgentID{ClaudeCode}})
	if err != nil {
		t.Fatal(err)
	}
	requireApplySuccess(t, manager.Apply(context.Background(), changes))
	want := []string{"claude", "mcp", "add", "--scope", "user", "disbug", "--", "/usr/local/bin/disbug", "mcp"}
	if !containsCall(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "using-disbug", "SKILL.md")); err != nil {
		t.Fatalf("Claude skill was not installed: %v", err)
	}
}

func TestCodexAndCursorShareOneSkillInstall(t *testing.T) {
	home := t.TempDir()
	for _, dir := range []string{".codex", ".cursor"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	manager := testManager(home, "default")
	changes, err := manager.Plan(context.Background(), Options{
		Agents: []AgentID{Codex, Cursor}, SkipMCP: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("changes = %#v, want one shared skill install", changes)
	}
}

func TestApplyContinuesAfterOneAgentFails(t *testing.T) {
	home := t.TempDir()
	manager := testManager(home, "default")
	manager.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "codex" && len(args) >= 2 && args[0] == "mcp" && args[1] == "add" {
			return []byte("codex add failed"), errors.New("exit status 1")
		}
		return nil, nil
	}
	changes := []Change{
		{Agent: Codex, AgentName: "Codex", Component: "MCP", Action: "configure", Target: manager.mcpTarget(Codex)},
		{Agent: ClaudeCode, AgentName: "Claude Code", Component: "MCP", Action: "configure", Target: manager.mcpTarget(ClaudeCode)},
	}

	result := manager.Apply(context.Background(), changes)

	if len(result.Applied) != 1 || result.Applied[0].Agent != ClaudeCode {
		t.Fatalf("applied = %#v, want Claude Code after Codex failure", result.Applied)
	}
	if len(result.Failures) != 1 || result.Failures[0].Change.Agent != Codex {
		t.Fatalf("failures = %#v, want Codex failure", result.Failures)
	}
}

func TestCodexConfigureRestoresPreviousConfigWhenAddFails(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	previous := []byte("[mcp_servers.disbug]\ncommand = \"old-disbug\"\n")
	if err := os.WriteFile(configPath, previous, 0o600); err != nil {
		t.Fatal(err)
	}
	manager := testManager(home, "default")
	manager.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "mcp" && args[1] == "remove" {
			if err := os.WriteFile(configPath, []byte("removed\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			return nil, nil
		}
		return []byte("unsupported add flags"), errors.New("exit status 2")
	}

	err := manager.configureMCP(context.Background(), Codex)

	if err == nil || !strings.Contains(err.Error(), "previous configuration restored") {
		t.Fatalf("configureMCP() error = %v, want restored-config guidance", err)
	}
	got, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(previous) {
		t.Fatalf("config after failed add = %q, want %q", got, previous)
	}
}

func TestMCPStatusAcceptsBareDisbugCommandFromDocs(t *testing.T) {
	home := t.TempDir()
	manager := testManager(home, "default")
	manager.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "codex" && len(args) >= 2 && args[0] == "mcp" && args[1] == "get" {
			return []byte(`{"transport":{"type":"stdio","command":"disbug","args":["mcp"]}}`), nil
		}
		return nil, errors.New("unexpected command")
	}
	cursorPath := manager.mcpTarget(Cursor)
	if err := os.MkdirAll(filepath.Dir(cursorPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cursorPath, []byte(`{"mcpServers":{"disbug":{"command":"disbug","args":["mcp"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, agent := range []AgentID{Codex, Cursor} {
		configured, err := manager.mcpConfigured(context.Background(), agent)
		if err != nil {
			t.Fatalf("mcpConfigured(%s) error = %v", agent, err)
		}
		if !configured {
			t.Fatalf("mcpConfigured(%s) = false for documented bare command", agent)
		}
	}
}

func testManager(home, profile string) *Manager {
	return &Manager{
		home:       home,
		binaryPath: "/usr/local/bin/disbug",
		profile:    profile,
		lookPath: func(name string) (string, error) {
			return "/usr/local/bin/" + name, nil
		},
		run: func(context.Context, string, ...string) ([]byte, error) {
			return nil, errors.New("not configured")
		},
	}
}

func containsCall(calls [][]string, want []string) bool {
	for _, call := range calls {
		if slicesEqual(call, want) {
			return true
		}
	}
	return false
}

func requireApplySuccess(t *testing.T, result ApplyResult) {
	t.Helper()
	if len(result.Failures) > 0 {
		t.Fatalf("Apply() failures = %#v", result.Failures)
	}
}
