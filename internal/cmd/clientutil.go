package cmd

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/token"
)

const defaultProfile = "default"

func newAuthenticatedClient(flags *RootFlags) (*client.Client, token.Token, error) {
	profile := defaultProfile
	if flags != nil && flags.Profile != "" {
		profile = flags.Profile
	}

	tok, err := token.Read(profile)
	if err != nil {
		if errors.Is(err, token.ErrProfileNotFound) {
			return nil, token.Token{}, errfmt.NoToken{}
		}
		return nil, token.Token{}, fmt.Errorf("read profile: %w", err)
	}
	if tok.Token == "" {
		return nil, tok, errfmt.NoToken{}
	}

	apiURL := tok.APIURL
	if apiURL == "" {
		apiURL = "https://disbug.io"
	}

	userAgent := fmt.Sprintf("disbug-cli/%s (%s/%s)", VersionString(), runtime.GOOS, runtime.GOARCH)
	return client.New(apiURL, tok.Token, userAgent, nil, nil, nil), tok, nil
}
