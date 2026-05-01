package cmd

import "errors"

var errStub = errors.New("not implemented yet (stub)")

type (
	// MCPCmd is a placeholder for the mcp command.
	MCPCmd struct{}
)

// Run returns the shared placeholder error.
func (MCPCmd) Run(bindings) error { return errStub }
