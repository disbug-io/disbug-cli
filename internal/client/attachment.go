package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

const maxAttachmentDownloadBytes int64 = 5 * 1024 * 1024

// DownloadedAttachment contains attachment bytes and response metadata.
type DownloadedAttachment struct {
	ID          int64  `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	Data        []byte `json:"-"`
}

// DownloadAttachment downloads one attachment through the scoped agent API.
func (c *Client) DownloadAttachment(
	ctx context.Context,
	pinRef ref.PinRef,
	attachmentID int64,
) (*DownloadedAttachment, error) {
	if attachmentID <= 0 {
		return nil, fmt.Errorf("attachment ID must be greater than zero")
	}
	if err := c.acquire(ctx); err != nil {
		return nil, err
	}
	defer c.release()

	path := scopedSessionPath(pinRef.Session) + fmt.Sprintf(
		"pins/by-number/%d/attachments/%d/download/",
		pinRef.Pin,
		attachmentID,
	)
	absoluteURL := c.absoluteURL(path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, absoluteURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, &errfmt.NetworkError{URL: absoluteURL, Cause: err}
	}
	if resp == nil {
		return nil, &errfmt.NetworkError{URL: absoluteURL, Cause: errors.New("nil HTTP response")}
	}
	if resp.Body != nil {
		defer func() {
			_ = resp.Body.Close()
		}()
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(resp)
	}
	if resp.ContentLength > maxAttachmentDownloadBytes {
		return nil, fmt.Errorf("attachment exceeds the 5 MB download limit")
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAttachmentDownloadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read attachment: %w", err)
	}
	if int64(len(data)) > maxAttachmentDownloadBytes {
		return nil, fmt.Errorf("attachment exceeds the 5 MB download limit")
	}

	return &DownloadedAttachment{
		ID:          attachmentID,
		Filename:    attachmentFilename(resp, attachmentID),
		ContentType: responseContentType(resp),
		SizeBytes:   int64(len(data)),
		Data:        data,
	}, nil
}

func attachmentFilename(resp *http.Response, attachmentID int64) string {
	if disposition := resp.Header.Get("Content-Disposition"); disposition != "" {
		if _, params, err := mime.ParseMediaType(disposition); err == nil {
			if filename := safeAttachmentFilename(params["filename"]); filename != "" {
				return filename
			}
		}
	}
	return "attachment-" + strconv.FormatInt(attachmentID, 10)
}

func safeAttachmentFilename(filename string) string {
	filename = strings.TrimSpace(strings.ReplaceAll(filename, "\\", "/"))
	filename = filepath.Base(filename)
	if filename == "" || filename == "." || filename == ".." || filename == "/" {
		return ""
	}
	return filename
}

func responseContentType(resp *http.Response) string {
	contentType := resp.Header.Get("Content-Type")
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil && mediaType != "" {
		return mediaType
	}
	return "application/octet-stream"
}
