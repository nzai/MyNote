//go:build windows

package ui

import (
	"testing"

	"github.com/rodrigocfd/windigo/win"
)

func TestOptionsDialogUsesEntireClientArea(t *testing.T) {
	layout := optionsPanelLayoutFor(626, 624, 96)

	if layout.panel.Left != 0 || layout.panel.Right != 626 {
		t.Fatalf(
			"dialog horizontal bounds = %d..%d, want 0..626",
			layout.panel.Left,
			layout.panel.Right,
		)
	}
	if layout.panel.Top != 0 || layout.panel.Bottom != 624 {
		t.Fatalf(
			"dialog vertical bounds = %d..%d, want 0..624",
			layout.panel.Top,
			layout.panel.Bottom,
		)
	}
}

func TestOptionsPanelLayoutKeepsControlsInsidePanel(t *testing.T) {
	layout := optionsPanelLayoutFor(626, 624, 96)
	rects := []struct {
		name string
		rect win.RECT
	}{
		{name: "preview", rect: layout.preview},
		{name: "font selector", rect: layout.fontSelector},
		{name: "size slider", rect: layout.sizeSlider},
		{name: "spacing slider", rect: layout.spacingSlider},
		{name: "auto-start", rect: layout.autoStartCheckbox},
		{name: "suppress close prompt", rect: layout.suppressCloseCheckbox},
		{name: "line numbers", rect: layout.lineNumbersCheckbox},
		{name: "cancel", rect: layout.cancelButton},
		{name: "save", rect: layout.saveButton},
	}
	for _, item := range rects {
		if item.rect.Left < layout.panel.Left ||
			item.rect.Right > layout.panel.Right ||
			item.rect.Top < layout.panel.Top ||
			item.rect.Bottom > layout.panel.Bottom {
			t.Fatalf("%s rect %+v escapes panel %+v", item.name, item.rect, layout.panel)
		}
	}
	if layout.saveButton.Left <= layout.cancelButton.Right {
		t.Fatalf(
			"save button starts at %d, want after cancel ending at %d",
			layout.saveButton.Left,
			layout.cancelButton.Right,
		)
	}
}

func TestOptionsSliderValueUsesRoundedIntegerPositions(t *testing.T) {
	tests := []struct {
		name        string
		x           int
		wantSize    int
		wantSpacing int
	}{
		{name: "left edge", x: 100, wantSize: 8, wantSpacing: 0},
		{name: "quarter", x: 150, wantSize: 18, wantSpacing: 1},
		{name: "middle", x: 200, wantSize: 28, wantSpacing: 2},
		{name: "right edge", x: 300, wantSize: 48, wantSpacing: 3},
		{name: "clamped left", x: 20, wantSize: 8, wantSpacing: 0},
		{name: "clamped right", x: 400, wantSize: 48, wantSpacing: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := sliderIntegerValue(test.x, 100, 300, 8, 48); got != test.wantSize {
				t.Fatalf("font slider value = %d, want %d", got, test.wantSize)
			}
			if got := sliderIntegerValue(test.x, 100, 300, 0, 3); got != test.wantSpacing {
				t.Fatalf("spacing slider value = %d, want %d", got, test.wantSpacing)
			}
		})
	}
}

func TestOptionsPanelHitTestUsesClickTargets(t *testing.T) {
	layout := optionsPanelLayoutFor(626, 624, 96)
	tests := []struct {
		name string
		x    int
		y    int
		want optionsPanelHit
	}{
		{name: "size slider", x: int(layout.sizeSlider.Left + 20), y: int(layout.sizeSlider.Top + 10), want: optionsHitSizeSlider},
		{name: "spacing slider", x: int(layout.spacingSlider.Left + 20), y: int(layout.spacingSlider.Top + 10), want: optionsHitSpacingSlider},
		{name: "auto start", x: int(layout.autoStartCheckbox.Left + 20), y: int(layout.autoStartCheckbox.Top + 8), want: optionsHitAutoStart},
		{name: "suppress prompt", x: int(layout.suppressCloseCheckbox.Left + 20), y: int(layout.suppressCloseCheckbox.Top + 8), want: optionsHitSuppressPrompt},
		{name: "line numbers", x: int(layout.lineNumbersCheckbox.Left + 20), y: int(layout.lineNumbersCheckbox.Top + 8), want: optionsHitLineNumbers},
		{name: "cancel", x: int(layout.cancelButton.Left + 10), y: int(layout.cancelButton.Top + 10), want: optionsHitCancel},
		{name: "save", x: int(layout.saveButton.Left + 10), y: int(layout.saveButton.Top + 10), want: optionsHitSave},
		{name: "blank", x: int(layout.panel.Left + 4), y: int(layout.panel.Top + 60), want: optionsHitNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := optionsPanelHitAt(layout, test.x, test.y); got != test.want {
				t.Fatalf("optionsPanelHitAt() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestLineSpacingOptionsMapToDisplayAndSliderIndex(t *testing.T) {
	tests := []struct {
		value   int
		display string
		index   int
	}{
		{value: 100, display: "1.0×", index: 0},
		{value: 125, display: "1.25×", index: 1},
		{value: 150, display: "1.5×", index: 2},
		{value: 200, display: "2.0×", index: 3},
	}
	for _, test := range tests {
		if got := lineSpacingDisplay(test.value); got != test.display {
			t.Fatalf("lineSpacingDisplay(%d) = %q, want %q", test.value, got, test.display)
		}
		if got := editorLineSpacingIndex(test.value); got != test.index {
			t.Fatalf("editorLineSpacingIndex(%d) = %d, want %d", test.value, got, test.index)
		}
	}
}
