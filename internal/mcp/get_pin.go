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
	Pin    string   `json:"pin" jsonschema:"pin reference e.g. 7392.2"`
	Source string   `json:"source,omitempty" jsonschema:"Source: auto, cloud, or local"`
	Fields []string `json:"fields,omitempty" jsonschema:"array of: screenshot console network events replay voice_note video all"`
}

func registerGetPin(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[GetPinInput, Result](srv, &sdkmcp.Tool{
		Name:        "get_pin",
		Description: "Get a Disbug pin by session.pin reference from cloud or local source.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		in GetPinInput,
	) (*sdkmcp.CallToolResult, Result, error) {
		source, err := routeSessionSource(in.Source, strings.TrimSpace(in.Pin), deps)
		if err != nil {
			return nil, nil, toolErr(err)
		}
		if source == sourceLocal {
			store, err := requireLocal(deps)
			if err != nil {
				return nil, nil, errors.New(err.Error())
			}
			sessionID, number, err := parseLocalPinRef(in.Pin)
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

		pinRef, err := ref.ParsePin(in.Pin)
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

		if err := deps.Client.RequireCapability(ctx, "pin_by_number"); err != nil {
			return nil, nil, errors.New(errfmt.Format(err))
		}
		fetch := ref.PinFetch{Pin: pinRef, Fields: fields}
		if fetch.NeedsFieldSelection() {
			if err := deps.Client.RequireCapability(ctx, "pin_field_selection"); err != nil {
				return nil, nil, errors.New(errfmt.Format(err))
			}
		}

		resp, err := deps.Client.GetPinByNumber(ctx, pinRef.Session, pinRef.Pin, fields)
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(err))
		}
		if resp == nil {
			return nil, nil, errors.New("disbug API returned no pin")
		}

		result := resultFrom(resp)
		return jsonResult(result), result, nil
	})
}
