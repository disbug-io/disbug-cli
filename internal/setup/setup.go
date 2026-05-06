package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

const hostName = "io.disbug.bridge"

var extensionIDPattern = regexp.MustCompile(`^[a-p]{32}$`)

// Options configures local native-host and MCP setup.
type Options struct {
	HomeDir      string
	GOOS         string
	BinaryPath   string
	ExtensionIDs []string
	SkipMCP      bool
}

// Result describes setup changes and detected integrations.
type Result struct {
	Manifests []string          `json:"manifests"`
	MCP       map[string]string `json:"mcp"`
	Skills    map[string]string `json:"skills"`
}

// ManifestDiagnostic reports whether a browser native messaging manifest is installed.
type ManifestDiagnostic struct {
	Path         string `json:"path"`
	Status       string `json:"status"`
	ActualPath   string `json:"actual_path,omitempty"`
	ExpectedPath string `json:"expected_path,omitempty"`
}

// HostManifest is the Chrome native messaging host manifest.
type HostManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

// Install writes native messaging manifests and optionally registers MCP configs.
func Install(opts Options) (Result, error) {
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}
	if opts.HomeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Result{}, err
		}
		opts.HomeDir = home
	}
	if opts.BinaryPath == "" {
		exe, err := os.Executable()
		if err != nil {
			return Result{}, err
		}
		opts.BinaryPath = exe
	}
	origins, err := allowedOrigins(opts.ExtensionIDs)
	if err != nil {
		return Result{}, err
	}

	manifest := HostManifest{
		Name:           hostName,
		Description:    "Disbug local AI bridge",
		Path:           opts.BinaryPath,
		Type:           "stdio",
		AllowedOrigins: origins,
	}

	targets := manifestTargets(opts.GOOS, opts.HomeDir)
	result := Result{MCP: map[string]string{}, Skills: map[string]string{}}
	for _, target := range targets {
		if err := writeManifest(target, manifest); err != nil {
			return Result{}, err
		}
		result.Manifests = append(result.Manifests, target)
	}
	if !opts.SkipMCP {
		result.MCP = registerMCP(opts.HomeDir)
		result.Skills = installAgentSkills(opts.HomeDir)
	}
	sort.Strings(result.Manifests)
	return result, nil
}

// ManifestDiagnostics checks whether known browser manifests point to the current CLI binary.
func ManifestDiagnostics(opts Options) []ManifestDiagnostic {
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}
	if opts.HomeDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			opts.HomeDir = home
		}
	}
	if opts.BinaryPath == "" {
		if exe, err := os.Executable(); err == nil {
			opts.BinaryPath = exe
		}
	}

	targets := manifestTargets(opts.GOOS, opts.HomeDir)
	diagnostics := make([]ManifestDiagnostic, 0, len(targets))
	for _, target := range targets {
		item := ManifestDiagnostic{Path: target, ExpectedPath: opts.BinaryPath}
		raw, err := os.ReadFile(target) //nolint:gosec // user-level native host manifest path.
		if err != nil {
			item.Status = "missing"
			diagnostics = append(diagnostics, item)
			continue
		}
		var manifest HostManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			item.Status = "outdated"
			diagnostics = append(diagnostics, item)
			continue
		}
		item.ActualPath = manifest.Path
		if manifest.Path == opts.BinaryPath {
			item.Status = "registered"
		} else {
			item.Status = "outdated"
		}
		diagnostics = append(diagnostics, item)
	}
	return diagnostics
}

func allowedOrigins(extensionIDs []string) ([]string, error) {
	seen := map[string]bool{}
	var origins []string
	for _, id := range extensionIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !extensionIDPattern.MatchString(id) {
			return nil, fmt.Errorf("invalid Chrome extension id %q", id)
		}
		origin := "chrome-extension://" + id + "/"
		if !seen[origin] {
			origins = append(origins, origin)
			seen[origin] = true
		}
	}
	if len(origins) == 0 {
		return nil, errors.New("at least one --extension-id is required")
	}
	return origins, nil
}

