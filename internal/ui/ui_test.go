package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewErrfWritesToStderrOnly(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	u := New(&stdout, &stderr, ColorAuto, true)

	u.Errf("error: %s", "failed")

	if got, want := stderr.String(), "error: failed"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

func TestColorEnabled(t *testing.T) {
	tests := []struct {
		name  string
		mode  ColorMode
		isTTY bool
		want  bool
	}{
		{name: "auto tty", mode: ColorAuto, isTTY: true, want: true},
		{name: "auto non tty", mode: ColorAuto, isTTY: false, want: false},
		{name: "always tty", mode: ColorAlways, isTTY: true, want: true},
		{name: "always non tty", mode: ColorAlways, isTTY: false, want: true},
		{name: "never tty", mode: ColorNever, isTTY: true, want: false},
		{name: "never non tty", mode: ColorNever, isTTY: false, want: false},
		{name: "unknown behaves like auto tty", mode: ColorMode(99), isTTY: true, want: true},
		{name: "unknown behaves like auto non tty", mode: ColorMode(99), isTTY: false, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := colorEnabled(tt.mode, tt.isTTY); got != tt.want {
				t.Fatalf("colorEnabled(%d, %t) = %t, want %t", tt.mode, tt.isTTY, got, tt.want)
			}
		})
	}
}

func TestParseColorMode(t *testing.T) {
	tests := []struct {
		input string
		want  ColorMode
	}{
		{input: "", want: ColorAuto},
		{input: "auto", want: ColorAuto},
		{input: " Always ", want: ColorAlways},
		{input: "never", want: ColorNever},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseColorMode(tt.input)
			if err != nil {
				t.Fatalf("ParseColorMode(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ParseColorMode(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseColorModeInvalidMentionsExpectedModes(t *testing.T) {
	_, err := ParseColorMode("sometimes")

	if err == nil {
		t.Fatal("ParseColorMode() error = nil, want error")
	}
	for _, want := range []string{"auto", "always", "never"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ParseColorMode() error = %q, want mention %q", err.Error(), want)
		}
	}
}

func TestNewResolvesColor(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if !New(&stdout, &stderr, ColorAuto, true).Color() {
		t.Fatal("Color() = false, want true")
	}
	if New(&stdout, &stderr, ColorAuto, false).Color() {
		t.Fatal("Color() = true, want false")
	}
}

func TestOutfWritesToStdout(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	u := New(&stdout, &stderr, ColorNever, false)

	u.Outf("hello %s", "world")

	if got, want := stdout.String(), "hello world"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestErrlnWritesLineToStderr(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	u := New(&stdout, &stderr, ColorNever, false)

	u.Errln("error:", "failed")

	if got, want := stderr.String(), "error: failed\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

func TestStdoutStderrReturnUnderlyingWriters(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	u := New(&stdout, &stderr, ColorNever, false)

	if u.Stdout() != &stdout {
		t.Fatal("Stdout() did not return underlying stdout writer")
	}
	if u.Stderr() != &stderr {
		t.Fatal("Stderr() did not return underlying stderr writer")
	}
}
