package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// GetPinsItem is one pin fetch request in the get_pins MCP tool input.
type GetPinsItem struct {
	Pin    string   `json:"pin" jsonschema:"pin reference e.g. 7392.2"`
	Fields []string `json:"fields,omitempty" jsonschema:"fields for this pin; defaults to default_fields"`
}

// GetPinsInput is the input for the get_pins MCP tool.
type GetPinsInput struct {
	Items         []GetPinsItem `json:"items" jsonschema:"array of {pin, fields?} entries"`
	DefaultFields []string      `json:"default_fields,omitempty" jsonschema:"fields used when an item omits its own list"`
}

func registerGetPins(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[GetPinsInput, client.BulkResult](srv, &sdkmcp.Tool{
		Name: "get_pins",
		Description: "Bulk fetch Disbug pins by session.pin references. Each pin may select its own fields; " +
			"items without fields use default_fields or all fields. Partial failures are returned in errors and " +
			"are not tool errors unless every item fails.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		in GetPinsInput,
	) (*sdkmcp.CallToolResult, client.BulkResult, error) {
		if deps == nil || deps.Client == nil {
			return nil, client.BulkResult{}, errors.New("disbug API client is not configured")
		}
		if len(in.Items) == 0 {
			return nil, client.BulkResult{}, errors.New(errfmt.Format(&errfmt.UsageError{
				Message: "at least one pin item is required",
			}))
		}

		defaultFields := in.DefaultFields
		var err error
		if len(defaultFields) > 0 {
			defaultFields, err = ref.NormalizeFields(defaultFields)
			if err != nil {
				return nil, client.BulkResult{}, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
			}
		}

		parsed := make([]ref.PinFetch, 0, len(in.Items))
		for _, item := range in.Items {
			rawRef := item.Pin
			if len(item.Fields) > 0 {
				rawRef = joinFields(item.Pin, item.Fields)
			}
			pinFetch, err := ref.ParsePinFetch(rawRef, defaultFields)
			if err != nil {
				return nil, client.BulkResult{}, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
			}
			parsed = append(parsed, pinFetch)
		}
		unique := ref.DedupAndUnion(parsed)

		if err := deps.Client.RequireCapability(ctx, "pin_field_selection"); err != nil {
			return nil, client.BulkResult{}, errors.New(errfmt.Format(err))
		}
		if err := deps.Client.RequireCapability(ctx, "pin_by_number"); err != nil {
			return nil, client.BulkResult{}, errors.New(errfmt.Format(err))
		}

		res := deps.Client.GetPinsBulk(ctx, unique)
		if res.AllFailed() {
			return nil, client.BulkResult{}, errors.New(bulkPinsFailureMessage(res))
		}

		return jsonResult(res), res, nil
	})
}

func joinFields(pin string, fields []string) string {
	return pin + ":" + strings.Join(fields, ",")
}

func bulkPinsFailureError(res client.BulkResult) error {
	if len(res.Errors) == 0 {
		return &errfmt.APIError{}
	}

	first := res.Errors[0]
	if res.FirstFailureExitCode() == 5 {
		return errfmt.NetworkError{}
	}

	return &errfmt.APIError{
		StatusCode: statusForBulkPinsExitCode(res.FirstFailureExitCode()),
		Code:       first.Code,
		Detail:     first.Message,
		RequestID:  first.RequestID,
	}
}

func bulkPinsFailureMessage(res client.BulkResult) string {
	underlying := errfmt.Format(bulkPinsFailureError(res))
	if len(res.Errors) == 0 {
		return underlying
	}

	return fmt.Sprintf(
		"all %d pin fetches failed; first failure %s: %s",
		len(res.Errors),
		res.Errors[0].Pin,
		underlying,
	)
}

func statusForBulkPinsExitCode(exitCode int) int {
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
