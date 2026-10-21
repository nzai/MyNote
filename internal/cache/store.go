package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nzai/MyNote/internal/clipboard"
	"github.com/nzai/MyNote/internal/core"
)

const (
	defaultMaxAge   = 7 * 24 * time.Hour
	defaultMaxBytes = 500 * 1024 * 1024
	writeChunkSize  = 256 * 1024
)

var validCaptureID = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

type Options struct {
	Root     string
	MaxAge   time.Duration
	MaxBytes int64
	Clock    func() time.Time
}

type Store struct {
	root     string
	maxAge   time.Duration
	maxBytes int64
	now      func() time.Time
}

type Record struct {
	ID           string
	Kind         core.Kind
	SourceFormat string
	CapturedAt   time.Time
	Bytes        int64
	ContentFile  string
	Directory    string
}

type metadata struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	SourceFormat string    `json:"source_format"`
	CapturedAt   time.Time `json:"captured_at"`
	Bytes        int64     `json:"bytes"`
	ContentFile  string    `json:"content_file"`
}

func NewStore(options Options) *Store {
	if options.MaxAge == 0 {
		options.MaxAge = defaultMaxAge
	}
	if options.MaxBytes == 0 {
		options.MaxBytes = defaultMaxBytes
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	root := options.Root
	if root != "" {
		root = filepath.Clean(root)
	}
	return &Store{
		root:     root,
		maxAge:   options.MaxAge,
		maxBytes: options.MaxBytes,
		now:      options.Clock,
	}
}

func (s *Store) Save(ctx context.Context, snapshot core.Snapshot) (record Record, err error) {
	if ctx == nil {
		return Record{}, errors.New("save cache record: context is nil")
	}
	root, err := validateCacheRoot(s.root)
	if err != nil {
		return Record{}, fmt.Errorf("save cache record: %w", err)
	}
	if s.maxAge <= 0 || s.maxBytes <= 0 {
		return Record{}, fmt.Errorf(
			"save cache record: invalid retention policy max_age=%s max_bytes=%d",
			s.maxAge,
			s.maxBytes,
		)
	}
	if !validCaptureID.MatchString(snapshot.CaptureID) {
		return Record{}, fmt.Errorf("save cache record: invalid capture ID %q", snapshot.CaptureID)
	}
	if snapshot.CapturedAt.IsZero() {
		return Record{}, fmt.Errorf("save cache record %q: captured_at is zero", snapshot.CaptureID)
	}
	if err := ctx.Err(); err != nil {
		return Record{}, fmt.Errorf("save cache record %q: %w", snapshot.CaptureID, err)
	}

	contentFile, err := contentFileName(snapshot)
	if err != nil {
		return Record{}, fmt.Errorf("save cache record %q: %w", snapshot.CaptureID, err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Record{}, fmt.Errorf("create cache root %q: %w", root, err)
	}

	partialDir, err := os.MkdirTemp(root, ".partial-")
	if err != nil {
		return Record{}, fmt.Errorf("create partial cache directory under %q: %w", root, err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if cleanupErr := os.RemoveAll(partialDir); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove partial cache directory %q: %w", partialDir, cleanupErr))
		}
	}()

	contentPath := filepath.Join(partialDir, contentFile)
	contentBytes, err := s.writeContent(ctx, contentPath, snapshot)
	if err != nil {
		return Record{}, fmt.Errorf("write cache content for %q: %w", snapshot.CaptureID, err)
	}

	meta := metadata{
		ID:           snapshot.CaptureID,
		Kind:         kindName(snapshot.Kind),
		SourceFormat: snapshot.SourceFormat,
		CapturedAt:   snapshot.CapturedAt,
		Bytes:        contentBytes,
		ContentFile:  contentFile,
	}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return Record{}, fmt.Errorf("encode cache metadata for %q: %w", snapshot.CaptureID, err)
	}
	metaBytes = append(metaBytes, '\n')
	if _, err := s.writeFile(ctx, filepath.Join(partialDir, "meta.json"), func(w io.Writer) error {
		_, err := w.Write(metaBytes)
		return err
	}); err != nil {
		return Record{}, fmt.Errorf("write cache metadata for %q: %w", snapshot.CaptureID, err)
	}

	if err := ctx.Err(); err != nil {
		return Record{}, fmt.Errorf("commit cache record %q: %w", snapshot.CaptureID, err)
	}
	finalDir := filepath.Join(root, snapshot.CaptureID)
	if err := os.Rename(partialDir, finalDir); err != nil {
		return Record{}, fmt.Errorf("commit cache directory %q: %w", finalDir, err)
	}
	committed = true

	record = Record{
		ID:           snapshot.CaptureID,
		Kind:         snapshot.Kind,
		SourceFormat: snapshot.SourceFormat,
		CapturedAt:   snapshot.CapturedAt,
		Bytes:        contentBytes,
		ContentFile:  contentFile,
		Directory:    finalDir,
	}
	return record, nil
}

func (s *Store) writeContent(
	ctx context.Context,
	path string,
	snapshot core.Snapshot,
) (int64, error) {
	switch snapshot.Kind {
	case core.KindText:
		return s.writeFile(ctx, path, func(w io.Writer) error {
			return copyString(ctx, w, snapshot.Text)
		})
	case core.KindImage:
		if snapshot.Image == nil {
			return 0, errors.New("image snapshot has no decoded image")
		}
		return s.writeFile(ctx, path, func(w io.Writer) error {
			return clipboard.EncodePNG(w, snapshot.Image)
		})
	default:
		return 0, fmt.Errorf("unsupported snapshot kind %d", snapshot.Kind)
	}
}

func (s *Store) writeFile(
	ctx context.Context,
	path string,
	write func(io.Writer) error,
) (size int64, err error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, fmt.Errorf("create file %q: %w", path, err)
	}
	closed := false
	defer func() {
		if !closed {
			if closeErr := file.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close failed file %q: %w", path, closeErr))
			}
		}
	}()

	if err := write(&contextWriter{ctx: ctx, writer: file}); err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	info, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat file %q before sync: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		return 0, fmt.Errorf("sync file %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return 0, fmt.Errorf("close file %q: %w", path, err)
	}
	closed = true
	return info.Size(), nil
}

