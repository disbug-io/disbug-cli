package nativehost

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/disbug-io/disbug-cli/internal/localstore"
)

const (
	protocolVersion         = 1
	maxNativeHostFrameBytes = 1024 * 1024
)

// Options configures the native messaging host.
type Options struct {
	StoreRoot string
	Version   string
}

type inboundMessage struct {
	Type        string                    `json:"type"`
	Protocol    int                       `json:"protocol,omitempty"`
	MessageID   string                    `json:"message_id,omitempty"`
	Metadata    localstore.ReportMetadata `json:"metadata,omitempty"`
	Path        string                    `json:"path,omitempty"`
	ContentType string                    `json:"content_type,omitempty"`
	DataBase64  string                    `json:"data_base64,omitempty"`
	Index       int                       `json:"index,omitempty"`
}

// Run serves Chrome native messaging frames until stdin reaches EOF.
func Run(ctx context.Context, stdin io.Reader, stdout io.Writer, opts Options) error {
	if ctx == nil {
		ctx = context.Background()
	}
	store, err := localstore.Open(opts.StoreRoot)
	if err != nil {
		_ = WriteFrame(stdout, errorFrame("internal_error", err.Error()))
		return nil
	}
	defer func() {
		_ = store.Close()
	}()

	version := opts.Version
	if version == "" {
		version = "dev"
	}

	var report *localstore.Report
	nextChunkIndex := map[string]int{}
	for {
		var msg inboundMessage
		if err := ReadFrame(stdin, &msg); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			_ = WriteFrame(stdout, errorFrame("retryable", err.Error()))
			continue
		}

		switch msg.Type {
		case "hello":
			if msg.Protocol != protocolVersion {
				_ = WriteFrame(stdout, map[string]any{
					"type":          "error",
					"code":          "protocol_mismatch",
					"detail":        fmt.Sprintf("Disbug native protocol mismatch: extension=%d host=%d", msg.Protocol, protocolVersion),
					"min_protocol":  protocolVersion,
					"max_protocol":  protocolVersion,
					"host_protocol": protocolVersion,
				})
				continue
			}
			_ = WriteFrame(stdout, map[string]any{
				"type":        "hello_ack",
				"protocol":    protocolVersion,
				"cli_version": version,
				"store_path":  store.Root(),
				"writable":    true,
			})
		case "begin_report":
			report, err = store.BeginReport(ctx, msg.Metadata)
			nextChunkIndex = map[string]int{}
			writeAckOrError(stdout, msg.MessageID, err)
		case "put_file":
			if report == nil {
				_ = WriteFrame(stdout, errorFrame("retryable", "begin_report is required before put_file"))
				continue
			}
			data, err := decodeBase64(msg.DataBase64)
			if err == nil {
				err = report.WriteFile(msg.Path, msg.ContentType, data)
			}
			writeAckOrError(stdout, msg.MessageID, err)
		case "put_file_chunk":
			if report == nil {
				_ = WriteFrame(stdout, errorFrame("retryable", "begin_report is required before put_file_chunk"))
				continue
			}
			if msg.Index != nextChunkIndex[msg.Path] {
				_ = WriteFrame(stdout, errorFrame("retryable", fmt.Sprintf("unexpected chunk index for %s", msg.Path)))
				continue
			}
			data, err := decodeBase64(msg.DataBase64)
			if err == nil {
				err = report.AppendFile(msg.Path, msg.ContentType, data)
			}
			if err != nil {
				_ = WriteFrame(stdout, errorFrame("retryable", err.Error()))
				continue
			}
			nextChunkIndex[msg.Path]++
			_ = WriteFrame(stdout, map[string]any{"type": "ack", "message_id": msg.MessageID})
		case "finish_file":
			if _, err := localstore.ValidateRelativePath(msg.Path); err != nil {
				_ = WriteFrame(stdout, errorFrame("retryable", err.Error()))
				continue
			}
			_ = WriteFrame(stdout, map[string]any{"type": "ack", "message_id": msg.MessageID})
		case "commit_report":
			if report == nil {
				_ = WriteFrame(stdout, errorFrame("retryable", "begin_report is required before commit_report"))
				continue
			}
			committed, err := report.Commit(ctx)
			if err != nil {
				_ = WriteFrame(stdout, errorFrame("retryable", err.Error()))
				continue
			}
			_ = WriteFrame(stdout, map[string]any{
				"type":        "committed",
				"report_id":   committed.ID,
				"report_path": committed.Path,
				"prompt":      committed.Prompt,
			})
			report = nil
		default:
			_ = WriteFrame(stdout, errorFrame("retryable", fmt.Sprintf("unknown message type %q", msg.Type)))
		}
	}
}

func decodeBase64(value string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode base64 data: %w", err)
	}
	return data, nil
}

func writeAckOrError(stdout io.Writer, messageID string, err error) {
	if err != nil {
		_ = WriteFrame(stdout, errorFrame("retryable", err.Error()))
		return
	}
	_ = WriteFrame(stdout, map[string]any{"type": "ack", "message_id": messageID})
}

func errorFrame(code, detail string) map[string]any {
	switch code {
	case "setup_required", "protocol_mismatch", "retryable", "internal_error":
	default:
		code = "internal_error"
	}
	return map[string]any{"type": "error", "code": code, "detail": detail}
}

// WriteFrame writes a Chrome native messaging frame.
func WriteFrame(w io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > maxNativeHostFrameBytes {
		return fmt.Errorf("native host response exceeds 1 MB")
	}
	var header [4]byte
	frameLength := uint32(len(data)) //nolint:gosec // maxNativeHostFrameBytes keeps this below uint32 max.
	binary.LittleEndian.PutUint32(header[:], frameLength)
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// ReadFrame reads a Chrome native messaging frame into dst.
func ReadFrame(r io.Reader, dst any) error {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}
	length := binary.LittleEndian.Uint32(header[:])
	if length == 0 {
		return errors.New("empty native message")
	}
	if length > 64*1024*1024 {
		return fmt.Errorf("native message exceeds 64 MB")
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}
