package cmd

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionStringDefault(t *testing.T) {
	assert.Equal(t, "dev", VersionString())
}

func TestExitCode(t *testing.T) {
	assert.Equal(t, 0, ExitCode(nil))
	assert.Equal(t, 1, ExitCode(errors.New("boom")))
}
