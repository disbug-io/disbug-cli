package outfmt

import (
	"encoding/json"
	"fmt"
	"io"
)

// WriteJSON encodes v as JSON to w, optionally formatted with two-space indentation.
func WriteJSON(w io.Writer, v any, pretty bool) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if pretty {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}
