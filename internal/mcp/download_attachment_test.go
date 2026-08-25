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

func TestDownloadAttachmentReturnsReadableTextResource(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"agent_name":"test","capabilities":["attachment_download"]}`)
		case "/api/teams/abb/projects/2/sessions/5/pins/by-number/2/attachments/9/download/":
			w.Header().Set("Content-Disposition", `attachment; filename="investigation.md"`)
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = io.WriteString(w, "# Investigation\nThe button request failed.")
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
	if metadata := firstTextContent(t, res); !strings.Contains(metadata, "investigation.md") {
		t.Fatalf("metadata = %q, want filename", metadata)
	}
	if len(res.Content) != 2 {
		t.Fatalf("content length = %d, want 2", len(res.Content))
	}
	resource, ok := res.Content[1].(*sdkmcp.EmbeddedResource)
	if !ok {
		t.Fatalf("attachment content type = %T, want *mcp.EmbeddedResource", res.Content[1])
	}
	if !strings.Contains(resource.Resource.Text, "button request failed") {
		t.Fatalf("resource text = %q, want attachment contents", resource.Resource.Text)
	}
}

func TestAttachmentToolResultUsesNativeImageAndBinaryContent(t *testing.T) {
	imageResult := attachmentToolResult(&client.DownloadedAttachment{
		ID:          10,
		Filename:    "screenshot.png",
		ContentType: "image/png",
		Data:        []byte("png-bytes"),
	}, Result{"id": int64(10)})
	image, ok := imageResult.Content[1].(*sdkmcp.ImageContent)
	if !ok {
		t.Fatalf("image content type = %T, want *mcp.ImageContent", imageResult.Content[1])
	}
	if got, want := string(image.Data), "png-bytes"; got != want {
		t.Fatalf("image data = %q, want %q", got, want)
	}

	binaryResult := attachmentToolResult(&client.DownloadedAttachment{
		ID:          11,
		Filename:    "report.pdf",
		ContentType: "application/pdf",
		Data:        []byte("pdf-bytes"),
	}, Result{"id": int64(11)})
	resource, ok := binaryResult.Content[1].(*sdkmcp.EmbeddedResource)
	if !ok {
		t.Fatalf("binary content type = %T, want *mcp.EmbeddedResource", binaryResult.Content[1])
	}
	if got, want := string(resource.Resource.Blob), "pdf-bytes"; got != want {
		t.Fatalf("binary data = %q, want %q", got, want)
	}
}
