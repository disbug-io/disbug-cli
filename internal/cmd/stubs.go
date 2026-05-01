package cmd

import "errors"

var errStub = errors.New("not implemented yet (stub)")

type (
	// LogoutCmd is a placeholder for the logout command.
	LogoutCmd struct{}
	// WhoamiCmd is a placeholder for the whoami command.
	WhoamiCmd struct{}
	// DoctorCmd is a placeholder for the doctor command.
	DoctorCmd struct{}
	// MCPCmd is a placeholder for the mcp command.
	MCPCmd struct{}
)

// Run returns the shared placeholder error.
func (LogoutCmd) Run(bindings) error { return errStub }

// Run returns the shared placeholder error.
func (WhoamiCmd) Run(bindings) error { return errStub }

// Run returns the shared placeholder error.
func (DoctorCmd) Run(bindings) error { return errStub }

// Run returns the shared placeholder error.
func (MCPCmd) Run(bindings) error { return errStub }
