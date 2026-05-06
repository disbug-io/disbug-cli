package mcp

import (
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/localstore"
)

const (
	sourceAuto  = "auto"
	sourceCloud = "cloud"
	sourceLocal = "local"
)

func normalizeSource(source string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "", sourceAuto:
		return sourceAuto, nil
	case sourceCloud:
		return sourceCloud, nil
	case sourceLocal:
		return sourceLocal, nil
	default:
		return "", errfmt.UsageError{Message: "source must be auto, cloud, or local"}
	}
}

func routeSessionSource(source string, id string, deps *Deps) (string, error) {
	normalized, err := normalizeSource(source)
	if err != nil {
		return "", err
	}
	if normalized != sourceAuto {
		return normalized, nil
	}
	if strings.HasPrefix(id, "local_") {
		return sourceLocal, nil
	}
	if deps != nil && !deps.CloudAvailable && deps.LocalStore != nil {
		return sourceLocal, nil
	}
	return sourceCloud, nil
}

func requireCloud(deps *Deps) error {
	if deps == nil || deps.Client == nil || !deps.CloudAvailable {
		return errfmt.NoToken{}
	}
	return nil
}

func requireLocal(deps *Deps) (*localstore.Store, error) {
	if deps == nil || deps.LocalStore == nil {
		return nil, errors.New("local Disbug store is not configured")
	}
	return deps.LocalStore, nil
}

func toolErr(err error) error {
	return errors.New(errfmt.Format(err))
}

func localNotFound(id string) error {
	return errors.New("local session not found: " + id)
}

func mapLocalErr(id string, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return localNotFound(id)
	}
	return err
}

func parseLocalPinRef(raw string) (string, int, error) {
	raw = strings.TrimSpace(raw)
	index := strings.LastIndex(raw, ".")
	if index <= 0 || index == len(raw)-1 {
		return "", 0, errfmt.UsageError{Message: "local pin must be formatted as local_<id>.<number>"}
	}
	sessionID := raw[:index]
	if !strings.HasPrefix(sessionID, "local_") {
		return "", 0, errfmt.UsageError{Message: "local pin must start with local_"}
	}
	number, err := strconv.Atoi(raw[index+1:])
	if err != nil || number <= 0 {
		return "", 0, errfmt.UsageError{Message: fmt.Sprintf("invalid local pin number in %q", raw)}
	}
	return sessionID, number, nil
}
