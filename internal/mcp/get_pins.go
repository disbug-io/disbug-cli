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
	Target string   `json:"target,omitempty" jsonschema:"Disbug report URL with ?pin=<number>"`
	URL    string   `json:"url,omitempty" jsonschema:"Disbug report URL with ?pin=<number>"`
	Pin    string   `json:"pin,omitempty" jsonschema:"Disbug report URL with ?pin=<number>, or local_<id>.<number> for source=local"`
	Fields []string `json:"fields,omitempty" jsonschema:"fields for this pin; defaults to default_fields"`
}

// GetPinsInput is the input for the get_pins MCP tool.
type GetPinsInput struct {
	Items         []GetPinsItem `json:"items" jsonschema:"array of {target|url|pin, fields?} entries"`
	Source        string        `json:"source,omitempty" jsonschema:"Source: auto, cloud, or local"`
	DefaultFields []string      `json:"default_fields,omitempty" jsonschema:"fields used when an item omits its own list"`
}

func registerGetPins(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[GetPinsInput, Result](srv, &sdkmcp.Tool{
		Name: "get_pins",
		Description: "Bulk fetch Disbug pins by report URLs. Each pin may select its own fields; " +
			"items without fields use default_fields or all fields. Partial failures are returned in errors and " +
			"are not tool errors unless every item fails.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		in GetPinsInput,
	) (*sdkmcp.CallToolResult, Result, error) {
		if len(in.Items) == 0 {
			return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{
				Message: "at least one pin item is required",
			}))
		}
		source, err := normalizeSource(in.Source)
		if err != nil {
			return nil, nil, toolErr(err)
		}
		if source == sourceLocal || (source == sourceAuto && strings.HasPrefix(itemTarget(in.Items[0]), "local_")) {
			result, err := getLocalPins(ctx, deps, in)
			if err != nil {
				return nil, nil, err
			}
			return jsonResult(result), result, nil
		}
		if err := requireCloud(deps); err != nil {
			return nil, nil, toolErr(err)
		}

		defaultFields := in.DefaultFields
		if len(defaultFields) > 0 {
			defaultFields, err = ref.NormalizeFields(defaultFields)
			if err != nil {
				return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
			}
		}

		parsed := make([]ref.PinFetch, 0, len(in.Items))
		for _, item := range in.Items {
			rawRef := itemTarget(item)
			pinFetch, err := ref.ParsePinFetch(rawRef, defaultFields)
			if err != nil {
				return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
			}
			if len(item.Fields) > 0 {
				fields, err := ref.NormalizeFields(item.Fields)
				if err != nil {
					return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
				}
				pinFetch.Fields = fields
			}
			parsed = append(parsed, pinFetch)
		}
		unique := ref.DedupAndUnion(parsed)

		if err := deps.Client.RequireCapability(ctx, "pin_field_selection"); err != nil {
			return nil, nil, errors.New(errfmt.Format(err))
		}
		if err := deps.Client.RequireCapability(ctx, "scoped_pin_lookup"); err != nil {
			return nil, nil, errors.New(errfmt.Format(err))
		}

		res := deps.Client.GetPinsBulk(ctx, unique)
		if res.AllFailed() {
			return nil, nil, errors.New(bulkPinsFailureMessage(res))
		}

		result := resultFrom(res)
		return jsonResult(result), result, nil
	})
}

func getLocalPins(ctx context.Context, deps *Deps, in GetPinsInput) (Result, error) {
	store, err := requireLocal(deps)
	if err != nil {
		return nil, errors.New(err.Error())
	}
	pins := make([]any, 0, len(in.Items))
	errs := make([]any, 0)
	for _, item := range in.Items {
		rawRef := itemTarget(item)
		sessionID, number, err := parseLocalPinRef(rawRef)
		if err != nil {
			errs = append(errs, map[string]any{"pin": rawRef, "error_message": errfmt.Format(err)})
			continue
		}
		fields := item.Fields
		if len(fields) == 0 {
			fields = in.DefaultFields
		}
		pin, err := store.GetPin(ctx, sessionID, number, fields)
		if err != nil {
			errs = append(errs, map[string]any{"pin": rawRef, "error_message": mapLocalErr(sessionID, err).Error()})
			continue
		}
		pins = append(pins, pin)
	}
	if len(pins) == 0 && len(errs) > 0 {
		return nil, errors.New("all local pin fetches failed")
	}
	return Result{"pins": pins, "errors": errs}, nil
}

func itemTarget(item GetPinsItem) string {
	if strings.TrimSpace(item.Target) != "" {
		return strings.TrimSpace(item.Target)
	}
	if strings.TrimSpace(item.URL) != "" {
		return strings.TrimSpace(item.URL)
	}
	return strings.TrimSpace(item.Pin)
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
