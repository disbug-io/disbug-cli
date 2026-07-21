package localbundle

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Bundle is a portable local Disbug session downloaded from the Chrome extension.
type Bundle struct {
	Path    string
	hash    string
	report  reportPayload
	fileMap map[string]reportFile
}

type reportPayload struct {
	SchemaVersion int            `json:"schema_version"`
	ExportedAt    string         `json:"exported_at"`
	Manifest      reportManifest `json:"manifest"`
	Session       reportSession  `json:"session"`
	Metadata      map[string]any `json:"metadata"`
	Files         []reportFile   `json:"files"`
}

type reportManifest struct {
	SourceURL        string `json:"source_url"`
	ExtensionVersion string `json:"extension_version"`
	SchemaVersion    int    `json:"schema_version"`
	PinCount         int    `json:"pin_count"`
	TotalSize        int64  `json:"total_size"`
}

type reportSession struct {
	ID        any            `json:"id"`
	Status    string         `json:"status"`
	Project   any            `json:"project"`
	Reporter  any            `json:"reporter"`
	URL       string         `json:"url"`
	UpdatedAt string         `json:"updated_at"`
	Metadata  map[string]any `json:"metadata"`
	Pins      []reportPin    `json:"pins"`
}

type reportPin struct {
	ID          string         `json:"id"`
	Number      int            `json:"number"`
	Feedback    string         `json:"feedback"`
	URL         string         `json:"url"`
	Selector    string         `json:"selector"`
	ElementInfo map[string]any `json:"element_info"`
	Component   any            `json:"component"`
	Position    any            `json:"position"`
	Metadata    map[string]any `json:"metadata"`
}

type reportFile struct {
	Path        string `json:"path"`
	ContentType string `json:"content_type"`
	Encoding    string `json:"encoding"`
	Content     string `json:"content"`
}

type reportLogs struct {
	Console     []map[string]any `json:"console"`
	ConsoleLogs []map[string]any `json:"console_logs"`
	Network     []map[string]any `json:"network"`
	NetworkLogs []map[string]any `json:"network_logs"`
	Events      []map[string]any `json:"events"`
	UserEvents  []map[string]any `json:"user_events"`
}

// Summary is the lightweight output for a local session.
type Summary struct {
	Source  string         `json:"source"`
	Path    string         `json:"path"`
	Session SessionSummary `json:"session"`
	Pins    []PinSummary   `json:"pins"`
}

// SessionSummary is the session-level summary of a local session.
type SessionSummary struct {
	ID        string `json:"id,omitempty"`
	Status    string `json:"status,omitempty"`
	URL       string `json:"url,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	PinCount  int    `json:"pin_count"`
}

// PinSummary is the per-pin summary in a local session.
type PinSummary struct {
	Number    int             `json:"number"`
	Feedback  string          `json:"feedback,omitempty"`
	URL       string          `json:"url,omitempty"`
	Selector  string          `json:"selector,omitempty"`
	Artifacts ArtifactSummary `json:"artifacts"`
}

// ArtifactSummary counts the artifacts captured for a pin.
type ArtifactSummary struct {
	Screenshot   bool `json:"screenshot"`
	Replay       bool `json:"replay"`
	ConsoleCount int  `json:"console_count"`
	NetworkCount int  `json:"network_count"`
	EventCount   int  `json:"event_count"`
}

// PinInspect is the field-selected output for one local session pin.
type PinInspect struct {
	Number        int              `json:"number"`
	Feedback      string           `json:"feedback,omitempty"`
	URL           string           `json:"url,omitempty"`
	Selector      string           `json:"selector,omitempty"`
	ElementInfo   map[string]any   `json:"element_info,omitempty"`
	Metadata      map[string]any   `json:"metadata,omitempty"`
	Artifacts     ArtifactSummary  `json:"artifacts"`
	Screenshot    *LocalAsset      `json:"screenshot,omitempty"`
	SessionReplay *LocalReplayFile `json:"session_replay,omitempty"`
	Console       []map[string]any `json:"console,omitempty"`
	Network       []map[string]any `json:"network,omitempty"`
	Events        []map[string]any `json:"events,omitempty"`
}

// LocalAsset describes a binary artifact (e.g. screenshot) extracted from a local session.
type LocalAsset struct {
	Path        string `json:"path"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes"`
}

// LocalReplayFile describes the rrweb replay artifact extracted from a local session.
type LocalReplayFile struct {
	Path         string `json:"path"`
	ContentType  string `json:"content_type,omitempty"`
	SizeBytes    int64  `json:"size_bytes"`
	EventCount   int    `json:"event_count"`
	RRWebVersion string `json:"rrweb_version,omitempty"`
	StartedAt    string `json:"started_at,omitempty"`
	EndedAt      string `json:"ended_at,omitempty"`
}

