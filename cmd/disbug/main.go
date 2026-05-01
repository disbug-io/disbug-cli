package main

import (
	"context"
	"os"

	"github.com/disbug-io/disbug-cli/internal/cmd"
)

func main() {
	ctx := context.Background()
	err := cmd.Execute(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	os.Exit(cmd.ExitCode(err))
}
