package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

const (
	onboardingWaitTimeout  = 5 * time.Minute
	onboardingPollInterval = 2 * time.Second
)

func (c *OnboardCmd) runStatus(ctx context.Context, b bindings) error {
	apiClient, profile, err := newAuthenticatedClient(b.Flags)
	if err != nil {
		return err
	}
	if c.APIURL != "" && strings.TrimRight(c.APIURL, "/") != strings.TrimRight(emptyDefault(profile.APIURL, "https://disbug.io"), "/") {
		return errfmt.UsageError{Message: "The saved profile uses a different API URL; run disbug onboard to connect it first."}
	}
	if err := apiClient.RequireCapability(ctx, "onboarding_setup"); err != nil {
		return err
	}
	var result *client.Onboarding
	if c.WaitFor == "" {
		result, err = apiClient.GetOnboardingForProject(ctx, c.ProjectID)
	} else {
		_, _ = fmt.Fprintf(b.Stderr, "Waiting for %s (up to five minutes)...\n", c.WaitFor)
		result, err = waitForOnboarding(ctx, apiClient, c.ProjectID, c.WaitFor, onboardingWaitTimeout, onboardingPollInterval)
	}
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(b.Stdout)
	if b.Flags != nil && b.Flags.Pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(result)
}

func waitForOnboarding(ctx context.Context, apiClient *client.Client, projectID int, milestone string, timeout, interval time.Duration) (*client.Onboarding, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		result, err := apiClient.GetOnboardingForProject(waitCtx, projectID)
		if err == nil && ((milestone == "extension_installed" && result.Progress.ExtensionInstalled) ||
			(milestone == "first_report_captured" && result.Progress.FirstReportCaptured)) {
			return result, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if waitCtx.Err() != nil {
			return nil, &errfmt.UserFacingError{Message: fmt.Sprintf("Timed out waiting for %s. Finish setup in Disbug, then rerun disbug onboard.", milestone)}
		}
		if err != nil {
			return nil, err
		}
		timer := time.NewTimer(interval)
		select {
		case <-waitCtx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
}
