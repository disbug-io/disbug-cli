package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

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
		resp := response(http.StatusOK, io.NopCloser(strings.NewReader("attachment body")))
		resp.Header.Set("Content-Disposition", `attachment; filename="../../notes.md"`)
		resp.Header.Set("Content-Type", "text/markdown; charset=utf-8")
		return resp, nil
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
	if got, want := string(attachment.Data), "attachment body"; got != want {
		t.Fatalf("Data = %q, want %q", got, want)
	}
}

func TestDownloadAttachmentRejectsOversizedResponse(t *testing.T) {
	doer := doerFunc(func(_ *http.Request) (*http.Response, error) {
		resp := response(http.StatusOK, io.NopCloser(strings.NewReader("small body")))
		resp.ContentLength = maxAttachmentDownloadBytes + 1
		return resp, nil
	})
	cli := New("https://api.example.test", "token", "test", &recordingSleeper{}, doer, nil)

	_, err := cli.DownloadAttachment(
		context.Background(),
		ref.PinRef{Session: testSessionRef, Pin: 2},
		9,
	)
	if err == nil || !strings.Contains(err.Error(), "5 MB") {
		t.Fatalf("DownloadAttachment() error = %v, want size-limit error", err)
	}
}
