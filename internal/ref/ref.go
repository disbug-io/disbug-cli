package ref

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// SessionRef identifies a cloud Disbug session by the same scoped identity used in report URLs.
type SessionRef struct {
	TeamSlug      string
	ProjectID     int64
	SessionNumber int64
}

// PinRef identifies a pin within a Disbug session.
type PinRef struct {
	Session SessionRef
	Pin     int64
}

// PinFetch identifies a pin and the fields to fetch for it.
type PinFetch struct {
	Pin    PinRef
	Fields []string
}

var canonicalFields = []string{
	"screenshot",
	"console",
	"network",
	"events",
	"replay",
	"voice_note",
	"video",
}

var wireFields = map[string]string{
	"screenshot": "screenshot",
	"console":    "console_logs",
	"network":    "network_logs",
	"events":     "user_events",
	"replay":     "session_replay",
	"voice_note": "voice_note",
	"video":      "video_recording",
}

// ParseSession parses a Disbug report URL.
func ParseSession(arg string) (SessionRef, error) {
	parsedURL, err := parseReportURL(arg)
	if err != nil {
		return SessionRef{}, fmt.Errorf("invalid session ref %q: %w", arg, err)
	}

	return parsedURL.Session, nil
}

// ParsePin parses a Disbug report URL with a pin query parameter.
func ParsePin(arg string) (PinRef, error) {
	parsedURL, err := parseReportURL(arg)
	if err != nil {
		return PinRef{}, fmt.Errorf("invalid pin ref %q: %w", arg, err)
	}
	if parsedURL.Pin == 0 {
		return PinRef{}, fmt.Errorf("invalid pin ref %q: missing pin query parameter", arg)
	}

	return PinRef{Session: parsedURL.Session, Pin: parsedURL.Pin}, nil
}

// ParsePinFetch parses a pin reference with an optional colon-delimited field override.
func ParsePinFetch(arg string, defaultFields []string) (PinFetch, error) {
	refArg, fieldsArg, hasFields := splitPinFetch(arg)
	pin, err := ParsePin(refArg)
	if err != nil {
		return PinFetch{}, err
	}

	fields := defaultFields
	if hasFields {
		if fieldsArg == "" {
			return PinFetch{}, fmt.Errorf("missing fields in pin fetch %q", arg)
		}
		fields = strings.Split(fieldsArg, ",")
	} else if queryFields := fieldsFromReportURL(refArg); len(queryFields) > 0 {
		fields = queryFields
	} else if len(defaultFields) == 0 {
		fields = []string{"all"}
	}

	normalizedFields, err := NormalizeFields(fields)
	if err != nil {
		return PinFetch{}, err
	}

	return PinFetch{Pin: pin, Fields: normalizedFields}, nil
}

// ParseReportPinNumber parses a positive pin number from a flag.
func ParseReportPinNumber(arg string) (int64, error) {
	return parsePositiveInt(arg)
}

// RefString returns a stable, human-readable scoped session reference.
func (r SessionRef) RefString() string {
	return fmt.Sprintf("%s/projects/%d/sessions/%d", r.TeamSlug, r.ProjectID, r.SessionNumber)
}

// RefString returns a stable, human-readable scoped pin reference.
func (r PinRef) RefString() string {
	return fmt.Sprintf("%s?pin=%d", r.Session.RefString(), r.Pin)
}

// NormalizeFields validates fields, removes duplicates, and returns canonical order.
func NormalizeFields(fields []string) ([]string, error) {
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty field list")
	}

	seen := make(map[string]bool, len(fields))
	hasAll := false

	for _, field := range fields {
		normalizedField := strings.TrimSpace(field)
		if normalizedField == "" {
			return nil, fmt.Errorf("empty field")
		}
		if !IsKnownField(normalizedField) {
			return nil, fmt.Errorf("unknown field %q", normalizedField)
		}
		if normalizedField == "all" {
			hasAll = true
		}
		seen[normalizedField] = true
	}

	if hasAll {
		if len(seen) > 1 {
			return nil, fmt.Errorf("all cannot be combined with other fields")
		}
		return []string{"all"}, nil
	}

	normalizedFields := make([]string, 0, len(seen))
	for _, field := range canonicalFields {
		if seen[field] {
			normalizedFields = append(normalizedFields, field)
		}
	}

	return normalizedFields, nil
}

