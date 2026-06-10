package mcp

import (
	"context"
	"errors"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// GetPinInput is the input for the get_pin MCP tool.
type GetPinInput struct {
	Target string   `json:"target,omitempty" jsonschema:"Disbug report URL with ?pin=<number>"`
	URL    string   `json:"url,omitempty" jsonschema:"Disbug report URL with ?pin=<number>"`
	Pin    string   `json:"pin,omitempty" jsonschema:"Disbug report URL with ?pin=<number>, or local_<id>.<number> for source=local"`
	Source string   `json:"source,omitempty" jsonschema:"Source: auto, cloud, or local"`
	Fields []string `json:"fields,omitempty" jsonschema:"array of: screenshot console network events replay voice_note video all"`
}

func registerGetPin(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[GetPinInput, Result](srv, &sdkmcp.Tool{
		Name:        "get_pin",
		Description: "Get a Disbug pin by report URL from cloud, or by local_<id>.<number> from local source.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		in GetPinInput,
	) (*sdkmcp.CallToolResult, Result, error) {
		target := strings.TrimSpace(in.Target)
		if target == "" {
			target = strings.TrimSpace(in.URL)
		}
		if target == "" {
			target = strings.TrimSpace(in.Pin)
		}
		source, err := routeSessionSource(in.Source, target, deps)
		if err != nil {
			return nil, nil, toolErr(err)
		}
		if source == sourceLocal {
			store, err := requireLocal(deps)
			if err != nil {
				return nil, nil, errors.New(err.Error())
			}
			sessionID, number, err := parseLocalPinRef(target)
			if err != nil {
				return nil, nil, toolErr(err)
			}
			resp, err := store.GetPin(ctx, sessionID, number, in.Fields)
			if err != nil {
				return nil, nil, errors.New(mapLocalErr(sessionID, err).Error())
			}
			result := Result(resp)
			return jsonResult(result), result, nil
		}
		if err := requireCloud(deps); err != nil {
			return nil, nil, toolErr(err)
		}

		pinRef, err := ref.ParsePin(target)
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
		}

		fields := in.Fields
		if len(fields) == 0 {
			fields = []string{"all"}
		}
		fields, err = ref.NormalizeFields(fields)
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
		}

		if err := deps.Client.RequireCapability(ctx, "pin_field_selection"); err != nil {
			return nil, nil, errors.New(errfmt.Format(err))
		}
		if err := deps.Client.RequireCapability(ctx, "scoped_pin_lookup"); err != nil {
			return nil, nil, errors.New(errfmt.Format(err))
		}

		resp, err := deps.Client.GetPinByNumber(ctx, pinRef.Session, pinRef.Pin, fields)
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(err))
		}
		if resp == nil {
			return nil, nil, errors.New("disbug API returned no pin")
		}

		resolved, err := deps.Client.ResolveReplay(ctx, resp, pinRef.Session.SessionNumber, pinRef.Pin)
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(err))
		}

		result := resultFrom(resolved)
		return jsonResult(result), result, nil
	})
}
