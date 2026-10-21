//go:build windows

package ui

import (
	"os"
	"testing"
	"unsafe"

	"github.com/nzai/MyNote/assets"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

func TestCascadePositionWrapsWithinWorkArea(t *testing.T) {
	work := win.RECT{Left: 0, Top: 0, Right: 1000, Bottom: 800}
	first := cascadePosition(work, 720, 520, 96, 0)
	if first.X != 40 || first.Y != 40 {
		t.Fatalf("first cascade position = (%d, %d), want (40, 40)", first.X, first.Y)
	}
	second := cascadePosition(work, 720, 520, 96, 1)
	if second.X != 64 || second.Y != 64 {
		t.Fatalf("second cascade position = (%d, %d), want (64, 64)", second.X, second.Y)
	}
	wrapped := cascadePosition(work, 720, 520, 96, 9)
	if wrapped.X != 40 || wrapped.Y != 40 {
		t.Fatalf("wrapped cascade position = (%d, %d), want reset (40, 40)", wrapped.X, wrapped.Y)
	}
}

func TestCascadePositionKeepsOversizeWindowInsideWorkArea(t *testing.T) {
	work := win.RECT{Left: 100, Top: 50, Right: 700, Bottom: 450}
	got := cascadePosition(work, 1000, 700, 96, 3)
	if got.X != 100 || got.Y != 50 {
		t.Fatalf("oversize window position = (%d, %d), want work area origin (100, 50)", got.X, got.Y)
	}
}

func TestTrayActionFor(t *testing.T) {
	tests := []struct {
		name   string
		lParam win.LPARAM
		want   trayAction
	}{
		{name: "legacy left up is ignored with version 4", lParam: win.LPARAM(co.WM_LBUTTONUP), want: trayActionNone},
		{name: "select", lParam: win.LPARAM(ninSelect), want: trayActionOpen},
		{name: "keyboard select", lParam: win.LPARAM(ninKeySelect), want: trayActionOpen},
		{name: "right up", lParam: win.LPARAM(co.WM_RBUTTONUP), want: trayActionMenu},
		{name: "other", lParam: win.LPARAM(co.WM_MOUSEMOVE), want: trayActionNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := trayActionFor(test.lParam); got != test.want {
				t.Fatalf("trayActionFor() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestExitCoordinatorWaitsForDeferredDocumentClose(t *testing.T) {
	var exit exitCoordinator
	destroyController := exit.closeBatch([]documentCloseDisposition{
		documentCloseDestroyed,
		documentCloseDeferred,
	})
	if destroyController {
		t.Fatal("controller must stay alive while a document save is pending")
	}
	if !exit.pending {
		t.Fatal("exit intent must be retained while the document closes")
	}
	if exit.documentDestroyed(1) {
		t.Fatal("controller must stay alive while another document remains")
	}
	if !exit.documentDestroyed(0) {
		t.Fatal("controller must exit after the last document closes")
	}
}

func TestExitCoordinatorCancelsPendingExitWhenAnyDocumentStaysOpen(t *testing.T) {
	var exit exitCoordinator
	exit.closeBatch([]documentCloseDisposition{documentCloseDeferred})
	exit.closeBatch([]documentCloseDisposition{documentCloseCanceled})
	if exit.pending {
		t.Fatal("a canceled close must revoke the pending exit")
	}
	if exit.documentDestroyed(0) {
		t.Fatal("controller must not exit after a canceled close")
	}
}

func TestExitCoordinatorCompletesImmediatelyWhenAllDocumentsClose(t *testing.T) {
	var exit exitCoordinator
	destroyController := exit.closeBatch([]documentCloseDisposition{
		documentCloseDestroyed,
		documentCloseDestroyed,
	})
	if !destroyController {
		t.Fatal("controller must exit when every document is already closed")
	}
	if exit.pending {
		t.Fatal("completed exit must not leave a pending intent")
	}
}

func TestCreateControllerAndTray(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 smoke test")
	}
	if got := unsafe.Sizeof(trayNotifyIconData{}); got != 976 {
		t.Fatalf("NOTIFYICONDATAW size = %d, want 976", got)
	}
	if !shellTrayAvailable() {
		t.Skip("interactive Windows shell tray is unavailable")
	}
	icon, err := loadAppIcon(assets.AppICO)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = icon.DestroyIcon()
	}()
	controller, err := NewController(ControllerOptions{Icon: icon})
	if err != nil {
		t.Fatal(err)
	}
	controller.Close()
}
