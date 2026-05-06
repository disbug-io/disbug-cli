package localstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	schemaVersion      = 1
	maxReportBytes     = 128 * 1024 * 1024
	maxFileBytes       = 32 * 1024 * 1024
	defaultListLimit   = 50
	maxListLimit       = 100
	localStoreOverride = "DISBUG_LOCAL_STORE_DIR"
)

// Store is the local report store used by native-host and MCP.
type Store struct {
	root string
	db   *sql.DB
}

// ReportMetadata is local-only report metadata supplied by the extension.
type ReportMetadata struct {
	URL              string `json:"url,omitempty"`
	CreatedAt        string `json:"created_at,omitempty"`
	ExtensionVersion string `json:"extension_version,omitempty"`
	SchemaVersion    int    `json:"schema_version,omitempty"`
	TotalSize        int64  `json:"total_size,omitempty"`
	FirstPinFeedback string `json:"first_pin_feedback,omitempty"`
	FirstPinURL      string `json:"first_pin_url,omitempty"`
}

// Pragmas reports the index SQLite runtime settings.
type Pragmas struct {
	JournalMode string
	Synchronous string
}

// Report is an in-progress local report written under a temporary directory.
type Report struct {
	store     *Store
	id        string
	tempPath  string
	metadata  ReportMetadata
	fileSizes map[string]int64
	totalSize int64
	committed bool
}

// CommittedReport describes a report after it has been indexed.
type CommittedReport struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Prompt    string `json:"prompt"`
	CreatedAt string `json:"created_at"`
}

// ListOptions configures local session listing.
type ListOptions struct {
	Limit int
	Query string
}

// ListResponse mirrors the cloud list response shape with local string ids.
type ListResponse struct {
	Results    []SessionSummary `json:"results"`
	NextCursor *string          `json:"next_cursor"`
	Count      int              `json:"count"`
}

// SessionSummary is a compact local session row.
type SessionSummary struct {
	ID               string `json:"id"`
	URL              string `json:"url"`
	Status           string `json:"status"`
	PinCount         int    `json:"pin_count"`
	FirstPinFeedback string `json:"first_pin_feedback"`
	UpdatedAt        string `json:"updated_at"`
	CreatedAt        string `json:"created_at"`
	ReportPath       string `json:"report_path"`
	TotalSize        int64  `json:"total_size"`
}

// PinSearchHit is a local pin search result with the parent session summary.
type PinSearchHit struct {
	Pin     map[string]any `json:"pin"`
	Session SessionSummary `json:"session"`
}

// PinSearchResponse is the local search_pins response.
type PinSearchResponse struct {
	Results []PinSearchHit `json:"results"`
	Total   int            `json:"total"`
}

// PruneOptions configures local retention cleanup.
type PruneOptions struct {
	OlderThan time.Duration
	Now       time.Time
}

