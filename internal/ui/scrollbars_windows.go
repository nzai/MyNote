//go:build windows

package ui

import (
	"fmt"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

const scrollbarUpdatePasses = 3

type scrollbarVisibility struct {
	horizontal bool
	vertical   bool
}

func (d *Document) updateScrollbars() {
	if d.edit == 0 || d.updatingBars {
		return
	}
	d.updatingBars = true
	_, _ = d.edit.SendMessage(co.WM_SETREDRAW, 0, 0)
	defer func() {
		_, _ = d.edit.SendMessage(co.WM_SETREDRAW, 1, 0)
		_ = d.edit.InvalidateRect(nil, true)
		d.updatingBars = false
	}()

	// The editor wraps text, so only vertical scrolling can ever be needed.
	visibility := scrollbarVisibility{vertical: true}
	d.setScrollbarVisibility(visibility)
	for pass := 0; pass < scrollbarUpdatePasses; pass++ {
		next := scrollbarVisibility{
			vertical: d.scrollbarNeeded(co.SBB_VERT),
		}
		if next == visibility {
			return
		}
		visibility = next
		d.setScrollbarVisibility(visibility)
	}
}

func (d *Document) scrollbarNeeded(bar co.SBB) bool {
	var info win.SCROLLINFO
	info.Mask = co.SIF_RANGE | co.SIF_PAGE
	info.SetCbSize()
	if err := d.edit.GetScrollInfo(bar, &info); err != nil {
		d.logError("query editor scroll range", err)
		return false
	}
	if bar == co.SBB_VERT && info.Page == 0 {
		// RichEdit can report a placeholder range before its first visible
		// layout pass. Its last-character position still reflects wrapped rows.
		return d.editorContentExceedsViewport()
	}
	return scrollRangeExceedsPage(info)
}

func (d *Document) editorContentExceedsViewport() bool {
	length, err := d.edit.SendMessage(co.WM_GETTEXTLENGTH, 0, 0)
	if err != nil || length == 0 {
		return false
	}
	position, err := d.edit.SendMessage(
		co.EM_POSFROMCHAR,
		win.WPARAM(length-1),
		0,
	)
	if err != nil {
		d.logError("position editor's last character", err)
		return true
	}
	x, y := decodeMousePosition(position)
	if x == -1 && y == -1 {
		return true
	}
	client, err := d.edit.GetClientRect()
	if err != nil {
		d.logError("measure editor for scroll range", err)
		return true
	}
	lineHeight := d.fontLineHeight()
	if lineHeight < 1 {
		lineHeight = 1
	}
	return y+lineHeight > int(client.Bottom)
}

func scrollRangeExceedsPage(info win.SCROLLINFO) bool {
	if info.Page == 0 {
		return info.Max > info.Min
	}
	extent := int64(info.Max) - int64(info.Min) + 1
	return extent > int64(info.Page)
}

func (d *Document) setScrollbarVisibility(visibility scrollbarVisibility) {
	visibility.horizontal = false
	if d.scrollbars.horizontal != visibility.horizontal {
		if err := d.edit.ShowScrollBar(co.SBB_HORZ, visibility.horizontal); err != nil {
			d.logError("set horizontal scrollbar visibility", fmt.Errorf("show=%v: %w", visibility.horizontal, err))
		}
	}
	if d.scrollbars.vertical != visibility.vertical {
		if err := d.edit.ShowScrollBar(co.SBB_VERT, visibility.vertical); err != nil {
			d.logError("set vertical scrollbar visibility", fmt.Errorf("show=%v: %w", visibility.vertical, err))
		}
	}
	d.scrollbars = visibility
}
