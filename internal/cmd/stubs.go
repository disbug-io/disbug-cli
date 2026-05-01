package cmd

import "errors"

var errStub = errors.New("not implemented yet (stub)")

type (
	// SessionCmd is a placeholder for the session command.
	SessionCmd struct{}
	// PinCmd is a placeholder for the pin command.
	PinCmd struct{}
	// PinsCmd is a placeholder for the pins command.
	PinsCmd struct{}
	// SearchCmd is a placeholder for the search command.
	SearchCmd struct{}
	// LoginCmd is a placeholder for the login command.
	LoginCmd struct{}
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
func (SessionCmd) Run(bindings) error { return errStub }

// Run returns the shared placeholder error.
func (PinCmd) Run(bindings) error { return errStub }

// Run returns the shared placeholder error.
func (PinsCmd) Run(bindings) error { return errStub }

// Run returns the shared placeholder error.
func (SearchCmd) Run(bindings) error { return errStub }

// Run returns the shared placeholder error.
func (LoginCmd) Run(bindings) error { return errStub }

// Run returns the shared placeholder error.
func (LogoutCmd) Run(bindings) error { return errStub }

// Run returns the shared placeholder error.
func (WhoamiCmd) Run(bindings) error { return errStub }

// Run returns the shared placeholder error.
func (DoctorCmd) Run(bindings) error { return errStub }

// Run returns the shared placeholder error.
func (MCPCmd) Run(bindings) error { return errStub }