func copyString(ctx context.Context, w io.Writer, text string) error {
	buffer := make([]byte, writeChunkSize)
	reader := strings.NewReader(text)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := reader.Read(buffer)
		if n > 0 {
			if _, err := w.Write(buffer[:n]); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func contentFileName(snapshot core.Snapshot) (string, error) {
	switch snapshot.Kind {
	case core.KindText:
		return "content.txt", nil
	case core.KindImage:
		return "content.png", nil
	default:
		return "", fmt.Errorf("unsupported snapshot kind %d", snapshot.Kind)
	}
}

func kindName(kind core.Kind) string {
	switch kind {
	case core.KindText:
		return "text"
	case core.KindImage:
		return "image"
	default:
		return ""
	}
}

func validateCacheRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("cache root is empty")
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("cache root must be absolute: %q", root)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve cache root %q: %w", root, err)
	}
	absolute = filepath.Clean(absolute)
	volume := filepath.VolumeName(absolute)
	volumeRoot := filepath.Clean(volume + string(filepath.Separator))
	if strings.EqualFold(absolute, volumeRoot) {
		return "", fmt.Errorf("refusing to use volume root %q as cache root", absolute)
	}
	return absolute, nil
}

func validateClearRoot(root string) (string, error) {
	return validateCacheRoot(root)
}

type contextWriter struct {
	ctx    context.Context
	writer io.Writer
}

func (w *contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := w.writer.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = w.ctx.Err()
	}
	return n, err
}