// Load parses a portable local Disbug session JSON file.
func Load(reportPath string) (*Bundle, error) {
	data, err := os.ReadFile(reportPath) //nolint:gosec // reportPath is the user-supplied local report file to inspect.
	if err != nil {
		return nil, fmt.Errorf("read local session: %w", err)
	}

	var report reportPayload
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parse local session: %w", err)
	}
	if len(report.Files) == 0 {
		return nil, fmt.Errorf("local session has no files")
	}

	fileMap := make(map[string]reportFile, len(report.Files))
	for _, file := range report.Files {
		clean, err := cleanBundlePath(file.Path)
		if err != nil {
			return nil, err
		}
		file.Path = clean
		fileMap[clean] = file
	}

	if report.Manifest.SourceURL == "" {
		_ = decodeJSONFile(fileMap, "manifest.json", &report.Manifest)
	}
	if len(report.Session.Pins) == 0 {
		_ = decodeJSONFile(fileMap, "session.json", &report.Session)
	}

	sum := sha256.Sum256(data)
	return &Bundle{
		Path:    reportPath,
		hash:    hex.EncodeToString(sum[:])[:16],
		report:  report,
		fileMap: fileMap,
	}, nil
}

// Summary returns the source/session/pin summary for the local session.
func (b *Bundle) Summary() (Summary, error) {
	pins := make([]PinSummary, 0, len(b.report.Session.Pins))
	for _, pin := range b.report.Session.Pins {
		artifacts, err := b.artifactSummary(pin.Number)
		if err != nil {
			return Summary{}, err
		}
		pins = append(pins, PinSummary{
			Number:    pin.Number,
			Feedback:  pin.Feedback,
			URL:       pin.URL,
			Selector:  pin.Selector,
			Artifacts: artifacts,
		})
	}

	pinCount := b.report.Manifest.PinCount
	if pinCount == 0 {
		pinCount = len(pins)
	}
	return Summary{
		Source: "local",
		Path:   b.Path,
		Session: SessionSummary{
			ID:        stringValue(b.report.Session.ID),
			Status:    b.report.Session.Status,
			URL:       firstNonEmpty(b.report.Session.URL, b.report.Manifest.SourceURL),
			UpdatedAt: b.report.Session.UpdatedAt,
			PinCount:  pinCount,
		},
		Pins: pins,
	}, nil
}

// InspectPin returns the field-selected details for one pin in the local session.
func (b *Bundle) InspectPin(number int, fields []string, cacheDir string) (PinInspect, error) {
	pin, ok := b.findPin(number)
	if !ok {
		return PinInspect{}, fmt.Errorf("pin %d not found", number)
	}

	artifacts, err := b.artifactSummary(number)
	if err != nil {
		return PinInspect{}, err
	}
	result := PinInspect{
		Number:      pin.Number,
		Feedback:    pin.Feedback,
		URL:         pin.URL,
		Selector:    pin.Selector,
		ElementInfo: pin.ElementInfo,
		Metadata:    pin.Metadata,
		Artifacts:   artifacts,
	}

	fields = expandFields(fields)
	if len(fields) == 0 {
		return result, nil
	}

	var logs *reportLogs
	for _, field := range fields {
		switch field {
		case "console", "network", "events":
			if logs == nil {
				loaded, err := b.logs(number)
				if err != nil {
					return PinInspect{}, err
				}
				logs = loaded
			}
			switch field {
			case "console":
				result.Console = firstLogSlice(logs.Console, logs.ConsoleLogs)
			case "network":
				result.Network = firstLogSlice(logs.Network, logs.NetworkLogs)
			case "events":
				result.Events = firstLogSlice(logs.Events, logs.UserEvents)
			}
		case "screenshot":
			asset, err := b.extractAsset(number, "screenshot.png", cacheDir)
			if err != nil {
				return PinInspect{}, err
			}
			result.Screenshot = asset
		case "replay":
			replay, err := b.extractReplay(number, cacheDir)
			if err != nil {
				return PinInspect{}, err
			}
			result.SessionReplay = replay
		case "voice_note", "video":
			// Local v1 sessions do not currently include these artifacts.
		}
	}

	return result, nil
}

func (b *Bundle) findPin(number int) (reportPin, bool) {
	for _, pin := range b.report.Session.Pins {
		if pin.Number == number {
			return pin, true
		}
	}
	return reportPin{}, false
}

func (b *Bundle) artifactSummary(number int) (ArtifactSummary, error) {
	logs, err := b.logs(number)
	if err != nil {
		return ArtifactSummary{}, err
	}
	return ArtifactSummary{
		Screenshot:   b.hasFile(number, "screenshot.png"),
		Replay:       b.hasFile(number, "replay.json.gz"),
		ConsoleCount: len(firstLogSlice(logs.Console, logs.ConsoleLogs)),
		NetworkCount: len(firstLogSlice(logs.Network, logs.NetworkLogs)),
		EventCount:   len(firstLogSlice(logs.Events, logs.UserEvents)),
	}, nil
}