// Open opens or initializes the local report store.
func Open(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		resolved, err := DefaultRoot()
		if err != nil {
			return nil, err
		}
		root = resolved
	}
	root = filepath.Clean(root)
	for _, dir := range []string{root, filepath.Join(root, "sessions"), filepath.Join(root, ".tmp")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create local store dir %s: %w", dir, err)
		}
		_ = os.Chmod(dir, 0o700)
	}

	db, err := sql.Open("sqlite", filepath.Join(root, "index.sqlite"))
	if err != nil {
		return nil, fmt.Errorf("open local index: %w", err)
	}
	store := &Store{root: root, db: db}
	if err := store.configure(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.cleanupTemps(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// DefaultRoot returns the platform data directory for local sessions.
func DefaultRoot() (string, error) {
	if override := strings.TrimSpace(os.Getenv(localStoreOverride)); override != "" {
		return filepath.Clean(override), nil
	}
	return defaultRootFor(runtime.GOOS, os.Getenv("HOME"), os.Getenv("XDG_DATA_HOME"), os.Getenv("LOCALAPPDATA"))
}

func defaultRootFor(goos, home, xdgDataHome, localAppData string) (string, error) {
	switch goos {
	case "darwin":
		if home == "" {
			return "", errors.New("HOME is not defined")
		}
		return filepath.Join(home, "Library", "Application Support", "disbug", "local-sessions"), nil
	case "windows":
		if localAppData == "" {
			return "", errors.New("LOCALAPPDATA is not defined")
		}
		return filepath.Join(localAppData, "disbug", "local-sessions"), nil
	default:
		if strings.TrimSpace(xdgDataHome) != "" {
			return filepath.Join(xdgDataHome, "disbug", "local-sessions"), nil
		}
		if home == "" {
			return "", errors.New("HOME is not defined")
		}
		return filepath.Join(home, ".local", "share", "disbug", "local-sessions"), nil
	}
}

func (s *Store) configure(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "PRAGMA journal_mode=WAL;"); err != nil {
		return fmt.Errorf("enable local index WAL: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "PRAGMA synchronous=NORMAL;"); err != nil {
		return fmt.Errorf("set local index synchronous=NORMAL: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  source_url TEXT NOT NULL DEFAULT '',
  extension_version TEXT NOT NULL DEFAULT '',
  schema_version INTEGER NOT NULL DEFAULT 1,
  report_path TEXT NOT NULL,
  total_size INTEGER NOT NULL DEFAULT 0,
  commit_status TEXT NOT NULL DEFAULT 'committed',
  first_pin_feedback TEXT NOT NULL DEFAULT '',
  first_pin_url TEXT NOT NULL DEFAULT '',
  pin_count INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS pins (
  session_id TEXT NOT NULL,
  pin_number INTEGER NOT NULL,
  pin_path TEXT NOT NULL,
  feedback TEXT NOT NULL DEFAULT '',
  url TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (session_id, pin_number),
  FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS sessions_created_at_idx ON sessions(created_at DESC);
CREATE INDEX IF NOT EXISTS pins_feedback_idx ON pins(feedback);
`); err != nil {
		return fmt.Errorf("migrate local index: %w", err)
	}
	return nil
}

// Close closes the local index.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Root returns the local store root.
func (s *Store) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

// Pragmas returns the SQLite journal and synchronous settings.
func (s *Store) Pragmas(ctx context.Context) (Pragmas, error) {
	var journal string
	if err := s.db.QueryRowContext(ctx, "PRAGMA journal_mode;").Scan(&journal); err != nil {
		return Pragmas{}, err
	}
	var syncValue int
	if err := s.db.QueryRowContext(ctx, "PRAGMA synchronous;").Scan(&syncValue); err != nil {
		return Pragmas{}, err
	}
	return Pragmas{JournalMode: journal, Synchronous: synchronousName(syncValue)}, nil
}

func synchronousName(value int) string {
	switch value {
	case 0:
		return "OFF"
	case 1:
		return "NORMAL"
	case 2:
		return "FULL"
	case 3:
		return "EXTRA"
	default:
		return strconv.Itoa(value)
	}
}

// BeginReport creates a new temporary report.
func (s *Store) BeginReport(ctx context.Context, metadata ReportMetadata) (*Report, error) {
	if s == nil {
		return nil, errors.New("local store is not configured")
	}
	if metadata.SchemaVersion == 0 {
		metadata.SchemaVersion = schemaVersion
	}
	if metadata.CreatedAt == "" {
		metadata.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if metadata.TotalSize < 0 || metadata.TotalSize > maxReportBytes {
		return nil, fmt.Errorf("report total size exceeds %d bytes", maxReportBytes)
	}

	id, err := newReportID(metadata.CreatedAt)
	if err != nil {
		return nil, err
	}
	tempPath := filepath.Join(s.root, ".tmp", id)
	if err := os.MkdirAll(tempPath, 0o700); err != nil {
		return nil, fmt.Errorf("create temp report dir: %w", err)
	}
	return &Report{
		store:     s,
		id:        id,
		tempPath:  tempPath,
		metadata:  metadata,
		fileSizes: map[string]int64{},
	}, nil
}

func newReportID(createdAt string) (string, error) {
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		t = time.Now().UTC()
	}
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate local report id: %w", err)
	}
	return "local_" + t.UTC().Format("20060102_150405") + "_" + hex.EncodeToString(random[:]), nil
}

// ValidateRelativePath rejects absolute paths, traversal, and empty path segments.
func ValidateRelativePath(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" {
		return "", errors.New("path is required")
	}
	if path.IsAbs(raw) || filepath.IsAbs(raw) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", raw)
	}
	clean := path.Clean(raw)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("path traversal is not allowed: %s", raw)
	}
	for _, segment := range strings.Split(clean, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("invalid path segment in %s", raw)
		}
	}
	return clean, nil
}

// WriteFile writes a complete file under the report temp directory.
func (r *Report) WriteFile(relPath string, _ string, data []byte) error {
	if r == nil {
		return errors.New("report is not initialized")
	}
	return r.writeFile(relPath, data, false)
}

// AppendFile appends chunk data under the report temp directory.
func (r *Report) AppendFile(relPath string, _ string, data []byte) error {
	if r == nil {
		return errors.New("report is not initialized")
	}
	return r.writeFile(relPath, data, true)
}

func (r *Report) writeFile(relPath string, data []byte, appendData bool) error {
	if r.committed {
		return errors.New("report is already committed")
	}
	clean, err := ValidateRelativePath(relPath)
	if err != nil {
		return err
	}
	newFileSize := int64(len(data))
	if appendData {
		newFileSize += r.fileSizes[clean]
	}
	if newFileSize > maxFileBytes {
		return fmt.Errorf("file %s exceeds %d bytes", clean, maxFileBytes)
	}
	nextTotal := r.totalSize + int64(len(data))
	if nextTotal > maxReportBytes {
		return fmt.Errorf("report exceeds %d bytes", maxReportBytes)
	}
	dest := filepath.Join(r.tempPath, filepath.FromSlash(clean))
	if !strings.HasPrefix(dest, r.tempPath+string(filepath.Separator)) {
		return fmt.Errorf("invalid report path %s", clean)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	flag := os.O_WRONLY | os.O_CREATE
	if appendData {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(dest, flag, 0o600) //nolint:gosec // path is validated under temp report dir.
	if err != nil {
		return fmt.Errorf("open report file %s: %w", clean, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write report file %s: %w", clean, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close report file %s: %w", clean, err)
	}
	r.fileSizes[clean] = newFileSize
	r.totalSize = nextTotal
	return nil
}

// Commit atomically moves the temp report into the sessions directory and indexes it.
func (r *Report) Commit(ctx context.Context) (*CommittedReport, error) {
	if r == nil || r.store == nil {
		return nil, errors.New("report is not initialized")
	}
	if r.committed {
		return nil, errors.New("report is already committed")
	}
	sessionPath := filepath.Join(r.tempPath, "session.json")
	if _, err := os.Stat(sessionPath); err != nil {
		return nil, fmt.Errorf("session.json is required: %w", err)
	}

	session, err := readJSONFile(sessionPath)
	if err != nil {
		return nil, fmt.Errorf("read session.json: %w", err)
	}
	session["id"] = r.id
	if session["url"] == nil || session["url"] == "" {
		session["url"] = r.metadata.URL
	}
	if session["status"] == nil || session["status"] == "" {
		session["status"] = "open"
	}
	if session["updated_at"] == nil || session["updated_at"] == "" {
		session["updated_at"] = r.metadata.CreatedAt
	}
	if err := writeJSONFile(sessionPath, session); err != nil {
		return nil, fmt.Errorf("write session.json: %w", err)
	}

	summary, pins := reportSummary(r.id, r.tempPath, r.metadata, session, r.totalSize)
	if err := r.writeManifest(summary); err != nil {
		return nil, err
	}

	finalPath := filepath.Join(r.store.root, "sessions", r.id)
	if err := os.Rename(r.tempPath, finalPath); err != nil {
		return nil, fmt.Errorf("commit report dir: %w", err)
	}
	summary.ReportPath = finalPath

	if err := r.store.insertCommitted(ctx, summary, pins); err != nil {
		_ = os.RemoveAll(finalPath)
		return nil, err
	}
	r.committed = true

	prompt := BuildPrompt(summary.ID, summary.ReportPath, summary.FirstPinFeedback, firstPinURL(pins, summary.URL))
	return &CommittedReport{
		ID:        summary.ID,
		Path:      summary.ReportPath,
		Prompt:    prompt,
		CreatedAt: summary.CreatedAt,
	}, nil
}

func (r *Report) writeManifest(summary SessionSummary) error {
	manifest := map[string]any{
		"report_id":         r.id,
		"created_at":        summary.CreatedAt,
		"source_url":        summary.URL,
		"extension_version": r.metadata.ExtensionVersion,
		"schema_version":    r.metadata.SchemaVersion,
		"total_size":        summary.TotalSize,
		"pin_count":         summary.PinCount,
	}
	return writeJSONFile(filepath.Join(r.tempPath, "manifest.json"), manifest)
}

func reportSummary(
	id string,
	basePath string,
	meta ReportMetadata,
	session map[string]any,
	totalSize int64,
) (SessionSummary, []pinIndex) {
	pins := readPins(basePath)
	firstFeedback := meta.FirstPinFeedback
	if firstFeedback == "" && len(pins) > 0 {
		firstFeedback = pins[0].Feedback
	}
	sourceURL := meta.URL
	if sourceURL == "" {
		sourceURL, _ = session["url"].(string)
	}
	updatedAt, _ := session["updated_at"].(string)
	if updatedAt == "" {
		updatedAt = meta.CreatedAt
	}
	status, _ := session["status"].(string)
	if status == "" {
		status = "open"
	}
	return SessionSummary{
		ID:               id,
		URL:              sourceURL,
		Status:           status,
		PinCount:         len(pins),
		FirstPinFeedback: firstFeedback,
		UpdatedAt:        updatedAt,
		CreatedAt:        meta.CreatedAt,
		TotalSize:        totalSize,
	}, pins
}

type pinIndex struct {
	Number   int
	Path     string
	Feedback string
	URL      string
}

func readPins(basePath string) []pinIndex {
	matches, _ := filepath.Glob(filepath.Join(basePath, "pin_*", "pin.json"))
	pins := make([]pinIndex, 0, len(matches))
	for _, match := range matches {
		pin, err := readJSONFile(match)
		if err != nil {
			continue
		}
		number := int(numberFromAny(pin["number"]))
		if number <= 0 {
			number = pinNumberFromDir(filepath.Base(filepath.Dir(match)))
		}
		feedback, _ := pin["feedback"].(string)
		pinURL, _ := pin["url"].(string)
		pins = append(pins, pinIndex{
			Number:   number,
			Path:     filepath.ToSlash(strings.TrimPrefix(match, basePath+string(filepath.Separator))),
			Feedback: feedback,
			URL:      pinURL,
		})
	}
	sort.Slice(pins, func(i, j int) bool {
		return pins[i].Number < pins[j].Number
	})
	return pins
}

func pinNumberFromDir(dir string) int {
	value, _ := strconv.Atoi(strings.TrimPrefix(dir, "pin_"))
	return value
}

func numberFromAny(v any) int64 {
	switch typed := v.(type) {
	case float64:
		return int64(typed)
	case int:
		return int64(typed)
	case int64:
		return typed
	case json.Number:
		value, _ := typed.Int64()
		return value
	default:
		return 0
	}
}

func (s *Store) insertCommitted(ctx context.Context, summary SessionSummary, pins []pinIndex) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if _, err := tx.ExecContext(ctx, `INSERT INTO sessions (
id, created_at, updated_at, source_url, extension_version, schema_version, report_path, total_size,
commit_status, first_pin_feedback, first_pin_url, pin_count
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'committed', ?, ?, ?)`,
		summary.ID,
		summary.CreatedAt,
		summary.UpdatedAt,
		summary.URL,
		"",
		schemaVersion,
		summary.ReportPath,
		summary.TotalSize,
		summary.FirstPinFeedback,
		firstPinURL(pins, summary.URL),
		summary.PinCount,
	); err != nil {
		return fmt.Errorf("insert local session: %w", err)
	}
	for _, pin := range pins {
		if _, err := tx.ExecContext(ctx, `INSERT INTO pins (
session_id, pin_number, pin_path, feedback, url
) VALUES (?, ?, ?, ?, ?)`,
			summary.ID,
			pin.Number,
			pin.Path,
			pin.Feedback,
			pin.URL,
		); err != nil {
			return fmt.Errorf("insert local pin: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit local index: %w", err)
	}
	return nil
}

func firstPinURL(pins []pinIndex, fallback string) string {
	if len(pins) > 0 && pins[0].URL != "" {
		return pins[0].URL
	}
	return fallback
}

// BuildPrompt returns the host-owned clipboard prompt.
func BuildPrompt(id, reportPath, firstPinFeedback, firstPinURL string) string {
	return fmt.Sprintf(`Debug a Disbug bug report saved locally on this machine.
ID: %s
Path: %s
First pin: "%s"
URL: %s
If you have the Disbug MCP installed, call: get_session(id="%s", source="local")
Otherwise, read manifest.json and pin_*/pin.json from the path above.`,
		id,
		reportPath,
		truncate(firstPinFeedback, 140),
		firstPinURL,
		id,
	)
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return strings.TrimSpace(value[:max-3]) + "..."
}

// ListSessions returns newest committed local sessions first.
func (s *Store) ListSessions(ctx context.Context, opts ListOptions) (ListResponse, error) {
	limit := clampLimit(opts.Limit)
	query := strings.TrimSpace(strings.ToLower(opts.Query))
	rows, err := s.db.QueryContext(ctx, `SELECT id, created_at, updated_at, source_url, commit_status,
pin_count, first_pin_feedback, report_path, total_size
FROM sessions
WHERE commit_status = 'committed'
ORDER BY created_at DESC
LIMIT ?`, limit)
	if err != nil {
		return ListResponse{}, err
	}
	defer rows.Close()

	var results []SessionSummary
	for rows.Next() {
		var item SessionSummary
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.URL,
			&item.Status,
			&item.PinCount,
			&item.FirstPinFeedback,
			&item.ReportPath,
			&item.TotalSize,
		); err != nil {
			return ListResponse{}, err
		}
		item.Status = "open"
		if query == "" || strings.Contains(strings.ToLower(item.ID+" "+item.URL+" "+item.FirstPinFeedback), query) {
			results = append(results, item)
		}
	}
	if err := rows.Err(); err != nil {
		return ListResponse{}, err
	}
	return ListResponse{Results: results, Count: len(results)}, nil
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultListLimit
	}
	if limit > maxListLimit {
		return maxListLimit
	}
	return limit
}

// SearchSessions searches local sessions by URL, id, and first pin feedback.
func (s *Store) SearchSessions(ctx context.Context, query string, limit int) (ListResponse, error) {
	return s.ListSessions(ctx, ListOptions{Limit: limit, Query: query})
}

// SearchPins searches local pin feedback and URLs.
func (s *Store) SearchPins(ctx context.Context, query string, limit int) (PinSearchResponse, error) {
	list, err := s.ListSessions(ctx, ListOptions{Limit: maxListLimit})
	if err != nil {
		return PinSearchResponse{}, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	limit = clampLimit(limit)
	var hits []PinSearchHit
	for _, session := range list.Results {
		pins, err := s.pinsForSession(ctx, session.ID)
		if err != nil {
			return PinSearchResponse{}, err
		}
		for _, pin := range pins {
			if query != "" && !strings.Contains(strings.ToLower(pin.Feedback+" "+pin.URL), query) {
				continue
			}
			full, err := s.GetPin(ctx, session.ID, pin.Number, nil)
			if err != nil {
				return PinSearchResponse{}, err
			}
			hits = append(hits, PinSearchHit{Pin: full, Session: session})
			if len(hits) >= limit {
				return PinSearchResponse{Results: hits, Total: len(hits)}, nil
			}
		}
	}
	return PinSearchResponse{Results: hits, Total: len(hits)}, nil
}

// LatestSession returns the newest committed local session.
func (s *Store) LatestSession(ctx context.Context) (SessionSummary, error) {
	list, err := s.ListSessions(ctx, ListOptions{Limit: 1})
	if err != nil {
		return SessionSummary{}, err
	}
	if len(list.Results) == 0 {
		return SessionSummary{}, fs.ErrNotExist
	}
	return list.Results[0], nil
}

// GetSessionSummary returns indexed local session metadata, including the report path.
func (s *Store) GetSessionSummary(ctx context.Context, id string) (SessionSummary, error) {
	return s.getSummary(ctx, id)
}

// GetSession reads a local session.json and stamps the local report id.
func (s *Store) GetSession(ctx context.Context, id string) (map[string]any, error) {
	summary, err := s.getSummary(ctx, id)
	if err != nil {
		return nil, err
	}
	session, err := readJSONFile(filepath.Join(summary.ReportPath, "session.json"))
	if err != nil {
		return nil, err
	}
	session["id"] = id
	if session["url"] == nil || session["url"] == "" {
		session["url"] = summary.URL
	}
	return session, nil
}

// GetPin reads a local pin and returns file path envelopes for heavy assets.
func (s *Store) GetPin(ctx context.Context, id string, number int, fields []string) (map[string]any, error) {
	summary, err := s.getSummary(ctx, id)
	if err != nil {
		return nil, err
	}
	pinRel := fmt.Sprintf("pin_%d/pin.json", number)
	pin, err := readJSONFile(filepath.Join(summary.ReportPath, filepath.FromSlash(pinRel)))
	if err != nil {
		return nil, err
	}
	pin["id"] = fmt.Sprintf("%s.%d", id, number)
	pin["number"] = number
	addAssetEnvelope(pin, "screenshot", filepath.Join(summary.ReportPath, fmt.Sprintf("pin_%d", number), "screenshot.png"), "image/png")
	addAssetEnvelope(pin, "session_replay", filepath.Join(summary.ReportPath, fmt.Sprintf("pin_%d", number), "replay.rrweb.json"), "application/json")

	if wantsHeavyLogs(fields) {
		logs, err := readJSONFile(filepath.Join(summary.ReportPath, fmt.Sprintf("pin_%d", number), "logs.json"))
		if err == nil {
			pin["console"] = arrayOrEmpty(logs["console"])
			pin["network"] = arrayOrEmpty(logs["network"])
			pin["events"] = arrayOrEmpty(logs["events"])
		}
	}
	return pin, nil
}

func addAssetEnvelope(pin map[string]any, key string, filePath string, contentType string) {
	info, err := os.Stat(filePath)
	if err != nil {
		return
	}
	pin[key] = map[string]any{
		"path":         filePath,
		"content_type": contentType,
		"size_bytes":   info.Size(),
	}
}

func wantsHeavyLogs(fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	for _, field := range fields {
		switch field {
		case "all", "console", "network", "events":
			return true
		}
	}
	return false
}

func arrayOrEmpty(value any) any {
	if value == nil {
		return []any{}
	}
	return value
}

func (s *Store) getSummary(ctx context.Context, id string) (SessionSummary, error) {
	var item SessionSummary
	if err := s.db.QueryRowContext(ctx, `SELECT id, created_at, updated_at, source_url, pin_count,
first_pin_feedback, report_path, total_size
FROM sessions WHERE id = ? AND commit_status = 'committed'`, id).Scan(
		&item.ID,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.URL,
		&item.PinCount,
		&item.FirstPinFeedback,
		&item.ReportPath,
		&item.TotalSize,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionSummary{}, fs.ErrNotExist
		}
		return SessionSummary{}, err
	}
	item.Status = "open"
	return item, nil
}

func (s *Store) pinsForSession(ctx context.Context, id string) ([]pinIndex, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT pin_number, pin_path, feedback, url
FROM pins WHERE session_id = ? ORDER BY pin_number`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pins []pinIndex
	for rows.Next() {
		var pin pinIndex
		if err := rows.Scan(&pin.Number, &pin.Path, &pin.Feedback, &pin.URL); err != nil {
			return nil, err
		}
		pins = append(pins, pin)
	}
	return pins, rows.Err()
}

// DeleteSession removes a local report directory and index rows.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	summary, err := s.getSummary(ctx, id)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM pins WHERE session_id = ?", id); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", id); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return os.RemoveAll(summary.ReportPath)
}

// Prune removes sessions older than the supplied retention duration.
func (s *Store) Prune(ctx context.Context, opts PruneOptions) (int, error) {
	if opts.OlderThan <= 0 {
		return 0, errors.New("older-than must be positive")
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	cutoff := now.Add(-opts.OlderThan)
	list, err := s.ListSessions(ctx, ListOptions{Limit: maxListLimit})
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, session := range list.Results {
		created, err := time.Parse(time.RFC3339, session.CreatedAt)
		if err != nil || !created.Before(cutoff) {
			continue
		}
		if err := s.DeleteSession(ctx, session.ID); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func (s *Store) cleanupTemps() error {
	tmpRoot := filepath.Join(s.root, ".tmp")
	entries, err := os.ReadDir(tmpRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			_ = os.RemoveAll(filepath.Join(tmpRoot, entry.Name()))
		}
	}
	return nil
}

func readJSONFile(filePath string) (map[string]any, error) {
	data, err := os.ReadFile(filePath) //nolint:gosec // file path is local store controlled.
	if err != nil {
		return nil, err
	}
	var value map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func writeJSONFile(filePath string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filePath, data, 0o600) //nolint:gosec // file path is local store controlled.
}
