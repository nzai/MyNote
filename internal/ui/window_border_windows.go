//go:build windows

package ui

import (
	"fmt"

	"github.com/rodrigocfd/windigo/win"
)

// windowBorderRects returns the rectangles that form a frame of the given
// thickness inside the client area. The frame is painted manually because the
// document window suppresses both the legacy non-client border and the DWM
// border, which previously left a white window indistinguishable from the white
// editor background.
func windowBorderRects(width, height, thickness int) []win.RECT {
	if width <= 0 || height <= 0 || thickness <= 0 {
		return nil
	}
	limit := width / 2
	if height/2 < limit {
		limit = height / 2
	}
	if limit < 1 {
		return nil
	}
	if thickness > limit {
		thickness = limit
	}
	rects := []win.RECT{
		{Left: 0, Top: 0, Right: int32(width), Bottom: int32(thickness)},
		{Left: 0, Top: int32(height - thickness), Right: int32(width), Bottom: int32(height)},
		{Left: 0, Top: int32(thickness), Right: int32(thickness), Bottom: int32(height - thickness)},
		{Left: int32(width - thickness), Top: int32(thickness), Right: int32(width), Bottom: int32(height - thickness)},
	}
	filled := rects[:0]
	for _, rect := range rects {
		if rect.Left < rect.Right && rect.Top < rect.Bottom {
			filled = append(filled, rect)
		}
	}
	return filled
}

func (d *Document) paintWindowBorder(hdc win.HDC, width, height, dpi int) error {
	brush, err := brushFor(colorWindowBorder)
	if err != nil {
		return err
	}
	for _, rect := range windowBorderRects(width, height, scaleDIP(onePixelDIP, dpi)) {
		if err := hdc.FillRect(&rect, brush); err != nil {
			return fmt.Errorf("paint window border: %w", err)
		}
	}
	return nil
}
