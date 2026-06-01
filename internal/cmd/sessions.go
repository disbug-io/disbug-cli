package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/outfmt"
)

// SessionsCmd lists sessions.
type SessionsCmd struct {
	Status  string `help:"Filter by status" enum:"open,resolved,dismissed," default:""`
	Project string `help:"Filter by project slug or ID"`
	Limit   int    `help:"Max results (1-100)" default:"50"`
	Cursor  string `help:"Pagination cursor"`
	Since   string `help:"Only include sessions newer than this duration, e.g. 30s, 15m, or 2h."`
}

// Run lists sessions and writes the paginated response as JSON.
func (c *SessionsCmd) Run(ctx context.Context, b bindings) error {
	if c.Limit < 1 || c.Limit > 100 {
		return &errfmt.UsageError{Message: "--limit must be between 1 and 100"}
	}
	_, sinceDuration, err := parseSinceFlag(c.Since)
	if err != nil {
		return err
	}
	createdAtAfter := ""
	if sinceDuration > 0 {
		createdAtAfter = time.Now().UTC().Add(-sinceDuration).Format(time.RFC3339)
	}

	cli, _, err := newAuthenticatedClient(b.Flags)
	if err != nil {
		return err
	}

	// Handle local pagination cursor
	cursor := c.Cursor
	var localOffset int
	if strings.HasPrefix(cursor, "local-offset:") {
		offsetStr := strings.TrimPrefix(cursor, "local-offset:")
		if val, err := strconv.Atoi(offsetStr); err == nil {
			localOffset = val
			cursor = "" // Don't pass local offset to backend
		}
	}

	// Fetch without status and project filters to avoid backend 500 errors.
	// We use a high limit to allow effective local filtering.
	resp, err := cli.ListSessions(ctx, &client.ListSessionsParams{
		Status:         c.Status,
		Project:        c.Project,
		Limit:          100,
		Cursor:         cursor,
		CreatedAtAfter: createdAtAfter,
	})
	if err != nil {
		return err
	}

	results := resp.Results

	// Local filtering for status
	if c.Status != "" {
		filtered := make([]client.SessionSummary, 0)
		for _, s := range results {
			if s.Status == c.Status {
				filtered = append(filtered, s)
			}
		}
		results = filtered
	}

	// Local filtering for project
	if c.Project != "" {
		filtered := make([]client.SessionSummary, 0)
		for _, s := range results {
			match := false
			switch p := s.Project.(type) {
			case float64:
				if fmt.Sprintf("%.0f", p) == c.Project {
					match = true
				}
			case string:
				if p == c.Project {
					match = true
				}
			case map[string]any:
				if slug, ok := p["slug"].(string); ok && slug == c.Project {
					match = true
				}
				if id, ok := p["id"].(float64); ok && fmt.Sprintf("%.0f", id) == c.Project {
					match = true
				}
			}
			if match {
				filtered = append(filtered, s)
			}
		}
		results = filtered
	}

	// Update count to reflect total items matching local filters
	totalMatching := len(results)
	resp.Count = totalMatching

	// Apply local offset and limit
	start := localOffset
	if start > totalMatching {
		start = totalMatching
	}
	end := start + c.Limit
	if end > totalMatching {
		end = totalMatching
	}

	resp.Results = results[start:end]
	resp.Count = len(resp.Results)

	// Set local next cursor if there are more results
	if end < totalMatching {
		nextCursor := fmt.Sprintf("local-offset:%d", end)
		resp.NextCursor = &nextCursor
	} else {
		resp.NextCursor = nil
	}

	return outfmt.WriteJSON(b.Stdout, resp, b.Flags.Pretty)
}
