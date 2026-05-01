package cmd

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

func TestVersionStringDefault(t *testing.T) {
	assert.Equal(t, "dev", VersionString())
}

func TestExitCode(t *testing.T) {
	assert.Equal(t, 0, ExitCode(nil))
	assert.Equal(t, 1, ExitCode(errors.New("boom")))
	assert.Equal(t, 4, ExitCode(errfmt.NoToken{}))
}
