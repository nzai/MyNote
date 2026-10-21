//go:build windows

package ui

import (
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/nzai/MyNote/internal/core"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

func TestEditorScrollRepaintsLineNumberGutter(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 line-number scroll test")
	}

	text := strings.Repeat(
		"this logical line is deliberately long enough to wrap across several visual rows "+
			"so line numbers must remain aligned while scrolling\r\n",
		80,
	)
	document, err := NewDocument(DocumentOptions{
		Snapshot: core.Snapshot{
			CaptureID: "line-number-scroll-paint-test",
			Kind:      core.KindText,
			Text:      text,
		},
		// Keep the GUI test out of the visible desktop while still allowing
		// Windows to deliver synchronous paint messages.
		Position: win.POINT{X: -20000, Y: -20000},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = document.native.hwnd.DestroyWindow()
	}()

	_ = document.native.hwnd.ShowWindow(co.SW_SHOWNOACTIVATE)
	_ = document.native.hwnd.UpdateWindow()
	installParentPaintProbe(t, document.native.hwnd)

	beforeFirst := firstVisibleEditorLine(t, document)
	parentPaintProbe.Store(0)

	for range 10 {
		_, _ = document.edit.SendMessage(
			co.WM_VSCROLL,
			win.WPARAM(co.SB_REQ_LINEDOWN),
			0,
		)
	}
	_ = document.edit.UpdateWindow()

	afterFirst := firstVisibleEditorLine(t, document)
	if afterFirst <= beforeFirst {
		t.Fatalf(
			"first visible editor line = %d after scrolling, want later than %d",
			afterFirst,
			beforeFirst,
		)
	}
	if paints := parentPaintProbe.Load(); paints == 0 {
		t.Fatalf(
			"document was not repainted synchronously after editor scrolled from line %d to %d",
			beforeFirst,
			afterFirst,
		)
	}
}

func firstVisibleEditorLine(t *testing.T, document *Document) int {
	t.Helper()
	value, err := document.edit.SendMessage(co.EM_GETFIRSTVISIBLELINE, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	return int(value)
}

var parentPaintProbe atomic.Int64

var parentPaintProbeSubclass = struct {
	sync.Once
	proc uintptr
}{}

func installParentPaintProbe(t *testing.T, hwnd win.HWND) {
	t.Helper()
	parentPaintProbeSubclass.Do(func() {
		parentPaintProbeSubclass.proc = syscall.NewCallback(
			parentPaintProbeWindowProc,
		)
	})
	if err := hwnd.SetWindowSubclass(
		parentPaintProbeSubclass.proc,
		2,
		nil,
	); err != nil {
		t.Fatal(err)
	}
}

func parentPaintProbeWindowProc(
	hwnd win.HWND,
	msg co.WM,
	wParam win.WPARAM,
	lParam win.LPARAM,
	subclassID uintptr,
	_ uintptr,
) uintptr {
	if msg == co.WM_PAINT {
		parentPaintProbe.Add(1)
	}
	if msg == co.WM_NCDESTROY {
		_ = hwnd.RemoveWindowSubclass(
			parentPaintProbeSubclass.proc,
			uint32(subclassID),
		)
	}
	return hwnd.DefSubclassProc(msg, wParam, lParam)
}
