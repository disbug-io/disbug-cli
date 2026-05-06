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

type LocalSessionsListCmd struct {
	Limit int `default:"50" help:"Maximum sessions to return."`
}

type LocalSessionsShowCmd struct {
	ID string `arg:"" name:"local-id" help:"Local session id."`
}

type LocalSessionsDeleteCmd struct {
	ID string `arg:"" name:"local-id" help:"Local session id."`
}

type LocalSessionsPruneCmd struct {
	OlderThan string `name:"older-than" required:"" help:"Delete sessions older than duration, e.g. 30d or 12h."`
}

type LocalSessionsPathCmd struct{}

func (c *LocalSessionsListCmd) Run(ctx context.Context, b bindings) error {
	store, err := localstore.Open("")
	if err != nil {
		return err
	}
	defer store.Close()
	resp, err := store.ListSessions(ctx, localstore.ListOptions{Limit: c.Limit})
	if err != nil {
		return err
	}
	return outfmt.WriteJSON(b.Stdout, resp, b.Flags.Pretty)
}

func (c *LocalSessionsShowCmd) Run(ctx context.Context, b bindings) error {
	store, err := localstore.Open("")
	if err != nil {
		return err
	}
	defer store.Close()
	session, err := store.GetSessionSummary(ctx, c.ID)
	if err != nil {
		return err
	}
	return outfmt.WriteJSON(b.Stdout, session, b.Flags.Pretty)
}

func (c *LocalSessionsDeleteCmd) Run(ctx context.Context, b bindings) error {
	store, err := localstore.Open("")
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.DeleteSession(ctx, c.ID); err != nil {
		return err
	}
	return outfmt.WriteJSON(b.Stdout, map[string]any{"deleted": c.ID}, b.Flags.Pretty)
}

func (c *LocalSessionsPruneCmd) Run(ctx context.Context, b bindings) error {
	olderThan, err := parseRetention(c.OlderThan)
	if err != nil {
		return err
	}
	store, err := localstore.Open("")
	if err != nil {
		return err
	}
	defer store.Close()
	removed, err := store.Prune(ctx, localstore.PruneOptions{OlderThan: olderThan})
	if err != nil {
		return err
	}
	return outfmt.WriteJSON(b.Stdout, map[string]any{"removed": removed}, b.Flags.Pretty)
}

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
