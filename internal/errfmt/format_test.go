package errfmt

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatNoToken(t *testing.T) {
	message := Format(NoToken{})

	assert.Contains(t, message, "No token found. Run: disbug login")
}

func TestFormatAPIErrorAuthRequired(t *testing.T) {
	message := Format(APIError{StatusCode: 401, Code: "auth_required", Detail: "ignored"})

	assert.Contains(t, message, "token")
	assert.Contains(t, message, "Run: disbug login")
}

func TestFormatAPIErrorAuthByStatus(t *testing.T) {
	message := Format(APIError{StatusCode: 401})

	assert.Contains(t, message, "token")
	assert.Contains(t, message, "Run: disbug login")
}

func TestFormatAPIErrorAgentReadOnly(t *testing.T) {
	message := Format(APIError{StatusCode: 403, Code: "agent_read_only"})

	assert.Equal(t, "This operation is denied for agent tokens (read-only).", message)
}

func TestFormatAPIErrorFreeTierLocked(t *testing.T) {
	message := Format(APIError{StatusCode: 403, Code: "free_tier_locked"})

	assert.Contains(t, message, "free-tier")
	assert.Contains(t, message, "https://disbug.io/billing")
}

func TestFormatAPIErrorNotFound(t *testing.T) {
	assert.Equal(t, "Bug not found.", Format(APIError{StatusCode: 404, Code: "not_found", Detail: "Bug not found."}))
	assert.Equal(t, "not found", Format(APIError{StatusCode: 404}))
}

func TestFormatAPIErrorRateLimited(t *testing.T) {
	assert.Contains(t, Format(APIError{StatusCode: 429, Code: "rate_limited"}), "rate-limited")
	assert.Contains(t, Format(APIError{StatusCode: 429}), "rate-limited")
}

func TestFormatAPIErrorServerIncludesRequestID(t *testing.T) {
	assert.Equal(
		t,
		"Server error (503). Request ID: req_123. Try again, or report this ID to support.",
		Format(APIError{StatusCode: 503, RequestID: "req_123"}),
	)
	assert.Equal(
		t,
		"Server error (500). Request ID: unknown. Try again, or report this ID to support.",
		Format(APIError{StatusCode: 500}),
	)
}

func TestFormatNetworkError(t *testing.T) {
	cause := errors.New("dial tcp timeout")

	assert.Equal(
		t,
		"Cannot reach https://api.disbug.io. Check your network. (cause: dial tcp timeout)",
		Format(NetworkError{URL: "https://api.disbug.io", Cause: cause}),
	)
	assert.Equal(
		t,
		"Cannot reach the Disbug API. Check your network. (cause: dial tcp timeout)",
		Format(NetworkError{Cause: cause}),
	)
}

func TestFormatUserFacingError(t *testing.T) {
	assert.Equal(t, "Choose a bug ID.", Format(UserFacingError{Message: "Choose a bug ID.", Cause: errors.New("ignored")}))
}

func TestFormatUsageError(t *testing.T) {
	assert.Equal(t, "usage: disbug open <ref>", Format(UsageError{Message: "usage: disbug open <ref>"}))
}

func TestFormatFallback(t *testing.T) {
	assert.Equal(t, "boom", Format(errors.New("boom")))
	assert.Equal(t, "", Format(nil))
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "nil", err: nil, want: 0},
		{name: "usage", err: UsageError{Message: "bad usage"}, want: 2},
		{name: "no token", err: NoToken{}, want: 4},
		{name: "api 401", err: APIError{StatusCode: 401}, want: 4},
		{name: "auth code with 403", err: APIError{StatusCode: 403, Code: "token_revoked"}, want: 4},
		{name: "auth code with 500", err: APIError{StatusCode: 500, Code: "owner_team_lost"}, want: 4},
		{name: "network", err: NetworkError{URL: "https://api.disbug.io", Cause: errors.New("timeout")}, want: 5},
		{name: "api 404", err: APIError{StatusCode: 404}, want: 6},
		{name: "api 403", err: APIError{StatusCode: 403}, want: 7},
		{name: "api 429", err: APIError{StatusCode: 429}, want: 8},
		{name: "api 500", err: APIError{StatusCode: 500}, want: 9},
		{name: "fallback", err: errors.New("boom"), want: 1},
		{name: "wrapped no token", err: fmt.Errorf("wrap: %w", NoToken{}), want: 4},
		{name: "wrapped auth code", err: fmt.Errorf("wrap: %w", APIError{StatusCode: 403, Code: "auth_required"}), want: 4},
		{name: "wrapped api", err: fmt.Errorf("wrap: %w", APIError{StatusCode: 429}), want: 8},
		{name: "wrapped api pointer", err: fmt.Errorf("wrap: %w", &APIError{StatusCode: 500}), want: 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ExitCode(tt.err))
		})
	}
}

func TestUserFacingErrorUnwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := UserFacingError{Message: "Try again.", Cause: cause}

	require.ErrorIs(t, err, cause)
}
