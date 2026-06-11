package client

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// ReplayFile is a downloaded replay file with lightweight metadata.
type ReplayFile struct {
	Path       string `json:"path"`
	DurationMs int    `json:"duration_ms"`
	EventCount int    `json:"event_count"`
	SizeBytes  int64  `json:"size_bytes"`
}

// DownloadReplay fetches a replay asset URL, decompresses if gzipped,
// writes to a cache file, and returns the path with metadata.
func DownloadReplay(ctx context.Context, asset *Asset, sessionID int64, pinNumber int64) (*ReplayFile, error) {
	if asset == nil || asset.URL == "" {
		return nil, nil
	}

	cacheDir, err := replayCacheDir()
	if err != nil {
		return nil, fmt.Errorf("replay cache dir: %w", err)
	}

	destPath := filepath.Join(cacheDir, fmt.Sprintf("%d-%d-replay.json", sessionID, pinNumber))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("replay download request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("replay download: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("replay download: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("replay download read: %w", err)
	}

	// Decompress if gzipped (magic bytes 0x1f 0x8b).
	if len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b {
		reader, err := gzip.NewReader(io.NopCloser(
			io.NewSectionReader(readerAtFromBytes(body), 0, int64(len(body))),
		))
		if err != nil {
			return nil, fmt.Errorf("replay gzip open: %w", err)
		}
		defer func() {
			_ = reader.Close()
		}()

		decompressed, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("replay gzip read: %w", err)
		}
		body = decompressed
	}

	if err := os.WriteFile(destPath, body, 0o600); err != nil {
		return nil, fmt.Errorf("replay write: %w", err)
	}

	meta := extractReplayMeta(body)
	meta.Path = destPath
	meta.SizeBytes = int64(len(body))

	return meta, nil
}

func replayCacheDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	dir := filepath.Join(cacheDir, "disbug", "replays")
	return dir, os.MkdirAll(dir, 0o750)
}

// extractReplayMeta parses top-level fields without loading the full events array.
func extractReplayMeta(data []byte) *ReplayFile {
	var envelope struct {
		DurationMs int               `json:"duration_ms"`
		Events     []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return &ReplayFile{}
	}
	return &ReplayFile{
		DurationMs: envelope.DurationMs,
		EventCount: len(envelope.Events),
	}
}

type bytesReaderAt struct {
	data []byte
}

func (b *bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b.data)) {
		return 0, io.EOF
	}
	n := copy(p, b.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func readerAtFromBytes(data []byte) *bytesReaderAt {
	return &bytesReaderAt{data: data}
}
