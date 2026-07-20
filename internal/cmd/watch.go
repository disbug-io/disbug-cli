package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	osExec "os/exec"
	"sort"
	"strings"
	"time"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

const defaultWatchPollInterval = 30 * time.Second

// WatchCmd streams new Disbug sessions as JSONL events.
type WatchCmd struct {
	CloudOnly    bool   `name:"cloud-only" help:"Watch cloud sessions only."`
	Since        string `help:"Backfill sessions newer than this duration before watching, e.g. 30s, 15m, or 2h."`
	Status       string `help:"Filter by status." enum:"open,resolved,dismissed," default:""`
	Project      string `help:"Filter by project slug."`
	Exec         string `help:"Run this shell command once per non-backfill event."`
	Format       string `help:"Output format." enum:"jsonl,text" default:"jsonl"`
	PollInterval string `help:"Polling interval." default:"30s"`
}

type watchEvent struct {
	ReportURL string     `json:"report_url"`
	Title     string     `json:"title"`
	CreatedAt string     `json:"created_at"`
	Pins      []watchPin `json:"pins"`
	id        string
	backfill  bool
}

type watchPin struct {
	Number   int    `json:"number"`
	Feedback string `json:"feedback"`
}

type cloudWatchOptions struct {
	Since   time.Time
	Status  string
	Project string
	Now     func() time.Time
}

type watchEmitter struct {
	stdout io.Writer
	stderr io.Writer
	format string
	exec   string
}

// Run streams new session events until the context is cancelled.
func (c *WatchCmd) Run(ctx context.Context, b bindings) error {
	cloudClient, _, err := newAuthenticatedClient(b.Flags)
	if err != nil {
		if c.CloudOnly {
			return &errfmt.UsageError{Message: "--cloud-only requires sign-in; run disbug login"}
		}
		return err
	}

	sinceRaw, sinceDuration, err := parseSinceFlag(c.Since)
	if err != nil {
		return err
	}
	pollInterval, err := parseWatchPollInterval(c.PollInterval)
	if err != nil {
		return err
	}

	sinceLabel := "none"
	if sinceRaw != "" {
		sinceLabel = sinceRaw
	}
	_, _ = fmt.Fprintf(b.Stderr, "watching: cloud  (since=%s, poll=%s)\n", sinceLabel, pollInterval)

	emitter := watchEmitter{
		stdout: b.Stdout,
		stderr: b.Stderr,
		format: c.Format,
		exec:   c.Exec,
	}
	dedupe := map[string]bool{}
	now := func() time.Time {
		return time.Now().UTC()
	}
	startedAt := now()
	if sinceDuration > 0 {
		backfillSince := startedAt.Add(-sinceDuration)
		events, err := cloudBackfillEvents(ctx, cloudClient, cloudWatchOptions{
			Since:   backfillSince,
			Status:  c.Status,
			Project: c.Project,
			Now:     now,
		})
		if err != nil {
			return err
		}
		if err := emitNewWatchEvents(ctx, events, dedupe, emitter); err != nil {
			return err
		}
	}

	return watchCloudPoll(ctx, cloudClient, cloudWatchOptions{Since: startedAt, Status: c.Status, Project: c.Project, Now: now}, pollInterval, dedupe, emitter)
}

func parseWatchPollInterval(raw string) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultWatchPollInterval, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, &errfmt.UsageError{Message: "--poll-interval must be a positive duration"}
	}
	return duration, nil
}

func cloudBackfillEvents(
	ctx context.Context,
	cli *client.Client,
	opts cloudWatchOptions,
) ([]watchEvent, error) {
	events, err := cloudEvents(ctx, cli, opts, true)
	if err != nil {
		return nil, err
	}
	sortWatchEvents(events)
	return events, nil
}

func cloudLiveEvents(
	ctx context.Context,
	cli *client.Client,
	opts cloudWatchOptions,
) ([]watchEvent, error) {
	return cloudEvents(ctx, cli, opts, false)
}

