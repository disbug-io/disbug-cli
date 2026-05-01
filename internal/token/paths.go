package token

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const appName = "disbug"

// ValidateProfileName returns an error unless name is a safe profile identifier.
func ValidateProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("invalid profile name %q", name)
	}

	for i, r := range name {
		if r > 127 {
			return fmt.Errorf("invalid profile name %q", name)
		}

		isAlphaNum := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
		if i == 0 && !isAlphaNum {
			return fmt.Errorf("invalid profile name %q", name)
		}
		if !isAlphaNum && r != '_' && r != '-' {
			return fmt.Errorf("invalid profile name %q", name)
		}
	}

	return nil
}

// Dir returns the directory used for profile token files.
func Dir() (string, error) {
	if xdgConfigHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, appName), nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}

	return filepath.Join(configDir, appName), nil
}

// ProfilePath returns the JSON file path for a validated profile name.
func ProfilePath(name string) (string, error) {
	if err := ValidateProfileName(name); err != nil {
		return "", err
	}

	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, name+".json"), nil
}
