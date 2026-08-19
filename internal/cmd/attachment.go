package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/outfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// AttachmentCmd groups attachment operations.
type AttachmentCmd struct {
	Download AttachmentDownloadCmd `cmd:"" name:"download" help:"Download an attachment by ID."`
}

// AttachmentDownloadCmd downloads one pin attachment.
type AttachmentDownloadCmd struct {
	Ref    string `arg:"" name:"url" help:"Disbug report URL with ?pin=<number>."`
	ID     int64  `arg:"" name:"attachment-id" help:"Attachment ID shown in session or pin output."`
	Output string `short:"o" help:"Destination file or directory; defaults to the attachment filename."`
	Stdout bool   `help:"Write the raw attachment bytes to stdout instead of a file."`
	Force  bool   `help:"Overwrite an existing destination file."`
}

// Run downloads the attachment and writes either its bytes or saved-file metadata.
func (c *AttachmentDownloadCmd) Run(ctx context.Context, b bindings) error {
	pinRef, err := ref.ParsePin(c.Ref)
	if err != nil {
		return &errfmt.UsageError{Message: err.Error()}
	}
	if c.ID <= 0 {
		return &errfmt.UsageError{Message: "attachment-id must be greater than zero"}
	}
	if c.Stdout && c.Output != "" {
		return &errfmt.UsageError{Message: "--stdout and --output cannot be used together"}
	}

	cli, _, err := newAuthenticatedClient(b.Flags)
	if err != nil {
		return err
	}
	if err := cli.RequireCapability(ctx, "attachment_download"); err != nil {
		return err
	}

	attachment, err := cli.DownloadAttachment(ctx, pinRef, c.ID)
	if err != nil {
		return err
	}
	if c.Stdout {
		_, err = b.Stdout.Write(attachment.Data)
		return err
	}

	destination, err := attachmentDestination(c.Output, attachment.Filename)
	if err != nil {
		return err
	}
	if err := writeAttachmentFile(destination, attachment.Data, c.Force); err != nil {
		return err
	}

	return outfmt.WriteJSON(b.Stdout, map[string]any{
		"id":           attachment.ID,
		"filename":     attachment.Filename,
		"content_type": attachment.ContentType,
		"size_bytes":   attachment.SizeBytes,
		"path":         destination,
	}, b.Flags.Pretty)
}

func attachmentDestination(output, filename string) (string, error) {
	if output == "" {
		return filename, nil
	}
	info, err := os.Stat(output)
	if err == nil && info.IsDir() {
		return filepath.Join(output, filename), nil
	}
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect attachment destination: %w", err)
	}
	return output, nil
}

func writeAttachmentFile(path string, data []byte, force bool) error {
	flags := os.O_WRONLY | os.O_CREATE
	if force {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}

	// #nosec G304 -- path is either an explicit CLI destination or a sanitized server filename.
	file, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("attachment destination already exists: %s (use --force to overwrite)", path)
		}
		return fmt.Errorf("create attachment destination: %w", err)
	}

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write attachment: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close attachment: %w", err)
	}
	return nil
}
