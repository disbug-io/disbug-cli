package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestGenerateStateWithDeterministicReader(t *testing.T) {
	input := strings.NewReader(strings.Repeat("a", stateBytes))

	got, err := generateState(input)
	if err != nil {
		t.Fatalf("generateState() error = %v, want nil", err)
	}
	if len(got) < 32 {
		t.Fatalf("len(generateState()) = %d, want >= 32", len(got))
	}

	want := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a", stateBytes)))
	if got != want {
		t.Fatalf("generateState() = %q, want %q", got, want)
	}
}

func TestGenerateStateWithCryptoRand(t *testing.T) {
	got, err := generateState(rand.Reader)
	if err != nil {
		t.Fatalf("generateState(rand.Reader) error = %v, want nil", err)
	}
	if len(got) < 32 {
		t.Fatalf("len(generateState(rand.Reader)) = %d, want >= 32", len(got))
	}
}

func TestGenerateStateCryptoRandStatesDiffer(t *testing.T) {
	first, err := generateState(rand.Reader)
	if err != nil {
		t.Fatalf("generate first state: %v", err)
	}
	second, err := generateState(rand.Reader)
	if err != nil {
		t.Fatalf("generate second state: %v", err)
	}

	if first == second {
		t.Fatal("two crypto/rand states matched")
	}
}

func TestGenerateStateShortReaderReturnsContextualError(t *testing.T) {
	_, err := generateState(strings.NewReader("short"))
	if err == nil {
		t.Fatal("generateState(short reader) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "generate state") {
		t.Fatalf("generateState(short reader) error = %q, want generate state context", err)
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("generateState(short reader) error = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestGenerateStateNilReaderUsesDefaultRandom(t *testing.T) {
	got, err := generateState(nil)
	if err != nil {
		t.Fatalf("generateState(nil) error = %v, want nil", err)
	}
	if len(got) < 32 {
		t.Fatalf("len(generateState(nil)) = %d, want >= 32", len(got))
	}
}