func manifestTargets(goos, home string) []string {
	hostFile := hostName + ".json"
	join := func(parts ...string) string {
		return filepath.Join(append([]string{home}, append(parts, "NativeMessagingHosts", hostFile)...)...)
	}
	switch goos {
	case "darwin":
		return []string{
			join("Library/Application Support/Google/Chrome"),
			join("Library/Application Support/Google/Chrome Beta"),
			join("Library/Application Support/Google/Chrome Canary"),
			join("Library/Application Support/Google/Chrome Dev"),
			join("Library/Application Support/Chromium"),
			join("Library/Application Support/BraveSoftware/Brave-Browser"),
			join("Library/Application Support/BraveSoftware/Brave-Browser-Beta"),
			join("Library/Application Support/BraveSoftware/Brave-Browser-Nightly"),
			join("Library/Application Support/Microsoft Edge"),
			join("Library/Application Support/Microsoft Edge Beta"),
			join("Library/Application Support/Microsoft Edge Canary"),
			join("Library/Application Support/Microsoft Edge Dev"),
			join("Library/Application Support/Vivaldi"),
			join("Library/Application Support/com.operasoftware.Opera"),
			join("Library/Application Support/Arc/User Data"),
		}
	case "linux":
		return []string{
			join(".config/google-chrome"),
			join(".config/google-chrome-beta"),
			join(".config/google-chrome-unstable"),
			join(".config/chromium"),
			join(".config/BraveSoftware/Brave-Browser"),
			join(".config/BraveSoftware/Brave-Browser-Beta"),
			join(".config/BraveSoftware/Brave-Browser-Nightly"),
			join(".config/microsoft-edge"),
			join(".config/microsoft-edge-beta"),
			join(".config/microsoft-edge-dev"),
			join(".config/vivaldi"),
			join(".config/opera"),
		}
	default:
		return nil
	}
}

func writeManifest(target string, manifest HostManifest) error {
	if target == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(target, data, 0o600) //nolint:gosec // target is a user-level native host manifest path.
}

func registerMCP(home string) map[string]string {
	status := map[string]string{}
	if home == "" {
		return status
	}
	status["claudeCode"] = registerClaudeCode()
	if err := mergeCodexConfig(filepath.Join(home, ".codex", "config.toml")); err != nil {
		status["codex"] = "not detected"
	} else {
		status["codex"] = "registered"
	}

	cursorPath, cursorDetected := cursorConfigPath(home)
	if cursorDetected {
		if err := mergeMCPJSON(cursorPath); err != nil {
			status["cursor"] = "outdated"
		} else {
			status["cursor"] = "registered"
		}
	} else {
		status["cursor"] = "not detected"
	}

	claudeDesktopPath := filepath.Join(home, "Library/Application Support/Claude/claude_desktop_config.json")
	if err := mergeMCPJSON(claudeDesktopPath); err != nil {
		status["claudeDesktop"] = "not detected"
	} else {
		status["claudeDesktop"] = "registered"
	}
	return status
}

// MCPStatuses reports known agent registration status without mutating files.
func MCPStatuses(home string) map[string]string {
	status := map[string]string{}
	if home == "" {
		return status
	}
	status["claudeCode"] = claudeCodeStatus(home)
	status["codex"] = codexStatus(filepath.Join(home, ".codex", "config.toml"))
	if cursorPath, detected := cursorConfigPath(home); detected {
		status["cursor"] = mcpJSONStatus(cursorPath)
	} else {
		status["cursor"] = "not detected"
	}
	status["claudeDesktop"] = mcpJSONStatus(filepath.Join(home, "Library/Application Support/Claude/claude_desktop_config.json"))
	return status
}

// SkillStatuses reports companion skill installation status without mutating files.
func SkillStatuses(home string) map[string]string {
	status := map[string]string{}
	for agent, target := range skillTargets(home) {
		if _, err := os.Stat(agentRoot(home, agent)); err != nil {
			status[agent] = "not detected"
			continue
		}
		if _, err := os.Stat(target); err != nil {
			status[agent] = "outdated"
		} else {
			status[agent] = "registered"
		}
	}
	return status
}

var (
	lookPath       = exec.LookPath
	commandContext = exec.CommandContext
)

