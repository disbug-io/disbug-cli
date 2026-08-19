package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// DownloadAttachmentInput identifies a pin attachment from session or pin output.
type DownloadAttachmentInput struct {
	Target       string `json:"target" jsonschema:"Disbug report URL with ?pin=<number>"`
	AttachmentID int64  `json:"attachment_id" jsonschema:"Attachment ID shown in session or pin output"`
}

func registerDownloadAttachment(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[DownloadAttachmentInput, Result](srv, &sdkmcp.Tool{
		Name:        "download_attachment",
		Description: "Download a Disbug pin attachment and return its content to the agent.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		in DownloadAttachmentInput,
	) (*sdkmcp.CallToolResult, Result, error) {
		if err := requireCloud(deps); err != nil {
			return nil, nil, toolErr(err)
		}

		pinRef, err := ref.ParsePin(strings.TrimSpace(in.Target))
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
		}
		if in.AttachmentID <= 0 {
			return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{
				Message: "attachment_id must be greater than zero",
			}))
		}
		if err := deps.Client.RequireCapability(ctx, "attachment_download"); err != nil {
			return nil, nil, errors.New(errfmt.Format(err))
		}

		attachment, err := deps.Client.DownloadAttachment(ctx, pinRef, in.AttachmentID)
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(err))
		}
		result := Result{
			"id":           attachment.ID,
			"filename":     attachment.Filename,
			"content_type": attachment.ContentType,
			"size_bytes":   attachment.SizeBytes,
		}
		return attachmentToolResult(attachment, result), result, nil
	})
}

func attachmentToolResult(attachment *client.DownloadedAttachment, metadata Result) *sdkmcp.CallToolResult {
	contents := []sdkmcp.Content{&sdkmcp.TextContent{Text: jsonText(metadata)}}
	uri := fmt.Sprintf("disbug://attachments/%d/%s", attachment.ID, url.PathEscape(attachment.Filename))

	switch {
	case strings.HasPrefix(attachment.ContentType, "image/"):
		contents = append(contents, &sdkmcp.ImageContent{
			Data:     attachment.Data,
			MIMEType: attachment.ContentType,
		})
	case isTextAttachment(attachment.ContentType):
		contents = append(contents, &sdkmcp.EmbeddedResource{Resource: &sdkmcp.ResourceContents{
			URI:      uri,
			MIMEType: attachment.ContentType,
			Text:     string(attachment.Data),
		}})
	default:
		contents = append(contents, &sdkmcp.EmbeddedResource{Resource: &sdkmcp.ResourceContents{
			URI:      uri,
			MIMEType: attachment.ContentType,
			Blob:     attachment.Data,
		}})
	}

	return &sdkmcp.CallToolResult{Content: contents}
}

func isTextAttachment(contentType string) bool {
	return strings.HasPrefix(contentType, "text/") ||
		contentType == "application/json" ||
		strings.HasSuffix(contentType, "+json")
}
