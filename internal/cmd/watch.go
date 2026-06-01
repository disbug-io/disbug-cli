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
	"strconv"
	"strings"
	"time"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/localstore"
)

const defaultWatchPollInterval = 30 * time.Second

// WatchCmd streams new Disbug sessions as JSONL events.
type WatchCmd struct {
	LocalOnly    bool   `name:"local-only" help:"Watch local sessions only."`
	CloudOnly    bool   `name:"cloud-only" help:"Watch cloud sessions only."`
	Since        string `help:"Backfill sessions newer than this duration before watching, e.g. 30s, 15m, or 2h."`
	Status       string `help:"Filter by status." enum:"open,resolved,dismissed," default:""`
	Project      string `help:"Filter by project slug."`
	Exec         string `help:"Run this shell command once per non-backfill event."`
	Format       string `help:"Output format." enum:"jsonl,text" default:"jsonl"`
	PollInterval string `help:"Polling interval." default:"30s"`
}

type watchEvent struct {
	Type      string       `json:"type"`
	Source    string       `json:"source"`
	Backfill  bool         `json:"backfill"`
	EmittedAt string       `json:"emitted_at"`
	Session   watchSession `json:"session"`
}

type watchSession struct {
	ID        string     `json:"id"`
	Ref       string     `json:"ref"`
	URL       string     `json:"url,omitempty"`
	CreatedAt string     `json:"created_at"`
	Status    string     `json:"status"`
	Project   string     `json:"project,omitempty"`
	SourceURL string     `json:"source_url,omitempty"`
	PinCount  int        `json:"pin_count"`
	Pins      []watchPin `json:"pins"`
}

type watchPin struct {
	PinNumber int    `json:"pin_number"`
	Feedback  string `json:"feedback"`
	URL       string `json:"url,omitempty"`
}

type localWatchOptions struct {
	Since  time.Time
	Status string
	Now    func() time.Time
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

// Run streams session.new events until the context is cancelled.
func (c *WatchCmd) Run(ctx context.Context, b bindings) error {
	if c.LocalOnly && c.CloudOnly {
		return &errfmt.UsageError{Message: "--local-only and --cloud-only cannot be combined"}
	}

	var cloudClient *client.Client
	localEnabled := !c.CloudOnly
	cloudEnabled := c.CloudOnly
	if c.CloudOnly {
		var err error
		cloudClient, _, err = newAuthenticatedClient(b.Flags)
		if err != nil {
			return &errfmt.UsageError{Message: "--cloud-only requires sign-in; run disbug login"}
		}
	} else if !c.LocalOnly {
		var err error
		cloudClient, _, err = newAuthenticatedClient(b.Flags)
		if err == nil {
			cloudEnabled = true
		} else {
			var noToken errfmt.NoToken
			if !errors.As(err, &noToken) {
				return err
			}
		}
	}

	sinceRaw, sinceDuration, err := parseSinceFlag(c.Since)
	if err != nil {
		return err
	}
	pollInterval, err := parseWatchPollInterval(c.PollInterval)
	if err != nil {
		return err
	}

	var store *localstore.Store
	if localEnabled {
		store, err = localstore.Open("")
		if err != nil {
			return err
		}
		defer func() {
			_ = store.Close()
		}()
	}

	sinceLabel := "none"
	if sinceRaw != "" {
		sinceLabel = sinceRaw
	}
	sourceLabels := make([]string, 0, 2)
	if localEnabled {
		sourceLabels = append(sourceLabels, "local")
	}
	if cloudEnabled {
		sourceLabels = append(sourceLabels, "cloud")
	}
	_, _ = fmt.Fprintf(b.Stderr, "watching: %s  (since=%s, poll=%s)\n", strings.Join(sourceLabels, ", "), sinceLabel, pollInterval)

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
	var liveSince time.Time
	if sinceDuration > 0 {
		liveSince = now().Add(-sinceDuration)
		var events []watchEvent
		if localEnabled {
			localEvents, err := localBackfillEvents(ctx, store, localWatchOptions{
				Since:  liveSince,
				Status: c.Status,
				Now:    now,
			})
			if err != nil {
				return err
			}
			events = append(events, localEvents...)
		}
		if cloudEnabled {
			cloudEvents, err := cloudBackfillEvents(ctx, cloudClient, cloudWatchOptions{
				Since:   liveSince,
				Status:  c.Status,
				Project: c.Project,
				Now:     now,
			})
			if err != nil {
				return err
			}
			events = append(events, cloudEvents...)
		}
		sortWatchEvents(events)
		if err := emitNewWatchEvents(ctx, events, dedupe, emitter); err != nil {
			return err
		}
	} else {
		if localEnabled {
			if err := seedLocalDedupe(ctx, store, dedupe); err != nil {
				return err
			}
		}
		if cloudEnabled {
			if err := seedCloudDedupe(ctx, cloudClient, c.Status, c.Project, dedupe); err != nil {
				return err
			}
		}
	}

	switch {
	case localEnabled && cloudEnabled:
		return watchCombinedPoll(ctx, store, cloudClient, localWatchOptions{Since: liveSince, Status: c.Status, Now: now}, cloudWatchOptions{Since: liveSince, Status: c.Status, Project: c.Project, Now: now}, pollInterval, dedupe, emitter)
	case cloudEnabled:
		return watchCloudPoll(ctx, cloudClient, cloudWatchOptions{Since: liveSince, Status: c.Status, Project: c.Project, Now: now}, pollInterval, dedupe, emitter)
	default:
		return watchLocalPoll(ctx, store, localWatchOptions{Since: liveSince, Status: c.Status, Now: now}, pollInterval, dedupe, emitter)
	}
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

func localBackfillEvents(
	ctx context.Context,
	store *localstore.Store,
	opts localWatchOptions,
) ([]watchEvent, error) {
	events, err := localEvents(ctx, store, opts, true)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Session.CreatedAt < events[j].Session.CreatedAt
	})
	return events, nil
}

