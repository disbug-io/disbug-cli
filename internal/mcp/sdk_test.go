package mcp

import (
	"context"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func callTool(t *testing.T, srv *sdkmcp.Server, name string, arguments any) (*sdkmcp.CallToolResult, error) {
	t.Helper()

	clientTransport, serverTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := srv.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server Connect() error = %v", err)
	}
	t.Cleanup(func() {
		_ = serverSession.Close()
	})

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "disbug-test", Version: "test"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client Connect() error = %v", err)
	}
	t.Cleanup(func() {
		_ = clientSession.Close()
	})

	return clientSession.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
}
