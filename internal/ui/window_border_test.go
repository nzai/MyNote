//go:build windows

package ui

import (
	"math"
	"testing"

	"github.com/rodrigocfd/windigo/win"
)

func TestWindowBorderRectsCoverFrameEdges(t *testing.T) {
	const (
		width     = 100
		height    = 60
		thickness = 2
	)
	rects := windowBorderRects(width, height, thickness)
	if len(rects) == 0 {
		t.Fatal("window border must produce at least one rectangle")
	}

	covered := func(x, y int) bool {
		for _, rect := range rects {
			if int(rect.Left) <= x && x < int(rect.Right) &&
				int(rect.Top) <= y && y < int(rect.Bottom) {
				return true
			}
		}
		return false
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			onFrame := x < thickness || x >= width-thickness ||
				y < thickness || y >= height-thickness
			if got := covered(x, y); got != onFrame {
				t.Fatalf("pixel (%d, %d): covered = %v, want %v", x, y, got, onFrame)
			}
		}
	}
}

func TestWindowBorderRectsClampOversizedThickness(t *testing.T) {
	for _, rect := range windowBorderRects(10, 10, 20) {
		if rect.Left >= rect.Right || rect.Top >= rect.Bottom {
			t.Fatalf("window border produced an empty or inverted rect: %+v", rect)
		}
	}
	if rects := windowBorderRects(0, 0, 1); rects != nil {
		t.Fatalf("window border for an empty window = %v, want nil", rects)
	}
}

func TestWindowBorderContrastsWithBackground(t *testing.T) {
	if ratio := contrastRatio(colorWindowBorder, colorWindowBackground); ratio < 2.0 {
		t.Fatalf(
			"window border contrast = %.2f:1, want at least 2.0:1 against the white background",
			ratio,
		)
	}
	if ratio := contrastRatio(colorWindowBorder, colorTitlebar); ratio < 1.5 {
		t.Fatalf(
			"window border contrast against the titlebar = %.2f:1, want at least 1.5:1",
			ratio,
		)
	}
}

func contrastRatio(a, b win.COLORREF) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func relativeLuminance(color win.COLORREF) float64 {
	channel := func(value byte) float64 {
		v := float64(value) / 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(byte(color)) +
		0.7152*channel(byte(uint32(color)>>8)) +
		0.0722*channel(byte(uint32(color)>>16))
}
