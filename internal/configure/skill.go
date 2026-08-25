package configure

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

//go:embed assets/using-disbug/SKILL.md
var bundledFiles embed.FS

func (m *Manager) skillState(target string) (string, error) {
	want, err := bundledFiles.ReadFile("assets/using-disbug/SKILL.md")
	if err != nil {
		return "", err
	}
	// target is derived from the current user's home directory and a fixed skill path.
	//nolint:gosec
	got, err := os.ReadFile(target)
	if errors.Is(err, os.ErrNotExist) {
		return "missing", nil
	}
	if err != nil {
		return "", err
	}
	if string(got) == string(want) {
		return "current", nil
	}
	markerPath := filepath.Join(filepath.Dir(target), ".disbug-managed.json")
	// markerPath is adjacent to the fixed skill target under the user's home directory.
	//nolint:gosec
	marker, markerErr := os.ReadFile(markerPath)
	if markerErr == nil {
		var metadata struct {
			ManagedBy string `json:"managed_by"`
		}
		if json.Unmarshal(marker, &metadata) == nil && metadata.ManagedBy == "disbug" {
			return "outdated", nil
		}
	}
	return "conflict", nil
}

func (m *Manager) installSkill(target string) error {
	data, err := bundledFiles.ReadFile("assets/using-disbug/SKILL.md")
	if err != nil {
		return err
	}
	if err := atomicWrite(target, data, 0o644); err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	metadata := map[string]any{
		"managed_by": "disbug",
		"version":    2,
		"sha256":     hex.EncodeToString(sum[:]),
	}
	marker, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(filepath.Dir(target), ".disbug-managed.json"), append(marker, '\n'), 0o644)
}
