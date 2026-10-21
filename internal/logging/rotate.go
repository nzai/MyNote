package logging

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const logTimeLayout = "20060102"

type RotatingWriter struct {
	mu          sync.Mutex
	dir         string
	prefix      string
	maxBytes    int64
	maxFiles    int
	now         func() time.Time
	file        *os.File
	currentDate string
	currentPath string
	currentSize int64
	closed      bool
}

func NewRotatingWriter(
	dir string,
	prefix string,
	maxBytes int64,
	maxFiles int,
	now func() time.Time,
) (*RotatingWriter, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("create rotating writer: log directory is empty")
	}
	if prefix == "" || filepath.Base(prefix) != prefix || strings.ContainsAny(prefix, `*?[\`) {
		return nil, fmt.Errorf("create rotating writer: invalid file prefix %q", prefix)
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("create rotating writer: max bytes must be positive, got %d", maxBytes)
	}
	if maxFiles <= 0 {
		return nil, fmt.Errorf("create rotating writer: max files must be positive, got %d", maxFiles)
	}
	if now == nil {
		now = time.Now
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create rotating writer directory %q: %w", dir, err)
	}

	return &RotatingWriter{
		dir:      dir,
		prefix:   prefix,
		maxBytes: maxBytes,
		maxFiles: maxFiles,
		now:      now,
	}, nil
}

func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, fs.ErrClosed
	}
	if len(p) == 0 {
		return 0, nil
	}
	if err := w.ensureCurrent(); err != nil {
		return 0, err
	}

	// A single record must remain contiguous even when it exceeds maxBytes.
	if w.currentSize > 0 && int64(len(p)) > w.maxBytes-w.currentSize {
		if err := w.closeCurrent(); err != nil {
			return 0, err
		}
		if err := w.rotateCurrent(); err != nil {
			return 0, err
		}
		if err := w.openCurrent(); err != nil {
			return 0, err
		}
	}

	n, err := w.file.Write(p)
	w.currentSize += int64(n)
	if err != nil {
		return n, fmt.Errorf("write log file %q: %w", w.currentPath, err)
	}
	return n, nil
}

func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}
	w.closed = true
	if err := w.closeCurrent(); err != nil {
		return err
	}
	return nil
}

func (w *RotatingWriter) ensureCurrent() error {
	date := w.now().Format(logTimeLayout)
	if w.file != nil && w.currentDate == date {
		return nil
	}
	if w.file != nil {
		if err := w.closeCurrent(); err != nil {
			return err
		}
	}

	nextPath := filepath.Join(w.dir, fmt.Sprintf("%s-%s.jsonl", w.prefix, date))
	_, statErr := os.Stat(nextPath)
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return fmt.Errorf("stat log file %q: %w", nextPath, statErr)
	}
	if errors.Is(statErr, fs.ErrNotExist) {
		if err := w.pruneForNewFile(nextPath); err != nil {
			return err
		}
	}

	w.currentDate = date
	w.currentPath = nextPath
	return w.openCurrent()
}

func (w *RotatingWriter) openCurrent() error {
	file, err := os.OpenFile(w.currentPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open log file %q: %w", w.currentPath, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("stat open log file %q: %w", w.currentPath, err)
	}

	w.file = file
	w.currentSize = info.Size()
	return nil
}

func (w *RotatingWriter) closeCurrent() error {
	if w.file == nil {
		return nil
	}
	path := w.currentPath
	err := w.file.Close()
	w.file = nil
	if err != nil {
		return fmt.Errorf("close log file %q: %w", path, err)
	}
	return nil
}

func (w *RotatingWriter) rotateCurrent() error {
	if w.maxFiles == 1 {
		if err := removeIfExists(w.currentPath); err != nil {
			return fmt.Errorf("remove rotated log file %q: %w", w.currentPath, err)
		}
		return nil
	}

	oldest := w.currentPath + "." + strconv.Itoa(w.maxFiles-1)
	if err := removeIfExists(oldest); err != nil {
		return fmt.Errorf("remove oldest rotated log file %q: %w", oldest, err)
	}
	for index := w.maxFiles - 2; index >= 1; index-- {
		source := w.currentPath + "." + strconv.Itoa(index)
		target := w.currentPath + "." + strconv.Itoa(index+1)
		if err := renameIfExists(source, target); err != nil {
			return fmt.Errorf("shift rotated log file %q to %q: %w", source, target, err)
		}
	}
	target := w.currentPath + ".1"
	if err := os.Rename(w.currentPath, target); err != nil {
		return fmt.Errorf("rotate log file %q to %q: %w", w.currentPath, target, err)
	}
	return nil
}

func (w *RotatingWriter) pruneForNewFile(newPath string) error {
	groups, total, err := w.logGroups()
	if err != nil {
		return err
	}
	limit := w.maxFiles - 1
	if total <= limit {
		return nil
	}

	bases := make([]string, 0, len(groups))
	for base := range groups {
		bases = append(bases, base)
	}
	sort.Strings(bases)
	newBase := baseLogPath(newPath)
	for _, base := range bases {
		if total <= limit {
			return nil
		}
		if base == newBase {
			continue
		}
		for _, path := range groups[base] {
			if err := removeIfExists(path); err != nil {
				return fmt.Errorf("prune old log file %q: %w", path, err)
			}
			total--
		}
	}

	if total <= limit {
		return nil
	}
	sameDay := append([]string(nil), groups[newBase]...)
	sort.Slice(sameDay, func(i, j int) bool {
		return suffixIndex(sameDay[i]) > suffixIndex(sameDay[j])
	})
	for _, path := range sameDay {
		if total <= limit {
			break
		}
		if err := removeIfExists(path); err != nil {
			return fmt.Errorf("prune old log file %q: %w", path, err)
		}
		total--
	}
	return nil
}

func (w *RotatingWriter) logGroups() (map[string][]string, int, error) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return nil, 0, fmt.Errorf("read log directory %q: %w", w.dir, err)
	}

	groups := make(map[string][]string)
	total := 0
	for _, entry := range entries {
		if entry.IsDir() || !w.matchesLogName(entry.Name()) {
			continue
		}
		path := filepath.Join(w.dir, entry.Name())
		base := baseLogPath(path)
		groups[base] = append(groups[base], path)
		total++
	}
	return groups, total, nil
}

func (w *RotatingWriter) matchesLogName(name string) bool {
	if !strings.HasPrefix(name, w.prefix+"-") {
		return false
	}
	index := strings.Index(name, ".jsonl")
	if index < len(w.prefix)+1 {
		return false
	}
	remainder := name[index+len(".jsonl"):]
	if remainder == "" {
		return true
	}
	if !strings.HasPrefix(remainder, ".") {
		return false
	}
	_, err := strconv.Atoi(remainder[1:])
	return err == nil
}

func baseLogPath(path string) string {
	if index := strings.Index(path, ".jsonl"); index >= 0 {
		return path[:index+len(".jsonl")]
	}
	return path
}

func suffixIndex(path string) int {
	index := strings.LastIndex(path, ".jsonl.")
	if index < 0 {
		return 0
	}
	value, err := strconv.Atoi(path[index+len(".jsonl."):])
	if err != nil {
		return 0
	}
	return value
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func renameIfExists(source, target string) error {
	if err := os.Rename(source, target); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else {
		return err
	}
}
