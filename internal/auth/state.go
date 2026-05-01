package auth

import (
	"encoding/base64"
	"fmt"
	"io"

	"github.com/disbug-io/disbug-cli/internal/seams"
)

const stateBytes = 32

func generateState(r io.Reader) (string, error) {
	if r == nil {
		r = seams.DefaultRandom()
	}

	buf := make([]byte, stateBytes)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}
