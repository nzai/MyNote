//go:build windows

package ui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/nzai/MyNote/internal/core"
)

func TestReadClipboardOrBlankFallsBackForUnusableContent(t *testing.T) {
	readErr := errors.New("capture clipboard format: unsupported clipboard format")
	blank := core.Snapshot{
		CaptureID: "blank-capture",
		Kind:      core.KindText,
	}

	result, err := readClipboardOrBlank(
		clipboardReaderFunc(func(context.Context) (core.Snapshot, error) {
			return core.Snapshot{}, readErr
		}),
		func() (core.Snapshot, error) {
			return blank, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot.Kind != core.KindText ||
		result.Snapshot.Text != "" ||
		result.Snapshot.CaptureID != "blank-capture" {
		t.Fatalf("fallback snapshot = %#v, want blank text", result.Snapshot)
	}
	if !errors.Is(result.Warning, readErr) {
		t.Fatalf("warning = %v, want original read error", result.Warning)
	}
}

func TestReadClipboardOrBlankKeepsSuccessfulSnapshot(t *testing.T) {
	success := core.Snapshot{
		CaptureID: "text-capture",
		Kind:      core.KindText,
		Text:      "clipboard text",
	}

	result, err := readClipboardOrBlank(
		clipboardReaderFunc(func(context.Context) (core.Snapshot, error) {
			return success, nil
		}),
		func() (core.Snapshot, error) {
			t.Fatal("fallback must not run after a successful read")
			return core.Snapshot{}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Snapshot, success) {
		t.Fatalf("snapshot = %#v, want %#v", result.Snapshot, success)
	}
	if result.Warning != nil {
		t.Fatalf("warning = %v, want nil", result.Warning)
	}
}

func TestReadClipboardOrBlankKeepsCapturedSnapshotWhenCleanupFails(t *testing.T) {
	closeErr := errors.New("close clipboard failed")
	success := core.Snapshot{
		CaptureID: "captured-text",
		Kind:      core.KindText,
		Text:      "clipboard text",
	}

	result, err := readClipboardOrBlank(
		clipboardReaderFunc(func(context.Context) (core.Snapshot, error) {
			return success, closeErr
		}),
		func() (core.Snapshot, error) {
			t.Fatal("fallback must not replace a successfully captured snapshot")
			return core.Snapshot{}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Snapshot, success) {
		t.Fatalf("snapshot = %#v, want %#v", result.Snapshot, success)
	}
	if !errors.Is(result.Warning, closeErr) {
		t.Fatalf("warning = %v, want cleanup error", result.Warning)
	}
}

func TestReadClipboardOrBlankReportsFallbackFailure(t *testing.T) {
	readErr := errors.New("clipboard is busy")
	blankErr := errors.New("capture ID source failed")

	_, err := readClipboardOrBlank(
		clipboardReaderFunc(func(context.Context) (core.Snapshot, error) {
			return core.Snapshot{}, readErr
		}),
		func() (core.Snapshot, error) {
			return core.Snapshot{}, blankErr
		},
	)
	if !errors.Is(err, readErr) || !errors.Is(err, blankErr) {
		t.Fatalf("error = %v, want both read and fallback errors", err)
	}
}

type clipboardReaderFunc func(context.Context) (core.Snapshot, error)

func (f clipboardReaderFunc) Read(ctx context.Context) (core.Snapshot, error) {
	return f(ctx)
}