func localLiveEvents(
	ctx context.Context,
	store *localstore.Store,
	opts localWatchOptions,
) ([]watchEvent, error) {
	return localEvents(ctx, store, opts, false)
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
		Status:  opts.Status,
		Project: opts.Project,
		Limit:   100,
	}
	if !opts.Since.IsZero() {
		params.CreatedAtAfter = opts.Since.UTC().Format(time.RFC3339)
	}
	resp, err := cli.ListSessions(ctx, params)
	if err != nil {
		return nil, err
	}

	now := opts.Now
	if now == nil {
		now = func() time.Time {
			return time.Now().UTC()
		}
	}
	events := make([]watchEvent, 0, len(resp.Results))
	for _, summary := range resp.Results {
		event, err := cloudWatchEvent(ctx, cli, summary, backfill, now())
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func localEvents(
	ctx context.Context,
	store *localstore.Store,
	opts localWatchOptions,
	backfill bool,
) ([]watchEvent, error) {
	list, err := store.ListSessions(ctx, localstore.ListOptions{
		Limit: 100,
		Since: opts.Since,
	})
	if err != nil {
		return nil, err
	}

	now := opts.Now
	if now == nil {
		now = func() time.Time {
			return time.Now().UTC()
		}
	}
	events := make([]watchEvent, 0, len(list.Results))
	for _, summary := range list.Results {
		if opts.Status != "" && summary.Status != opts.Status {
			continue
		}
		event, err := localWatchEvent(ctx, store, summary, backfill, now())
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func localWatchEvent(
	ctx context.Context,
	store *localstore.Store,
	summary localstore.SessionSummary,
	backfill bool,
	emittedAt time.Time,
) (watchEvent, error) {
	session, err := store.GetSession(ctx, summary.ID)
	if err != nil {
		return watchEvent{}, err
	}
	pins := watchPinsFromSession(session)
	return watchEvent{
		Type:      "session.new",
		Source:    "local",
		Backfill:  backfill,
		EmittedAt: emittedAt.UTC().Format(time.RFC3339),
		Session: watchSession{
			ID:        summary.ID,
			Ref:       "disbug://local/" + summary.ID,
			CreatedAt: summary.CreatedAt,
			Status:    summary.Status,
			SourceURL: summary.URL,
			PinCount:  summary.PinCount,
			Pins:      pins,
		},
	}, nil
}

func cloudWatchEvent(
	ctx context.Context,
	cli *client.Client,
	summary client.SessionSummary,
	backfill bool,
	emittedAt time.Time,
) (watchEvent, error) {
	detail, err := cli.GetSession(ctx, summary.ID)
	if err != nil {
		return watchEvent{}, err
	}

	pins := make([]watchPin, 0, len(detail.Pins))
	for _, pin := range detail.Pins {
		pinURL := ""
		if pin.URL != nil {
			pinURL = *pin.URL
		}
		pins = append(pins, watchPin{
			PinNumber: int(pin.Number),
			Feedback:  pin.Feedback,
			URL:       pinURL,
		})
	}
	sort.SliceStable(pins, func(i, j int) bool {
		return pins[i].PinNumber < pins[j].PinNumber
	})

	id := strconv.FormatInt(summary.ID, 10)
	createdAt := summary.CreatedAt
	if createdAt == "" {
		createdAt = summary.UpdatedAt
	}
	if createdAt == "" {
		createdAt = detail.UpdatedAt
	}
	status := summary.Status
	if status == "" {
		status = detail.Status
	}
	project := ""
	if slug := projectSlug(summary.Project); slug != "" {
		project = slug
	} else if slug := projectSlug(detail.Project); slug != "" {
		project = slug
	}
	sourceURL := summary.URL
	if sourceURL == "" {
		sourceURL = detail.URL
	}
	pinCount := summary.PinCount
	if pinCount == 0 {
		pinCount = len(pins)
	}

	return watchEvent{
		Type:      "session.new",
		Source:    "cloud",
		Backfill:  backfill,
		EmittedAt: emittedAt.UTC().Format(time.RFC3339),
		Session: watchSession{
			ID:        id,
			Ref:       "disbug://cloud/" + id,
			URL:       sourceURL,
			CreatedAt: createdAt,
			Status:    status,
			Project:   project,
			SourceURL: sourceURL,
			PinCount:  pinCount,
			Pins:      pins,
		},
	}, nil
}

func watchPinsFromSession(session map[string]any) []watchPin {
	rawPins, _ := session["pins"].([]any)
	pins := make([]watchPin, 0, len(rawPins))
	for _, raw := range rawPins {
		pinMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		number := intFromAny(pinMap["pin_number"])
		if number == 0 {
			number = intFromAny(pinMap["number"])
		}
		feedback, _ := pinMap["feedback"].(string)
		url, _ := pinMap["url"].(string)
		pins = append(pins, watchPin{
			PinNumber: number,
			Feedback:  feedback,
			URL:       url,
		})
	}
	sort.SliceStable(pins, func(i, j int) bool {
		return pins[i].PinNumber < pins[j].PinNumber
	})
	return pins
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		parsed, _ := strconv.Atoi(typed)
		return parsed
	default:
		return 0
	}
}

func projectSlug(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		slug, _ := typed["slug"].(string)
		return slug
	case client.Project:
		return typed.Slug
	case *client.Project:
		if typed == nil {
			return ""
		}
		return typed.Slug
	default:
		return ""
	}
}

func seedLocalDedupe(ctx context.Context, store *localstore.Store, dedupe map[string]bool) error {
	list, err := store.ListSessions(ctx, localstore.ListOptions{Limit: 100})
	if err != nil {
		return err
	}
	for _, summary := range list.Results {
		dedupe[dedupeKey("local", summary.ID)] = true
	}
	return nil
}

func seedCloudDedupe(ctx context.Context, cli *client.Client, status string, project string, dedupe map[string]bool) error {
	resp, err := cli.ListSessions(ctx, &client.ListSessionsParams{
		Status:  status,
		Project: project,
		Limit:   100,
	})
	if err != nil {
		return err
	}
	for _, summary := range resp.Results {
		dedupe[dedupeKey("cloud", strconv.FormatInt(summary.ID, 10))] = true
	}
	return nil
}

func watchLocalPoll(
	ctx context.Context,
	store *localstore.Store,
	opts localWatchOptions,
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
			events, err := localLiveEvents(ctx, store, opts)
			if err != nil {
				if watchContextDone(ctx, err) {
					return nil
				}
				return err
			}
			sort.SliceStable(events, func(i, j int) bool {
				return events[i].Session.CreatedAt < events[j].Session.CreatedAt
			})
			if err := emitNewWatchEvents(ctx, events, dedupe, emitter); err != nil {
				return err
			}
		}
	}
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
		}
	}
}