func cloudEvents(
	ctx context.Context,
	cli *client.Client,
	opts cloudWatchOptions,
	backfill bool,
) ([]watchEvent, error) {
	params := &client.ListSessionsParams{
		Status:      opts.Status,
		Project:     opts.Project,
		Limit:       100,
		IncludePins: true,
	}
	if !opts.Since.IsZero() {
		params.CreatedAtAfter = opts.Since.UTC().Format(time.RFC3339)
	}
	resp, err := cli.ListSessions(ctx, params)
	if err != nil {
		return nil, err
	}

	events := make([]watchEvent, 0, len(resp.Results))
	for _, summary := range resp.Results {
		event, err := cloudWatchEvent(summary, backfill)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func cloudWatchEvent(
	summary client.SessionSummary,
	backfill bool,
) (watchEvent, error) {
	_, err := summary.SessionRef()
	if err != nil {
		return watchEvent{}, err
	}

	pins := make([]watchPin, 0, len(summary.Pins))
	for _, pin := range summary.Pins {
		pins = append(pins, watchPin{
			Number:   int(pin.Number),
			Feedback: pin.Feedback,
		})
	}
	sort.SliceStable(pins, func(i, j int) bool {
		return pins[i].Number < pins[j].Number
	})

	id := summary.ScopedID()
	createdAt := summary.CreatedAt
	if createdAt == "" {
		createdAt = summary.UpdatedAt
	}

	return watchEvent{
		ReportURL: summary.ReportURL,
		Title:     summary.Title,
		CreatedAt: createdAt,
		Pins:      pins,
		id:        id,
		backfill:  backfill,
	}, nil
}

func watchCloudPoll(
	ctx context.Context,
	cli *client.Client,
	opts cloudWatchOptions,
	pollInterval time.Duration,
	dedupe map[string]bool,
	emitter watchEmitter,
) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			pollStartedAt := time.Now().UTC()
			if opts.Now != nil {
				pollStartedAt = opts.Now()
			}
			events, err := cloudLiveEvents(ctx, cli, opts)
			if err != nil {
				if watchContextDone(ctx, err) {
					return nil
				}
				return err
			}
			sortWatchEvents(events)
			if err := emitNewWatchEvents(ctx, events, dedupe, emitter); err != nil {
				return err
			}
			opts.Since = pollStartedAt
		}
	}
}

func sortWatchEvents(events []watchEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].CreatedAt < events[j].CreatedAt
	})
}

func emitNewWatchEvents(ctx context.Context, events []watchEvent, dedupe map[string]bool, emitter watchEmitter) error {
	for _, event := range events {
		key := event.id
		if dedupe[key] {
			continue
		}
		dedupe[key] = true
		if err := emitter.emit(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func watchContextDone(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func (e watchEmitter) emit(ctx context.Context, event watchEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode watch event: %w", err)
	}

	switch e.format {
	case "", "jsonl":
		if _, err := fmt.Fprintln(e.stdout, string(data)); err != nil {
			return err
		}
	case "text":
		if _, err := fmt.Fprintln(e.stdout, textWatchEvent(event)); err != nil {
			return err
		}
	default:
		return &errfmt.UsageError{Message: "--format must be jsonl or text"}
	}
	if flusher, ok := e.stdout.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			return err
		}
	}

	if e.exec != "" && !event.backfill {
		e.runExec(ctx, event, string(data))
	}
	return nil
}

func textWatchEvent(event watchEvent) string {
	firstFeedback := ""
	if len(event.Pins) > 0 {
		firstFeedback = event.Pins[0].Feedback
	}
	if firstFeedback == "" {
		firstFeedback = "(no pins)"
	}
	title := event.Title
	if title == "" {
		title = event.ReportURL
	}
	return fmt.Sprintf("[NEW] %s - %s (%d pins)", title, firstFeedback, len(event.Pins))
}

func (e watchEmitter) runExec(ctx context.Context, event watchEvent, eventJSON string) {
	command := osExec.CommandContext(ctx, "sh", "-c", e.exec) //nolint:gosec // --exec is an explicit user-provided shell hook.
	command.Env = append(os.Environ(),
		"DISBUG_EVENT_ID="+event.id,
		"DISBUG_EVENT_SOURCE=cloud",
		"DISBUG_EVENT_REF="+event.ReportURL,
		"DISBUG_EVENT_URL="+event.ReportURL,
		"DISBUG_EVENT_JSON="+eventJSON,
	)
	command.Stdout = io.Discard
	command.Stderr = e.stderr
	if err := command.Run(); err != nil {
		_, _ = fmt.Fprintf(e.stderr, "warning: --exec failed for %s: %v\n", event.ReportURL, err)
	}
}
