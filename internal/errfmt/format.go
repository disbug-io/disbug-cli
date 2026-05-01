package errfmt

import (
	"errors"
	"fmt"
)

// Format returns a human-facing single-line message for err.
func Format(err error) string {
	if err == nil {
		return ""
	}

	var userFacing UserFacingError
	if errors.As(err, &userFacing) {
		return userFacing.Message
	}

	var usage UsageError
	if errors.As(err, &usage) {
		return usage.Message
	}
	var usagePtr *UsageError
	if errors.As(err, &usagePtr) && usagePtr != nil {
		return usagePtr.Message
	}

	var noToken NoToken
	if errors.As(err, &noToken) {
		return "No token found. Run: disbug login"
	}

	if api, ok := asAPIError(err); ok {
		return formatAPIError(api)
	}

	var network NetworkError
	if errors.As(err, &network) {
		target := network.URL
		if target == "" {
			target = "the Disbug API"
		}
		if network.Cause == nil {
			return fmt.Sprintf("Cannot reach %s. Check your network.", target)
		}
		return fmt.Sprintf("Cannot reach %s. Check your network. (cause: %s)", target, network.Cause.Error())
	}

	return err.Error()
}

// ExitCode returns the stable CLI process exit code for err.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}

	var usage UsageError
	if errors.As(err, &usage) {
		return 2
	}
	var usagePtr *UsageError
	if errors.As(err, &usagePtr) && usagePtr != nil {
		return 2
	}

	var noToken NoToken
	if errors.As(err, &noToken) {
		return 4
	}

	var network NetworkError
	if errors.As(err, &network) {
		return 5
	}

	if api, ok := asAPIError(err); ok {
		switch {
		case isAuthError(api):
			return 4
		case api.StatusCode == 404:
			return 6
		case api.StatusCode == 403:
			return 7
		case api.StatusCode == 429:
			return 8
		case api.StatusCode >= 500:
			return 9
		}
	}

	return 1
}

func asAPIError(err error) (APIError, bool) {
	var api APIError
	if errors.As(err, &api) {
		return api, true
	}

	var apiPtr *APIError
	if errors.As(err, &apiPtr) && apiPtr != nil {
		return *apiPtr, true
	}

	return APIError{}, false
}

func formatAPIError(err APIError) string {
	switch {
	case isAuthError(err):
		return "Your token was rejected or no token was found. Run: disbug login"
	case err.Code == "agent_read_only":
		return "This operation is denied for agent tokens (read-only)."
	case err.Code == "free_tier_locked":
		return "This operation is locked on the free-tier. Upgrade at https://disbug.io/billing."
	case err.Code == "not_found" || err.StatusCode == 404:
		if err.Detail != "" {
			return err.Detail
		}
		return "not found"
	case err.Code == "rate_limited" || err.StatusCode == 429:
		return "You are rate-limited. Try again later."
	case err.StatusCode >= 500:
		requestID := err.RequestID
		if requestID == "" {
			requestID = "unknown"
		}
		return fmt.Sprintf(
			"Server error (%d). Request ID: %s. Try again, or report this ID to support.",
			err.StatusCode,
			requestID,
		)
	case err.Detail != "":
		return err.Detail
	case err.Code != "" && err.StatusCode > 0:
		return fmt.Sprintf("API error (%d, %s).", err.StatusCode, err.Code)
	case err.Code != "":
		return fmt.Sprintf("API error (%s).", err.Code)
	case err.StatusCode > 0:
		return fmt.Sprintf("API error (%d).", err.StatusCode)
	default:
		return "API error."
	}
}

func isAuthError(err APIError) bool {
	switch err.Code {
	case "auth_required", "token_revoked", "owner_team_lost":
		return true
	default:
		return err.StatusCode == 401
	}
}