// IsKnownField reports whether field is part of the user-facing field vocabulary.
func IsKnownField(field string) bool {
	if field == "all" {
		return true
	}
	_, ok := wireFields[field]
	return ok
}

// WireFieldName returns the API wire field name for a validated user-facing field.
func WireFieldName(field string) string {
	wireField, ok := wireFields[field]
	if !ok {
		panic(fmt.Sprintf("unknown field %q", field))
	}
	return wireField
}

// DedupAndUnion merges fetches for the same pin while preserving first-seen pin order.
func DedupAndUnion(fetches []PinFetch) []PinFetch {
	indexByPin := make(map[PinRef]int, len(fetches))
	merged := make([]PinFetch, 0, len(fetches))

	for _, fetch := range fetches {
		index, ok := indexByPin[fetch.Pin]
		if !ok {
			indexByPin[fetch.Pin] = len(merged)
			merged = append(merged, PinFetch{Pin: fetch.Pin, Fields: unionFields(fetch.Fields)})
			continue
		}

		merged[index].Fields = unionFields(merged[index].Fields, fetch.Fields)
	}

	return merged
}

type parsedReportURL struct {
	Session SessionRef
	Pin     int64
}

func parseReportURL(arg string) (parsedReportURL, error) {
	value := strings.TrimSpace(arg)
	if value == "" {
		return parsedReportURL{}, fmt.Errorf("empty value")
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return parsedReportURL{}, fmt.Errorf("expected Disbug report URL")
	}

	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) != 5 || parts[1] != "projects" || parts[3] != "sessions" {
		return parsedReportURL{}, fmt.Errorf("expected path /<team>/projects/<project>/sessions/<session>/")
	}

	teamSlug, err := url.PathUnescape(parts[0])
	if err != nil || teamSlug == "" {
		return parsedReportURL{}, fmt.Errorf("invalid team slug")
	}
	projectID, err := parsePositiveInt(parts[2])
	if err != nil {
		return parsedReportURL{}, fmt.Errorf("invalid project id: %w", err)
	}
	sessionNumber, err := parsePositiveInt(parts[4])
	if err != nil {
		return parsedReportURL{}, fmt.Errorf("invalid session number: %w", err)
	}

	pin := int64(0)
	if pinRaw := strings.TrimSpace(u.Query().Get("pin")); pinRaw != "" {
		pin, err = parsePositiveInt(pinRaw)
		if err != nil {
			return parsedReportURL{}, fmt.Errorf("invalid pin number: %w", err)
		}
	}

	return parsedReportURL{
		Session: SessionRef{TeamSlug: teamSlug, ProjectID: projectID, SessionNumber: sessionNumber},
		Pin:     pin,
	}, nil
}

func fieldsFromReportURL(arg string) []string {
	u, err := url.Parse(strings.TrimSpace(arg))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	fieldsRaw := strings.TrimSpace(u.Query().Get("fields"))
	if fieldsRaw == "" {
		return nil
	}
	return strings.Split(fieldsRaw, ",")
}

func splitPinFetch(arg string) (string, string, bool) {
	if strings.Contains(arg, "://") {
		return arg, "", false
	}
	parts := strings.Split(arg, ":")
	if len(parts) == 2 {
		return parts[0], parts[1], true
	}
	return arg, "", false
}

func unionFields(fieldLists ...[]string) []string {
	seen := make(map[string]bool)
	for _, fieldList := range fieldLists {
		for _, field := range fieldList {
			if field == "all" {
				return []string{"all"}
			}
			seen[field] = true
		}
	}

	fields := make([]string, 0, len(seen))
	for _, field := range canonicalFields {
		if seen[field] {
			fields = append(fields, field)
		}
	}

	return fields
}

func parsePositiveInt(arg string) (int64, error) {
	if arg == "" {
		return 0, fmt.Errorf("empty value")
	}

	for _, r := range arg {
		if !unicode.IsDigit(r) {
			return 0, fmt.Errorf("not a positive integer")
		}
	}

	value, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, fmt.Errorf("not a positive integer")
	}

	return value, nil
}
