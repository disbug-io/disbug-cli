package mcp

import (
	"context"
	"errors"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/localbundle"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// InspectLocalReportInput is the input for the inspect_local_report MCP tool.
type InspectLocalReportInput struct {
	Path     string   `json:"path" jsonschema:"Path to a downloaded Disbug local report JSON file"`
	Pin      int      `json:"pin,omitempty" jsonschema:"Pin number to inspect; omit for a session summary"`
	Fields   []string `json:"fields,omitempty" jsonschema:"array of: screenshot console network events replay voice_note video all"`
	CacheDir string   `json:"cache_dir,omitempty" jsonschema:"Directory for decoded local artifacts"`
}

type inspectLocalReportPinOutput struct {
	Source string                 `json:"source"`
	Path   string                 `json:"path"`
	Pin    localbundle.PinInspect `json:"pin"`
}

func registerInspectLocalReport(srv *sdkmcp.Server, _ *Deps) {
	sdkmcp.AddTool[InspectLocalReportInput, Result](srv, &sdkmcp.Tool{
		Name: "inspect_local_report",
		Description: "Inspect a downloaded local Disbug report JSON file by filesystem path. " +
			"Returns a lightweight summary by default; pass pin and fields to inspect pin artifacts.",
	}, func(
		_ context.Context,
		_ *sdkmcp.CallToolRequest,
		in InspectLocalReportInput,
	) (*sdkmcp.CallToolResult, Result, error) {
		bundle, err := localbundle.Load(in.Path)
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
		}

		if in.Pin <= 0 {
			summary, err := bundle.Summary()
			if err != nil {
				return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
			}
			result := resultFrom(summary)
			return jsonResult(result), result, nil
		}

		fields := in.Fields
		if len(fields) > 0 {
			fields, err = ref.NormalizeFields(fields)
			if err != nil {
				return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
			}
		}

		pin, err := bundle.InspectPin(in.Pin, fields, in.CacheDir)
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
		}

		result := resultFrom(inspectLocalReportPinOutput{
			Source: "local",
			Path:   bundle.Path,
			Pin:    pin,
		})
		return jsonResult(result), result, nil
	})
}
