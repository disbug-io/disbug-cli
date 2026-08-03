package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	fetchTimeout     = 2 * time.Second
	maxResponseBytes = 1 << 16
)

// GitHubLatest returns the latest disbug release tag from GitHub. It is the
// default FetchFunc used by Notice in released builds. The timeout is short and
// all errors are returned to the caller, which treats them as "no notice".
func GitHubLatest(ctx context.Context) (string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		reqCtx,
		http.MethodGet,
		"https://api.github.com/repos/disbug-io/disbug-cli/releases/latest",
		nil,
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github releases returned status %d", resp.StatusCode)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&payload); err != nil {
		return "", err
	}
	return payload.TagName, nil
}
