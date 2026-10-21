package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/nzai/MyNote/internal/core"
)

var generatedCaptureID = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}-[0-9a-f]{8}$`)

type InvalidRecord struct {
	Path string
	Err  error
}

type CleanupResult struct {
	Removed        int
	BytesRemoved   int64
	RemainingBytes int64
	Invalid        []InvalidRecord
}

type managedRecord struct {
	id          string
	path        string
	metadata    metadata
	diskBytes   int64
	contentSize int64
}

func (s *Store) Enforce(ctx context.Context) (CleanupResult, error) {
	if ctx == nil {
		return CleanupResult{}, errors.New("enforce cache retention: context is nil")
	}
	root, err := validateCacheRoot(s.root)
	if err != nil {
		return CleanupResult{}, fmt.Errorf("enforce cache retention: %w", err)
	}
	if s.maxAge <= 0 || s.maxBytes <= 0 {
		return CleanupResult{}, fmt.Errorf(
			"enforce cache retention: invalid policy max_age=%s max_bytes=%d",
			s.maxAge,
			s.maxBytes,
		)
	}
	if err := ctx.Err(); err != nil {
		return CleanupResult{}, fmt.Errorf("enforce cache retention: %w", err)
	}

	records, invalid, err := s.scanRecords(ctx, root)
	if err != nil {
		return CleanupResult{}, err
	}
	result := CleanupResult{Invalid: invalid}
	cutoff := s.now().Add(-s.maxAge)

	remaining := make([]managedRecord, 0, len(records))
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("enforce cache retention before expiration delete: %w", err)
		}
		if !record.metadata.CapturedAt.Before(cutoff) {
			remaining = append(remaining, record)
			continue
		}
		if err := removeManagedRecord(root, record.path); err != nil {
			return result, fmt.Errorf("delete expired cache record %q: %w", record.id, err)
		}
		result.Removed++
		result.BytesRemoved += record.diskBytes
	}

	var total int64
	for _, record := range remaining {
		total += record.diskBytes
	}
	sort.Slice(remaining, func(i, j int) bool {
		if remaining[i].metadata.CapturedAt.Equal(remaining[j].metadata.CapturedAt) {
			return remaining[i].id < remaining[j].id
		}
		return remaining[i].metadata.CapturedAt.Before(remaining[j].metadata.CapturedAt)
	})
	for len(remaining) > 0 && total > s.maxBytes {
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("enforce cache retention before size delete: %w", err)
		}
		record := remaining[0]
		remaining = remaining[1:]
		if err := removeManagedRecord(root, record.path); err != nil {
			return result, fmt.Errorf("delete oversized cache record %q: %w", record.id, err)
		}
		total -= record.diskBytes
		result.Removed++
		result.BytesRemoved += record.diskBytes
	}

	if err := ctx.Err(); err != nil {
		return result, fmt.Errorf("enforce cache retention before rescan: %w", err)
	}
	rescanned, invalid, err := s.scanRecords(ctx, root)
	if err != nil {
		return result, err
	}
	result.Invalid = invalid
	for _, record := range rescanned {
		result.RemainingBytes += record.diskBytes
	}
	return result, nil
}

func (s *Store) Clear(ctx context.Context) error {
	if ctx == nil {
		return errors.New("clear cache: context is nil")
	}
	root, err := validateClearRoot(s.root)
	if err != nil {
		return fmt.Errorf("clear cache: %w", err)
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read cache root %q: %w", root, err)
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("clear cache before deleting %q: %w", entry.Name(), err)
		}
		target := filepath.Join(root, entry.Name())
		if err := ensureDirectChild(root, target); err != nil {
			return fmt.Errorf("clear cache entry %q: %w", entry.Name(), err)
		}
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("remove cache entry %q: %w", target, err)
		}
	}
	return nil
}

func (s *Store) scanRecords(
	ctx context.Context,
	root string,
) ([]managedRecord, []InvalidRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan cache records: %w", err)
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read cache root %q: %w", root, err)
	}

	var records []managedRecord
	var invalid []InvalidRecord
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, nil, fmt.Errorf("scan cache records: %w", err)
		}
		if !entry.IsDir() || !generatedCaptureID.MatchString(entry.Name()) {
			continue
		}
		path := filepath.Join(root, entry.Name())
		record, err := s.readManagedRecord(entry.Name(), path)
		if err != nil {
			invalid = append(invalid, InvalidRecord{Path: path, Err: err})
			continue
		}
		records = append(records, record)
	}
	return records, invalid, nil
}

func (s *Store) readManagedRecord(id, path string) (managedRecord, error) {
	metaBytes, err := os.ReadFile(filepath.Join(path, "meta.json"))
	if err != nil {
		return managedRecord{}, fmt.Errorf("read metadata: %w", err)
	}
	var meta metadata
	decoder := json.NewDecoder(bytes.NewReader(metaBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&meta); err != nil {
		return managedRecord{}, fmt.Errorf("decode metadata: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return managedRecord{}, fmt.Errorf("decode metadata trailing data: %w", err)
	}
	if meta.ID != id {
		return managedRecord{}, fmt.Errorf("metadata ID %q does not match directory", meta.ID)
	}
	kind, err := parseKind(meta.Kind)
	if err != nil {
		return managedRecord{}, err
	}
	if meta.CapturedAt.IsZero() {
		return managedRecord{}, errors.New("metadata captured_at is zero")
	}
	if meta.Bytes < 0 {
		return managedRecord{}, fmt.Errorf("metadata bytes is negative: %d", meta.Bytes)
	}
	expectedContent, err := contentNameForKind(kind)
	if err != nil {
		return managedRecord{}, err
	}
	if meta.ContentFile != expectedContent {
		return managedRecord{}, fmt.Errorf("metadata content_file %q, want %q", meta.ContentFile, expectedContent)
	}
	contentInfo, err := os.Stat(filepath.Join(path, meta.ContentFile))
	if err != nil {
		return managedRecord{}, fmt.Errorf("stat content file %q: %w", meta.ContentFile, err)
	}
	if !contentInfo.Mode().IsRegular() {
		return managedRecord{}, fmt.Errorf("content file %q is not a regular file", meta.ContentFile)
	}
	if contentInfo.Size() != meta.Bytes {
		return managedRecord{}, fmt.Errorf(
			"content size %d does not match metadata %d",
			contentInfo.Size(),
			meta.Bytes,
		)
	}
	diskBytes, err := recordDiskSize(path)
	if err != nil {
		return managedRecord{}, err
	}
	return managedRecord{
		id:          id,
		path:        path,
		metadata:    meta,
		diskBytes:   diskBytes,
		contentSize: contentInfo.Size(),
	}, nil
}

func recordDiskSize(root string) (int64, error) {
	var total int64
	const maxInt64 = int64(^uint64(0) >> 1)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("cache record contains symbolic link %q", path)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("cache record contains non-regular file %q", path)
		}
		if info.Size() > 0 && total > maxInt64-info.Size() {
			return errors.New("cache record size overflow")
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("calculate cache record size %q: %w", root, err)
	}
	return total, nil
}

func parseKind(value string) (core.Kind, error) {
	switch value {
	case "text":
		return core.KindText, nil
	case "image":
		return core.KindImage, nil
	default:
		return 0, fmt.Errorf("unknown metadata kind %q", value)
	}
}

func contentNameForKind(kind core.Kind) (string, error) {
	switch kind {
	case core.KindText:
		return "content.txt", nil
	case core.KindImage:
		return "content.png", nil
	default:
		return "", fmt.Errorf("unsupported snapshot kind %d", kind)
	}
}

func ensureDirectChild(root, target string) error {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve cache root %q: %w", root, err)
	}
	absoluteTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve cache target %q: %w", target, err)
	}
	relative, err := filepath.Rel(filepath.Clean(absoluteRoot), filepath.Clean(absoluteTarget))
	if err != nil {
		return fmt.Errorf("compare cache paths: %w", err)
	}
	if relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(relative) ||
		filepath.Dir(relative) != "." {
		return fmt.Errorf("target %q is not a direct child of cache root %q", target, root)
	}
	return nil
}

func removeManagedRecord(root, path string) error {
	if err := ensureDirectChild(root, path); err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return nil
}
