package cmd

import (
	"errors"
	"testing"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/token"
)

func TestNewAuthenticatedClientMissingTokenProfileReturnsNoToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_TOKEN", "")
	t.Setenv("DISBUG_API_URL", "")

	client, tok, err := newAuthenticatedClient(&RootFlags{Profile: "missing"})
	if !isNoToken(err) {
		t.Fatalf("newAuthenticatedClient() error = %v, want errfmt.NoToken", err)
	}
	if client != nil {
		t.Fatalf("newAuthenticatedClient() client = %#v, want nil", client)
	}
	if tok != (token.Token{}) {
		t.Fatalf("newAuthenticatedClient() token = %#v, want zero token", tok)
	}
}

func TestNewAuthenticatedClientStoredProfileReturnsClientAndToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_TOKEN", "")
	t.Setenv("DISBUG_API_URL", "")

	want := token.Token{
		Token:          "stored-token",
		APIURL:         "https://api.example.com",
		AgentName:      "agent",
		Team:           "Team",
		TeamSlug:       "team",
		CreatedByEmail: "user@example.com",
		CreatedAt:      "2026-01-02T03:04:05Z",
	}
	if err := token.Write("work", want, false); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	client, got, err := newAuthenticatedClient(&RootFlags{Profile: "work"})
	if err != nil {
		t.Fatalf("newAuthenticatedClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("newAuthenticatedClient() client = nil, want client")
	}
	if got != want {
		t.Fatalf("newAuthenticatedClient() token = %#v, want %#v", got, want)
	}
}

func TestNewAuthenticatedClientEmptyStoredTokenReturnsNoToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_TOKEN", "")
	t.Setenv("DISBUG_API_URL", "")

	if err := token.Write("default", token.Token{APIURL: "https://api.example.com"}, false); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	client, tok, err := newAuthenticatedClient(nil)
	if !isNoToken(err) {
		t.Fatalf("newAuthenticatedClient() error = %v, want errfmt.NoToken", err)
	}
	if client != nil {
		t.Fatalf("newAuthenticatedClient() client = %#v, want nil", client)
	}
	if tok.Token != "" {
		t.Fatalf("newAuthenticatedClient() token.Token = %q, want empty", tok.Token)
	}
}

func TestNewAuthenticatedClientEnvironmentTokenOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_TOKEN", "env-token")
	t.Setenv("DISBUG_API_URL", "https://env.example.com")

	client, got, err := newAuthenticatedClient(&RootFlags{Profile: "missing"})
	if err != nil {
		t.Fatalf("newAuthenticatedClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("newAuthenticatedClient() client = nil, want client")
	}

	want := token.Token{Token: "env-token", APIURL: "https://env.example.com"}
	if got != want {
		t.Fatalf("newAuthenticatedClient() token = %#v, want %#v", got, want)
	}
}

func TestNewAuthenticatedClientInvalidProfileNameReturnsNonNoTokenError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_TOKEN", "")
	t.Setenv("DISBUG_API_URL", "")

	client, tok, err := newAuthenticatedClient(&RootFlags{Profile: "../default"})
	if err == nil {
		t.Fatal("newAuthenticatedClient() error = nil, want error")
	}
	if isNoToken(err) {
		t.Fatalf("newAuthenticatedClient() error = %v, want non-NoToken error", err)
	}
	if client != nil {
		t.Fatalf("newAuthenticatedClient() client = %#v, want nil", client)
	}
	if tok != (token.Token{}) {
		t.Fatalf("newAuthenticatedClient() token = %#v, want zero token", tok)
	}
}

func isNoToken(err error) bool {
	var noToken errfmt.NoToken
	return errors.As(err, &noToken)
}
