package save

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nzai/MyNote/internal/clipboard"
	"github.com/nzai/MyNote/internal/core"
)

const saveWriteChunkSize = 256 * 1024

type AtomicWriter struct{}

func NewAtomicWriter() *AtomicWriter {
	return &AtomicWriter{}
}

func (w *AtomicWriter) Write(
	ctx context.Context,
	path string,
	snapshot core.Snapshot,
	text string,
) (err error) {
	if ctx == nil {
		return errors.New("write document: context is nil")
	}
	if strings.TrimSpace(path) == "" {
		return errors.New("write document: destination path is empty")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("write document %q: %w", path, err)
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	temp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %q: %w", path, err)
	}
	tempPath := temp.Name()
	closed := false
	committed := false
	defer func() {
		if !closed {
			if closeErr := temp.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close temporary file %q: %w", tempPath, closeErr))
			}
		}
		if !committed {
			if removeErr := os.Remove(tempPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				err = errors.Join(err, fmt.Errorf("remove temporary file %q: %w", tempPath, removeErr))
			}
		}
	}()

	writer := &writeContext{ctx: ctx, writer: temp}
	switch snapshot.Kind {
	case core.KindText:
		if err := writeChunks(writer, normalizeCRLF(text)); err != nil {
			return fmt.Errorf("write text document %q: %w", path, err)
		}
	case core.KindImage:
		if snapshot.Image == nil {
			return fmt.Errorf("write image document %q: image is nil", path)
		}
		if err := clipboard.EncodePNG(writer, snapshot.Image); err != nil {
			return fmt.Errorf("write image document %q: %w", path, err)
		}
	default:
		return fmt.Errorf("write document %q: unsupported snapshot kind %d", path, snapshot.Kind)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("write document %q before sync: %w", path, err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync temporary file for %q: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary file for %q: %w", path, err)
	}
	closed = true
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("write document %q before replace: %w", path, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace destination %q atomically: %w", path, err)
	}
	committed = true
	return nil
}

func normalizeCRLF(text string) []byte {
	unix := strings.ReplaceAll(text, "\r\n", "\n")
	unix = strings.ReplaceAll(unix, "\r", "\n")
	return []byte(strings.ReplaceAll(unix, "\n", "\r\n"))
}

func writeChunks(w io.Writer, data []byte) error {
	for len(data) > 0 {
		chunkSize := saveWriteChunkSize
		if len(data) < chunkSize {
			chunkSize = len(data)
		}
		n, err := w.Write(data[:chunkSize])
		if err != nil {
			return err
		}
		if n != chunkSize {
			return io.ErrShortWrite
		}
		data = data[chunkSize:]
	}
	return nil
}

type writeContext struct {
	ctx    context.Context
	writer io.Writer
}

func (w *writeContext) Write(p []byte) (int, error) {
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
