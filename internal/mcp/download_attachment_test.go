package mcp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/client"
)

func TestDownloadAttachmentReturnsTextImageAndBinaryContent(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		contentType string
		body        string
		kind        string
	}{
		{name: "text", filename: "investigation.md", contentType: "text/markdown", body: "# Investigation", kind: "text"},
		{name: "json", filename: "request.json", contentType: "application/problem+json", body: `{"status":500}`, kind: "text"},
		{name: "image", filename: "screenshot.png", contentType: "image/png", body: "png-bytes", kind: "image"},
		{name: "binary", filename: "report.pdf", contentType: "application/pdf", body: "pdf-bytes", kind: "binary"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/me/":
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"agent_name":"test","capabilities":["attachment_download"]}`)
				case "/api/teams/abb/projects/2/sessions/5/pins/by-number/2/attachments/9/download/":
					w.Header().Set("Content-Disposition", `attachment; filename="`+tt.filename+`"`)
					w.Header().Set("Content-Type", tt.contentType)
					_, _ = io.WriteString(w, tt.body)
				default:
					t.Fatalf("unexpected request path: %s", r.URL.Path)
				}
			}))
			t.Cleanup(backend.Close)

			cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
			srv := newServer(&Deps{Client: cli})
			res, err := callTool(t, srv, "download_attachment", map[string]any{
				"target":        "https://staging.disbug.us/abb/projects/2/sessions/5/?pin=2",
				"attachment_id": 9,
			})
			if err != nil {
				t.Fatalf("CallTool(download_attachment) error = %v, want nil", err)
			}
			if res.IsError {
				t.Fatalf("download_attachment IsError = true: %#v", res.Content)
			}
			if metadata := firstTextContent(t, res); !strings.Contains(metadata, tt.filename) {
				t.Fatalf("metadata = %q, want filename %q", metadata, tt.filename)
			}
			if len(res.Content) != 2 {
				t.Fatalf("content length = %d, want 2", len(res.Content))
			}

			switch tt.kind {
			case "text":
				resource, ok := res.Content[1].(*sdkmcp.EmbeddedResource)
				if !ok || resource.Resource == nil {
					t.Fatalf("attachment content type = %T, want *mcp.EmbeddedResource", res.Content[1])
				}
				if got, want := resource.Resource.Text, tt.body; got != want {
					t.Fatalf("resource text = %q, want %q", got, want)
				}
				if len(resource.Resource.Blob) != 0 {
					t.Fatalf("resource blob = %q, want empty", resource.Resource.Blob)
				}
			case "image":
				image, ok := res.Content[1].(*sdkmcp.ImageContent)
				if !ok {
					t.Fatalf("attachment content type = %T, want *mcp.ImageContent", res.Content[1])
				}
				if got, want := string(image.Data), tt.body; got != want {
					t.Fatalf("image data = %q, want %q", got, want)
				}
			case "binary":
				resource, ok := res.Content[1].(*sdkmcp.EmbeddedResource)
				if !ok || resource.Resource == nil {
					t.Fatalf("attachment content type = %T, want *mcp.EmbeddedResource", res.Content[1])
				}
				if got, want := string(resource.Resource.Blob), tt.body; got != want {
					t.Fatalf("resource blob = %q, want %q", got, want)
				}
				if resource.Resource.Text != "" {
					t.Fatalf("resource text = %q, want empty", resource.Resource.Text)
				}
			}
		})
	}
}

func TestDownloadAttachmentMissingCapabilityReturnsToolError(t *testing.T) {
	downloadCalled := false
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"agent_name":"test","capabilities":[]}`)
		default:
			downloadCalled = true
			t.Fatalf("download endpoint called without capability: %s", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})
	res, err := callTool(t, srv, "download_attachment", map[string]any{
		"target":        "https://staging.disbug.us/abb/projects/2/sessions/5/?pin=2",
		"attachment_id": 9,
	})
	if err != nil {
		t.Fatalf("CallTool(download_attachment) error = %v, want nil tool error result", err)
	}
	if !res.IsError {
		t.Fatal("download_attachment IsError = false, want true")
	}
	if downloadCalled {
		t.Fatal("download endpoint was called")
	}
	if got := firstTextContent(t, res); !strings.Contains(got, "attachment_download") {
		t.Fatalf("error content = %q, want missing capability", got)
	}
}
