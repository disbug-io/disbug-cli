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

func TestExecuteCompletionBashPlaceholder(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Execute(context.Background(), []string{"completion", "bash"}, nil, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, "# disbug bash completion (placeholder; implemented in Phase 6)\n", stdout.String())
	assert.Empty(t, stderr.String())
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
