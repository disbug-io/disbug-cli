package token

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
)

const defaultAPIURL = "https://disbug.io"

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
	AgentName      string `json:"agent_name,omitempty"`
	Team           string `json:"team,omitempty"`
	TeamSlug       string `json:"team_slug,omitempty"`
	CreatedByEmail string `json:"created_by_email,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
}

// Read loads a token profile unless DISBUG_TOKEN provides an environment override.
func Read(name string) (Token, error) {
	if envToken := os.Getenv("DISBUG_TOKEN"); envToken != "" {
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

	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s: %w", path, ErrProfileExists)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat token profile %s: %w", path, err)
		}
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

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("encode token profile %s: %w", path, err)
	}
	data = append(data, '\n')

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
