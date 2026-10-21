//go:build windows

package ui

import "github.com/rodrigocfd/windigo/co"

const (
	titlebarHeightDIP   = 40
	titlebarPaddingDIP  = 12
	titleSaveGapDIP     = 8
	iconTitleGapDIP     = 8
	saveButtonWidthDIP  = 44
	closeButtonWidthDIP = 44
	resizeBorderDIP     = 6
	statusbarHeightDIP  = 28
)

type titlebarButton uint8

const (
	titlebarButtonNone titlebarButton = iota
	titlebarButtonSave
	titlebarButtonClose
)

type titlebarLayout struct {
	iconLeft   int
	titleLeft  int
	titleRight int
	saveLeft   int
	saveRight  int
	closeLeft  int
}

func titlebarLayoutFor(width, dpi, titleWidth int) titlebarLayout {
	if titleWidth < 0 {
		titleWidth = 0
	}
	iconLeft := scaleDIP(titlebarPaddingDIP, dpi)
	titleLeft := iconLeft + scaleDIP(appIconWidthDIP+iconTitleGapDIP, dpi)
	gap := scaleDIP(titleSaveGapDIP, dpi)
	saveWidth := scaleDIP(saveButtonWidthDIP, dpi)
	closeWidth := scaleDIP(closeButtonWidthDIP, dpi)

	closeLeft := width - closeWidth
	maxSaveLeft := closeLeft - saveWidth - gap
	if maxSaveLeft < titleLeft {
		maxSaveLeft = titleLeft
	}
	saveLeft := titleLeft + titleWidth + gap
	if saveLeft > maxSaveLeft {
		saveLeft = maxSaveLeft
	}
	saveRight := saveLeft + saveWidth
	titleRight := saveLeft - gap
	if titleRight < titleLeft {
		titleRight = titleLeft
	}
	return titlebarLayout{
		iconLeft:   iconLeft,
		titleLeft:  titleLeft,
		titleRight: titleRight,
		saveLeft:   saveLeft,
		saveRight:  saveRight,
		closeLeft:  closeLeft,
	}
}

func titlebarHitTest(width, height, dpi int, layout titlebarLayout, x, y int) co.HT {
	if x < 0 || y < 0 || x >= width || y >= height {
		return co.HT_NOWHERE
	}

	border := scaleDIP(resizeBorderDIP, dpi)
	onLeft := x < border
	onRight := x >= width-border
	onTop := y < border
	onBottom := y >= height-border

	switch {
	case onTop && onLeft:
		return co.HT_TOPLEFT
	case onTop && onRight:
		return co.HT_TOPRIGHT
	case onBottom && onLeft:
		return co.HT_BOTTOMLEFT
	case onBottom && onRight:
		return co.HT_BOTTOMRIGHT
	case onLeft:
		return co.HT_LEFT
	case onRight:
		return co.HT_RIGHT
	case onTop:
		return co.HT_TOP
	case onBottom:
		return co.HT_BOTTOM
	}

	if y < scaleDIP(titlebarHeightDIP, dpi) {
		if titlebarButtonAt(layout, height, dpi, x, y) != titlebarButtonNone {
			return co.HT_CLIENT
		}
		return co.HT_CAPTION
	}
	return co.HT_CLIENT
}

func titlebarButtonAt(layout titlebarLayout, height, dpi, x, y int) titlebarButton {
	if x < 0 || y < 0 || y >= height {
		return titlebarButtonNone
	}
	if y >= scaleDIP(titlebarHeightDIP, dpi) {
		return titlebarButtonNone
	}
	if x >= layout.saveLeft && x < layout.saveRight {
		return titlebarButtonSave
	}
	if x >= layout.closeLeft {
		return titlebarButtonClose
	}
	return titlebarButtonNone
}

func scaleDIP(value, dpi int) int {
	if dpi <= 0 {
		dpi = 96
	}
	return (value*dpi + 48) / 96
}

func decodeMousePosition(value uintptr) (x, y int) {
	return int(int16(uint16(value))), int(int16(uint16(value >> 16)))
}
