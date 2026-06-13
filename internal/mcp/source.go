package mcp

import (
	"errors"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

const (
	sourceAuto  = "auto"
	sourceCloud = "cloud"
)

func normalizeSource(source string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "", sourceAuto:
		return sourceAuto, nil
	case sourceCloud:
		return sourceCloud, nil
	default:
		return "", errfmt.UsageError{Message: "source must be auto or cloud"}
	}
}

func routeSessionSource(source string, _ string, _ *Deps) (string, error) {
	normalized, err := normalizeSource(source)
	if err != nil {
		return "", err
	}
	if normalized != sourceAuto {
		return normalized, nil
	}
	return sourceCloud, nil
}

func requireCloud(deps *Deps) error {
	if deps == nil || deps.Client == nil || !deps.CloudAvailable {
		return errfmt.NoToken{}
	}
	return nil
}

func toolErr(err error) error {
	return errors.New(errfmt.Format(err))
}
