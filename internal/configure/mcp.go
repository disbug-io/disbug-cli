package configure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func (m *Manager) mcpConfigured(ctx context.Context, agent AgentID) (bool, error) {
	switch agent {
	case Codex:
		output, err := m.run(ctx, "codex", "mcp", "get", "disbug", "--json")
		if err != nil {
			// A missing server is reported as a command error by Codex.
			//nolint:nilerr
			return false, nil
		}
		var config struct {
			Transport struct {
				Type    string   `json:"type"`
				Command string   `json:"command"`
				Args    []string `json:"args"`
			} `json:"transport"`
		}
		if err := json.Unmarshal(output, &config); err != nil {
			// Unrecognized output means this integration is not usable yet.
			//nolint:nilerr
			return false, nil
		}
		return config.Transport.Type == "stdio" && m.sameCommand(config.Transport.Command, m.binaryPath) && slicesEqual(config.Transport.Args, m.serverArgs()), nil
	case ClaudeCode, Cursor:
		return m.jsonMCPConfigured(m.mcpTarget(agent))
	default:
		return false, fmt.Errorf("unsupported agent %q", agent)
	}
}

func (m *Manager) jsonMCPConfigured(path string) (bool, error) {
	// path is derived from the current user's home directory and a fixed agent target.
	//nolint:gosec
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return false, fmt.Errorf("parse %s: %w", path, err)
	}
	var servers map[string]struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if err := json.Unmarshal(root["mcpServers"], &servers); err != nil {
		// A missing or differently shaped mcpServers value is simply unconfigured.
		//nolint:nilerr
		return false, nil
	}
	server, ok := servers["disbug"]
	return ok && m.sameCommand(server.Command, m.binaryPath) && slicesEqual(server.Args, m.serverArgs()), nil
}

func (m *Manager) configureMCP(ctx context.Context, agent AgentID) error {
	switch agent {
	case Codex:
		args := make([]string, 0, 5+len(m.serverArgs()))
		args = append(args, "mcp", "add", "disbug", "--", m.binaryPath)
		args = append(args, m.serverArgs()...)
		return m.replaceCLIConfig(
			ctx,
			agent,
			"codex",
			[]string{"mcp", "remove", "disbug"},
			args,
		)
	case ClaudeCode:
		args := make([]string, 0, 7+len(m.serverArgs()))
		args = append(args, "mcp", "add", "--scope", "user", "disbug", "--", m.binaryPath)
		args = append(args, m.serverArgs()...)
		return m.replaceCLIConfig(
			ctx,
			agent,
			"claude",
			[]string{"mcp", "remove", "--scope", "user", "disbug"},
			args,
		)
	case Cursor:
		return m.writeJSONMCP(m.mcpTarget(agent))
	default:
		return fmt.Errorf("unsupported agent %q", agent)
	}
}

func (m *Manager) replaceCLIConfig(
	ctx context.Context,
	agent AgentID,
	command string,
	removeArgs []string,
	addArgs []string,
) error {
	snapshot, err := takeFileSnapshot(m.mcpTarget(agent))
	if err != nil {
		return fmt.Errorf("back up existing configuration: %w", err)
	}

	_, _ = m.run(ctx, command, removeArgs...)
	output, err := m.run(ctx, command, addArgs...)
	if err == nil {
		return nil
	}

	addErr := commandError(output, err)
	if restoreErr := snapshot.restore(); restoreErr != nil {
		return fmt.Errorf(
			"replace configuration: %w",
			errors.Join(addErr, fmt.Errorf("restore previous configuration: %w", restoreErr)),
		)
	}
	return fmt.Errorf("%w (previous configuration restored)", addErr)
}

type fileSnapshot struct {
	path   string
	data   []byte
	mode   os.FileMode
	exists bool
}

func takeFileSnapshot(path string) (fileSnapshot, error) {
	snapshot := fileSnapshot{path: path}
	// path is derived from the current user's home directory and a fixed agent target.
	//nolint:gosec
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return snapshot, nil
	}
	if err != nil {
		return snapshot, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return snapshot, err
	}
	snapshot.data = data
	snapshot.mode = info.Mode().Perm()
	snapshot.exists = true
	return snapshot, nil
}

func (s fileSnapshot) restore() error {
	if s.exists {
		return atomicWrite(s.path, s.data, s.mode)
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (m *Manager) writeJSONMCP(path string) error {
	root := map[string]any{}
	// path is derived from the current user's home directory and a fixed agent target.
	//nolint:gosec
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	servers, ok := root["mcpServers"].(map[string]any)
	if !ok {
		servers = map[string]any{}
		root["mcpServers"] = servers
	}
	servers["disbug"] = map[string]any{"command": m.binaryPath, "args": m.serverArgs()}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'), 0o600)
}

func (m *Manager) sameCommand(left, right string) bool {
	left = m.resolveBareCommand(left)
	right = m.resolveBareCommand(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func (m *Manager) resolveBareCommand(command string) string {
	if command == "" || filepath.Base(command) != command {
		return command
	}
	resolved, err := m.lookPath(command)
	if err != nil {
		return command
	}
	return resolved
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func commandError(output []byte, err error) error {
	message := strings.TrimSpace(string(output))
	if message == "" {
		return err
	}
	return fmt.Errorf("%s: %w", message, err)
}
