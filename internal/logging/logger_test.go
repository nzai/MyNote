package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScopeFilterDropsUnlistedDebug(t *testing.T) {
	var buf bytes.Buffer
	factory := NewWithWriter(&buf, slog.LevelInfo, map[string]bool{"cache": true})

	factory.Scope("ui").Debug("hidden")
	factory.Scope("cache").Debug("visible")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], `"msg":"visible"`) {
		t.Fatalf("unexpected logs: %q", buf.String())
	}
}

func TestNewWritesScopedJSONLinesToDatedFile(t *testing.T) {
	dir := t.TempDir()
	now := func() time.Time {
		return time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local)
	}
	factory, err := New(Options{
		Dir:         dir,
		Prefix:      "clipnote",
		MaxBytes:    4096,
		MaxFiles:    2,
		Level:       slog.LevelInfo,
		DebugScopes: []string{"cache"},
		Now:         now,
	})
	if err != nil {
		t.Fatal(err)
	}
	factory.Scope("cache").Debug("visible", "task_id", "task-1", "input_id", "capture-1")
	if err := factory.Close(); err != nil {
		t.Fatal(err)
	}
	if err := factory.Close(); err != nil {
		t.Fatalf("second close should be idempotent: %v", err)
	}

	path := filepath.Join(dir, "clipnote-20261021.jsonl")
	line := strings.TrimSpace(readFile(t, path))
	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("decode log line %q: %v", line, err)
	}
	if entry["scope"] != "cache" || entry["task_id"] != "task-1" || entry["input_id"] != "capture-1" {
		t.Fatalf("unexpected fields: %#v", entry)
	}
	if entry["level"] != "DEBUG" || entry["msg"] != "visible" {
		t.Fatalf("unexpected event: %#v", entry)
	}
}

func TestRotatingWriterDoesNotRotateBelowSizeLimit(t *testing.T) {
	dir := t.TempDir()
	now := func() time.Time {
		return time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local)
	}
	writer, err := NewRotatingWriter(dir, "clipnote", 16, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("1234567890")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "clipnote-*.jsonl*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one current file, got %v", files)
	}
	if got := readFile(t, files[0]); got != "1234567890" {
		t.Fatalf("file content = %q", got)
	}
}

func TestRotatingWriterRotatesAtSizeLimit(t *testing.T) {
	dir := t.TempDir()
	now := func() time.Time {
		return time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local)
	}
	writer, err := NewRotatingWriter(dir, "clipnote", 16, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("1234567890")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("abcdefghij")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	current := filepath.Join(dir, "clipnote-20261021.jsonl")
	previous := current + ".1"
	if got := readFile(t, previous); got != "1234567890" {
		t.Fatalf("rotated content = %q", got)
	}
	if got := readFile(t, current); got != "abcdefghij" {
		t.Fatalf("current content = %q", got)
	}
}

func TestRotatingWriterKeepsConfiguredFileCount(t *testing.T) {
	dir := t.TempDir()
	now := func() time.Time {
		return time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local)
	}
	writer, err := NewRotatingWriter(dir, "clipnote", 16, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range []string{"1234567890", "abcdefghij", "klmnopqrst"} {
		if _, err := writer.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "clipnote-*.jsonl*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected two files, got %v", files)
	}
	current := filepath.Join(dir, "clipnote-20261021.jsonl")
	if got := readFile(t, current+".1"); got != "abcdefghij" {
		t.Fatalf("rotated content = %q", got)
	}
	if got := readFile(t, current); got != "klmnopqrst" {
		t.Fatalf("current content = %q", got)
	}
}

func TestRotatingWriterStartsNewFileOnLocalDateChange(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 21, 23, 59, 0, 0, time.Local)
	clock := func() time.Time { return now }
	writer, err := NewRotatingWriter(dir, "clipnote", 1024, 2, clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("first")); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := writer.Write([]byte("second")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	first := filepath.Join(dir, "clipnote-20261021.jsonl")
	second := filepath.Join(dir, "clipnote-20261022.jsonl")
	if got := readFile(t, first); got != "first" {
		t.Fatalf("first file content = %q", got)
	}
	if got := readFile(t, second); got != "second" {
		t.Fatalf("second file content = %q", got)
	}
}

func TestRotatingWriterKeepsFileCountAcrossDates(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local)
	clock := func() time.Time { return now }
	writer, err := NewRotatingWriter(dir, "clipnote", 1024, 2, clock)
	if err != nil {
		t.Fatal(err)
	}
	for _, day := range []string{"first", "second", "third"} {
		if _, err := writer.Write([]byte(day)); err != nil {
			t.Fatal(err)
		}
		now = now.Add(24 * time.Hour)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	oldest := filepath.Join(dir, "clipnote-20261021.jsonl")
	if _, err := os.Stat(oldest); !os.IsNotExist(err) {
		t.Fatalf("oldest log should be removed, stat error = %v", err)
	}
	for _, path := range []string{
		filepath.Join(dir, "clipnote-20261022.jsonl"),
		filepath.Join(dir, "clipnote-20261023.jsonl"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected retained file %q: %v", path, err)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
