package cmd

import (
	"context"
	"net/http"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/outfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// PinsCmd fetches multiple pins by session and pin number.
type PinsCmd struct {
	Refs   []string `arg:"" name:"refs" help:"One or more pin refs (e.g. 7392.2 or 7392.2:console,network)"`
	Fields string   `help:"Default fields when ref omits :<fields>" default:"all"`
}

// Run fetches pins in bulk and writes the aggregate result as JSON.
func (c *PinsCmd) Run(ctx context.Context, b bindings) error {
	if len(c.Refs) == 0 {
		return &errfmt.UsageError{Message: "at least one pin ref is required"}
	}

	defaultFields, err := ref.NormalizeFields(strings.Split(c.Fields, ","))
	if err != nil {
		return &errfmt.UsageError{Message: err.Error()}
	}

	parsed := make([]ref.PinFetch, 0, len(c.Refs))
	for _, rawRef := range c.Refs {
		pinFetch, err := ref.ParsePinFetch(rawRef, defaultFields)
		if err != nil {
			return &errfmt.UsageError{Message: err.Error()}
		}
		parsed = append(parsed, pinFetch)
	}
	unique := ref.DedupAndUnion(parsed)

	cli, _, err := newAuthenticatedClient(b.Flags)
	if err != nil {
		return err
	}

	if err := cli.RequireCapability(ctx, "pin_by_number"); err != nil {
		return err
	}
	if ref.AnyNeedsFieldSelection(unique) {
		if err := cli.RequireCapability(ctx, "pin_field_selection"); err != nil {
			return err
		}
	}

	res := cli.GetPinsBulk(ctx, unique)
	if err := outfmt.WriteJSON(b.Stdout, res, b.Flags.Pretty); err != nil {
		return err
	}
	if res.AllFailed() {
		return bulkFailureError(res)
	}

	return nil
}

func bulkFailureError(res client.BulkResult) error {
	if len(res.Errors) == 0 {
		return &errfmt.APIError{}
	}

	first := res.Errors[0]
	if res.FirstFailureExitCode() == 5 {
		return errfmt.NetworkError{}
	}

	return &errfmt.APIError{
		StatusCode: statusForBulkExitCode(res.FirstFailureExitCode()),
		Code:       first.Code,
		Detail:     first.Message,
		RequestID:  first.RequestID,
	}
}

func statusForBulkExitCode(exitCode int) int {
	switch exitCode {
	case 4:
		return http.StatusUnauthorized
	case 6:
		return http.StatusNotFound
	case 7:
		return http.StatusForbidden
	case 8:
		return http.StatusTooManyRequests
	case 9:
		return http.StatusInternalServerError
	default:
		return 0
	}
}
