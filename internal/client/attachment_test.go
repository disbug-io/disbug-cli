package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

func TestDownloadAttachmentReturnsScopedBytesAndSafeMetadata(t *testing.T) {
	doer := doerFunc(func(req *http.Request) (*http.Response, error) {
		if got, want := req.URL.Path, "/api/teams/acme/projects/42/sessions/5/pins/by-number/2/attachments/9/download/"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := req.Header.Get("Authorization"), "Bearer token"; got != want {
			t.Fatalf("Authorization = %q, want %q", got, want)
		}
		if got, want := req.Header.Get("Accept"), "*/*"; got != want {
			t.Fatalf("Accept = %q, want %q", got, want)
		}
		return attachmentResponse(http.StatusOK, "attachment body", http.Header{
			"Content-Disposition": {`attachment; filename="../../notes.md"`},
			"Content-Type":        {"text/markdown; charset=utf-8"},
		}), nil
	})
	cli := New("https://api.example.test", "token", "test", &recordingSleeper{}, doer, nil)

	attachment, err := cli.DownloadAttachment(
		context.Background(),
		ref.PinRef{Session: testSessionRef, Pin: 2},
		9,
	)
	if err != nil {
		t.Fatalf("DownloadAttachment() error = %v, want nil", err)
	}
	if got, want := attachment.Filename, "notes.md"; got != want {
		t.Fatalf("Filename = %q, want %q", got, want)
	}
	if got, want := attachment.ContentType, "text/markdown"; got != want {
		t.Fatalf("ContentType = %q, want %q", got, want)
	}
	if got, want := attachment.SizeBytes, int64(len("attachment body")); got != want {
		t.Fatalf("SizeBytes = %d, want %d", got, want)
	}
	if got, want := string(attachment.Data), "attachment body"; got != want {
		t.Fatalf("Data = %q, want %q", got, want)
	}
}

func TestDownloadAttachmentFilenameSanitization(t *testing.T) {
	tests := map[string]string{
		`../../notes.md`:           "notes.md",
		`..\\..\\request.json`:     "request.json",
		`report<>:"|?*.pdf`:        "report_______.pdf",
		`.diagnostic.json`:         ".diagnostic.json",
		"  investigation.txt  ":    "investigation.txt",
		"folder/line\nbreak.md":    "line_break.md",
		"folder/trailing dots... ": "trailing dots",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			if got := sanitizeAttachmentFilename(input); got != want {
				t.Fatalf("sanitizeAttachmentFilename(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestDownloadAttachmentRejectsOversizedResponse(t *testing.T) {
	t.Run("declared content length", func(t *testing.T) {
		doer := doerFunc(func(*http.Request) (*http.Response, error) {
			resp := attachmentResponse(http.StatusOK, "small body", nil)
			resp.ContentLength = maxAttachmentDownloadBytes + 1
			return resp, nil
		})
		cli := New("https://api.example.test", "token", "test", &recordingSleeper{}, doer, nil)

		_, err := cli.DownloadAttachment(context.Background(), ref.PinRef{Session: testSessionRef, Pin: 2}, 9)
		if err == nil || !strings.Contains(err.Error(), "5 MB") {
			t.Fatalf("DownloadAttachment() error = %v, want size-limit error", err)
		}
	})

	t.Run("streamed body", func(t *testing.T) {
		doer := doerFunc(func(*http.Request) (*http.Response, error) {
			return attachmentResponse(
				http.StatusOK,
				strings.Repeat("x", int(maxAttachmentDownloadBytes)+1),
				nil,
			), nil
		})
		cli := New("https://api.example.test", "token", "test", &recordingSleeper{}, doer, nil)

		_, err := cli.DownloadAttachment(context.Background(), ref.PinRef{Session: testSessionRef, Pin: 2}, 9)
		if err == nil || !strings.Contains(err.Error(), "5 MB") {
			t.Fatalf("DownloadAttachment() error = %v, want size-limit error", err)
		}
	})
}

func TestDownloadAttachmentHandlesAuthorizationNotFoundAndNetworkErrors(t *testing.T) {
	for _, tt := range []struct {
		name       string
		statusCode int
	}{
		{name: "unauthorized", statusCode: http.StatusUnauthorized},
		{name: "not found", statusCode: http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			doer := doerFunc(func(*http.Request) (*http.Response, error) {
				return attachmentResponse(
					tt.statusCode,
					`{"code":"not_found","detail":"attachment not found","request_id":"req-1"}`,
					http.Header{"Content-Type": {"application/json"}},
				), nil
			})
			cli := New("https://api.example.test", "token", "test", &recordingSleeper{}, doer, nil)

			_, err := cli.DownloadAttachment(context.Background(), ref.PinRef{Session: testSessionRef, Pin: 2}, 9)
			var apiErr *errfmt.APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.statusCode {
				t.Fatalf("DownloadAttachment() error = %#v, want API status %d", err, tt.statusCode)
			}
		})
	}

	wantErr := errors.New("dial failed")
	doer := doerFunc(func(*http.Request) (*http.Response, error) { return nil, wantErr })
	cli := New("https://api.example.test", "token", "test", &recordingSleeper{}, doer, nil)
	_, err := cli.DownloadAttachment(context.Background(), ref.PinRef{Session: testSessionRef, Pin: 2}, 9)
	var networkErr *errfmt.NetworkError
	if !errors.As(err, &networkErr) || !errors.Is(networkErr, wantErr) {
		t.Fatalf("DownloadAttachment() error = %#v, want wrapped network error", err)
	}
}

func attachmentResponse(status int, body string, header http.Header) *http.Response {
	if header == nil {
		header = make(http.Header)
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
