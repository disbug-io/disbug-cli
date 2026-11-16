package token

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const defaultAPIURL = "https://app.disbug.io"

// legacyAPIHost is the API host that v0.4.0 and earlier saved in profiles.
// The API now lives at defaultAPIURL.
const legacyAPIHost = "disbug.io"

var (
	// ErrProfileNotFound is returned when a requested profile file does not exist.
	ErrProfileNotFound = errors.New("profile not found")
	// ErrProfileExists is returned when writing an existing profile without force.
	ErrProfileExists = errors.New("profile already exists")
)

// Token stores the persisted credentials and profile metadata.
type Token struct {
	Token          string `json:"token"`
	APIURL         string `json:"api_url"`
	AgentName      string `json:"agent_name"`
	Team           string `json:"team"`
	TeamSlug       string `json:"team_slug"`
	CreatedByEmail string `json:"created_by_email"`
	CreatedAt      string `json:"created_at"`
}

// Read loads a token profile unless DISBUG_TOKEN provides an environment override.
func Read(name string) (Token, error) {
	if envToken := strings.TrimSpace(os.Getenv("DISBUG_TOKEN")); envToken != "" {
		apiURL := os.Getenv("DISBUG_API_URL")
		if apiURL == "" {
			apiURL = defaultAPIURL
		}

		return Token{Token: envToken, APIURL: apiURL}, nil
	}

	path, err := ProfilePath(name)
	if err != nil {
		return Token{}, err
	}

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Token{}, fmt.Errorf("%s: %w", path, ErrProfileNotFound)
		}
		return Token{}, fmt.Errorf("stat token profile %s: %w", path, err)
	}

	if err := checkProfileMode(path, info.Mode().Perm()); err != nil {
		return Token{}, err
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is built from a validated profile name under the config dir.
	if err != nil {
		return Token{}, fmt.Errorf("read token profile %s: %w", path, err)
	}

	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return Token{}, fmt.Errorf("parse token profile %s: %w", path, err)
	}

	if isLegacyAPIURL(token.APIURL) {
		token.APIURL = defaultAPIURL
		if err := replaceProfile(path, token); err != nil {
			fmt.Fprintf(os.Stderr, "warning: using the new API address %s for profile %q, but could not save it: %v\n", defaultAPIURL, name, err)
		} else {
			fmt.Fprintf(os.Stderr, "note: profile %q now uses the new API address %s\n", name, defaultAPIURL)
		}
	}

	if apiURL := os.Getenv("DISBUG_API_URL"); apiURL != "" {
		token.APIURL = apiURL
	}

	return token, nil
}

// Write persists a token profile, refusing to overwrite unless force is true.
func Write(name string, token Token, force bool) error {
	path, err := ProfilePath(name)
	if err != nil {
		return err
	}

	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create token config dir %s: %w", dir, err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // token config directory must be user-only searchable.
			return fmt.Errorf("chmod token config dir %s: %w", dir, err)
		}
	}

	data, err := encodeProfile(path, token)
	if err != nil {
		return err
	}

	if !force {
		return writeNewProfile(path, data)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write token profile %s: %w", path, err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("chmod token profile %s: %w", path, err)
		}
	}

	return nil
}

func encodeProfile(path string, token Token) ([]byte, error) {
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode token profile %s: %w", path, err)
	}
	return append(data, '\n'), nil
}

// isLegacyAPIURL reports whether apiURL is the bare disbug.io address that
// old releases saved, with or without a trailing slash, over http or https.
func isLegacyAPIURL(apiURL string) bool {
	apiURL = strings.ToLower(strings.TrimRight(strings.TrimSpace(apiURL), "/"))
	return apiURL == "https://"+legacyAPIHost || apiURL == "http://"+legacyAPIHost
}

// replaceProfile atomically rewrites an existing profile file with mode 0600,
// so a concurrent reader sees either the old or the new file, never a partial one.
func replaceProfile(path string, token Token) error {
	data, err := encodeProfile(path, token)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary profile: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary profile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary profile: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0o600); err != nil {
			return fmt.Errorf("chmod temporary profile: %w", err)
		}
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace profile: %w", err)
	}

	return nil
}

func writeNewProfile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // path is built from a validated profile name under the config dir.
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s: %w", path, ErrProfileExists)
		}
		return fmt.Errorf("create token profile %s: %w", path, err)
	}

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write token profile %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close token profile %s: %w", path, err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("chmod token profile %s: %w", path, err)
		}
	}

	return nil
}

// Delete removes a token profile and ignores missing files.
func Delete(name string) error {
	path, err := ProfilePath(name)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete token profile %s: %w", path, err)
	}

	return nil
}

func checkProfileMode(path string, mode os.FileMode) error {
	if runtime.GOOS == "windows" {
		return nil
	}

	if mode&0o004 != 0 {
		return fmt.Errorf("token profile %s has insecure mode %04o; run chmod 600 %s", path, mode, path)
	}

	if mode != 0o600 {
		fmt.Fprintf(os.Stderr, "warning: token profile %s has mode %04o; recommended mode is 0600\n", path, mode)
	}

	return nil
}