func watchCombinedPoll(
	ctx context.Context,
	store *localstore.Store,
	cli *client.Client,
	localOpts localWatchOptions,
	cloudOpts cloudWatchOptions,
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
			events, err := localLiveEvents(ctx, store, localOpts)
			if err != nil {
				if watchContextDone(ctx, err) {
					return nil
				}
				return err
			}
			cloudEvents, err := cloudLiveEvents(ctx, cli, cloudOpts)
			if err != nil {
				if watchContextDone(ctx, err) {
					return nil
				}
				return err
			}
			events = append(events, cloudEvents...)
			sortWatchEvents(events)
			if err := emitNewWatchEvents(ctx, events, dedupe, emitter); err != nil {
				return err
			}
		}
	}
}

func sortWatchEvents(events []watchEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Session.CreatedAt < events[j].Session.CreatedAt
	})
}

func emitNewWatchEvents(ctx context.Context, events []watchEvent, dedupe map[string]bool, emitter watchEmitter) error {
	for _, event := range events {
		key := dedupeKey(event.Source, event.Session.ID)
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

func dedupeKey(source string, id string) string {
	return source + ":" + id
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

	if e.exec != "" && !event.Backfill {
		e.runExec(ctx, event, string(data))
	}
	return nil
}

func textWatchEvent(event watchEvent) string {
	firstFeedback := ""
	if len(event.Session.Pins) > 0 {
		firstFeedback = event.Session.Pins[0].Feedback
	}
	if firstFeedback == "" {
		firstFeedback = "(no pins)"
	}
	return fmt.Sprintf(
		"[NEW %s] %s - %s (%d pins)",
		event.Source,
		event.Session.ID,
		firstFeedback,
		event.Session.PinCount,
	)
}

func (e watchEmitter) runExec(ctx context.Context, event watchEvent, eventJSON string) {
	command := osExec.CommandContext(ctx, "sh", "-c", e.exec) //nolint:gosec // --exec is an explicit user-provided shell hook.
	command.Env = append(os.Environ(),
		"DISBUG_EVENT_ID="+event.Session.ID,
		"DISBUG_EVENT_SOURCE="+event.Source,
		"DISBUG_EVENT_REF="+event.Session.Ref,
		"DISBUG_EVENT_URL="+event.Session.URL,
		"DISBUG_EVENT_JSON="+eventJSON,
	)
	command.Stdout = io.Discard
	command.Stderr = e.stderr
	if err := command.Run(); err != nil {
		_, _ = fmt.Fprintf(e.stderr, "warning: --exec failed for %s: %v\n", event.Session.Ref, err)
	}
}
