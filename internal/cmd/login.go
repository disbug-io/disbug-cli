package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/disbug-io/disbug-cli/internal/auth"
	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/seams"
	"github.com/disbug-io/disbug-cli/internal/token"
)

const (
	loginWaitTimeout = 5 * time.Minute
	loginCloseDelay  = 500 * time.Millisecond
)

// LoginCmd logs in to Disbug and persists a token profile.
type LoginCmd struct {
	Name           string  `help:"Agent name to pre-fill. Defaults to hostname."`
	APIURL         string  `name:"api-url" env:"DISBUG_API_URL" default:"https://disbug.io" help:"Disbug API URL."`
	ListenAddr     string  `name:"listen-addr" help:"Local listener host:port override."`
	Manual         bool    `help:"Print auth URL and paste redirect URL back instead of listening."`
	NoBrowser      bool    `name:"no-browser" help:"Print auth URL instead of opening the browser."`
	Force          bool    `help:"Overwrite an existing token profile."`
	Token          *string `help:"Set token directly."`
	TokenFromStdin bool    `name:"token-from-stdin" help:"Read token from the first stdin line."`
	TokenFromEnv   bool    `name:"token-from-env" help:"Read token from DISBUG_LOGIN_TOKEN."`
}

// Run executes the login flow.
func (c *LoginCmd) Run(ctx context.Context, b bindings) error {
	if err := c.validateFlags(); err != nil {
		return err
	}

	name := resolvedAgentName(c.Name)
	userAgent := loginUserAgent()
	sleeper := seams.DefaultSleeper()

	tokenStr, err := c.acquireToken(ctx, b, name, sleeper)
	if err != nil {
		return err
	}

	me, err := client.New(c.APIURL, tokenStr, userAgent, nil, nil, nil).Me(ctx)
	if err != nil {
		return err
	}

	profileName := defaultProfile
	if b.Flags != nil && b.Flags.Profile != "" {
		profileName = b.Flags.Profile
	}

	createdAt := seams.DefaultClock().Now().UTC().Format(time.RFC3339)
	profile := token.Token{
		Token:          tokenStr,
		APIURL:         c.APIURL,
		AgentName:      me.AgentName,
		Team:           me.Team,
		TeamSlug:       me.TeamSlug,
		CreatedByEmail: me.CreatedByEmail,
		CreatedAt:      createdAt,
	}
	if err := token.Write(profileName, profile, c.Force); err != nil {
		if errors.Is(err, token.ErrProfileExists) {
			return errfmt.UserFacingError{
				Message: "Profile already exists. Use --force to overwrite it or --profile to choose another profile.",
				Cause:   err,
			}
		}
		return err
	}

	_, err = fmt.Fprintf(
		b.Stdout,
		"Logged in as %s for team %s.\n",
		emptyDefault(me.AgentName, name),
		me.Team,
	)
	return err
}

func (c *LoginCmd) validateFlags() error {
	modes := 0
	for _, enabled := range []bool{c.Manual, c.Token != nil, c.TokenFromStdin, c.TokenFromEnv} {
		if enabled {
			modes++
		}
	}
	if modes > 1 {
		return errfmt.UsageError{Message: "--manual, --token, --token-from-stdin, and --token-from-env are mutually exclusive"}
	}
	if c.Manual && c.ListenAddr != "" {
		return errfmt.UsageError{Message: "--manual and --listen-addr are mutually exclusive"}
	}

	return nil
}

func (c *LoginCmd) acquireToken(ctx context.Context, b bindings, name string, sleeper seams.Sleeper) (string, error) {
	switch {
	case c.Token != nil:
		value := strings.TrimSpace(*c.Token)
		if value == "" {
			return "", errfmt.UsageError{Message: "--token requires a non-empty value"}
		}
		_, _ = fmt.Fprintln(b.Stderr, "warning: using token provided on the command line")
		return value, nil
	case c.TokenFromStdin:
		line, err := readFirstLine(b.Stdin)
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return "", errfmt.UsageError{Message: "--token-from-stdin requires a non-empty first line"}
		}
		_, _ = fmt.Fprintln(b.Stderr, "warning: using token read from stdin")
		return line, nil
	case c.TokenFromEnv:
		value := strings.TrimSpace(os.Getenv("DISBUG_LOGIN_TOKEN"))
		if value == "" {
			return "", errfmt.UsageError{Message: "DISBUG_LOGIN_TOKEN is empty or not set"}
		}
		_, _ = fmt.Fprintln(b.Stderr, "warning: using token read from DISBUG_LOGIN_TOKEN")
		return value, nil
	case c.Manual:
		return c.acquireManualToken(b, name)
	default:
		return c.acquireBrowserToken(ctx, b, name, sleeper)
	}
}

