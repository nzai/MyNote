//go:build windows

package ui

import (
	"fmt"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

func paintDoubleBuffered(
	target win.HDC,
	width, height int,
	draw func(win.HDC) error,
) error {
	if width <= 0 || height <= 0 {
		return nil
	}
	memory, err := target.CreateCompatibleDC()
	if err != nil {
		return fmt.Errorf("create offscreen paint DC: %w", err)
	}
	defer func() {
		_ = memory.DeleteDC()
	}()
	bitmap, err := target.CreateCompatibleBitmap(width, height)
	if err != nil {
		return fmt.Errorf("create offscreen paint bitmap: %w", err)
	}
	oldBitmap, err := memory.SelectObjectBmp(bitmap)
	if err != nil {
		_ = bitmap.DeleteObject()
		return fmt.Errorf("select offscreen paint bitmap: %w", err)
	}
	defer func() {
		_, _ = memory.SelectObjectBmp(oldBitmap)
		_ = bitmap.DeleteObject()
	}()

	if err := draw(memory); err != nil {
		return err
	}
	if err := target.BitBlt(
		win.POINT{},
		win.SIZE{Cx: int32(width), Cy: int32(height)},
		memory,
		win.POINT{},
		co.ROP_SRCCOPY,
	); err != nil {
		return fmt.Errorf("present buffered paint frame: %w", err)
	}
	return nil
}
