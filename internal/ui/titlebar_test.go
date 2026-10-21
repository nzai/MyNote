//go:build windows

package ui

import (
	"testing"

	"github.com/rodrigocfd/windigo/co"
)

func TestTitlebarHitTestAt96DPI(t *testing.T) {
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
		{name: "save stays client", x: 22, y: 20, want: co.HT_CLIENT},
		{name: "close stays client", x: 698, y: 20, want: co.HT_CLIENT},
		{name: "draggable title", x: 120, y: 20, want: co.HT_CAPTION},
		{name: "content", x: 350, y: 100, want: co.HT_CLIENT},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := titlebarHitTest(720, 520, 96, test.x, test.y); got != test.want {
				t.Fatalf("titlebarHitTest() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestTitlebarHitTestScalesAt192DPI(t *testing.T) {
	if got := titlebarHitTest(1440, 1040, 192, 20, 20); got != co.HT_CLIENT {
		t.Fatalf("scaled save area hit = %v, want HT_CLIENT", got)
	}
	if got := titlebarHitTest(1440, 1040, 192, 120, 40); got != co.HT_CAPTION {
		t.Fatalf("scaled title area hit = %v, want HT_CAPTION", got)
	}
}

func TestTitlebarButtonAt(t *testing.T) {
	tests := []struct {
		name string
		x    int
		y    int
		want titlebarButton
	}{
		{name: "save", x: 10, y: 20, want: titlebarButtonSave},
		{name: "close", x: 700, y: 20, want: titlebarButtonClose},
		{name: "title", x: 100, y: 20, want: titlebarButtonNone},
		{name: "content", x: 100, y: 80, want: titlebarButtonNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := titlebarButtonAt(720, 520, 96, test.x, test.y); got != test.want {
				t.Fatalf("titlebarButtonAt() = %v, want %v", got, test.want)
			}
		})
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
