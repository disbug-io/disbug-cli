package errfmt

import "fmt"

// APIError represents a structured error response from the Disbug API.
type APIError struct {
	StatusCode int
	Code       string
	Detail     string
	RequestID  string
}

func (e APIError) Error() string {
	if e.Detail != "" {
		return e.Detail
	}
	if e.Code != "" && e.StatusCode > 0 {
		return fmt.Sprintf("api error: status %d, code %s", e.StatusCode, e.Code)
	}
	if e.Code != "" {
		return fmt.Sprintf("api error: code %s", e.Code)
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("api error: status %d", e.StatusCode)
	}
	return "api error"
}

// NetworkError represents a failure to reach the Disbug API.
type NetworkError struct {
	URL   string
	Cause error
}

func (e NetworkError) Error() string {
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return "network error"
}

func (e NetworkError) Unwrap() error {
	return e.Cause
}

// UserFacingError carries a message that should be shown to users unchanged.
type UserFacingError struct {
	Message string
	Cause   error
}

func (e UserFacingError) Error() string {
	return e.Message
}

func (e UserFacingError) Unwrap() error {
	return e.Cause
}

// UsageError represents invalid CLI usage.
type UsageError struct {
	Message string
}

func (e UsageError) Error() string {
	return e.Message
}

// NoToken means no usable Disbug token was available.
type NoToken struct{}

func (e NoToken) Error() string {
	return "no token found"
}
