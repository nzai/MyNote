//go:build windows

package ui

import (
	"image"
	"image/color"
	"os"
	"testing"

	"github.com/nzai/MyNote/internal/core"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

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
