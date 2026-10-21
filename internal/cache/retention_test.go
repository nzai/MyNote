package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nzai/MyNote/internal/core"
)

func TestEnforceDeletesExpiredThenOldest(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.Local)
	writer := NewStore(Options{
		Root:     root,
		MaxAge:   365 * 24 * time.Hour,
		MaxBytes: 1 << 30,
		Clock:    func() time.Time { return now },
	})

	expiredAt := now.Add(-8 * 24 * time.Hour)
	oldFreshAt := now.Add(-2 * time.Hour)
	newFreshAt := now.Add(-1 * time.Hour)
	expiredID := "20261013T120000-aaaa0001"
	oldFreshID := "20261021T100000-aaaa0002"
	newFreshID := "20261021T110000-aaaa0003"
	saveTestRecord(t, writer, expiredID, expiredAt, "expired")
	saveTestRecord(t, writer, oldFreshID, oldFreshAt, "older")
	saveTestRecord(t, writer, newFreshID, newFreshAt, "newer")

	expiredSize := directorySize(t, filepath.Join(root, expiredID))
	oldFreshSize := directorySize(t, filepath.Join(root, oldFreshID))
	newFreshSize := directorySize(t, filepath.Join(root, newFreshID))
	enforcer := NewStore(Options{
		Root:     root,
		MaxAge:   7 * 24 * time.Hour,
		MaxBytes: newFreshSize,
		Clock:    func() time.Time { return now },
	})

	result, err := enforcer.Enforce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 2 {
		t.Fatalf("removed = %d, want 2", result.Removed)
	}
	if result.BytesRemoved != expiredSize+oldFreshSize {
		t.Fatalf("bytes removed = %d, want %d", result.BytesRemoved, expiredSize+oldFreshSize)
	}
	if result.RemainingBytes != newFreshSize {
		t.Fatalf("remaining bytes = %d, want %d", result.RemainingBytes, newFreshSize)
	}
	assertMissing(t, filepath.Join(root, expiredID))
	assertMissing(t, filepath.Join(root, oldFreshID))
	if _, err := os.Stat(filepath.Join(root, newFreshID, "meta.json")); err != nil {
		t.Fatalf("newest fresh record should remain: %v", err)
	}
}

func TestEnforceReportsInvalidMetadataAndPreservesUnrelatedData(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "cache")
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.Local)
	store := NewStore(Options{
		Root:     root,
		MaxAge:   7 * 24 * time.Hour,
		MaxBytes: 1 << 20,
		Clock:    func() time.Time { return now },
	})

	invalidID := "20261021T100000-deadbeef"
	invalidDir := filepath.Join(root, invalidID)
	if err := os.MkdirAll(invalidDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(invalidDir, "meta.json"), []byte(`{"id":"wrong"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	unrelatedDir := filepath.Join(root, "keep-me")
	if err := os.MkdirAll(unrelatedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	outsidePath := filepath.Join(parent, "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := store.Enforce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Invalid) != 1 || result.Invalid[0].Path != invalidDir {
		t.Fatalf("invalid records = %#v", result.Invalid)
	}
	for _, path := range []string{invalidDir, unrelatedDir, outsidePath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("unexpected removal of %q: %v", path, err)
		}
	}
}

func TestClearRemovesOnlyDirectChildren(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "cache")
	store := NewStore(Options{Root: root, Clock: fixedClock})
	if err := os.MkdirAll(filepath.Join(root, "record"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".partial-123"), 0o700); err != nil {
		t.Fatal(err)
	}
	outsidePath := filepath.Join(parent, "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := store.Clear(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("cache root still has entries: %v", entries)
	}
	if _, err := os.Stat(outsidePath); err != nil {
		t.Fatalf("outside file was removed: %v", err)
	}
}

func TestCleanupPartialsRemovesOnlyPartialDirectories(t *testing.T) {
	root := t.TempDir()
	partial := filepath.Join(root, ".partial-crashed")
	if err := os.MkdirAll(partial, 0o700); err != nil {
		t.Fatal(err)
	}
	regular := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(regular, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewStore(Options{Root: root})
	removed, err := store.CleanupPartials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	assertMissing(t, partial)
	if _, err := os.Stat(regular); err != nil {
		t.Fatalf("regular cache-root file was removed: %v", err)
	}
}

func TestClearRejectsEmptyRelativeAndVolumeRoots(t *testing.T) {
	if err := NewStore(Options{Root: ""}).Clear(context.Background()); err == nil {
		t.Fatal("empty cache root must be rejected")
	}
	if err := NewStore(Options{Root: "."}).Clear(context.Background()); err == nil {
		t.Fatal("relative cache root must be rejected")
	}

	volume := filepath.VolumeName(t.TempDir())
	if volume == "" {
		t.Skip("platform has no volume root")
	}
	volumeRoot := volume + string(filepath.Separator)
	if _, err := validateClearRoot(volumeRoot); err == nil {
		t.Fatalf("volume root %q must be rejected", volumeRoot)
	}
}

func TestEnforceHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := NewStore(Options{Root: t.TempDir(), Clock: fixedClock})

	_, err := store.Enforce(ctx)
	if err == nil {
		t.Fatal("expected context cancellation")
	}
}

func saveTestRecord(t *testing.T, store *Store, id string, at time.Time, text string) {
	t.Helper()
	_, err := store.Save(context.Background(), core.Snapshot{
		CaptureID:    id,
		Kind:         core.KindText,
		Text:         text,
		CapturedAt:   at,
		SourceFormat: "CF_UNICODETEXT",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func directorySize(t *testing.T, dir string) int64 {
	t.Helper()
	var total int64
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return total
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("path %q still exists or stat failed: %v", path, err)
	}
}
