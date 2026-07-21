package cmd

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

func TestExecuteNoArgsShowsHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Execute(context.Background(), nil, nil, &stdout, &stderr)

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "disbug")
	assert.Empty(t, stderr.String())
}

func TestExecuteVersionPrintsVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Execute(context.Background(), []string{"version"}, nil, &stdout, &stderr)

	require.NoError(t, err)
	assert.NotEmpty(t, stdout.String())
	assert.Contains(t, stdout.String(), VersionString())
	assert.Empty(t, stderr.String())
}

func TestExecuteUnknownFlagReturnsUsageError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Execute(context.Background(), []string{"--definitely-not-a-flag"}, nil, &stdout, &stderr)

	require.Error(t, err)
	assert.Equal(t, 2, ExitCode(err))
	var usage errfmt.UsageError
	assert.True(t, errors.As(err, &usage))
	assert.Empty(t, stdout.String())
}

func TestExecuteCompletionGeneratesShellScripts(t *testing.T) {
	tests := []struct {
		shell string
		want  []string
	}{
		{
			shell: "bash",
			want:  []string{"_disbug_completion()", "complete -F _disbug_completion disbug"},
		},
		{
			shell: "zsh",
			want:  []string{"#compdef disbug", "_disbug_completion()", "compdef _disbug_completion disbug"},
		},
		{
			shell: "fish",
			want:  []string{"function __disbug_complete", "complete -c disbug -f -a"},
		},
		{
			shell: "powershell",
			want:  []string{"Register-ArgumentCompleter -Native -CommandName disbug", "$wordToComplete"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			err := Execute(context.Background(), []string{"completion", tt.shell}, nil, &stdout, &stderr)

			require.NoError(t, err)
			for _, want := range tt.want {
				assert.Contains(t, stdout.String(), want)
			}
			assert.Contains(t, stdout.String(), "sessions")
			assert.Contains(t, stdout.String(), "mcp")
			assert.NotContains(t, stdout.String(), "placeholder")
			assert.Empty(t, stderr.String())
		})
	}
}

func TestExecuteMCPHelpIncludesCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Execute(context.Background(), []string{"mcp", "--help"}, nil, &stdout, &stderr)

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Run MCP integration commands.")
	assert.Empty(t, stderr.String())
}

func TestExecuteHelpDoesNotExposeNativeHostCommands(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Execute(context.Background(), []string{"--help"}, nil, &stdout, &stderr)

	require.NoError(t, err)
	assert.NotContains(t, stdout.String(), "native-host")
	assert.NotContains(t, stdout.String(), "setup-local")
	assert.NotContains(t, stdout.String(), "local-sessions")
	assert.Empty(t, stderr.String())
}

func TestExecuteNativeHostCommandsAreUnknown(t *testing.T) {
	for _, command := range []string{"native-host", "setup-local", "local-sessions"} {
		t.Run(command, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			err := Execute(context.Background(), []string{command, "--help"}, nil, &stdout, &stderr)

			require.Error(t, err)
			var usage errfmt.UsageError
			assert.True(t, errors.As(err, &usage))
			assert.Empty(t, stdout.String())
		})
	}
}
