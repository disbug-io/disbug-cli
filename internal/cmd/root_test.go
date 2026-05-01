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

func TestExecuteStubCommandReturnsExitCodeOne(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Execute(context.Background(), []string{"login"}, nil, &stdout, &stderr)

	require.Error(t, err)
	assert.Equal(t, 1, ExitCode(err))
	assert.Contains(t, stderr.String(), "not implemented yet (stub)")
	assert.Empty(t, stdout.String())
}
