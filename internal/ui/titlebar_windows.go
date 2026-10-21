//go:build windows

package ui

import "github.com/rodrigocfd/windigo/co"

const (
	titlebarHeightDIP   = 40
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

func titlebarHitTest(width, height, dpi, x, y int) co.HT {
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
		if titlebarButtonAt(width, height, dpi, x, y) != titlebarButtonNone {
			return co.HT_CLIENT
		}
		return co.HT_CAPTION
	}
	return co.HT_CLIENT
}

func titlebarButtonAt(width, height, dpi, x, y int) titlebarButton {
	if x < 0 || y < 0 || x >= width || y >= height {
		return titlebarButtonNone
	}
	if y >= scaleDIP(titlebarHeightDIP, dpi) {
		return titlebarButtonNone
	}
	if x < scaleDIP(saveButtonWidthDIP, dpi) {
		return titlebarButtonSave
	}
	if x >= width-scaleDIP(closeButtonWidthDIP, dpi) {
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
