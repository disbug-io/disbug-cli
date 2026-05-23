package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/localstore"
	"github.com/disbug-io/disbug-cli/internal/outfmt"
)

// LocalSessionsCmd manages local reports written by native-host.
type LocalSessionsCmd struct {
	List   LocalSessionsListCmd   `cmd:"" name:"list" help:"List local sessions."`
	Show   LocalSessionsShowCmd   `cmd:"" name:"show" help:"Show local session metadata."`
	Delete LocalSessionsDeleteCmd `cmd:"" name:"delete" help:"Delete a local session."`
	Prune  LocalSessionsPruneCmd  `cmd:"" name:"prune" help:"Delete old local sessions."`
	Path   LocalSessionsPathCmd   `cmd:"" name:"path" help:"Print the local session store path."`
}

// LocalSessionsListCmd lists committed local sessions.
type LocalSessionsListCmd struct {
	Limit int    `default:"50" help:"Maximum sessions to return."`
	Since string `help:"Only include sessions newer than this duration, e.g. 30s, 15m, or 2h."`
}

// LocalSessionsShowCmd shows one local session.
type LocalSessionsShowCmd struct {
	ID string `arg:"" name:"local-id" help:"Local session id."`
}

// LocalSessionsDeleteCmd deletes one local session.
type LocalSessionsDeleteCmd struct {
	ID string `arg:"" name:"local-id" help:"Local session id."`
}

// LocalSessionsPruneCmd deletes local sessions older than the requested age.
type LocalSessionsPruneCmd struct {
	OlderThan string `name:"older-than" required:"" help:"Delete sessions older than duration, e.g. 30d or 12h."`
}

// LocalSessionsPathCmd prints the local session store path.
type LocalSessionsPathCmd struct{}

// Run lists local sessions and writes the response as JSON.
func (c *LocalSessionsListCmd) Run(ctx context.Context, b bindings) error {
	_, sinceDuration, err := parseSinceFlag(c.Since)
	if err != nil {
		return err
	}

	store, err := localstore.Open("")
	if err != nil {
		return err
	}
	defer func() {
		_ = store.Close()
	}()
	opts := localstore.ListOptions{Limit: c.Limit}
	if sinceDuration > 0 {
		opts.Since = time.Now().UTC().Add(-sinceDuration)
	}
	resp, err := store.ListSessions(ctx, opts)
	if err != nil {
		return err
	}
	return outfmt.WriteJSON(b.Stdout, resp, b.Flags.Pretty)
}

// Run shows local session metadata as JSON.
func (c *LocalSessionsShowCmd) Run(ctx context.Context, b bindings) error {
	store, err := localstore.Open("")
	if err != nil {
		return err
	}
	defer func() {
		_ = store.Close()
	}()
	session, err := store.GetSessionSummary(ctx, c.ID)
	if err != nil {
		return err
	}
	return outfmt.WriteJSON(b.Stdout, session, b.Flags.Pretty)
}

// Run deletes a local session and writes a confirmation as JSON.
func (c *LocalSessionsDeleteCmd) Run(ctx context.Context, b bindings) error {
	store, err := localstore.Open("")
	if err != nil {
		return err
	}
	defer func() {
		_ = store.Close()
	}()
	if err := store.DeleteSession(ctx, c.ID); err != nil {
		return err
	}
	return outfmt.WriteJSON(b.Stdout, map[string]any{"deleted": c.ID}, b.Flags.Pretty)
}

// Run prunes local sessions older than the requested duration.
func (c *LocalSessionsPruneCmd) Run(ctx context.Context, b bindings) error {
	olderThan, err := parseRetention(c.OlderThan)
	if err != nil {
		return err
	}
	store, err := localstore.Open("")
	if err != nil {
		return err
	}
	defer func() {
		_ = store.Close()
	}()
	removed, err := store.Prune(ctx, localstore.PruneOptions{OlderThan: olderThan})
	if err != nil {
		return err
	}
	return outfmt.WriteJSON(b.Stdout, map[string]any{"removed": removed}, b.Flags.Pretty)
}

// Run prints the local session store path.
func (c *LocalSessionsPathCmd) Run(ctx context.Context, b bindings) error {
	root, err := localstore.DefaultRoot()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(b.Stdout, root)
	_ = ctx
	return nil
}

func parseRetention(raw string) (time.Duration, error) {
	if len(raw) > 1 && raw[len(raw)-1] == 'd' {
		hours, err := time.ParseDuration(raw[:len(raw)-1] + "h")
		if err != nil {
			return 0, &errfmt.UsageError{Message: "--older-than must be a duration like 30d or 12h"}
		}
		return hours * 24, nil
	}
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return 0, &errfmt.UsageError{Message: "--older-than must be a duration like 30d or 12h"}
	}
	return duration, nil
}
