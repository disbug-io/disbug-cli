package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/mcp"
)

// RootFlags are global flags accepted by every command.
type RootFlags struct {
	Pretty  bool   `help:"Indent JSON output with 2 spaces."`
	Profile string `default:"default" env:"DISBUG_PROFILE" help:"Configuration profile to use."`
	Verbose bool   `short:"v" help:"Enable verbose logging."`
}

// CLI is the root Kong command graph.
type CLI struct {
	RootFlags

	Version kong.VersionFlag `name:"version" help:"Show version."`

	Sessions SessionsCmd `cmd:"" name:"sessions" help:"List sessions."`
	Session  SessionCmd  `cmd:"" name:"session" help:"Show a session by report URL."`
	Pin      PinCmd      `cmd:"" name:"pin" help:"Show a pin by report URL."`
	Pins     PinsCmd     `cmd:"" name:"pins" help:"Fetch pins by report URL."`
	Search   SearchCmd   `cmd:"" name:"search" help:"Search Disbug data."`
	Watch    WatchCmd    `cmd:"" name:"watch" help:"Print new session notifications."`
	Inspect  InspectCmd  `cmd:"" name:"inspect" help:"Inspect a downloaded local report JSON file."`

	Login  LoginCmd  `cmd:"" name:"login" help:"Log in to Disbug."`
	Logout LogoutCmd `cmd:"" name:"logout" help:"Log out of Disbug."`
	Whoami WhoamiCmd `cmd:"" name:"whoami" help:"Show the current user."`
	Doctor DoctorCmd `cmd:"" name:"doctor" help:"Check CLI configuration."`

	MCP        MCPCmd        `cmd:"" name:"mcp" help:"Run MCP integration commands."`
	Completion CompletionCmd `cmd:"" name:"completion" help:"Generate shell completion scripts."`
	VersionCmd VersionCmd    `cmd:"" name:"version" help:"Print the disbug version."`
}

type bindings struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Flags  *RootFlags
}

type exitPanic struct {
	code int
}

// Execute parses args and dispatches to the selected command.
func Execute(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if len(args) == 0 {
		args = []string{"--help"}
	}

	level := slog.LevelWarn
	defer func() {
		if recovered := recover(); recovered != nil {
			exit, ok := recovered.(exitPanic)
			if !ok {
				panic(recovered)
			}
			if exit.code == 0 {
				err = nil
				return
			}
			err = errfmt.UsageError{Message: fmt.Sprintf("disbug exited with code %d", exit.code)}
		}
	}()

	cli := CLI{}
	b := bindings{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Flags:  &cli.RootFlags,
	}
	mcp.SetVersion(VersionString())

	parser, parseErr := kong.New(
		&cli,
		kong.Name("disbug"),
		kong.Description("Disbug command line interface."),
		kong.Vars{"version": VersionString()},
		kong.Writers(stdout, stderr),
		kong.Exit(func(code int) {
			panic(exitPanic{code: code})
		}),
		kong.Bind(b),
		kong.BindTo(ctx, (*context.Context)(nil)),
	)
	if parseErr != nil {
		return parseErr
	}

	kctx, parseErr := parser.Parse(args)
	if parseErr != nil {
		usageErr := errfmt.UsageError{Message: parseErr.Error()}
		if message := errfmt.Format(usageErr); message != "" {
			_, _ = fmt.Fprintln(stderr, message)
		}
		return usageErr
	}

	if cli.Verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level})))

	if runErr := kctx.Run(); runErr != nil {
		if message := errfmt.Format(runErr); message != "" {
			_, _ = fmt.Fprintln(stderr, message)
		}
		return runErr
	}
	return nil
}
