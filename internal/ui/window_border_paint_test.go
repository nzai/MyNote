//go:build windows

package ui

import (
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/rodrigocfd/windigo/win"
)

var gdi32GetPixel = syscall.NewLazyDLL("gdi32.dll").NewProc("GetPixel")

func TestDocumentPaintsWindowBorder(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 window border paint test")
	}
	document := newSelectionTestDocument(t, "border")

	width, height, err := clientSize(document.native.hwnd)
	if err != nil {
		t.Fatal(err)
	}
	// A compatible bitmap taken from a memory DC is monochrome, so anchor both
	// the bitmap and the memory DC to the screen DC instead.
	screen, err := win.HWND(0).GetDC()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = win.HWND(0).ReleaseDC(screen)
	}()
	memDC, err := screen.CreateCompatibleDC()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = memDC.DeleteDC()
	}()
	bitmap, err := screen.CreateCompatibleBitmap(width, height)
	if err != nil {
		t.Fatal(err)
	}
	previous, err := memDC.SelectObjectBmp(bitmap)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = memDC.SelectObjectBmp(previous)
		_ = bitmap.DeleteObject()
	}()

	if err := document.paint(memDC); err != nil {
		t.Fatal(err)
	}

	for name, point := range map[string][2]int{
		"top edge":    {width / 2, 0},
		"left edge":   {0, height / 2},
		"right edge":  {width - 1, height / 2},
		"bottom edge": {width / 2, height - 1},
	} {
		got, err := pixelAt(memDC, point[0], point[1])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != colorWindowBorder {
			t.Fatalf("%s color = %#08x, want window border %#08x", name, uint32(got), uint32(colorWindowBorder))
		}
	}

	inner, err := pixelAt(memDC, width*3/4, height/2)
	if err != nil {
		t.Fatal(err)
	}
	if inner != colorWindowBackground {
		t.Fatalf("content background color = %#08x, want %#08x", uint32(inner), uint32(colorWindowBackground))
	}
}

func pixelAt(hdc win.HDC, x, y int) (win.COLORREF, error) {
	ret, _, _ := gdi32GetPixel.Call(uintptr(hdc), uintptr(int32(x)), uintptr(int32(y)))
	if ret == 0xFFFFFFFF {
		return 0, errors.New("GetPixel failed")
	}
	return win.COLORREF(uint32(ret)), nil
}
