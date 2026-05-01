package ref

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// SessionRef identifies a Disbug session.
type SessionRef struct {
	ID int64
}

// PinRef identifies a pin within a Disbug session.
type PinRef struct {
	Session int64
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

// ParseSession parses a positive integer session reference.
func ParseSession(arg string) (SessionRef, error) {
	id, err := parsePositiveInt(arg)
	if err != nil {
		return SessionRef{}, fmt.Errorf("invalid session ref %q: %w", arg, err)
	}

	return SessionRef{ID: id}, nil
}

// ParsePin parses a positive session and pin reference in session.pin form.
func ParsePin(arg string) (PinRef, error) {
	parts := strings.Split(arg, ".")
	if len(parts) != 2 {
		return PinRef{}, fmt.Errorf("invalid pin ref %q", arg)
	}

	session, err := parsePositiveInt(parts[0])
	if err != nil {
		return PinRef{}, fmt.Errorf("invalid pin ref %q: %w", arg, err)
	}

	pin, err := parsePositiveInt(parts[1])
	if err != nil {
		return PinRef{}, fmt.Errorf("invalid pin ref %q: %w", arg, err)
	}

	return PinRef{Session: session, Pin: pin}, nil
}

// ParsePinFetch parses a pin reference with an optional colon-delimited field override.
func ParsePinFetch(arg string, defaultFields []string) (PinFetch, error) {
	parts := strings.Split(arg, ":")
	if len(parts) > 2 {
		return PinFetch{}, fmt.Errorf("invalid pin fetch %q", arg)
	}

	pin, err := ParsePin(parts[0])
	if err != nil {
		return PinFetch{}, err
	}

	fields := defaultFields
	if len(parts) == 2 {
		if parts[1] == "" {
			return PinFetch{}, fmt.Errorf("missing fields in pin fetch %q", arg)
		}
		fields = strings.Split(parts[1], ",")
	} else if len(defaultFields) == 0 {
		fields = []string{"all"}
	}

	normalizedFields, err := NormalizeFields(fields)
	if err != nil {
		return PinFetch{}, err
	}

	return PinFetch{Pin: pin, Fields: normalizedFields}, nil
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
