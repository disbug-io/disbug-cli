package mcp

import (
	"context"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const sdkTestTimeout = 3 * time.Second

func callTool(t *testing.T, srv *sdkmcp.Server, name string, arguments any) (*sdkmcp.CallToolResult, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), sdkTestTimeout)
	t.Cleanup(cancel)

	clientTransport, serverTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server Connect() error = %v", err)
	}
	t.Cleanup(func() {
		_ = serverSession.Close()
	})

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "disbug-test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client Connect() error = %v", err)
	}
	t.Cleanup(func() {
		_ = clientSession.Close()
	})

	return clientSession.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
}
