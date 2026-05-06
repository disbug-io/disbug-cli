package mcp

import (
	"context"
	"errors"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerGetLatestLocalSession(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[struct{}, Result](srv, &sdkmcp.Tool{
		Name:        "get_latest_local_session",
		Description: "Return the most recently committed local Disbug session.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		_ struct{},
	) (*sdkmcp.CallToolResult, Result, error) {
		store, err := requireLocal(deps)
		if err != nil {
			return nil, nil, errors.New(err.Error())
		}
		latest, err := store.LatestSession(ctx)
		if err != nil {
			return nil, nil, errors.New(mapLocalErr("latest", err).Error())
		}
		result := resultFrom(latest)
		return jsonResult(result), result, nil
	})
}
