package cmd

import (
	"context"
	"errors"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/token"
)

// LogoutCmd logs out by revoking the current token and removing the local profile.
type LogoutCmd struct {
	LocalOnly bool `name:"local-only" help:"Skip server revoke; just delete local profile."`
}

// Run executes logout.
func (c *LogoutCmd) Run(ctx context.Context, b bindings) error {
	profile := defaultProfile
	if b.Flags != nil && b.Flags.Profile != "" {
		profile = b.Flags.Profile
	}

	if c.LocalOnly {
		return token.Delete(profile)
	}

	cli, _, err := newAuthenticatedClient(b.Flags)
	if err != nil {
		var noToken errfmt.NoToken
		if errors.As(err, &noToken) {
			return token.Delete(profile)
		}
		return err
	}

	if err := cli.RevokeToken(ctx); err != nil {
		if isTokenRevoked(err) {
			return token.Delete(profile)
		}
		return err
	}

	return token.Delete(profile)
}

func isTokenRevoked(err error) bool {
	var api errfmt.APIError
	if errors.As(err, &api) {
		return api.Code == "token_revoked"
	}

	var apiPtr *errfmt.APIError
	if errors.As(err, &apiPtr) && apiPtr != nil {
		return apiPtr.Code == "token_revoked"
	}

	return false
}
