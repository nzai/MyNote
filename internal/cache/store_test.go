package cache

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nzai/MyNote/internal/core"
)

func TestStoreSaveWritesTextAndMetadataAtomically(t *testing.T) {
	root := t.TempDir()
	store := NewStore(Options{
		Root:     root,
		MaxAge:   7 * 24 * time.Hour,
		MaxBytes: 500 << 20,
		Clock:    fixedClock,
	})

	record, err := store.Save(context.Background(), core.Snapshot{
		CaptureID:    "20261021T153012-7f3a",
		Kind:         core.KindText,
		Text:         "hello",
		CapturedAt:   fixedClock(),
		SourceFormat: "CF_UNICODETEXT",
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.ID != "20261021T153012-7f3a" {
		t.Fatalf("record ID = %q", record.ID)
	}

	contentPath := filepath.Join(root, record.ID, "content.txt")
	if got := readTestFile(t, contentPath); got != "hello" {
		t.Fatalf("content = %q", got)
	}
	metaPath := filepath.Join(root, record.ID, "meta.json")
	var meta map[string]any
	if err := json.Unmarshal([]byte(readTestFile(t, metaPath)), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["id"] != record.ID || meta["kind"] != "text" || meta["content_file"] != "content.txt" {
		t.Fatalf("unexpected metadata %#v", meta)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".partial-") {
			t.Fatalf("partial directory leaked: %s", entry.Name())
		}
	}
}

func TestStoreSavesImageAsPNG(t *testing.T) {
	root := t.TempDir()
	store := NewStore(Options{Root: root, Clock: fixedClock})
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 255})

	record, err := store.Save(context.Background(), core.Snapshot{
		CaptureID:    "20261021T153012-7f3a",
		Kind:         core.KindImage,
		Image:        img,
		CapturedAt:   fixedClock(),
		SourceFormat: "CF_DIB",
	})
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(filepath.Join(root, record.ID, "content.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := png.Decode(file); err != nil {
		t.Fatalf("decode cached PNG: %v", err)
	}
}

func TestStoreRejectsPathTraversalCaptureID(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "cache")
	store := NewStore(Options{Root: root, Clock: fixedClock})

	_, err := store.Save(context.Background(), core.Snapshot{
		CaptureID:    "../outside",
		Kind:         core.KindText,
		Text:         "unsafe",
		CapturedAt:   fixedClock(),
		SourceFormat: "CF_UNICODETEXT",
	})
	if err == nil {
		t.Fatal("expected invalid capture ID error")
	}
	assertMissing(t, filepath.Join(parent, "outside"))
}

func TestStoreFailureRemovesPartialDirectory(t *testing.T) {
	root := t.TempDir()
	store := NewStore(Options{Root: root, Clock: fixedClock})

	_, err := store.Save(context.Background(), core.Snapshot{
		CaptureID:    "20261021T153012-7f3a",
		Kind:         core.KindImage,
		CapturedAt:   fixedClock(),
		SourceFormat: "CF_DIB",
	})
	if err == nil {
		t.Fatal("expected missing image error")
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed save left entries: %v", entries)
	}
}

func TestStoreDoesNotOverwriteExistingRecord(t *testing.T) {
	root := t.TempDir()
	store := NewStore(Options{Root: root, Clock: fixedClock})
	id := "20261021T153012-7f3a"
	saveTestRecord(t, store, id, fixedClock(), "first")

	_, err := store.Save(context.Background(), core.Snapshot{
		CaptureID:    id,
		Kind:         core.KindText,
		Text:         "second",
		CapturedAt:   fixedClock(),
		SourceFormat: "CF_UNICODETEXT",
	})
	if err == nil {
		t.Fatal("expected duplicate ID error")
	}
	if got := readTestFile(t, filepath.Join(root, id, "content.txt")); got != "first" {
		t.Fatalf("existing content changed to %q", got)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".partial-") {
			t.Fatalf("partial directory leaked: %s", entry.Name())
		}
	}
}

func fixedClock() time.Time {
	return time.Date(2026, 10, 21, 15, 30, 12, 0, time.Local)
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
