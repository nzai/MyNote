//go:build windows

package ui

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nzai/MyNote/internal/core"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

func TestSaveCompletionShowsNotice(t *testing.T) {
	document := newSaveTestDocument(func(context.Context) (DocumentSaveResult, error) {
		return DocumentSaveResult{Path: `C:\Temp\notes.txt`, Text: "captured"}, nil
	})
	document.saveState.SetText("captured")

	if !document.saveDocument(false) {
		t.Fatal("expected save job to start")
	}
	waitForSaveResult(t, document)
	document.handleSaveCompletion()

	status := document.statusText()
	if !strings.Contains(status, "已保存到 notes.txt") {
		t.Fatalf("status = %q, want a save notice naming the saved file", status)
	}
	if !strings.Contains(status, "Shift") {
		t.Fatalf("status = %q, want the Shift save-as hint", status)
	}
}

func TestSaveNoticeExpires(t *testing.T) {
	document := newSaveTestDocument(func(context.Context) (DocumentSaveResult, error) {
		return DocumentSaveResult{}, nil
	})
	document.saveNotice = "已保存到 notes.txt · 10:00:00"
	document.saveNoticeUntil = time.Now().Add(-time.Second)

	if status := document.statusText(); strings.Contains(status, "已保存到") {
		t.Fatalf("status = %q, want the expired notice to disappear", status)
	}
}

func TestSaveNoticeKeepsPriorStatusAfterExpiry(t *testing.T) {
	document := newSaveTestDocument(func(context.Context) (DocumentSaveResult, error) {
		return DocumentSaveResult{Path: `C:\Temp\notes.txt`, Text: "captured"}, nil
	})
	document.saveState.SetText("captured")

	if !document.saveDocument(false) {
		t.Fatal("expected save job to start")
	}
	waitForSaveResult(t, document)
	document.handleSaveCompletion()

	document.saveNoticeUntil = time.Now().Add(-time.Second)
	status := document.statusText()
	if strings.Contains(status, "已保存到") {
		t.Fatalf("status = %q, want the notice to be gone", status)
	}
	if !strings.Contains(status, "已保存") {
		t.Fatalf("status = %q, want the persistent saved state", status)
	}
	if !strings.Contains(status, `C:\Temp\notes.txt`) {
		t.Fatalf("status = %q, want the saved path", status)
	}
}

func TestSaveNoticeTimerClearsNotice(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 save notice timer test")
	}
	lockGUIThread(t)
	document, err := NewDocument(DocumentOptions{
		Snapshot: core.Snapshot{
			CaptureID: "save-notice-timer",
			Kind:      core.KindText,
			Text:      "timer",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = document.native.hwnd.DestroyWindow()
	}()

	document.showSaveNotice(`C:\Temp\notes.txt`)
	if status := document.statusText(); !strings.Contains(status, "已保存到") {
		t.Fatalf("status = %q, want the notice right after saving", status)
	}

	deadline := time.Now().Add(saveNoticeDuration + 3*time.Second)
	var msg win.MSG
	for time.Now().Before(deadline) {
		if win.PeekMessage(&msg, document.native.hwnd, 0, 0, co.PM_REMOVE) {
			win.TranslateMessage(&msg)
			win.DispatchMessage(&msg)
			if !strings.Contains(document.statusText(), "已保存到") {
				return
			}
			continue
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("notice still visible after %s", saveNoticeDuration)
}
