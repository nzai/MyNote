//go:build windows

package ui

import (
	"github.com/rodrigocfd/windigo/win"
)

const (
	optionsWindowWidthDIP  = 628
	optionsWindowHeightDIP = 664
	optionsPanelPaddingDIP = 26
	optionsSliderHeightDIP = 28
	optionsButtonHeightDIP = 34
	optionsCancelWidthDIP  = 78
	optionsSaveWidthDIP    = 94
	optionsButtonGapDIP    = 10
)

type optionsPanelLayout struct {
	panel                 win.RECT
	description           win.RECT
	preview               win.RECT
	fontLabel             win.RECT
	fontSelector          win.RECT
	sizeLabel             win.RECT
	sizeSlider            win.RECT
	spacingLabel          win.RECT
	spacingSlider         win.RECT
	spacingLabels         win.RECT
	autoStartCheckbox     win.RECT
	suppressCloseCheckbox win.RECT
	lineNumbersCheckbox   win.RECT
	cancelButton          win.RECT
	saveButton            win.RECT
	errorText             win.RECT
	footerDividerY        int
}

type optionsPanelHit uint8

const (
	optionsHitNone optionsPanelHit = iota
	optionsHitAutoStart
	optionsHitSuppressPrompt
	optionsHitLineNumbers
	optionsHitSizeSlider
	optionsHitSpacingSlider
	optionsHitCancel
	optionsHitSave
)

func optionsPanelLayoutFor(width, height, dpi int) optionsPanelLayout {
	return optionsPanelContentsLayout(
		win.RECT{Right: int32(width), Bottom: int32(height)},
		dpi,
	)
}

func optionsPanelContentsLayout(panel win.RECT, dpi int) optionsPanelLayout {
	padding := scaleDIP(optionsPanelPaddingDIP, dpi)
	left := int(panel.Left) + padding
	right := int(panel.Right) - padding
	rel := func(value int) int {
		return int(panel.Top) + scaleDIP(value, dpi)
	}
	row := func(topValue, heightValue int) win.RECT {
		return win.RECT{
			Left:   int32(left),
			Top:    int32(rel(topValue)),
			Right:  int32(right),
			Bottom: int32(rel(topValue + heightValue)),
		}
	}

	buttonHeight := scaleDIP(optionsButtonHeightDIP, dpi)
	saveWidth := scaleDIP(optionsSaveWidthDIP, dpi)
	cancelWidth := scaleDIP(optionsCancelWidthDIP, dpi)
	buttonGap := scaleDIP(optionsButtonGapDIP, dpi)
	saveRight := right
	saveLeft := saveRight - saveWidth
	cancelRight := saveLeft - buttonGap
	cancelLeft := cancelRight - cancelWidth
	buttonBottom := int(panel.Bottom) - scaleDIP(28, dpi)
	buttonTop := buttonBottom - buttonHeight

	return optionsPanelLayout{
		panel:                 panel,
		description:           row(12, 22),
		preview:               row(44, 112),
		fontLabel:             row(168, 24),
		fontSelector:          row(191, 40),
		sizeLabel:             row(244, 22),
		sizeSlider:            row(268, optionsSliderHeightDIP),
		spacingLabel:          row(304, 22),
		spacingSlider:         row(328, optionsSliderHeightDIP),
		spacingLabels:         row(358, 20),
		autoStartCheckbox:     row(399, 28),
		suppressCloseCheckbox: row(433, 28),
		lineNumbersCheckbox:   row(467, 28),
		cancelButton: win.RECT{
			Left:   int32(cancelLeft),
			Top:    int32(buttonTop),
			Right:  int32(cancelRight),
			Bottom: int32(buttonBottom),
		},
		saveButton: win.RECT{
			Left:   int32(saveLeft),
			Top:    int32(buttonTop),
			Right:  int32(saveRight),
			Bottom: int32(buttonBottom),
		},
		errorText: win.RECT{
			Left:   int32(left),
			Top:    int32(buttonTop),
			Right:  int32(cancelLeft - buttonGap),
			Bottom: int32(buttonBottom),
		},
		footerDividerY: buttonTop - scaleDIP(18, dpi),
	}
}

func optionsPanelHitAt(layout optionsPanelLayout, x, y int) optionsPanelHit {
	targets := []struct {
		rect win.RECT
		hit  optionsPanelHit
	}{
		{rect: layout.sizeSlider, hit: optionsHitSizeSlider},
		{rect: layout.spacingSlider, hit: optionsHitSpacingSlider},
		{rect: layout.autoStartCheckbox, hit: optionsHitAutoStart},
		{rect: layout.suppressCloseCheckbox, hit: optionsHitSuppressPrompt},
		{rect: layout.lineNumbersCheckbox, hit: optionsHitLineNumbers},
		{rect: layout.cancelButton, hit: optionsHitCancel},
		{rect: layout.saveButton, hit: optionsHitSave},
	}
	for _, target := range targets {
		if pointInRect(target.rect, x, y) {
			return target.hit
		}
	}
	return optionsHitNone
}

func optionsHoverInvalidRects(
	layout optionsPanelLayout,
	previous optionsPanelHit,
	next optionsPanelHit,
) []win.RECT {
	rects := make([]win.RECT, 0, 2)
	for _, hit := range []optionsPanelHit{previous, next} {
		if rect, visible := optionsHitVisualRect(layout, hit); visible {
			rects = append(rects, rect)
		}
	}
	return rects
}

func optionsHitVisualRect(
	layout optionsPanelLayout,
	hit optionsPanelHit,
) (win.RECT, bool) {
	switch hit {
	case optionsHitAutoStart:
		return layout.autoStartCheckbox, true
	case optionsHitSuppressPrompt:
		return layout.suppressCloseCheckbox, true
	case optionsHitLineNumbers:
		return layout.lineNumbersCheckbox, true
	case optionsHitCancel:
		return layout.cancelButton, true
	case optionsHitSave:
		return layout.saveButton, true
	default:
		// Sliders have no hover-only visual state.
		return win.RECT{}, false
	}
}

func sliderIntegerValue(x, left, right, minValue, maxValue int) int {
	if right <= left || maxValue <= minValue {
		return minValue
	}
	if x <= left {
		return minValue
	}
	if x >= right {
		return maxValue
	}
	width := right - left
	valueRange := maxValue - minValue
	return minValue + ((x-left)*valueRange+width/2)/width
}

func sliderThumbX(value, minValue, maxValue, left, right int) int {
	if maxValue <= minValue {
		return left
	}
	if value < minValue {
		value = minValue
	}
	if value > maxValue {
		value = maxValue
	}
	return left + (value-minValue)*(right-left)/(maxValue-minValue)
}

func pointInRect(rect win.RECT, x, y int) bool {
	return x >= int(rect.Left) && x < int(rect.Right) &&
		y >= int(rect.Top) && y < int(rect.Bottom)
}
