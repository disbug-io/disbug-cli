package configure

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// AgentID identifies an agent integration supported by configure.
type AgentID string

const (
	// Codex identifies the Codex integration.
	Codex AgentID = "codex"
	// ClaudeCode identifies the Claude Code integration.
	ClaudeCode AgentID = "claude-code"
	// Cursor identifies the Cursor integration.
	Cursor AgentID = "cursor"
)

var supportedAgents = []AgentID{Codex, ClaudeCode, Cursor}

// Agent describes a supported agent and whether it appears to be installed.
type Agent struct {
	ID       AgentID
	Name     string
	Detected bool
}

// Change is one user-visible configuration mutation.
type Change struct {
	Agent     AgentID
	AgentName string
	Component string
	Action    string
	Target    string
}

// Status describes the local Disbug integration for one detected agent.
type Status struct {
	Agent     AgentID
	AgentName string
	MCP       bool
	Skill     bool
}

// Options controls planning and applying configuration.
type Options struct {
	Agents    []AgentID
	SkipMCP   bool
	SkipSkill bool
	Force     bool
}

type commandRunner func(context.Context, string, ...string) ([]byte, error)

// Manager detects, plans, applies, and checks agent integrations.
type Manager struct {
	home       string
	binaryPath string
	profile    string
	lookPath   func(string) (string, error)
	run        commandRunner
}

// New creates a manager for the current user and executable.
func New(profile string) (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find home directory: %w", err)
	}
	binaryPath, err := exec.LookPath("disbug")
	if err != nil {
		binaryPath, err = os.Executable()
		if err != nil {
			return nil, fmt.Errorf("find disbug executable: %w", err)
		}
	}
	return &Manager{
		home:       home,
		binaryPath: binaryPath,
		profile:    profile,
		lookPath:   exec.LookPath,
		run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			// name is selected internally from the fixed supported-agent executables.
			//nolint:gosec
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		},
	}, nil
}

// ParseAgent validates an agent flag value.
func ParseAgent(value string) (AgentID, error) {
	id := AgentID(strings.ToLower(strings.TrimSpace(value)))
	for _, supported := range supportedAgents {
		if id == supported {
			return id, nil
		}
	}
	return "", fmt.Errorf("unsupported agent %q (choose codex, claude-code, or cursor)", value)
}

// Agents returns all supported agents with local detection results.
func (m *Manager) Agents() []Agent {
	return []Agent{
		{ID: Codex, Name: displayName(Codex), Detected: m.detected(Codex)},
		{ID: ClaudeCode, Name: displayName(ClaudeCode), Detected: m.detected(ClaudeCode)},
		{ID: Cursor, Name: displayName(Cursor), Detected: m.detected(Cursor)},
	}
}

// Plan returns the exact configuration changes that Apply will perform.
func (m *Manager) Plan(ctx context.Context, opts Options) ([]Change, error) {
	agents, err := m.selectedAgents(opts.Agents)
	if err != nil {
		return nil, err
	}
	if opts.SkipMCP && opts.SkipSkill {
		return nil, errors.New("--skip-mcp and --skip-skill cannot be used together")
	}

	changes := make([]Change, 0, len(agents)*2)
	seenSkills := map[string]bool{}
	for _, agent := range agents {
		if !opts.SkipMCP {
			configured, statusErr := m.mcpConfigured(ctx, agent)
			if statusErr != nil {
				return nil, statusErr
			}
			if !configured {
				changes = append(changes, Change{
					Agent: agent, AgentName: displayName(agent), Component: "MCP", Action: "configure", Target: m.mcpTarget(agent),
				})
			}
		}
		if !opts.SkipSkill {
			target := m.skillTarget(agent)
			if seenSkills[target] {
				continue
			}
			seenSkills[target] = true
			state, stateErr := m.skillState(target)
			if stateErr != nil {
				return nil, stateErr
			}
			if state == "conflict" && !opts.Force {
				return nil, fmt.Errorf("%s already exists and is not managed by Disbug; rerun with --force to replace it", target)
			}
			if state != "current" {
				action := "install"
				if state != "missing" {
					action = "update"
				}
				changes = append(changes, Change{
					Agent: agent, AgentName: displayName(agent), Component: "skill", Action: action, Target: target,
				})
			}
		}
	}
	return changes, nil
}

