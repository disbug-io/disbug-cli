package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/disbug-io/disbug-cli/internal/configure"
)

func TestWriteApplyResultReportsSuccessesAndFailures(t *testing.T) {
	var output bytes.Buffer
	writeApplyResult(&output, configure.ApplyResult{
		Applied: []configure.Change{
			{AgentName: "Codex", Component: "skill", Target: "/tmp/skill"},
		},
		Failures: []configure.ApplyFailure{
			{
				Change: configure.Change{AgentName: "Claude Code", Component: "MCP", Target: "/tmp/config"},
				Err:    errors.New("permission denied"),
			},
		},
	})

	for _, want := range []string{
		"OK     Codex",
		"FAILED Claude Code",
		"permission denied",
		"Applied 1 of 2 changes; 1 failed.",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output = %q, want %q", output.String(), want)
		}
	}
}