func registerClaudeCode() string {
	claudePath, err := lookPath("claude")
	if err != nil {
		return "not detected"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := commandContext(ctx, claudePath, "mcp", "add", "--transport", "stdio", "disbug", "--", "disbug", "mcp")
	if err := cmd.Run(); err != nil {
		return "outdated"
	}
	return "registered"
}

func claudeCodeStatus(home string) string {
	return mcpJSONStatus(filepath.Join(home, ".claude.json"))
}

func mergeCodexConfig(path string) error {
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return err
	}
	data, _ := os.ReadFile(path) //nolint:gosec // user config path.
	text := string(data)
	if strings.Contains(text, "[mcp_servers.disbug]") {
		return nil
	}
	if strings.TrimSpace(text) != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += "\n[mcp_servers.disbug]\ncommand = \"disbug\"\nargs = [\"mcp\"]\n"
	return os.WriteFile(path, []byte(text), 0o600) //nolint:gosec // user config path.
}

func codexStatus(path string) string {
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return "not detected"
	}
	data, err := os.ReadFile(path) //nolint:gosec // user config path.
	if err != nil {
		return "outdated"
	}
	if strings.Contains(string(data), "[mcp_servers.disbug]") {
		return "registered"
	}
	return "outdated"
}

func cursorConfigPath(home string) (string, bool) {
	dotCursor := filepath.Join(home, ".cursor")
	if _, err := os.Stat(dotCursor); err == nil {
		return filepath.Join(dotCursor, "mcp.json"), true
	}
	cursorUser := filepath.Join(home, "Library/Application Support/Cursor/User")
	if _, err := os.Stat(cursorUser); err == nil {
		return filepath.Join(cursorUser, "mcp.json"), true
	}
	return filepath.Join(dotCursor, "mcp.json"), false
}

func mcpJSONStatus(path string) string {
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return "not detected"
	}
	data, err := os.ReadFile(path) //nolint:gosec // user config path.
	if err != nil {
		if os.IsNotExist(err) {
			return "not detected"
		}
		return "outdated"
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return "outdated"
	}
	servers, _ := config["mcpServers"].(map[string]any)
	if servers == nil {
		return "outdated"
	}
	if _, ok := servers["disbug"]; ok {
		return "registered"
	}
	return "outdated"
}

func mergeMCPJSON(path string) error {
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return err
	}
	var config map[string]any
	data, err := os.ReadFile(path) //nolint:gosec // user config path.
	if err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &config); err != nil {
			return err
		}
	} else {
		config = map[string]any{}
	}
	servers, _ := config["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
		config["mcpServers"] = servers
	}
	servers["disbug"] = map[string]any{
		"command": "disbug",
		"args":    []string{"mcp"},
	}
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return os.WriteFile(path, encoded, 0o600) //nolint:gosec // user config path.
}

func installAgentSkills(home string) map[string]string {
	status := map[string]string{}
	for agent, target := range skillTargets(home) {
		if _, err := os.Stat(agentRoot(home, agent)); err != nil {
			status[agent] = "not detected"
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			status[agent] = "outdated"
			continue
		}
		if err := os.WriteFile(target, []byte(disbugLocalSkill), 0o600); err != nil { //nolint:gosec // user skill file.
			status[agent] = "outdated"
			continue
		}
		status[agent] = "registered"
	}
	return status
}

func skillTargets(home string) map[string]string {
	return map[string]string{
		"codex":  filepath.Join(home, ".codex", "skills", "disbug-local", "SKILL.md"),
		"claude": filepath.Join(home, ".claude", "skills", "disbug-local", "SKILL.md"),
	}
}

func agentRoot(home, agent string) string {
	switch agent {
	case "claude":
		return filepath.Join(home, ".claude")
	default:
		return filepath.Join(home, "."+agent)
	}
}

const disbugLocalSkill = `---
name: disbug-local
description: Use when a user pastes a Disbug local report prompt or asks to debug a locally saved Disbug report.
---

# Disbug Local Report Handoff

When the prompt includes "Debug a Disbug bug report saved locally on this machine":

1. Prefer the Disbug MCP tool call named in the prompt:
   get_session(id="<local_report_id>", source="local")
2. If MCP is unavailable, read the absolute report path from the prompt.
3. Start with manifest.json and session.json.
4. Read pin_<n>/pin.json for the relevant pins.
5. Read logs.json, screenshot.png, or replay.rrweb.json only when needed.

Keep cloud upload separate from local debugging. Local report paths refer to files on the user's machine.
`