// Statuses returns local MCP and skill status for detected agents.
func (m *Manager) Statuses(ctx context.Context) []Status {
	statuses := make([]Status, 0, len(supportedAgents))
	for _, agent := range supportedAgents {
		if !m.detected(agent) {
			continue
		}
		mcpOK, _ := m.mcpConfigured(ctx, agent)
		skillOK, _ := m.skillState(m.skillTarget(agent))
		statuses = append(statuses, Status{
			Agent: agent, AgentName: displayName(agent), MCP: mcpOK, Skill: skillOK == "current",
		})
	}
	return statuses
}

func (m *Manager) selectedAgents(requested []AgentID) ([]AgentID, error) {
	if len(requested) > 0 {
		unique := map[AgentID]bool{}
		selected := make([]AgentID, 0, len(requested))
		for _, agent := range requested {
			parsed, err := ParseAgent(string(agent))
			if err != nil {
				return nil, err
			}
			if !m.detected(parsed) {
				return nil, fmt.Errorf("%s was not detected; install it or choose another --agent", displayName(parsed))
			}
			if !unique[parsed] {
				unique[parsed] = true
				selected = append(selected, parsed)
			}
		}
		return selected, nil
	}

	selected := make([]AgentID, 0, len(supportedAgents))
	for _, agent := range supportedAgents {
		if m.detected(agent) {
			selected = append(selected, agent)
		}
	}
	if len(selected) == 0 {
		return nil, errors.New("no supported agents detected; install Codex, Claude Code, or Cursor, or pass --agent after installation")
	}
	return selected, nil
}

func (m *Manager) detected(agent AgentID) bool {
	switch agent {
	case Codex:
		_, err := m.lookPath("codex")
		return err == nil
	case ClaudeCode:
		_, err := m.lookPath("claude")
		return err == nil
	case Cursor:
		for _, executable := range []string{"cursor", "cursor-agent"} {
			if _, err := m.lookPath(executable); err == nil {
				return true
			}
		}
		info, err := os.Stat(filepath.Join(m.home, ".cursor"))
		return err == nil && info.IsDir()
	default:
		return false
	}
}

func displayName(agent AgentID) string {
	switch agent {
	case Codex:
		return "Codex"
	case ClaudeCode:
		return "Claude Code"
	case Cursor:
		return "Cursor"
	default:
		return string(agent)
	}
}

func (m *Manager) mcpTarget(agent AgentID) string {
	switch agent {
	case Codex:
		return filepath.Join(m.home, ".codex", "config.toml")
	case ClaudeCode:
		return filepath.Join(m.home, ".claude.json")
	case Cursor:
		return filepath.Join(m.home, ".cursor", "mcp.json")
	default:
		return ""
	}
}

func (m *Manager) skillTarget(agent AgentID) string {
	root := filepath.Join(m.home, ".agents", "skills")
	if agent == ClaudeCode {
		root = filepath.Join(m.home, ".claude", "skills")
	}
	return filepath.Join(root, "using-disbug", "SKILL.md")
}

func (m *Manager) serverArgs() []string {
	if m.profile == "" || m.profile == "default" {
		return []string{"mcp"}
	}
	return []string{"--profile", m.profile, "mcp"}
}

// SortChanges keeps plan output stable across platforms and tests.
func SortChanges(changes []Change) {
	sort.SliceStable(changes, func(i, j int) bool {
		if changes[i].Agent == changes[j].Agent {
			return changes[i].Component < changes[j].Component
		}
		return changes[i].Agent < changes[j].Agent
	})
}