func (c *LoginCmd) acquireBrowserToken(
	ctx context.Context,
	b bindings,
	name string,
	sleeper seams.Sleeper,
) (string, error) {
	state, err := auth.GenerateState(seams.DefaultRandom())
	if err != nil {
		return "", err
	}

	listener, err := auth.NewListener(state, auth.SuccessHTML, auth.ErrorHTML, c.ListenAddr, nil, sleeper)
	if err != nil {
		return "", err
	}

	callback := callbackURL(c.ListenAddr, listener.Port())
	authURL := auth.BuildAuthURL(c.APIURL, callback, state, name)
	if c.NoBrowser {
		_, _ = fmt.Fprintf(b.Stderr, "Open this URL to log in:\n%s\n", authURL)
	} else {
		_, _ = fmt.Fprintf(b.Stderr, "Opening browser for Disbug login:\n%s\n", authURL)
		if err := auth.Open(authURL); err != nil {
			_ = listener.Close()
			return "", errfmt.UserFacingError{Message: fmt.Sprintf("Could not open browser: %s", err), Cause: err}
		}
	}

	result, err := listener.Wait(ctx, loginWaitTimeout)
	if err != nil {
		_ = listener.Close()
		if errors.Is(err, context.DeadlineExceeded) {
			return "", errfmt.UserFacingError{Message: "Timed out waiting for browser login.", Cause: err}
		}
		return "", errfmt.UserFacingError{Message: fmt.Sprintf("Browser login failed: %s", err), Cause: err}
	}
	if result.Err != nil {
		_ = listener.Close()
		return "", errfmt.UserFacingError{Message: fmt.Sprintf("Browser login failed: %s", result.Err), Cause: result.Err}
	}

	sleeper.Sleep(loginCloseDelay)
	if err := listener.Close(); err != nil {
		return "", err
	}

	return result.Token, nil
}

func (c *LoginCmd) acquireManualToken(b bindings, name string) (string, error) {
	state, err := auth.GenerateState(seams.DefaultRandom())
	if err != nil {
		return "", err
	}

	port, err := auth.BindFreePort()
	if err != nil {
		return "", err
	}
	callback := fmt.Sprintf("http://127.0.0.1:%d/cb", port)
	authURL := auth.BuildAuthURL(c.APIURL, callback, state, name)
	_, _ = fmt.Fprintf(b.Stderr, "Open this URL to log in:\n%s\nPaste the final redirect URL here:\n", authURL)

	rawPaste, err := readFirstLine(b.Stdin)
	if err != nil {
		return "", err
	}
	tokenStr, gotState, err := auth.ParsePastebackURL(rawPaste)
	if err != nil {
		return "", errfmt.UserFacingError{Message: fmt.Sprintf("Could not parse pasted redirect URL: %s", err), Cause: err}
	}
	if gotState != state {
		return "", errfmt.UserFacingError{Message: "Pasted redirect URL state did not match."}
	}

	return strings.TrimSpace(tokenStr), nil
}

func callbackURL(listenAddr string, port int) string {
	host := "127.0.0.1"
	if listenAddr != "" {
		if parsedHost, _, err := net.SplitHostPort(listenAddr); err == nil {
			parsedHost = strings.Trim(parsedHost, "[]")
			switch parsedHost {
			case "", "0.0.0.0":
				host = "127.0.0.1"
			case "::":
				host = "::1"
			default:
				host = parsedHost
			}
		}
	}

	return (&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		Path:   "/cb",
	}).String()
}

func readFirstLine(r ioReader) (string, error) {
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", errfmt.UsageError{Message: "expected input but reached end of file"}
	}

	return scanner.Text(), nil
}

type ioReader interface {
	Read([]byte) (int, error)
}

func resolvedAgentName(name string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}

	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		return "disbug-agent"
	}

	return hostname
}

func loginUserAgent() string {
	return fmt.Sprintf("disbug-cli/%s (%s/%s)", VersionString(), runtime.GOOS, runtime.GOARCH)
}

func emptyDefault(value, fallback string) string {
	if value != "" {
		return value
	}

	return fallback
}
