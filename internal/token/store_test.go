package token

import (
	"encoding/json"
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

func TestProfilePathWhitespaceXDGConfigHomeFallsBackToUserConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "   ")

	var wantRoot string
	switch runtime.GOOS {
	case "windows":
		t.Setenv("APPDATA", home)
		wantRoot = home
	case "darwin":
		t.Setenv("HOME", home)
		wantRoot = filepath.Join(home, "Library", "Application Support")
	default:
		t.Setenv("HOME", home)
		wantRoot = filepath.Join(home, ".config")
	}

	got, err := ProfilePath("default")
	if err != nil {
		t.Fatalf("ProfilePath() error = %v", err)
	}

	whitespacePath := filepath.Join("   ", "disbug", "default.json")
	if got == whitespacePath {
		t.Fatalf("ProfilePath() = %q, should not use whitespace XDG_CONFIG_HOME", got)
	}

	want := filepath.Join(wantRoot, "disbug", "default.json")
	if got != want {
		t.Fatalf("ProfilePath() = %q, want %q", got, want)
	}
}

func TestDefaultConfigDirForPlatform(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		home    string
		appdata string
		want    string
	}{
		{name: "linux", goos: "linux", home: "/home/test", want: filepath.Join("/home/test", ".config")},
		{name: "darwin", goos: "darwin", home: "/Users/test", want: filepath.Join("/Users/test", "Library", "Application Support")},
		{name: "windows", goos: "windows", appdata: `C:\Users\test\AppData\Roaming`, want: `C:\Users\test\AppData\Roaming`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := defaultConfigDir(tt.goos, tt.home, tt.appdata)
			if err != nil {
				t.Fatalf("defaultConfigDir() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("defaultConfigDir() = %q, want %q", got, tt.want)
			}
		})
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

	got, err = Read("default")
	if err != nil {
		t.Fatalf("Read() after conflict error = %v", err)
	}
	if got != first {
		t.Fatalf("Read() after conflict = %#v, want %#v", got, first)
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

func TestReadAppliesAPIURLEnvOverrideToStoredProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_API_URL", "https://env.example.com")

	if err := Write("default", Token{Token: "stored-token", APIURL: "https://stored.example.com"}, false); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got, err := Read("default")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if got.APIURL != "https://env.example.com" {
		t.Fatalf("Read().APIURL = %q, want env override", got.APIURL)
	}
}

func TestWritePersistsExpectedJSONKeys(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	token := Token{
		Token:          "token-1",
		APIURL:         "https://api.example.com",
		AgentName:      "agent",
		Team:           "Team",
		TeamSlug:       "team",
		CreatedByEmail: "user@example.com",
		CreatedAt:      "2026-01-02T03:04:05Z",
	}
	if err := Write("default", token, false); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	path, err := ProfilePath("default")
	if err != nil {
		t.Fatalf("ProfilePath() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	want := map[string]any{
		"token":            "token-1",
		"api_url":          "https://api.example.com",
		"agent_name":       "agent",
		"team":             "Team",
		"team_slug":        "team",
		"created_by_email": "user@example.com",
		"created_at":       "2026-01-02T03:04:05Z",
	}
	if len(got) != len(want) {
		t.Fatalf("persisted keys = %#v, want %#v", got, want)
	}
	for key, wantValue := range want {
		if got[key] != wantValue {
			t.Fatalf("persisted %q = %#v, want %#v", key, got[key], wantValue)
		}
	}
}

func TestWritePersistsCompleteJSONShapeForMinimalToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := Write("default", Token{Token: "token-1", APIURL: "https://api.example.com"}, false); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	path, err := ProfilePath("default")
	if err != nil {
		t.Fatalf("ProfilePath() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	want := map[string]any{
		"token":            "token-1",
		"api_url":          "https://api.example.com",
		"agent_name":       "",
		"team":             "",
		"team_slug":        "",
		"created_by_email": "",
		"created_at":       "",
	}
	if len(got) != len(want) {
		t.Fatalf("persisted keys = %#v, want %#v", got, want)
	}
	for key, wantValue := range want {
		if got[key] != wantValue {
			t.Fatalf("persisted %q = %#v, want %#v", key, got[key], wantValue)
		}
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

	want := Token{Token: "env-token", APIURL: "https://app.disbug.io"}
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

func TestReadMigratesLegacyAPIURL(t *testing.T) {
	for _, legacy := range []string{"https://disbug.io", "https://disbug.io/", "http://disbug.io", "http://disbug.io/", " HTTPS://Disbug.io// "} {
		t.Run(legacy, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("DISBUG_TOKEN", "")
			t.Setenv("DISBUG_API_URL", "")

			stored := Token{
				Token:          "secret",
				APIURL:         legacy,
				AgentName:      "agent",
				Team:           "Team",
				TeamSlug:       "team",
				CreatedByEmail: "user@example.com",
				CreatedAt:      "2026-01-02T03:04:05Z",
			}
			if err := Write("default", stored, false); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			want := stored
			want.APIURL = "https://app.disbug.io"

			var got Token
			stdout, stderr := captureOutput(t, func() {
				var err error
				got, err = Read("default")
				if err != nil {
					t.Fatalf("Read() error = %v", err)
				}
			})
			if got != want {
				t.Fatalf("Read() = %#v, want %#v", got, want)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want nothing", stdout)
			}
			if strings.Count(stderr, "\n") != 1 || !strings.Contains(stderr, "https://app.disbug.io") {
				t.Fatalf("stderr = %q, want one line naming the new API address", stderr)
			}

			path, err := ProfilePath("default")
			if err != nil {
				t.Fatalf("ProfilePath() error = %v", err)
			}
			if saved := readProfileFile(t, path); saved != want {
				t.Fatalf("saved profile = %#v, want %#v", saved, want)
			}
			if runtime.GOOS != "windows" {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatalf("Stat() error = %v", err)
				}
				if mode := info.Mode().Perm(); mode != 0o600 {
					t.Fatalf("profile mode = %v, want 0600", mode)
				}
			}
			entries, err := os.ReadDir(filepath.Dir(path))
			if err != nil {
				t.Fatalf("ReadDir() error = %v", err)
			}
			if len(entries) != 1 {
				t.Fatalf("config dir has %d entries, want only the profile (no temporary files)", len(entries))
			}

			stdout, stderr = captureOutput(t, func() {
				if _, err := Read("default"); err != nil {
					t.Fatalf("second Read() error = %v", err)
				}
			})
			if stdout != "" || stderr != "" {
				t.Fatalf("second Read() printed stdout=%q stderr=%q, want nothing", stdout, stderr)
			}
		})
	}
}

func TestReadKeepsOtherAPIURLs(t *testing.T) {
	for _, apiURL := range []string{"", "https://app.disbug.io", "https://disbug.io/api", "https://staging.disbug.io", "https://disbug.io.example.com", "http://localhost:8000"} {
		t.Run(apiURL, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("DISBUG_TOKEN", "")
			t.Setenv("DISBUG_API_URL", "")

			if err := Write("default", Token{Token: "secret", APIURL: apiURL}, false); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			path, err := ProfilePath("default")
			if err != nil {
				t.Fatalf("ProfilePath() error = %v", err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}

			var got Token
			stdout, stderr := captureOutput(t, func() {
				got, err = Read("default")
			})
			if err != nil {
				t.Fatalf("Read() error = %v", err)
			}
			if got.APIURL != apiURL {
				t.Fatalf("Read().APIURL = %q, want %q", got.APIURL, apiURL)
			}
			if stdout != "" || stderr != "" {
				t.Fatalf("Read() printed stdout=%q stderr=%q, want nothing", stdout, stderr)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			if string(after) != string(before) {
				t.Fatalf("profile changed from %q to %q", before, after)
			}
		})
	}
}

func TestReadMigratesLegacyAPIURLUnderEnvOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_TOKEN", "")
	t.Setenv("DISBUG_API_URL", "http://localhost:8000")

	if err := Write("default", Token{Token: "secret", APIURL: "https://disbug.io"}, false); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	var got Token
	captureOutput(t, func() {
		var err error
		got, err = Read("default")
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
	})
	if got.APIURL != "http://localhost:8000" {
		t.Fatalf("Read().APIURL = %q, want the DISBUG_API_URL override", got.APIURL)
	}

	path, err := ProfilePath("default")
	if err != nil {
		t.Fatalf("ProfilePath() error = %v", err)
	}
	if saved := readProfileFile(t, path); saved.APIURL != "https://app.disbug.io" {
		t.Fatalf("saved APIURL = %q, want https://app.disbug.io", saved.APIURL)
	}
}

func TestReadUsesNewAPIURLWhenProfileCannotBeRewritten(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions are Unix-specific")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_TOKEN", "")
	t.Setenv("DISBUG_API_URL", "")

	if err := Write("default", Token{Token: "secret", APIURL: "https://disbug.io"}, false); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error = %v", err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	var got Token
	stdout, stderr := captureOutput(t, func() {
		got, err = Read("default")
	})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got.APIURL != "https://app.disbug.io" {
		t.Fatalf("Read().APIURL = %q, want https://app.disbug.io", got.APIURL)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want nothing", stdout)
	}
	if strings.Count(stderr, "\n") != 1 || !strings.Contains(stderr, "warning:") {
		t.Fatalf("stderr = %q, want one warning line", stderr)
	}

	path, err := ProfilePath("default")
	if err != nil {
		t.Fatalf("ProfilePath() error = %v", err)
	}
	if saved := readProfileFile(t, path); saved.APIURL != "https://disbug.io" {
		t.Fatalf("saved APIURL = %q, want the original value left in place", saved.APIURL)
	}
}

func readProfileFile(t *testing.T, path string) Token {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	return token
}

func captureOutput(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	stderr = captureStderr(t, func() {
		stdout = captureFile(t, &os.Stdout, fn)
	})
	return stdout, stderr
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	return captureFile(t, &os.Stderr, fn)
}

func captureFile(t *testing.T, target **os.File, fn func()) string {
	t.Helper()

	original := *target
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	defer reader.Close()

	*target = writer
	defer func() {
		*target = original
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
