package token

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidateProfileName(t *testing.T) {
	for _, name := range []string{"default", "work", "personal", "test_1", "a-b-c", "default0"} {
		t.Run("valid "+name, func(t *testing.T) {
			if err := ValidateProfileName(name); err != nil {
				t.Fatalf("ValidateProfileName(%q) error = %v", name, err)
			}
		})
	}

	for _, name := range []string{"", "../default", "bad name", "bad/name", `bad\name`, ".hidden", "has.dot", "défault"} {
		t.Run("invalid "+name, func(t *testing.T) {
			if err := ValidateProfileName(name); err == nil {
				t.Fatalf("ValidateProfileName(%q) error = nil, want error", name)
			}
		})
	}
}

func TestProfilePathUsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	got, err := ProfilePath("default")
	if err != nil {
		t.Fatalf("ProfilePath() error = %v", err)
	}

	want := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "disbug", "default.json")
	if got != want {
		t.Fatalf("ProfilePath() = %q, want %q", got, want)
	}
}

func TestWriteReadRoundTripAndOverwrite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	first := Token{
		Token:          "token-1",
		APIURL:         "https://api.example.com",
		AgentName:      "agent",
		Team:           "Team",
		TeamSlug:       "team",
		CreatedByEmail: "user@example.com",
		CreatedAt:      "2026-01-02T03:04:05Z",
	}

	if err := Write("default", first, false); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got, err := Read("default")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got != first {
		t.Fatalf("Read() = %#v, want %#v", got, first)
	}

	if err := Write("default", Token{Token: "conflict"}, false); !errors.Is(err, ErrProfileExists) {
		t.Fatalf("Write() error = %v, want ErrProfileExists", err)
	}

	replacement := Token{Token: "token-2", APIURL: "https://new.example.com"}
	if err := Write("default", replacement, true); err != nil {
		t.Fatalf("Write(force) error = %v", err)
	}

	got, err = Read("default")
	if err != nil {
		t.Fatalf("Read() after force error = %v", err)
	}
	if got != replacement {
		t.Fatalf("Read() after force = %#v, want %#v", got, replacement)
	}
}

func TestWriteFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions are Unix-specific")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := Write("default", Token{Token: "secret"}, false); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	path, err := ProfilePath("default")
	if err != nil {
		t.Fatalf("ProfilePath() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("profile mode = %v, want 0600", got)
	}
}

func TestWriteConfigDirectoryPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions are Unix-specific")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := Write("default", Token{Token: "secret"}, false); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error = %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("config dir mode = %v, want 0700", got)
	}
}

func TestReadMissingProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	_, err := Read("default")
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("Read() error = %v, want ErrProfileNotFound", err)
	}
}

func TestReadEnvOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_TOKEN", "env-token")
	t.Setenv("DISBUG_API_URL", "https://env.example.com")

	got, err := Read("default")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	want := Token{Token: "env-token", APIURL: "https://env.example.com"}
	if got != want {
		t.Fatalf("Read() = %#v, want %#v", got, want)
	}
}

func TestReadEnvOverrideTrimsToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_TOKEN", "  dba_envoverride000000000  ")

	got, err := Read("default")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if got.Token != "dba_envoverride000000000" {
		t.Fatalf("Read().Token = %q, want trimmed token", got.Token)
	}
}

func TestReadWhitespaceOnlyEnvTokenDoesNotOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_TOKEN", "   ")

	_, err := Read("default")
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("Read() error = %v, want ErrProfileNotFound", err)
	}
}

func TestReadEnvOverrideDefaultsAPIURLAndSkipsProfilePathValidation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("DISBUG_TOKEN", "env-token")

	got, err := Read("../bad")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	want := Token{Token: "env-token", APIURL: "https://disbug.io"}
	if got != want {
		t.Fatalf("Read() = %#v, want %#v", got, want)
	}
}

func TestReadRefusesWorldReadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions are Unix-specific")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := Write("default", Token{Token: "secret"}, false); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	path, err := ProfilePath("default")
	if err != nil {
		t.Fatalf("ProfilePath() error = %v", err)
	}
	if err := os.Chmod(path, 0o604); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}

	_, err = Read("default")
	if err == nil {
		t.Fatal("Read() error = nil, want world-readable refusal")
	}
	for _, want := range []string{path, "0604", "chmod 600"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Read() error = %q, want substring %q", err.Error(), want)
		}
	}
}

func TestReadWarnsOnGroupReadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions are Unix-specific")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := Write("default", Token{Token: "secret"}, false); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	path, err := ProfilePath("default")
	if err != nil {
		t.Fatalf("ProfilePath() error = %v", err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}

	stderr := captureStderr(t, func() {
		got, err := Read("default")
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if got.Token != "secret" {
			t.Fatalf("Read().Token = %q, want secret", got.Token)
		}
	})

	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, path) || !strings.Contains(stderr, "0640") {
		t.Fatalf("stderr = %q, want warning with path and mode", stderr)
	}
}

func TestDeleteRemovesProfileAndIsIdempotent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := Write("default", Token{Token: "secret"}, false); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	path, err := ProfilePath("default")
	if err != nil {
		t.Fatalf("ProfilePath() error = %v", err)
	}

	if err := Delete("default"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat() error = %v, want missing file", err)
	}
	if err := Delete("default"); err != nil {
		t.Fatalf("Delete() second call error = %v", err)
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	defer reader.Close()

	os.Stderr = writer
	defer func() {
		os.Stderr = original
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	return string(output)
}
