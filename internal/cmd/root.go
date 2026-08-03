package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/mcp"
	"github.com/disbug-io/disbug-cli/internal/selfupdate"
	"github.com/disbug-io/disbug-cli/internal/token"
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

	Sessions SessionsCmd `cmd:"" name:"sessions" help:"List cloud sessions, optionally filtered by status or time."`
	Session  SessionCmd  `cmd:"" name:"session" help:"Show a cloud session summary from its report URL."`
	Pin      PinCmd      `cmd:"" name:"pin" help:"Fetch selected evidence for one pin from a report URL."`
	Pins     PinsCmd     `cmd:"" name:"pins" help:"Fetch evidence for pins from multiple report URLs."`
	Search   SearchCmd   `cmd:"" name:"search" help:"Search cloud sessions or pins."`
	Watch    WatchCmd    `cmd:"" name:"watch" help:"Stream newly saved cloud sessions as events."`
	Inspect  InspectCmd  `cmd:"" name:"inspect" help:"Inspect a downloaded local report JSON file."`

	Login     LoginCmd     `cmd:"" name:"login" help:"Authenticate and save a Disbug token profile."`
	Logout    LogoutCmd    `cmd:"" name:"logout" help:"Remove a saved Disbug token profile."`
	Whoami    WhoamiCmd    `cmd:"" name:"whoami" help:"Show the authenticated agent and team."`
	Configure ConfigureCmd `cmd:"" name:"configure" help:"Connect Disbug MCP and its workflow skill to AI coding agents."`
	Doctor    DoctorCmd    `cmd:"" name:"doctor" help:"Check login, backend compatibility, and agent integrations."`

	MCP        MCPCmd        `cmd:"" name:"mcp" help:"Run the Disbug MCP server over stdio."`
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
		kong.Description("Read Disbug reports from the terminal or connect them to an AI coding agent.\n\nGetting started: disbug login, then disbug configure, then disbug doctor.\nRun disbug <command> --help for command-specific flags."),
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

	runErr := kctx.Run()
	if runErr != nil {
		if message := errfmt.Format(runErr); message != "" {
			_, _ = fmt.Fprintln(stderr, message)
		}
	}
	maybeNotifyUpdate(ctx, selectedCommand(kctx), stderr)
	return runErr
}

// maybeNotifyUpdate prints a best-effort "newer version available" nudge to
// stderr. It is a no-op for dev builds, opted-out shells, and commands whose
// output must stay uncontaminated (the MCP stdio stream, shell completion).
func maybeNotifyUpdate(ctx context.Context, command string, stderr io.Writer) {
	if updateCheckDisabled(command) {
		return
	}
	dir, err := token.Dir()
	if err != nil {
		return
	}
	if notice := selfupdate.Notice(ctx, version, dir, time.Now(), selfupdate.GitHubLatest); notice != "" {
		_, _ = fmt.Fprintf(stderr, "\n%s\n", notice)
	}
}

func updateCheckDisabled(command string) bool {
	if os.Getenv("DISBUG_NO_UPDATE_CHECK") != "" {
		return true
	}
	switch command {
	case "mcp", "completion", "version":
		return true
	default:
		return false
	}
}

func selectedCommand(kctx *kong.Context) string {
	if kctx == nil {
		return ""
	}
	fields := strings.Fields(kctx.Command())
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