func (b *Bundle) logs(number int) (*reportLogs, error) {
	file, ok := b.fileForPin(number, "logs.json")
	if !ok {
		return &reportLogs{}, nil
	}
	var logs reportLogs
	if err := decodeJSONContent(file, &logs); err != nil {
		return nil, err
	}
	return &logs, nil
}

func (b *Bundle) hasFile(number int, name string) bool {
	_, ok := b.fileForPin(number, name)
	return ok
}

func (b *Bundle) fileForPin(number int, name string) (reportFile, bool) {
	file, ok := b.fileMap[fmt.Sprintf("pin_%d/%s", number, name)]
	if ok {
		return file, true
	}
	file, ok = b.fileMap[fmt.Sprintf("pins/%d/%s", number, name)]
	return file, ok
}

func (b *Bundle) extractAsset(number int, name string, cacheDir string) (*LocalAsset, error) {
	file, ok := b.fileForPin(number, name)
	if !ok {
		return nil, nil
	}
	data, err := file.bytes()
	if err != nil {
		return nil, err
	}
	path, err := b.writeArtifact(number, name, data, cacheDir)
	if err != nil {
		return nil, err
	}
	return &LocalAsset{
		Path:        path,
		ContentType: file.ContentType,
		SizeBytes:   int64(len(data)),
	}, nil
}

func (b *Bundle) extractReplay(number int, cacheDir string) (*LocalReplayFile, error) {
	file, ok := b.fileForPin(number, "replay.json.gz")
	if !ok {
		return nil, nil
	}
	data, err := file.bytes()
	if err != nil {
		return nil, err
	}
	path, err := b.writeArtifact(number, "replay.json.gz", data, cacheDir)
	if err != nil {
		return nil, err
	}
	meta := replayMeta(data)
	return &LocalReplayFile{
		Path:         path,
		ContentType:  file.ContentType,
		SizeBytes:    int64(len(data)),
		EventCount:   meta.EventCount,
		RRWebVersion: meta.RRWebVersion,
		StartedAt:    meta.StartedAt,
		EndedAt:      meta.EndedAt,
	}, nil
}

func (b *Bundle) writeArtifact(number int, name string, data []byte, cacheDir string) (string, error) {
	root := cacheDir
	if root == "" {
		defaultRoot, err := defaultCacheDir()
		if err != nil {
			return "", err
		}
		root = defaultRoot
	}
	destDir := filepath.Join(root, b.hash, fmt.Sprintf("pin_%d", number))
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return "", fmt.Errorf("create artifact cache: %w", err)
	}
	dest := filepath.Join(destDir, name)
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		return "", fmt.Errorf("write artifact cache: %w", err)
	}
	return dest, nil
}

func defaultCacheDir() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache dir: %w", err)
	}
	return filepath.Join(root, "disbug", "local-reports"), nil
}

func decodeJSONFile(fileMap map[string]reportFile, path string, dest any) error {
	file, ok := fileMap[path]
	if !ok {
		return fmt.Errorf("missing %s", path)
	}
	return decodeJSONContent(file, dest)
}

func decodeJSONContent(file reportFile, dest any) error {
	data, err := file.bytes()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("parse %s: %w", file.Path, err)
	}
	return nil
}

func (f reportFile) bytes() ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(f.Encoding)) {
	case "", "utf-8", "utf8":
		return []byte(f.Content), nil
	case "base64":
		data, err := base64.StdEncoding.DecodeString(f.Content)
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", f.Path, err)
		}
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported encoding %q for %s", f.Encoding, f.Path)
	}
}

type replayMetadata struct {
	EventCount   int
	RRWebVersion string
	StartedAt    string
	EndedAt      string
}

func replayMeta(data []byte) replayMetadata {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return replayMetadata{}
	}
	defer func() { _ = reader.Close() }()

	body, err := io.ReadAll(reader)
	if err != nil {
		return replayMetadata{}
	}
	var envelope struct {
		RRWebVersion string            `json:"rrweb_version"`
		StartedAt    string            `json:"started_at"`
		EndedAt      string            `json:"ended_at"`
		Events       []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return replayMetadata{}
	}
	return replayMetadata{
		EventCount:   len(envelope.Events),
		RRWebVersion: envelope.RRWebVersion,
		StartedAt:    envelope.StartedAt,
		EndedAt:      envelope.EndedAt,
	}
}

func cleanBundlePath(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("local session contains a file with no path")
	}
	clean := path.Clean(strings.TrimPrefix(value, "/"))
	if clean == "." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("unsafe local session path %q", raw)
	}
	return clean, nil
}

func expandFields(fields []string) []string {
	if len(fields) == 1 && fields[0] == "all" {
		return []string{"screenshot", "console", "network", "events", "replay", "voice_note", "video"}
	}
	return fields
}

func firstLogSlice(primary, fallback []map[string]any) []map[string]any {
	if primary != nil {
		return primary
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return fmt.Sprintf("%.0f", typed)
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}
