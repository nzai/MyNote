//go:build windows

package ui

import (
	"context"
	"testing"
	"time"

	"github.com/nzai/MyNote/internal/core"
	"github.com/rodrigocfd/windigo/win"
)

func TestAsyncSaveKeepsNewerEditsDirty(t *testing.T) {
	release := make(chan struct{})
	document := newSaveTestDocument(func(context.Context) (DocumentSaveResult, error) {
		<-release
		return DocumentSaveResult{Path: `C:\Temp\captured.txt`, Text: "captured"}, nil
	})
	document.saveState.SetText("captured")

	if !document.saveDocument(false) {
		t.Fatal("expected save job to start")
	}
	document.saveState.SetText("newer")
	close(release)
	waitForSaveResult(t, document)
	document.handleSaveCompletion()

	if !document.saveState.Dirty() {
		t.Fatal("newer edits must remain dirty after the captured text is saved")
	}
	if document.saveState.Path() != `C:\Temp\captured.txt` {
		t.Fatalf("saved path = %q", document.saveState.Path())
	}
}

func TestAsyncSaveCancelKeepsDirtyState(t *testing.T) {
	document := newSaveTestDocument(func(ctx context.Context) (DocumentSaveResult, error) {
		<-ctx.Done()
		return DocumentSaveResult{}, ctx.Err()
	})
	document.saveState.SetText("edited")

	if !document.saveDocument(false) {
		t.Fatal("expected save job to start")
	}
	document.saveCancel()
	waitForSaveResult(t, document)
	document.handleSaveCompletion()

	if document.saveStatus != "保存已取消" || !document.saveState.Dirty() {
		t.Fatalf("status = %q, dirty = %v", document.saveStatus, document.saveState.Dirty())
	}
}

func TestAsyncSaveCancelRevokesDeferredClose(t *testing.T) {
	document := newSaveTestDocument(func(ctx context.Context) (DocumentSaveResult, error) {
		<-ctx.Done()
		return DocumentSaveResult{}, ctx.Err()
	})
	document.saveState.SetText("edited")
	document.closeAfterSave = true
	closeCanceled := false
	document.onCloseCanceled = func(*Document) {
		closeCanceled = true
	}

	if !document.saveDocument(false) {
		t.Fatal("expected save job to start")
	}
	document.saveCancel()
	waitForSaveResult(t, document)
	document.handleSaveCompletion()

	if !closeCanceled {
		t.Fatal("canceling a close-triggered save must revoke the exit intent")
	}
	if document.closeAfterSave {
		t.Fatal("deferred close must be cleared after save cancellation")
	}
}

func TestSuppressClosePromptClosesDirtyDocument(t *testing.T) {
	document := newSaveTestDocument(func(context.Context) (DocumentSaveResult, error) {
		return DocumentSaveResult{}, nil
	})
	document.saveState.SetText("edited")
	document.editorSettings.SuppressClosePrompt = true

	if !document.confirmClose() {
		t.Fatal("dirty document must close without prompting when suppression is enabled")
	}
}

func newSaveTestDocument(run func(context.Context) (DocumentSaveResult, error)) *Document {
	snapshot := core.Snapshot{CaptureID: "save-test", Kind: core.KindText}
	return &Document{
		snapshot:     snapshot,
		saveState:    core.NewSaveState(""),
		saveProgress: newProgressState(time.Millisecond),
		save: func(_ win.HWND, _ bool) (DocumentSaveJob, error) {
			return DocumentSaveJob{Run: run}, nil
		},
	}
}

func waitForSaveResult(t *testing.T, document *Document) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		document.saveResultMu.Lock()
		ready := document.saveResult != nil
		document.saveResultMu.Unlock()
		if ready {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("save result was not produced before timeout")
}
