package cmd

import (
	"context"

	"github.com/disbug-io/disbug-cli/internal/nativehost"
)

// NativeHostCmd runs the Chrome native messaging host.
type NativeHostCmd struct{}

// Run serves native messaging over stdin/stdout.
func (c *NativeHostCmd) Run(ctx context.Context, b bindings) error {
	return nativehost.Run(ctx, b.Stdin, b.Stdout, nativehost.Options{
		Version: VersionString(),
	})
}
