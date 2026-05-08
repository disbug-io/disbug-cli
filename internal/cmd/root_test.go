package cmd

import (
	"bytes"
	"context"
	"errors"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/nativehost"
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

func TestExecuteChromeNativeHostLaunchRunsNativeHost(t *testing.T) {
	var stdin bytes.Buffer
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	require.NoError(t, nativehost.WriteFrame(&stdin, map[string]any{
		"type":     "hello",
		"protocol": 1,
	}))

	err := Execute(
		context.Background(),
		[]string{"chrome-extension://cbfgdbbedpniplghinlebdmlaflnkdah/", "0"},
		&stdin,
		&stdout,
		&stderr,
	)

	require.NoError(t, err)
	assert.Empty(t, stderr.String())

	var frame map[string]any
	require.NoError(t, nativehost.ReadFrame(&stdout, &frame))
	assert.Equal(t, "hello_ack", frame["type"])
	assert.Equal(t, float64(1), frame["protocol"])

	_, decodeErr := json.Marshal(frame)
	require.NoError(t, decodeErr)
}
