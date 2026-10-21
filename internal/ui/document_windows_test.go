//go:build windows

package ui

import (
	"image"
	"image/color"
	"os"
	"strings"
	"testing"

	"github.com/nzai/MyNote/assets"
	"github.com/nzai/MyNote/internal/core"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

func TestDocumentWindowUsesAppIcon(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 window icon test")
	}
	icon, err := loadAppIcon(assets.AppICO)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = icon.DestroyIcon()
	}()

	document, err := NewDocument(DocumentOptions{
		Snapshot: core.Snapshot{
			CaptureID: "window-icon-test",
			Kind:      core.KindText,
			Text:      "icon",
		},
		Icon: icon,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = document.native.hwnd.DestroyWindow()
	}()

	for _, test := range []struct {
		name string
		size co.ICON_SZ
	}{
		{name: "small", size: co.ICON_SZ_SMALL},
		{name: "big", size: co.ICON_SZ_BIG},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := document.native.hwnd.SendMessage(
				co.WM_GETICON,
				win.WPARAM(test.size),
				0,
			)
			if err != nil {
				t.Fatal(err)
			}
			if win.HICON(got) != icon {
				t.Fatalf("window icon = %#x, want %#x", got, uintptr(icon))
			}
		})
	}
}

func TestTextDocumentSelectAllSelectsEntireEditor(t *testing.T) {
	document := newSelectionTestDocument(t, "alpha beta gamma")

	document.executeCommand(CommandSelectAll)

	start, end := editorSelection(t, document)
	if start != 0 || end != 16 {
		t.Fatalf("selection = %d..%d, want 0..16", start, end)
	}
}

func TestTextEditorDoubleClickSelectsWord(t *testing.T) {
	document := newSelectionTestDocument(t, "alpha beta gamma")
	x, y := editorCharacterPosition(t, document, 8)
	for _, message := range []co.WM{
		co.WM_LBUTTONDOWN,
		co.WM_LBUTTONUP,
		co.WM_LBUTTONDBLCLK,
		co.WM_LBUTTONUP,
	} {
		if _, err := document.edit.SendMessage(
			message,
			0,
			mousePosition(x, y),
		); err != nil {
			t.Fatal(err)
		}
	}

	start, end := editorSelection(t, document)
	selected := "alpha beta gamma"[start:end]
	if strings.TrimSpace(selected) != "beta" {
		t.Fatalf("selected text = %q, want word %q", selected, "beta")
	}
}

func newSelectionTestDocument(t *testing.T, text string) *Document {
	t.Helper()
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 editor selection test")
	}
	document, err := NewDocument(DocumentOptions{
		Snapshot: core.Snapshot{
			CaptureID: "editor-selection-test",
			Kind:      core.KindText,
			Text:      text,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	document.native.hwnd.ShowWindow(co.SW_HIDE)
	_, _ = document.edit.SetFocus()
	t.Cleanup(func() {
		_ = document.native.hwnd.DestroyWindow()
	})
	return document
}

func editorCharacterPosition(t *testing.T, document *Document, charIndex int) (int, int) {
	t.Helper()
	position, err := document.edit.SendMessage(
		co.EM_POSFROMCHAR,
		win.WPARAM(charIndex),
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	x, y := decodeMousePosition(position)
	if x < 0 || y < 0 {
		t.Fatalf("character %d position = (%d, %d)", charIndex, x, y)
	}
	return x + 2, y + 2
}

func editorSelection(t *testing.T, document *Document) (int, int) {
	t.Helper()
	value, err := document.edit.SendMessage(co.EM_GETSEL, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	return int(uint16(value)), int(uint16(value >> 16))
}

func mousePosition(x, y int) win.LPARAM {
	return win.LPARAM(uintptr(uint16(x)) | uintptr(uint16(y))<<16)
}

func TestDocumentDwmAttributesRemoveVisibleBorder(t *testing.T) {
	attrs := documentDwmAttributes()
	if len(attrs) == 0 {
		t.Fatal("document must configure at least one DWM attribute")
	}
	for _, attr := range attrs {
		if borderColor, ok := attr.BorderColor(); ok {
			if borderColor != win.COLORREF_DWMA_NONE {
				t.Fatalf("DWM border color = %#x, want none", borderColor)
			}
			return
		}
	}
	t.Fatal("document DWM attributes do not remove the visible border")
}

func TestDocumentSuppressesLegacyNonClientBorder(t *testing.T) {
	document := &Document{}
	ret, handled := document.handleNativeMessage(co.WM_NCACTIVATE, 1, 0)
	if !handled || ret != 1 {
		t.Fatalf("WM_NCACTIVATE = (%d, %v), want (1, true)", ret, handled)
	}
	ret, handled = document.handleNativeMessage(co.WM_NCPAINT, 1, 0)
	if !handled || ret != 0 {
		t.Fatalf("WM_NCPAINT = (%d, %v), want (0, true)", ret, handled)
	}
}

func TestCreateDocumentWindows(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 smoke test")
	}
	t.Run("text", func(t *testing.T) {
		window, err := createDocumentForTest(core.Snapshot{
			CaptureID: "test-text",
			Kind:      core.KindText,
			Text:      "hello",
		})
		if err != nil {
			t.Fatal(err)
		}
		window.Close()
	})
	t.Run("image", func(t *testing.T) {
		img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
		img.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255})
		img.SetNRGBA(0, 1, color.NRGBA{B: 255, A: 255})
		img.SetNRGBA(1, 1, color.NRGBA{R: 255, G: 255, A: 255})
		window, err := createDocumentForTest(core.Snapshot{
			CaptureID: "test-image",
			Kind:      core.KindImage,
			Image:     img,
		})
		if err != nil {
			t.Fatal(err)
		}
		window.Close()
	})
}
