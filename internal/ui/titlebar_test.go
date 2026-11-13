//go:build windows

package ui

import (
	"testing"

	"github.com/rodrigocfd/windigo/co"
)

func TestTitlebarHitTestAt96DPI(t *testing.T) {
	layout := titlebarLayoutFor(720, 96)
	tests := []struct {
		name string
		x    int
		y    int
		want co.HT
	}{
		{name: "left edge", x: 2, y: 250, want: co.HT_LEFT},
		{name: "right edge", x: 718, y: 250, want: co.HT_RIGHT},
		{name: "top edge", x: 350, y: 2, want: co.HT_TOP},
		{name: "bottom edge", x: 350, y: 518, want: co.HT_BOTTOM},
		{name: "top left corner", x: 2, y: 2, want: co.HT_TOPLEFT},
		{name: "top right corner", x: 718, y: 2, want: co.HT_TOPRIGHT},
		{name: "bottom left corner", x: 2, y: 518, want: co.HT_BOTTOMLEFT},
		{name: "bottom right corner", x: 718, y: 518, want: co.HT_BOTTOMRIGHT},
		{name: "app icon stays caption", x: 20, y: 20, want: co.HT_CAPTION},
		{name: "save stays client", x: layout.saveLeft + 10, y: 20, want: co.HT_CLIENT},
		{name: "options stays client", x: layout.optionsLeft + 10, y: 20, want: co.HT_CLIENT},
		{name: "close stays client", x: layout.closeLeft + 10, y: 20, want: co.HT_CLIENT},
		{name: "draggable title", x: layout.titleLeft + 40, y: 20, want: co.HT_CAPTION},
		{name: "content", x: 350, y: 100, want: co.HT_CLIENT},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := titlebarHitTest(720, 520, 96, layout, test.x, test.y); got != test.want {
				t.Fatalf("titlebarHitTest() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestTitlebarHitTestScalesAt192DPI(t *testing.T) {
	layout := titlebarLayoutFor(1440, 192)
	if got := titlebarHitTest(1440, 1040, 192, layout, layout.saveLeft+20, 40); got != co.HT_CLIENT {
		t.Fatalf("scaled save area hit = %v, want HT_CLIENT", got)
	}
	if got := titlebarHitTest(1440, 1040, 192, layout, layout.titleLeft+40, 40); got != co.HT_CAPTION {
		t.Fatalf("scaled title area hit = %v, want HT_CAPTION", got)
	}
}

func TestTitlebarButtonAt(t *testing.T) {
	layout := titlebarLayoutFor(720, 96)
	tests := []struct {
		name string
		x    int
		y    int
		want titlebarButton
	}{
		{name: "app icon", x: 20, y: 20, want: titlebarButtonNone},
		{name: "save", x: layout.saveLeft + 10, y: 20, want: titlebarButtonSave},
		{name: "options", x: layout.optionsLeft + 10, y: 20, want: titlebarButtonOptions},
		{name: "close", x: layout.closeLeft + 10, y: 20, want: titlebarButtonClose},
		{name: "title", x: layout.titleLeft + 40, y: 20, want: titlebarButtonNone},
		{name: "content", x: 100, y: 80, want: titlebarButtonNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := titlebarButtonAt(layout, 520, 96, test.x, test.y); got != test.want {
				t.Fatalf("titlebarButtonAt() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestTitlebarLayoutPlacesButtonsNextToIcon(t *testing.T) {
	layout := titlebarLayoutFor(720, 96)
	if layout.iconLeft != 12 {
		t.Fatalf("iconLeft = %d, want 12", layout.iconLeft)
	}
	if layout.saveLeft != layout.iconLeft+16+8 {
		t.Fatalf("saveLeft = %d, want icon end plus gap %d", layout.saveLeft, layout.iconLeft+24)
	}
	if layout.optionsLeft != layout.saveRight {
		t.Fatalf("optionsLeft = %d, want saveRight %d", layout.optionsLeft, layout.saveRight)
	}
	if layout.titleLeft != layout.optionsRight+8 {
		t.Fatalf("titleLeft = %d, want optionsRight plus gap %d", layout.titleLeft, layout.optionsRight+8)
	}
	if layout.titleLeft <= layout.optionsRight {
		t.Fatalf("title starts at %d, overlapping options ending at %d", layout.titleLeft, layout.optionsRight)
	}
	if layout.titleRight != layout.closeLeft-8 {
		t.Fatalf("titleRight = %d, want close button minus gap %d", layout.titleRight, layout.closeLeft-8)
	}
}

func TestTitlebarLayoutKeepsButtonsSeparateInNarrowWindow(t *testing.T) {
	layout := titlebarLayoutFor(320, 96)
	if layout.optionsRight > layout.closeLeft {
		t.Fatalf(
			"options button ends at %d and overlaps close button starting at %d",
			layout.optionsRight,
			layout.closeLeft,
		)
	}
	if layout.titleRight < layout.titleLeft {
		t.Fatalf("title layout is inverted: %+v", layout)
	}
}

func TestDecodeSignedMouseWord(t *testing.T) {
	const packed = uintptr(0x0012_FFEE)
	x, y := decodeMousePosition(packed)
	if x != -18 || y != 18 {
		t.Fatalf("decodeMousePosition() = (%d, %d), want (-18, 18)", x, y)
	}
}

func TestStatusCancelHitAt96DPI(t *testing.T) {
	if !statusCancelHit(720, 520, 96, 700, 505) {
		t.Fatal("expected status-bar cancel hit")
	}
	if statusCancelHit(720, 520, 96, 640, 505) {
		t.Fatal("status text area must not trigger cancel")
	}
}
